package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/spider"
)

// SpiderService 采集编排: 手动/定时触发、任务监控控制、清库、分类覆盖、cron 调度、失败页补采。
type SpiderService struct {
	engine   *spider.Engine
	sources  repository.CollectSourceRepository
	crons    repository.CronTaskRepository
	health   repository.SourceHealthRepository   // 读: 自动采集跳过已停采死源(可空)
	failures repository.CollectFailureRepository // 页级失败台账
	tasks    *TaskRunService                     // 任务运行台账(可空: 空则不记录, 不影响采集)

	// OnSettled 采集落库后的回调(横图 worker 用, 组合根注入, 可空)。
	// 触发即重算轮播集合: 新采集的片可能进入兜底榜单, 立即补横图。panic 由 safeNotify 隔离。
	OnSettled func()

	// baseCtx 优雅停机根 ctx(SetBaseCtx 注入, 缺省 Background)。所有后台采集
	// (手动触发/定时任务/补采)从这里派生 —— SIGTERM 时取消信号能传进引擎,
	// 引擎收尾后 WaitJobs 可等待在跑协程退出。
	baseCtx context.Context

	// jobs 在跑后台采集协程计数(优雅停机时等它们收尾, 见 WaitJobs)。
	jobs sync.WaitGroup

	mu      sync.Mutex
	cronLib *cron.Cron
}

func NewSpiderService(engine *spider.Engine, sources repository.CollectSourceRepository, crons repository.CronTaskRepository, health repository.SourceHealthRepository, failures repository.CollectFailureRepository, tasks *TaskRunService) *SpiderService {
	return &SpiderService{engine: engine, sources: sources, crons: crons, health: health, failures: failures, tasks: tasks}
}

// SetBaseCtx 注入优雅停机根 ctx(组合根在启动监听前调用, 只调一次)。
func (s *SpiderService) SetBaseCtx(ctx context.Context) {
	s.mu.Lock()
	s.baseCtx = ctx
	s.mu.Unlock()
}

// bgCtx 后台采集应使用的根 ctx(未注入时退回 Background, 行为与旧版一致)。
func (s *SpiderService) bgCtx() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.baseCtx != nil {
		return s.baseCtx
	}
	return context.Background()
}

// WaitJobs 等待所有在跑后台采集协程退出(优雅停机收尾用)。超时返回 false ——
// 引擎对 ctx 取消会快速中止在跑页, 正常远早于超时收敛; 真超时说明有协程卡死,
// 交由进程退出兜底(单条落库是事务, 数据不会坏)。
func (s *SpiderService) WaitJobs(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		s.jobs.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// isSuppressed 健康检查是否已自动停采该源。fail-open: 缺行/出错/未注入一律 false, 绝不因健康设施故障而停采。
func (s *SpiderService) isSuppressed(ctx context.Context, src *entity.CollectSource) bool {
	if src == nil {
		return false
	}
	if s.health == nil {
		return false
	}
	h, err := s.health.Get(ctx, src.Id)
	if err != nil || h == nil {
		return false
	}
	return h.Suppressed
}

const collectLockTTL = 6 * time.Hour

// runOne 对单源采集一次(同源重入由分布式锁阻止)。同步执行。
// 返回采集错误; 拿不到锁(同源已有采集在跑)返回 ErrConflict —— 那不是失败, 只是"这轮没轮上"。
func (s *SpiderService) runOne(ctx context.Context, src *entity.CollectSource, hours int) error {
	lockKey := cache.KeyLockCron("src:" + src.Id)
	token, ok, _ := cache.TryLock(ctx, lockKey, collectLockTTL)
	if !ok {
		log.Printf("spider: 源 %s 上次采集未结束, 跳过", src.Id)
		return domain.ErrConflict
	}
	// Unlock 必须脱离取消: 优雅停机时 ctx 已取消, 用它解锁会失败 → 6h TTL 的
	// 锁残留, 把部署后的前几轮自动采集全卡死。分布式锁的收尾动作不随停机取消。
	defer cache.Unlock(context.WithoutCancel(ctx), lockKey, token)
	if err := s.engine.Collect(ctx, src, hours); err != nil {
		log.Printf("spider: 采集 %s 失败: %v", src.Id, err)
		return err
	}
	// 落库成功 → 通知横图 worker 重算(新片可能进入轮播兜底榜单)。失败也可能写了部分数据, 同样触发。
	safeNotify("spider", s.OnSettled)
	return nil
}

// StartCollect 后台异步触发单源采集(handler 返回 202)。源停用 / 不存在 → 错误。
// ctx 取自 baseCtx(优雅停机根), 不取 HTTP 请求 ctx —— 采集必须活过请求生命周期,
// 但要能被 SIGTERM 取消(旧版 Background 是停机传不进取消信号的根因)。
// 同时登记一条手动采集运行记录(kind=manual, type=collect), 供任务管理页展示/重跑。
func (s *SpiderService) StartCollect(sourceId string, hours int) error {
	ctx := s.bgCtx()
	src, err := s.sources.Get(ctx, sourceId)
	if err != nil {
		return err
	}
	if src.State != entity.StateEnabled {
		return domain.ErrInvalidArgument
	}
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("spider StartCollect panic: %v", r)
			}
		}()
		runId := s.tasks.Start(entity.TaskRunKindManual, entity.TaskRunCollect, 0, src.Id,
			"手动采集："+src.Name, hours)
		err := s.runOne(ctx, src, hours)
		if err != nil {
			status, msg := entity.TaskRunFailed, err.Error()
			if errors.Is(err, domain.ErrConflict) {
				status = entity.TaskRunCanceled
				msg = "同源已有采集在跑, 本次跳过"
			}
			s.tasks.Finish(runId, status, 0, 0, 0, "", msg)
			return
		}
		// 引擎已把进度写进 Redis job, 收尾时快照补齐页数统计
		p, _ := cache.JobSnapshot(finishCtx(ctx), src.Id)
		msg := "采集完成"
		if p.Failed > 0 {
			msg = fmt.Sprintf("采集完成, %d 页失败(可在失败记录中补采)", p.Failed)
		}
		s.tasks.Finish(runId, entity.TaskRunSuccess, p.Total, p.Done, p.Failed, msg, "")
	}()
	return nil
}

// runFinishCtx 台账收尾写用 ctx(脱离停机取消): 优雅停机时在跑任务被取消,
// 收尾登记仍要落地, 否则"任务为什么没跑完"查不到。
func finishCtx(ctx context.Context) context.Context { return context.WithoutCancel(ctx) }

// SourceFilmResult 单源按片名搜索/采集的结果(供新增页预览与按源采集聚合展示)。
type SourceFilmResult struct {
	SourceId   string             `json:"sourceId"`
	SourceName string             `json:"sourceName"`
	Master     bool               `json:"master"`
	Hits       []spider.SearchHit `json:"hits,omitempty"`       // 搜索预览结果(SearchSources 用)
	Collected  int                `json:"collected"`            // 采集落库条数
	Picked     *spider.SearchHit  `json:"picked,omitempty"`     // 智能采集精确命中并采下的那部(供前端提示采了哪部)
	Candidates []spider.SearchHit `json:"candidates,omitempty"` // 多义未采: 全部命中作候选交前端选(见 CollectOneSource 契约)
	Error      string             `json:"error,omitempty"`      // 该源失败原因(不阻断其它源)
}

// resolveSources 解析目标源: ids 为空 → 全部启用源; 否则按 id 取(过滤停用/不存在)。
func (s *SpiderService) resolveSources(ctx context.Context, ids []string) ([]entity.CollectSource, error) {
	if len(ids) == 0 {
		return s.sources.List(ctx, true) // 仅启用
	}
	out := make([]entity.CollectSource, 0, len(ids))
	for _, id := range ids {
		src, err := s.sources.Get(ctx, id)
		if err != nil || src.State != entity.StateEnabled {
			continue
		}
		out = append(out, *src)
	}
	return out, nil
}

// SearchSources 按片名在指定源(空=全部启用源)搜索影片信息(只读, 不落库)。供新增页"搜索站点影片信息"。
func (s *SpiderService) SearchSources(ctx context.Context, keyword string, ids []string) ([]SourceFilmResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, domain.ErrInvalidArgument
	}
	srcs, err := s.resolveSources(ctx, ids)
	if err != nil {
		return nil, err
	}
	results := make([]SourceFilmResult, len(srcs))
	var wg sync.WaitGroup
	for i := range srcs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[i].Error = "panic"
				}
			}()
			src := &srcs[i]
			r := SourceFilmResult{SourceId: src.Id, SourceName: src.Name, Master: src.Grade == entity.GradeMaster}
			if hits, e := s.engine.SearchPreview(ctx, src, keyword); e != nil {
				r.Error = e.Error()
			} else {
				r.Hits = hits
			}
			results[i] = r
		}(i)
	}
	wg.Wait()
	return results, nil
}

// pickExactHits 精确过滤: 取归一化片名(NormalizeName)与关键字完全相同的命中(智能采集判唯一/多义用)。
func pickExactHits(hits []spider.SearchHit, keyword string) []spider.SearchHit {
	wantKey := domain.NormalizeName(strings.TrimSpace(keyword))
	exact := make([]spider.SearchHit, 0, len(hits))
	for _, h := range hits {
		if domain.NormalizeName(h.Name) == wantKey {
			exact = append(exact, h)
		}
	}
	return exact
}

// CollectOneSource 智能采集单个源:
//   - vodId>0: 直接 engine.CollectByIds(src, [vodId]) 精确采(前端从候选里选定后调), 返回 {Collected}。
//   - vodId==0: 先 SearchPreview 搜片名 → **精确过滤**(NormalizeName(hit.Name)==NormalizeName(keyword)):
//   - 恰好 1 个精确命中 → 直接 CollectByIds 采它, 返回 {Collected, Picked};
//   - 0 个精确(仅模糊命中)或 >1 个精确 → **不采**, 返回 {Candidates: 全部 hits} 交前端选;
//   - hits 为空 → {Collected:0}。
//
// 返回契约: Candidates 非空 ⟺ 需前端介入选片(本源未落库, Collected==0); 否则按 Collected 判采集结果。
func (s *SpiderService) CollectOneSource(ctx context.Context, sourceId, keyword string, vodId int64) (SourceFilmResult, error) {
	src, err := s.sources.Get(ctx, sourceId)
	if err != nil {
		return SourceFilmResult{}, err
	}
	// 单源手动采集(后台影片详情"按源采集该片")是管理员显式动作 → 允许停用源也能采;
	// 多源/全部流程已在 resolveSources 过滤为仅启用源, 不会到这里采到停用源。
	// (旧逻辑 State!=enabled 直接返回 ErrInvalidArgument, 导致点停用源采集报"非法参数"。)
	r := SourceFilmResult{SourceId: src.Id, SourceName: src.Name, Master: src.Grade == entity.GradeMaster}

	if vodId > 0 { // 前端已选定: 精确采该 vodId
		n, e := s.engine.CollectByIds(ctx, src, []int64{vodId})
		if e != nil {
			r.Error = e.Error()
		} else {
			safeNotify("spider", s.OnSettled)
		}
		r.Collected = n
		return r, nil
	}

	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return SourceFilmResult{}, domain.ErrInvalidArgument
	}
	hits, e := s.engine.SearchPreview(ctx, src, keyword)
	if e != nil {
		r.Error = e.Error()
		return r, nil
	}
	if len(hits) == 0 {
		return r, nil // 无命中, Collected=0
	}
	exact := pickExactHits(hits, keyword)
	if len(exact) == 1 { // 唯一精确命中 → 直接采
		picked := exact[0]
		n, ce := s.engine.CollectByIds(ctx, src, []int64{picked.SourceVodID})
		if ce != nil {
			r.Error = ce.Error()
			return r, nil
		}
		safeNotify("spider", s.OnSettled)
		r.Collected = n
		r.Picked = &picked
		return r, nil
	}
	// 0 个精确(仅模糊)或 >1 个精确 → 多义, 不采, 全部 hits 作候选交前端选
	r.Candidates = hits
	return r, nil
}

// CollectFilm 按片名在指定源(空=全部启用源)采集影片(落库)。逐源调 CollectOneSource(vodId=0) 智能采:
// 某源精确唯一命中即采、某源多义则返候选; 单源失败不阻断其它源。
func (s *SpiderService) CollectFilm(ctx context.Context, keyword string, ids []string) ([]SourceFilmResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, domain.ErrInvalidArgument
	}
	srcs, err := s.resolveSources(ctx, ids)
	if err != nil {
		return nil, err
	}
	results := make([]SourceFilmResult, 0, len(srcs))
	for i := range srcs {
		src := &srcs[i]
		r, e := s.CollectOneSource(ctx, src.Id, keyword, 0)
		if e != nil {
			// 单源解析失败(停用/不存在)不阻断其它源, 记错占位
			r = SourceFilmResult{SourceId: src.Id, SourceName: src.Name, Master: src.Grade == entity.GradeMaster, Error: e.Error()}
		}
		results = append(results, r)
	}
	return results, nil
}

// CollectSummary 一轮批量采集的汇总(定时任务台账收尾用)。
type CollectSummary struct {
	Sources     int    // 尝试的源数
	OK          int    // 成功源数
	Failed      int    // 失败源数
	Skipped     int    // 跳过源数(同源在跑 / 已自动停采)
	Total       int    // 页数合计
	Done        int    // 成功页合计
	FailedPages int    // 失败页合计
	FirstErr    string // 首个源失败原因
}

// summarizeJob 把一个源的快照并进汇总。
func summarizeJob(s *CollectSummary, src *entity.CollectSource, err error) {
	s.Sources++
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			s.Skipped++
		} else {
			s.Failed++
			if s.FirstErr == "" {
				s.FirstErr = err.Error()
			}
		}
		return
	}
	s.OK++
	if p, e := cache.JobSnapshot(finishCtx(context.Background()), src.Id); e == nil {
		s.Total += p.Total
		s.Done += p.Done
		s.FailedPages += p.Failed
	}
}

// AutoCollect 采集所有已启用源(主站在前, 由 List 的 grade 排序保证), 返回汇总。
func (s *SpiderService) AutoCollect(ctx context.Context, hours int) CollectSummary {
	var sum CollectSummary
	srcs, err := s.sources.List(ctx, true)
	if err != nil {
		log.Printf("spider AutoCollect list err: %v", err)
		return sum
	}
	for i := range srcs {
		if s.isSuppressed(ctx, &srcs[i]) {
			log.Printf("spider: 源 %s 已被健康检查自动停采, 跳过", srcs[i].Id)
			sum.Sources++
			sum.Skipped++
			continue
		}
		summarizeJob(&sum, &srcs[i], s.runOne(ctx, &srcs[i], hours))
	}
	return sum
}

// BatchCollect 采集指定源, 返回汇总。
func (s *SpiderService) BatchCollect(ctx context.Context, hours int, ids ...string) CollectSummary {
	var sum CollectSummary
	for _, id := range ids {
		src, err := s.sources.Get(ctx, id)
		if err != nil || src.State != entity.StateEnabled {
			continue
		}
		if s.isSuppressed(ctx, src) {
			log.Printf("spider: 源 %s 已被健康检查自动停采, 跳过", id)
			sum.Sources++
			sum.Skipped++
			continue
		}
		summarizeJob(&sum, src, s.runOne(ctx, src, hours))
	}
	return sum
}

// ---- 任务监控/控制(状态存 Redis, 多副本一致) ----

func (s *SpiderService) Jobs(ctx context.Context) ([]cache.JobProgress, error) {
	return cache.JobList(ctx)
}
func (s *SpiderService) PauseJob(ctx context.Context, sourceId string) {
	cache.JobSetState(ctx, sourceId, cache.JobPaused)
}
func (s *SpiderService) ResumeJob(ctx context.Context, sourceId string) {
	cache.JobSetState(ctx, sourceId, cache.JobRunning)
}
func (s *SpiderService) CancelJob(ctx context.Context, sourceId string) {
	cache.JobSetState(ctx, sourceId, cache.JobCanceled)
}

// FilmZero 清空影片库(/spider/reset)。
func (s *SpiderService) FilmZero(ctx context.Context) error {
	return s.engine.Zero(ctx)
}

// CategoryCover 重采并覆盖分类(同步执行, 登记台账)。
func (s *SpiderService) CategoryCover(ctx context.Context, sourceId string) error {
	src, err := s.sources.Get(ctx, sourceId)
	if err != nil {
		return err
	}
	runId := s.tasks.Start(entity.TaskRunKindManual, entity.TaskRunCategoryCover, 0, src.Id,
		"分类覆盖："+src.Name, 0)
	if err := s.engine.CoverCategories(ctx, src); err != nil {
		s.tasks.Finish(runId, entity.TaskRunFailed, 0, 0, 0, "", err.Error())
		return err
	}
	s.tasks.Finish(runId, entity.TaskRunSuccess, 0, 0, 0, "分类已覆盖", "")
	return nil
}

// ---- 失败页补采 ----

// 补采策略参数。
const (
	// recoverIncrementalMaxHours 增量窗口上限(180 天)。超过它就意味着"这次采集要扫的范围已经太大",
	// 此时扩窗重扫的代价高于按页码精确重放。
	recoverIncrementalMaxHours = 4320
	// recoverBatchLimit 单轮最多处理多少条待补采记录(防一次补采把源站打穿)。
	recoverBatchLimit = 200
	// recoverTimeout 单轮补采的时间上限。补采是逐源逐页的重活, 但不能无上限地跑:
	// 早先用 context.Background() 起 goroutine, 一旦跑起来就再也收不回来(连部署重启都不受影响)。
	// 取与采集锁 TTL 同量级, 保证超时不会远远晚于锁失效。
	recoverTimeout = collectLockTTL
)

// FailurePageSize 失败台账列表的默认每页条数。
// 供 handler 生成响应里的 page/size 复用, 避免"取数层归一化用 20、响应口径用另一个数"的漂移。
const FailurePageSize = 20

// RecoverResult 一轮补采的统计。
type RecoverResult struct {
	Scanned  int `json:"scanned"`  // 检查过的待补采记录数
	Widened  int `json:"widened"`  // 经"扩大时间窗整段重扫"处理掉的记录数(含被宽窗顺带覆盖的)
	Replayed int `json:"replayed"` // 经"精确重放单页"处理掉的记录数
	Failed   int `json:"failed"`   // 补采仍失败的记录数
	Busy     int `json:"busy"`     // 因该源正在采集而顺延的记录数
}

// RecoverPending 按失败台账补采。ids 为空 → 处理全部待补采记录; 非空 → 只处理这些 id。
//
// 分流依据是"页码在时间维度上还准不准":
//   - 增量采集(hours>0)失败: 数据已经随时间往前走了, 重放旧页码没有意义 → 把时间窗扩到
//     「原窗口 + 距失败已过的小时数」整段重扫, 既补上漏掉的页, 也不会重复太多。一次宽窗重扫
//     覆盖该源宽窗范围内(失败时间不早于宽窗起点)的所有增量失败, 故这些记录一并归档, 不必逐条重放。
//   - 全量/超长范围(hours<=0 或超上限)失败: 页码语义稳定, 按「源 + 时长 + 页码」精确重放那一页。
func (s *SpiderService) RecoverPending(ctx context.Context, ids []int64) RecoverResult {
	var res RecoverResult
	if s.failures == nil || s.engine == nil {
		return res
	}
	pending, err := s.failures.ListPending(ctx, ids, recoverBatchLimit)
	if err != nil {
		log.Printf("spider RecoverPending list err: %v", err)
		return res
	}

	doneSource := map[string]bool{} // 已由宽窗重扫覆盖过的源, 同轮不再重复重扫
	for i := range pending {
		f := pending[i]
		res.Scanned++

		src, err := s.sources.Get(ctx, f.SourceId)
		if err != nil {
			// 源已被删除 → 台账无从补起, 直接归档, 免得永远挂在待处理里
			_ = s.failures.MarkHandled(ctx, []int64{f.Id})
			continue
		}
		if s.isSuppressed(ctx, src) {
			// 源已被健康检查自动停采: 补采也只会失败, 保留记录等源恢复后再补
			log.Printf("spider: 源 %s 已自动停采, 失败记录 %d 暂不补采", src.Id, f.Id)
			continue
		}

		if f.Hours > 0 && f.Hours <= recoverIncrementalMaxHours {
			if doneSource[src.Id] {
				continue
			}
			window := widenedHours(f)
			if err := s.runOne(ctx, src, window); err != nil {
				if errors.Is(err, domain.ErrConflict) {
					res.Busy++
				} else {
					res.Failed++
				}
				continue
			}
			n, e := s.failures.MarkHandledIncrementalCovered(ctx, src.Id, coveredSinceMs(f), widenedHours(f))
			if e != nil {
				log.Printf("spider: 归档增量失败记录 err: %v", e)
			}
			res.Widened += int(n)
			doneSource[src.Id] = true
			continue
		}

		if err := s.recoverOnePage(ctx, src, f); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				res.Busy++
			} else {
				res.Failed++
			}
			continue
		}
		if err := s.failures.MarkHandled(ctx, []int64{f.Id}); err != nil {
			log.Printf("spider: 归档失败记录 %d err: %v", f.Id, err)
		}
		res.Replayed++
	}
	log.Printf("spider RecoverPending 完成: %+v", res)
	return res
}

// widenedHours 把失败记录的窗口扩到「原窗口 + 自失败起已过的小时数」, 上限 recoverIncrementalMaxHours。
func widenedHours(f entity.CollectFailure) int {
	elapsed := 0
	if f.CreatedAt > 0 {
		elapsed = int(time.Since(time.UnixMilli(f.CreatedAt)).Hours())
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if w := f.Hours + elapsed; w < recoverIncrementalMaxHours {
		return w
	}
	return recoverIncrementalMaxHours
}

// coveredSinceMs 宽窗重扫的覆盖起点(毫秒时间戳): 宽窗 = 当前往前 widenedHours 小时,
// 失败时间不早于该起点的同源增量页都已被这次重扫恢复到, 可一并归档。
func coveredSinceMs(f entity.CollectFailure) int64 {
	return time.Now().UnixMilli() - int64(widenedHours(f))*3600*1000
}

// recoverOnePage 精确重放单页(走引擎的单页采集, 不动影子表)。
func (s *SpiderService) recoverOnePage(ctx context.Context, src *entity.CollectSource, f entity.CollectFailure) error {
	lockKey := cache.KeyLockCron("src:" + src.Id)
	token, ok, _ := cache.TryLock(ctx, lockKey, collectLockTTL)
	if !ok {
		log.Printf("spider: 源 %s 正在采集, 失败记录 %d 顺延", src.Id, f.Id)
		return domain.ErrConflict
	}
	defer cache.Unlock(ctx, lockKey, token)
	if _, err := s.engine.CollectOnePage(ctx, src, f.PageNo, f.Hours); err != nil {
		log.Printf("spider: 补采 %s 第 %d 页失败: %v", src.Id, f.PageNo, err)
		return err
	}
	safeNotify("spider", s.OnSettled)
	return nil
}

// RecoverAsync 后台触发补采(handler 返回 202)。ids 为空 → 全部待补采。
// ctx 取自 baseCtx: goroutine 起出去之后仍能自行收敛(自带超时), 且能被优雅停机取消。
// 登记一条补采运行记录(kind=manual, type=recover)。
func (s *SpiderService) RecoverAsync(ids []int64) {
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("spider RecoverAsync panic: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(s.bgCtx(), recoverTimeout)
		defer cancel()
		name := "失败页补采"
		if len(ids) > 0 {
			name = fmt.Sprintf("失败页补采(%d 条)", len(ids))
		}
		runId := s.tasks.Start(entity.TaskRunKindManual, entity.TaskRunRecover, 0, "", name, 0)
		res := s.RecoverPending(ctx, ids)
		s.finishRecoverRun(runId, res)
	}()
}

// ListFailures 后台失败台账列表(status 见 entity.Failure* 常量)。
//
// 分页在这里就地归一化: 本项目约定"归一化由取数层负责"(ManageService/UserService/
// FilmService 同款), 缺了它 size 缺省时 page.Limit() 就是 0 → SQL 编译成 LIMIT 0,
// 返回空列表但 total 正常, 页面会显示"共 N 条、列表空"。
func (s *SpiderService) ListFailures(ctx context.Context, status int8, page repository.Page) ([]entity.CollectFailure, int64, error) {
	if s.failures == nil {
		return nil, 0, nil
	}
	return s.failures.List(ctx, status, page.Normalize(FailurePageSize))
}

// ClearHandledFailures 清理已处理记录, 返回删除条数。
func (s *SpiderService) ClearHandledFailures(ctx context.Context) (int64, error) {
	if s.failures == nil {
		return 0, nil
	}
	return s.failures.DeleteHandled(ctx)
}

// PendingFailureCount 待补采条数(仪表盘用)。
func (s *SpiderService) PendingFailureCount(ctx context.Context) int64 {
	if s.failures == nil {
		return 0
	}
	n, err := s.failures.CountPending(ctx)
	if err != nil {
		return 0
	}
	return n
}

// ---- cron 调度 ----

// StartScheduler 启动 cron: 注册全部已启用任务。应用启动时调用一次。
func (s *SpiderService) StartScheduler(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cronLib = cron.New(cron.WithSeconds())
	s.registerTasks(ctx)
	s.cronLib.Start()
}

// ReloadCron 重新加载 cron 任务(后台增删改 cron 后调用)。
func (s *SpiderService) ReloadCron(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cronLib != nil {
		s.cronLib.Stop()
	}
	s.cronLib = cron.New(cron.WithSeconds())
	s.registerTasks(ctx)
	s.cronLib.Start()
}

func (s *SpiderService) registerTasks(ctx context.Context) {
	tasks, err := s.crons.List(ctx)
	if err != nil {
		log.Printf("spider registerTasks err: %v", err)
		return
	}
	for _, t := range tasks {
		if t.State != entity.StateEnabled {
			continue
		}
		spec := strings.ReplaceAll(t.Spec, "?", "*") // robfig 不支持 Quartz '?'
		task := t                                    // 捕获副本
		_, e := s.cronLib.AddFunc(spec, func() {
			s.jobs.Add(1)
			defer s.jobs.Done()
			ctx := s.bgCtx() // 优雅停机根 ctx: SIGTERM 取消在跑轮次, 引擎收尾后进程可退
			lockKey := cache.KeyLockCron("task:" + strconv.FormatInt(task.Id, 10))
			tok, ok, _ := cache.TryLock(ctx, lockKey, collectLockTTL)
			if !ok {
				log.Printf("cron task %d 上次未结束, 跳过", task.Id)
				return
			}
			defer cache.Unlock(context.WithoutCancel(ctx), lockKey, tok)
			s.executeTask(ctx, &task, entity.TaskRunKindCron)
		})
		if e != nil {
			log.Printf("cron add task %d (spec=%q) err: %v", task.Id, spec, e)
		}
	}
}

// TriggerCron 手动触发一个定时任务立即执行一次(后台"立即执行"按钮), 返回 202 语义。
// 登记为 kind=manual —— 任务管理页按"手动任务"展示, 与定时到点触发区分。
func (s *SpiderService) TriggerCron(ctx context.Context, id int64) error {
	t, err := s.crons.Get(ctx, id)
	if err != nil {
		return err
	}
	if t.State != entity.StateEnabled {
		return domain.ErrInvalidArgument
	}
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("spider TriggerCron panic: %v", r)
			}
		}()
		s.executeTask(s.bgCtx(), t, entity.TaskRunKindManual)
	}()
	return nil
}

// taskTypeOf 定时任务的台账类型。
func taskTypeOf(t *entity.CronTask) string {
	if t.Model == entity.CronModelRecover {
		return entity.TaskRunRecover
	}
	return entity.TaskRunCollect
}

// cronDisplayName 定时任务显示名(带 kind 前缀, 区分定时触发/手动执行)。
func cronDisplayName(t *entity.CronTask, kind string) string {
	prefix := "定时任务"
	if kind == entity.TaskRunKindManual {
		prefix = "手动执行"
	}
	name := fmt.Sprintf("%s#%d", prefix, t.Id)
	if t.Remark != "" {
		name += " " + t.Remark
	}
	return name
}

// executeTask 执行一个定时任务的动作一次, 并登记台账。cron 闭包与 TriggerCron 共用。
func (s *SpiderService) executeTask(ctx context.Context, t *entity.CronTask, kind string) {
	runId := s.tasks.Start(kind, taskTypeOf(t), t.Id, "", cronDisplayName(t, kind), t.Time)
	switch t.Model {
	case entity.CronModelAutoAll:
		sum := s.AutoCollect(ctx, t.Time)
		s.finishCollectRun(runId, sum)
	case entity.CronModelRecover:
		res := s.RecoverPending(ctx, nil)
		s.finishRecoverRun(runId, res)
	default:
		sum := s.BatchCollect(ctx, t.Time, t.SourceIds...)
		s.finishCollectRun(runId, sum)
	}
}

// finishCollectRun 把一轮批量采集汇总收尾进台账。
func (s *SpiderService) finishCollectRun(runId int64, sum CollectSummary) {
	msg := fmt.Sprintf("成功 %d 个源, 跳过 %d 个", sum.OK, sum.Skipped)
	if sum.Done > 0 || sum.FailedPages > 0 {
		msg += fmt.Sprintf(", 采 %d 页", sum.Done)
		if sum.FailedPages > 0 {
			msg += fmt.Sprintf(", 失败 %d 页", sum.FailedPages)
		}
	}
	if sum.Failed > 0 {
		err := fmt.Sprintf("%d 个源失败", sum.Failed)
		if sum.FirstErr != "" {
			err += ": " + sum.FirstErr
		}
		s.tasks.Finish(runId, entity.TaskRunFailed, sum.Total, sum.Done, sum.FailedPages, msg, err)
		return
	}
	s.tasks.Finish(runId, entity.TaskRunSuccess, sum.Total, sum.Done, sum.FailedPages, msg, "")
}

// finishRecoverRun 把一轮补采结果收尾进台账。
func (s *SpiderService) finishRecoverRun(runId int64, res RecoverResult) {
	msg := fmt.Sprintf("检查 %d 条待补采: 宽窗覆盖 %d, 精确重放 %d, 顺延 %d",
		res.Scanned, res.Widened, res.Replayed, res.Busy)
	if res.Failed > 0 {
		s.tasks.Finish(runId, entity.TaskRunFailed, res.Scanned, res.Widened+res.Replayed, res.Failed,
			msg, fmt.Sprintf("%d 条补采仍失败", res.Failed))
		return
	}
	s.tasks.Finish(runId, entity.TaskRunSuccess, res.Scanned, res.Widened+res.Replayed, res.Failed, msg, "")
}

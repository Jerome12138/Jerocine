package service

// 豆瓣 ID / 评分：手工刷新 + 每周定时同步（2026-11-25）
//
// 手工：管理端影片详情页可改片名搜索、从候选中挑一条写入（db_id_src=2）。
// 定时：每周一 04:00 刷新 ①近半年更新过的片 ②有 db_id 但无评分的历史欠账（每天限量）。
//
// 速率：豆瓣接口吃限速，任务单线程跑、每次请求前在 **3~5s 随机**（固定节拍容易被风控），
// 靠"每周跑一批"消化，不在一次运行里跑满。

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"server/internal/domain/repository"
	"server/internal/douban"
)

// manualDouban 手工刷新用的客户端（低频，包级单例即可）。
var manualDouban = douban.New()

// doubanBatchCancel 当前批次的取消句柄（有批次在跑时非 nil）。
// 让"中途停止"成为可能：之前批次用 context.WithoutCancel 脱离请求后，外部没有任何手段叫停，
// 只能重启容器（2026-11-26 对端全量 400 时白跑两小时才发现）。
var doubanBatchCancel context.CancelFunc

// doubanMaxConsecutiveFail uint 连续失败达到该值即中止整批：
// 对端整段封禁/限流时（2026-11-26 实测 rexxar 全量 400），不熔断就要白跑两小时。
const doubanMaxConsecutiveFail = 5

// StopDoubanBatch 中止当前正在跑的豆瓣批次（评分同步 / 欠账补查）。
// 返回是否有批次被叫停（false = 当前没有批次在跑）。
func (s *FilmService) StopDoubanBatch() bool {
	cancel := doubanBatchCancel
	if cancel == nil {
		return false
	}
	cancel()
	log.Printf("[douban] 已请求中止当前批次")
	return true
}

// scoreSyncClient 定时同步用的客户端（3~5s 随机间隔）。
var scoreSyncClient = func() *douban.Client {
	c := douban.New()
	c.SetJitter(DoubanJitterFor(doubanDefaultIntervalMs))
	return c
}()

const (
	// doubanDefaultIntervalMs 默认单次间隔（毫秒）。2026-11-26 实测：3~5s（≈0.25rps）连续跑 ~50 次
	// 后服务器 IP 即被豆瓣封（400 subject_ip_rate_limit），而同接口从非机房 IP 正常 ⇒ 机房 IP 额度很低。
	// 默认降到 10s（≈360 次/小时），并允许在任务中心「接口间隔」里调（真正生效，见 RunDefinition）。
	doubanDefaultIntervalMs = 10_000
	// doubanMinIntervalMs 下限：再快就是自寻封禁
	doubanMinIntervalMs = 3_000
)

// DoubanJitterFor 把配置的间隔（毫秒）折成 ±20% 抖动区间；低于下限按 3s 处理。
// 抽成纯函数：任务中心改「接口间隔」后由 RunDefinition 调它，单测可直接验边界。
func DoubanJitterFor(intervalMs int) (min, max time.Duration) {
	if intervalMs < doubanMinIntervalMs {
		intervalMs = doubanMinIntervalMs
	}
	d := time.Duration(intervalMs) * time.Millisecond
	return d * 8 / 10, d * 12 / 10
}

// SetDoubanJitter 让任务中心配置的「接口间隔」真正生效（此前这个字段只存库不生效）。
func (s *FilmService) SetDoubanJitter(intervalMs int) {
	min, max := DoubanJitterFor(intervalMs)
	scoreSyncClient.SetJitter(min, max)
}

const (
	// doubanSyncLimit 每轮刷新的上限（近半年按上线日期实测 ~1.8k，留足余量）
	doubanSyncLimit = 4000
	// doubanSyncWindow 近本年的判定窗口（按 movie.pub_date，上线日期）
	doubanSyncWindow = -180 * 24 * time.Hour
)

// DoubanRefreshResult 手工刷新的结果（前端直接展示）。
type DoubanRefreshResult struct {
	Mid     int64   `json:"mid"`
	DbId    int64   `json:"dbId"`
	DbScore float64 `json:"dbScore"`
	Votes   int     `json:"votes"`
	Title   string  `json:"title"`
	Year    int     `json:"year"`
	Warning string  `json:"warning,omitempty"`
}

// FilmBrief 轻量片名/年份（豆瓣候选默认关键字用）。
type FilmBrief struct {
	Name string `json:"name"`
	Year int    `json:"year"`
}

func (s *FilmService) FilmBrief(ctx context.Context, mid int64) (FilmBrief, error) {
	m, err := s.movie.GetByMid(ctx, mid)
	if err != nil {
		return FilmBrief{}, err
	}
	return FilmBrief{Name: m.Name, Year: m.Year}, nil
}

// SearchDoubanCandidates 按片名取豆瓣候选（管理端弹窗里可改片名重搜）。
func (s *FilmService) SearchDoubanCandidates(ctx context.Context, name string) ([]douban.Suggestion, error) {
	if len([]rune(name)) < 1 {
		return nil, fmt.Errorf("片名不能为空")
	}
	return manualDouban.Suggest(ctx, name)
}

// RefreshFilmDouban 写回豆瓣 id/评分。
//   - dbId > 0：直接用用户选中的条目（不再自动匹配）；
//   - dbId = 0：按片名自动匹配（宁缺勿错）。
func (s *FilmService) RefreshFilmDouban(ctx context.Context, mid int64, dbId int64) (DoubanRefreshResult, error) {
	var out DoubanRefreshResult
	m, err := s.movie.GetByMid(ctx, mid)
	if err != nil {
		return out, err
	}

	useId := dbId
	title, year := m.Name, m.Year
	if useId <= 0 {
		sugg, serr := manualDouban.Suggest(ctx, m.Name)
		if serr != nil {
			return out, fmt.Errorf("豆瓣搜索失败: %w", serr)
		}
		best, ok := douban.PickSuggestion(sugg, m.Name, m.Year)
		if !ok {
			return out, fmt.Errorf("豆瓣未找到匹配条目（片名「%s」年份 %d）：宁可不写，也不猜", m.Name, m.Year)
		}
		useId, title, year = best.Id, best.Title, best.Year
	}

	score, votes, rerr := manualDouban.SubjectRating(ctx, useId)
	warn := ""
	if rerr != nil {
		// 拿到 id 本身就有价值（集标识等依赖它），评分拿不到只提示不失败
		warn = "评分获取失败：" + rerr.Error()
	}
	if err := s.movie.SetDouban(ctx, mid, useId, score); err != nil {
		return out, err
	}
	return DoubanRefreshResult{
		Mid: mid, DbId: useId, DbScore: score, Votes: votes,
		Title: title, Year: year, Warning: warn,
	}, nil
}

// StartDoubanScoreSync 每周一 **03:00** 自动刷新豆瓣评分（进程内定时器）。
func (s *FilmService) StartDoubanScoreSync(ctx context.Context) {
	go func() {
		for {
			next := nextMonday3am(time.Now())
			log.Printf("[douban-sync] 下次评分同步：%s", next.Format("2006-01-02 15:04"))
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
			}
			s.RunDoubanScoreSync(ctx)
		}
	}()
}

// nextMonday3am 下一个周一 03:00（本地时区；今天还没到点则算今天）。
func nextMonday3am(now time.Time) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	for t.Weekday() != time.Monday || !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// RunDoubanScoreSync 执行一轮同步：
// 按**影片上线日期**（movie.pub_date）取近半年、且已有豆瓣 id 的片，刷新评分。
//
// 为什么不再补"有 id 无评分"的历史欠账：实测那批 2.7 万条里绝大多数是半年前的老片，
// 豆瓣本来就没有评分，反复查只是浪费请求。
func (s *FilmService) RunDoubanScoreSync(ctx context.Context) {
	if !doubanBatchBusy.CompareAndSwap(false, true) {
		log.Printf("[douban-sync] 已有豆瓣刷分任务在跑，本轮跳过")
		return
	}
	defer doubanBatchBusy.Store(false)
	s.runDoubanScoreSyncLocked(ctx)
}

// RunDoubanScoreSyncOnce 同步跑一轮"近半年评分同步"（供统一任务中心调度调用）。
// 已被占用时直接返回错误（单飞），不阻塞等待。
func (s *FilmService) RunDoubanScoreSyncOnce(ctx context.Context) error {
	if !doubanBatchBusy.CompareAndSwap(false, true) {
		return fmt.Errorf("已有豆瓣刷分任务在执行中")
	}
	defer doubanBatchBusy.Store(false)
	s.runDoubanScoreSyncLocked(ctx)
	return nil
}

// StartDoubanScoreSyncOnce 手动触发一次"近半年评分同步"（异步，供任务中心"立即执行"用）。
func (s *FilmService) StartDoubanScoreSyncOnce(ctx context.Context) error {
	if !doubanBatchBusy.CompareAndSwap(false, true) {
		return fmt.Errorf("已有豆瓣刷分任务在执行中，请稍后再试")
	}
	// 脱离请求 ctx（页面关掉也要跑完），但**保留一个可取消的句柄**供"中途停止"用
	bg, cancel := context.WithCancel(context.WithoutCancel(ctx))
	doubanBatchCancel = cancel
	go func() {
		defer doubanBatchBusy.Store(false)
		defer func() { doubanBatchCancel = nil; cancel() }()
		s.runDoubanScoreSyncLocked(bg)
	}()
	return nil
}

// runDoubanScoreSyncLocked 调用方必须已持有 doubanBatchBusy。
func (s *FilmService) runDoubanScoreSyncLocked(ctx context.Context) {
	start := time.Now()
	setDoubanBatchProgress(func(p *DoubanBatchProgress) {
		*p = DoubanBatchProgress{
			State: "running", Task: "douban_score",
			StartedAt: start.UnixMilli(), Message: "开始刷新近半年评分",
		}
	})
	from := time.Now().Add(doubanSyncWindow).Format("2006-01")
	targets, err := s.movie.ListDoubanScoreTargets(ctx, repository.DoubanScoreQuery{
		PubMonthFrom: from,
		Limit:        doubanSyncLimit,
	})
	if err != nil {
		log.Printf("[douban-sync] 取片单失败: %v", err)
		return
	}

	total := len(targets)
	updated, noScore, failed := 0, 0, 0
	consecutiveFail := 0 // 连续失败计数（成功即清零，用于熔断）
	log.Printf("[douban-sync] 开始：上线月份 ≥ %s 共 %d 部", from, total)

	for i, t := range targets {
		if ctx.Err() != nil {
			log.Printf("[douban-sync] 已取消，进度 %d/%d（更新 %d）", i, total, updated)
			return
		}
		score, _, err := scoreSyncClient.SubjectRating(ctx, t.DbId)
		if err != nil {
			failed++
			consecutiveFail++
			if failed%50 == 1 {
				log.Printf("[douban-sync] 取评分失败(mid=%d id=%d): %v", t.Mid, t.DbId, err)
			}
			if consecutiveFail >= doubanMaxConsecutiveFail {
				// 对端整段封禁/限流时不要白跑整批（2026-11-26 实测 rexxar 全量 400）
				log.Printf("[douban-sync] 连续失败 %d 次，已中止（疑似被限流/风控）: 进度 %d/%d", consecutiveFail, i, total)
				setDoubanBatchProgress(func(p *DoubanBatchProgress) {
					p.State = "failed"
					p.Message = fmt.Sprintf("连续失败 %d 次已中止（疑似被豆瓣限流），进度 %d/%d", consecutiveFail, i, total)
				})
				return
			}
			continue
		}
		consecutiveFail = 0
		if score <= 0 {
			noScore++ // 豆瓣本身暂无评分（新片/冷门），跳过
			continue
		}
		if err := s.movie.UpdateDbScore(ctx, t.Mid, score); err != nil {
			failed++
			continue
		}
		updated++
		if (i+1)%100 == 0 {
			log.Printf("[douban-sync] 进度 %d/%d 更新=%d 无评分=%d 失败=%d", i+1, total, updated, noScore, failed)
		}
	}
	log.Printf("[douban-sync] 完成：目标 %d，更新 %d，豆瓣无评分 %d，失败 %d，耗时 %s",
		total, updated, noScore, failed, time.Since(start).Round(time.Second))
}

// ---------------------------------------------------------------------------
// 豆瓣评分欠账补查（有 db_id 无评分）
//
// 定位：**默认不启用**（不注册定时器），只支持手动触发；等任务中心落地后注册为
// `builtin:douban_backfill`（enabled=false）。
// 与每周同步共用一把单飞锁，避免两条链路同时打豆瓣。
// ---------------------------------------------------------------------------

// doubanBackfillDefaultLimit 单次补查上限（3~5s/条 → 2000 条约 2~3 小时）
const doubanBackfillDefaultLimit = 2000

// DoubanBatchProgress 豆瓣批量刷分的进度快照。
type DoubanBatchProgress struct {
	State string `json:"state"` // idle / running / done / failed
	// Task 当前在跑的是哪个豆瓣任务（douban_score / douban_backfill）——两条任务共用一把单飞锁，
	// 列表据此把进度归到正确的那一行。
	Task      string `json:"task"`
	Total     int    `json:"total"`
	Done      int    `json:"done"`
	Updated   int    `json:"updated"`
	NoScore   int    `json:"noScore"`
	Failed    int    `json:"failed"`
	Message   string `json:"message"`
	StartedAt int64  `json:"startedAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

var (
	doubanBatchMu   sync.Mutex
	doubanBatchProg = DoubanBatchProgress{State: "idle"}
	// doubanBatchBusy 单飞：定时同步与手动补查共用一个"豆瓣外部调用批"名额
	doubanBatchBusy atomic.Bool
)

// DoubanBatchProgress 读进度快照。
func (s *FilmService) DoubanBatchProgress() DoubanBatchProgress {
	doubanBatchMu.Lock()
	defer doubanBatchMu.Unlock()
	return doubanBatchProg
}

func setDoubanBatchProgress(f func(p *DoubanBatchProgress)) {
	doubanBatchMu.Lock()
	defer doubanBatchMu.Unlock()
	f(&doubanBatchProg)
	doubanBatchProg.UpdatedAt = time.Now().UnixMilli()
}

// StartDoubanScoreBackfill 手动触发欠账补查（异步，立即返回）。
func (s *FilmService) StartDoubanScoreBackfill(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = doubanBackfillDefaultLimit
	}
	if limit > 20000 {
		limit = 20000
	}
	if !doubanBatchBusy.CompareAndSwap(false, true) {
		return fmt.Errorf("已有豆瓣刷分任务在执行中，请稍后再试")
	}
	targets, err := s.movie.ListDoubanScoreTargets(ctx,
		repository.DoubanScoreQuery{OnlyMissing: true, Limit: limit})
	if err != nil {
		doubanBatchBusy.Store(false)
		return err
	}
	if len(targets) == 0 {
		doubanBatchBusy.Store(false)
		return fmt.Errorf("没有需要补查的影片（有豆瓣 id 但无评分的已为 0）")
	}
	setDoubanBatchProgress(func(p *DoubanBatchProgress) {
		*p = DoubanBatchProgress{
			State: "running", Task: "douban_backfill", Total: len(targets),
			StartedAt: time.Now().UnixMilli(), Message: "开始补查欠账",
		}
	})
	// 脱离请求 ctx（页面关掉也要跑完），但保留可取消句柄供"中途停止"
	bg, cancel := context.WithCancel(context.WithoutCancel(ctx))
	doubanBatchCancel = cancel
	go func() {
		defer func() { doubanBatchCancel = nil; cancel() }()
		s.runDoubanBackfill(bg, targets)
	}()
	return nil
}

func (s *FilmService) runDoubanBackfill(ctx context.Context, targets []repository.DoubanTarget) {
	defer doubanBatchBusy.Store(false)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[douban-backfill] panic: %v", r)
			setDoubanBatchProgress(func(p *DoubanBatchProgress) {
				p.State, p.Message = "failed", fmt.Sprintf("执行异常: %v", r)
			})
		}
	}()

	start := time.Now()
	consecutiveFail := 0 // 连续失败计数（成功/无评分即清零）
	for i, t := range targets {
		if ctx.Err() != nil {
			setDoubanBatchProgress(func(p *DoubanBatchProgress) { p.State, p.Message = "failed", "已取消" })
			return
		}
		score, _, err := scoreSyncClient.SubjectRating(ctx, t.DbId)
		switch {
		case err != nil:
			consecutiveFail++
			setDoubanBatchProgress(func(p *DoubanBatchProgress) { p.Done, p.Failed = i+1, p.Failed+1 })
			if consecutiveFail >= doubanMaxConsecutiveFail {
				log.Printf("[douban-backfill] 连续失败 %d 次，已中止（疑似被限流/风控）: 进度 %d/%d", consecutiveFail, i, len(targets))
				setDoubanBatchProgress(func(p *DoubanBatchProgress) {
					p.State = "failed"
					p.Message = fmt.Sprintf("连续失败 %d 次已中止（疑似被豆瓣限流），进度 %d/%d", consecutiveFail, i+1, len(targets))
				})
				return
			}
		case score <= 0:
			// 豆瓣确实没分：跳过，不再纠缠（这正是"半年前老片大概率没分"的那批）
			consecutiveFail = 0
			setDoubanBatchProgress(func(p *DoubanBatchProgress) { p.Done, p.NoScore = i+1, p.NoScore+1 })
		default:
			if uerr := s.movie.UpdateDbScore(ctx, t.Mid, score); uerr != nil {
				setDoubanBatchProgress(func(p *DoubanBatchProgress) { p.Done, p.Failed = i+1, p.Failed+1 })
				continue
			}
			consecutiveFail = 0
			setDoubanBatchProgress(func(p *DoubanBatchProgress) { p.Done, p.Updated = i+1, p.Updated+1 })
		}
		if (i+1)%50 == 0 {
			p := s.DoubanBatchProgress()
			log.Printf("[douban-backfill] 进度 %d/%d 更新=%d 无评分=%d 失败=%d", p.Done, p.Total, p.Updated, p.NoScore, p.Failed)
		}
	}
	p := s.DoubanBatchProgress()
	setDoubanBatchProgress(func(x *DoubanBatchProgress) {
		x.State = "done"
		x.Message = fmt.Sprintf("补查完成：更新 %d，豆瓣无评分 %d，失败 %d（耗时 %s）",
			p.Updated, p.NoScore, p.Failed, time.Since(start).Round(time.Second))
	})
	log.Printf("[douban-backfill] %s", p.Message)
}

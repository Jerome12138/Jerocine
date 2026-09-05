package service

import (
	"context"
	"fmt"
	"log"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/platform/blobstore"
	"server/internal/spider"
)

// ManageService 后台 CRUD 编排(瘦层: 委派仓储 + 缓存失效)。
// 注: cron 实际调度注册、采集源深度校验、手动加片的多源补全 与采集引擎耦合, 留待引擎里程碑接入。
type ManageService struct {
	sources  repository.CollectSourceRepository
	crons    repository.CronTaskRepository
	siteCfg  repository.SiteConfigRepository
	versions repository.AppVersionRepository
	files    repository.FileRepository
	category repository.CategoryRepository
	search   repository.SearchRepository
	movie    repository.MovieRepository
	play     repository.PlaySourceRepository
	tx       repository.TxManager
	users    *UserService
	blob     blobstore.BlobStore
	health   repository.SourceHealthRepository
	prober   *spider.Fetcher
}

func NewManageService(
	sources repository.CollectSourceRepository, crons repository.CronTaskRepository,
	siteCfg repository.SiteConfigRepository, versions repository.AppVersionRepository,
	files repository.FileRepository, category repository.CategoryRepository,
	search repository.SearchRepository, movie repository.MovieRepository,
	play repository.PlaySourceRepository,
	health repository.SourceHealthRepository,
	tx repository.TxManager, users *UserService, blob blobstore.BlobStore,
) *ManageService {
	return &ManageService{
		sources: sources, crons: crons, siteCfg: siteCfg, versions: versions,
		files: files, category: category, search: search, movie: movie, play: play,
		health: health, tx: tx, users: users, blob: blob,
		prober: spider.NewFetcherWithTimeout(probeTimeout),
	}
}

// 采集源实测 / 健康度参数。
const (
	probeTimeout     = 6 * time.Second // 单次探测超时
	probeCount       = 3               // 单源探测次数(取中位/成功率)
	probeConcurrency = 6               // 全量测速并发上限

	healthFailThreshold = 3                // 连续失败达此次数 → 自动停采
	healthCheckInterval = time.Hour        // 定时健康检查间隔(代码默认 1h)
	healthInitialDelay  = 90 * time.Second // 启动后首次检查延迟(避免冷启动抖动)
)

// UploadImage 保存上传图片到 BlobStore + files 元数据, 返回记录。
func (s *ManageService) UploadImage(ctx context.Context, fileName string, data []byte, relevanceId int64) (*entity.FileInfo, error) {
	ext := strings.ToLower(path.Ext(fileName))
	if ext == "" {
		ext = ".jpg"
	}
	key := "gallery/" + strconv.FormatInt(time.Now().UnixNano(), 10) + ext
	url, err := s.blob.Save(ctx, key, data)
	if err != nil {
		return nil, err
	}
	f := &entity.FileInfo{Link: url, ObjectKey: key, RelevanceId: relevanceId, Type: entity.FileTypeCover, FileType: strings.TrimPrefix(ext, ".")}
	if err := s.files.Create(ctx, f); err != nil {
		return nil, err
	}
	return f, nil
}

// UploadApk 保存上传 APK 到 BlobStore, 返回访问 URL(文件名经 blobstore 白名单校验防穿越)。
func (s *ManageService) UploadApk(ctx context.Context, fileName string, data []byte) (string, error) {
	base := path.Base(strings.ReplaceAll(fileName, "\\", "/"))
	return s.blob.Save(ctx, "apk/"+base, data)
}

// DashboardData 仪表盘统计。
type DashboardData struct {
	FilmCount    int64 `json:"filmCount"`
	CollectCount int64 `json:"collectCount"`
	CronCount    int64 `json:"cronCount"`
	TodayNew     int64 `json:"todayNew"`     // 今日新增影片(按 created_at)
	WeekNew      int64 `json:"weekNew"`      // 近 7 天新增影片
	DownSources  int64 `json:"downSources"`  // 已被自动停采的死源数
}

func (s *ManageService) Dashboard(ctx context.Context) DashboardData {
	d := DashboardData{}
	if _, total, err := s.search.Filter(ctx, repository.FilterSpec{}, repository.Page{Current: 1, Size: 1}); err == nil {
		d.FilmCount = total
	}
	if srcs, err := s.sources.List(ctx, false); err == nil {
		d.CollectCount = int64(len(srcs))
	}
	if crons, err := s.crons.List(ctx); err == nil {
		d.CronCount = int64(len(crons))
	}
	// 今日/近一周新增 (created_at 毫秒; 用服务端本地时区, 部署为北京时间)
	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if n, err := s.search.CountCreatedSince(ctx, startOfToday.UnixMilli()); err == nil {
		d.TodayNew = n
	}
	if n, err := s.search.CountCreatedSince(ctx, now.AddDate(0, 0, -7).UnixMilli()); err == nil {
		d.WeekNew = n
	}
	if s.health != nil {
		if hs, err := s.health.List(ctx); err == nil {
			for _, h := range hs {
				if h.Suppressed {
					d.DownSources++
				}
			}
		}
	}
	return d
}

// ---- 站点配置 ----

func (s *ManageService) GetSite(ctx context.Context) (*entity.SiteConfig, error) {
	return s.siteCfg.Get(ctx)
}

func (s *ManageService) SaveSite(ctx context.Context, c *entity.SiteConfig) error {
	if err := s.siteCfg.Save(ctx, c); err != nil {
		return err
	}
	cache.InvalidateConfig(ctx)
	return nil
}

// ---- 采集源 ----

func (s *ManageService) ListSources(ctx context.Context) ([]entity.CollectSource, error) {
	return s.sources.List(ctx, false)
}

func (s *ManageService) GetSource(ctx context.Context, id string) (*entity.CollectSource, error) {
	return s.sources.Get(ctx, id)
}

// UpsertSource 新增/编辑采集源(含 ClientOnly 仅端侧标记, 由 handler 绑定 JSON clientOnly → repo Upsert 全列写入)。
func (s *ManageService) UpsertSource(ctx context.Context, src *entity.CollectSource) error {
	dup, err := s.sources.ExistsByUri(ctx, src.Uri, src.Id)
	if err != nil {
		return err
	}
	if dup {
		return domain.ErrConflict
	}
	if err := s.sources.Upsert(ctx, src); err != nil {
		return err
	}
	cache.InvalidateConfig(ctx)
	return nil
}

func (s *ManageService) DeleteSource(ctx context.Context, id string) error {
	src, err := s.sources.Get(ctx, id)
	if err != nil {
		return err
	}
	if src.Grade == entity.GradeMaster {
		return domain.ErrConflict // 主站禁删
	}
	if err := s.sources.Delete(ctx, id); err != nil {
		return err
	}
	cache.InvalidateConfig(ctx)
	return nil
}

// CollectTestResult 采集源实测结果(多次探测取统计)。
type CollectTestResult struct {
	Ok            bool   `json:"ok"`            // 至少一次成功并解析到影片
	Probes        int    `json:"probes"`        // 探测次数
	OkCount       int    `json:"okCount"`       // 成功次数
	LatencyMs     int64  `json:"latencyMs"`     // 成功探测的中位延时(采集 API)
	BestMs        int64  `json:"bestMs"`        // 最快一次延时
	Films         int    `json:"films"`         // 首页解析到的影片数(0 可疑)
	PageCount     int    `json:"pageCount"`     // 分页总数
	Total         int    `json:"total"`         // 目录总片数(资源最全判定)
	PlayLatencyMs int64  `json:"playLatencyMs"` // 抽样 m3u8 播放延时(0=未测)
	Message       string `json:"message"`
}

// SourceTestResult 批量测速结果(带站点标识, 便于前端排序)。
type SourceTestResult struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	CollectTestResult
}

// clientOnlyMessage 仅端侧源的服务端测速占位说明(前端据此提示"服务器不测, 端侧自测")。
const clientOnlyMessage = "仅端侧可达, 服务器不测速"

// clientOnlyResult 仅端侧源的服务端测速结果: 不探测、不判失败(Ok=true 占位), 端侧另做。
func clientOnlyResult() CollectTestResult {
	return CollectTestResult{Ok: true, Message: clientOnlyMessage}
}

// TestSource 对单源连续打真实采集请求(ac=detail), 实测延时/成功率/可采集性, 并写入健康度。
// ClientOnly 源服务器访问不到, 直接返回占位结果且不写健康度(不计失败/不自动停采)。
func (s *ManageService) TestSource(ctx context.Context, id string) (CollectTestResult, error) {
	src, err := s.sources.Get(ctx, id)
	if err != nil {
		return CollectTestResult{}, err
	}
	if src.ClientOnly {
		return clientOnlyResult(), nil
	}
	res := s.probeSource(ctx, src)
	s.recordHealth(ctx, src.Id, res)
	return res, nil
}

// TestAllSources 并发测速全部采集源(含停用, 便于启用前评估), 并写入健康度。
func (s *ManageService) TestAllSources(ctx context.Context) ([]SourceTestResult, error) {
	srcs, err := s.sources.List(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]SourceTestResult, len(srcs))
	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for i := range srcs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			src := srcs[i]
			// ClientOnly 源服务器测不到, 跳过探测与健康度写入(不计失败/不自动停采)。
			if src.ClientOnly {
				out[i] = SourceTestResult{Id: src.Id, Name: src.Name, CollectTestResult: clientOnlyResult()}
				return
			}
			res := s.probeSource(ctx, &src)
			s.recordHealth(ctx, src.Id, res)
			out[i] = SourceTestResult{Id: src.Id, Name: src.Name, CollectTestResult: res}
		}(i)
	}
	wg.Wait()
	return out, nil
}

// probeSource 连续探测 probeCount 次后汇总(prober 复用 safehttp 客户端, 并发安全),
// 并量目录大小 + 抽样一条 m3u8 的播放延时。
func (s *ManageService) probeSource(ctx context.Context, src *entity.CollectSource) CollectTestResult {
	lats := make([]int64, 0, probeCount)
	films, pageCount, total := 0, 0, 0
	sampleM3u8 := ""
	var lastErr error
	for i := 0; i < probeCount; i++ {
		pr, err := s.prober.Probe(ctx, src)
		if err != nil {
			lastErr = err
			continue
		}
		lats = append(lats, pr.LatencyMs)
		if pr.Films > films {
			films = pr.Films
		}
		if pr.PageCount > pageCount {
			pageCount = pr.PageCount
		}
		if pr.Total > total {
			total = pr.Total
		}
		if sampleM3u8 == "" && pr.SampleM3u8 != "" {
			sampleM3u8 = pr.SampleM3u8
		}
	}
	res := summarizeProbes(probeCount, lats, films, lastErr)
	res.PageCount = pageCount
	if total == 0 && pageCount > 0 && films > 0 {
		total = pageCount * films // total 缺省时按 分页数×首页片数 估算目录大小
	}
	res.Total = total
	// 抽样播放延时: 拿到 m3u8 样本时计时拉取一次(best-effort, 失败留 0)
	if sampleM3u8 != "" {
		if pl, err := s.prober.MeasureURL(ctx, sampleM3u8); err == nil {
			res.PlayLatencyMs = pl
		}
	}
	return res
}

// summarizeProbes 纯函数: 把多次探测的延时/影片数/末次错误汇总成实测结果(便于单测)。
func summarizeProbes(probes int, lats []int64, films int, lastErr error) CollectTestResult {
	r := CollectTestResult{Probes: probes, OkCount: len(lats), Films: films}
	if len(lats) == 0 {
		r.Message = "全部探测失败"
		if lastErr != nil {
			r.Message = lastErr.Error()
		}
		return r
	}
	sorted := append([]int64(nil), lats...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	r.BestMs = sorted[0]
	r.LatencyMs = sorted[len(sorted)/2] // 中位
	r.Ok = films > 0
	switch {
	case films == 0:
		r.Message = "连通但首页未解析到影片(格式不符或需鉴权)"
	case r.OkCount < probes:
		r.Message = fmt.Sprintf("可用但不稳定 (%d/%d 成功)", r.OkCount, probes)
	default:
		r.Message = "连通正常"
	}
	return r
}

// ---- 健康度 / 自动停采 ----

// SourceHealthView 健康度面板单条(采集源元信息 + 健康快照)。
type SourceHealthView struct {
	Id               string `json:"id"`
	Name             string `json:"name"`
	State            bool   `json:"state"`      // 管理员启用开关(与 suppressed 正交)
	ClientOnly       bool   `json:"clientOnly"` // 仅端侧可达: 服务端不测速, 端侧另测
	Grade            int8   `json:"grade"`
	IsMaster         bool   `json:"isMaster"` // 当前主站(grade=0)
	Status           string `json:"status"`
	Suppressed       bool   `json:"suppressed"`
	LatencyMs        int64  `json:"latencyMs"`
	Films            int    `json:"films"`
	PageCount        int    `json:"pageCount"`
	Collected        int64  `json:"collected"`      // 已采集片数(movie_play_source 该源行数, 实时)
	Total            int    `json:"total"`          // 目录总片数(资源最全)
	PlayLatencyMs    int64  `json:"playLatencyMs"`  // 抽样播放延时
	OkCount          int    `json:"okCount"`
	Probes           int    `json:"probes"`
	ConsecutiveFails int    `json:"consecutiveFails"`
	Message          string `json:"message"`
	CheckedAt        int64  `json:"checkedAt"`
}

// ListHealth 健康度面板: 全部采集源左连健康快照(无快照 → unknown)。
func (s *ManageService) ListHealth(ctx context.Context) ([]SourceHealthView, error) {
	srcs, err := s.sources.List(ctx, false)
	if err != nil {
		return nil, err
	}
	hm := map[string]entity.SourceHealth{}
	if s.health != nil {
		if hs, e := s.health.List(ctx); e == nil {
			for _, h := range hs {
				hm[h.SourceId] = h
			}
		}
	}
	// 已采集片数: 一次 GROUP BY site_id 实时取全量(准确且高性能, 反映刚采完的状态)。
	var collected map[string]int64
	if s.play != nil {
		collected, _ = s.play.CountBySite(ctx)
	}
	out := make([]SourceHealthView, 0, len(srcs))
	for _, src := range srcs {
		v := SourceHealthView{
			Id: src.Id, Name: src.Name, State: src.State == entity.StateEnabled,
			ClientOnly: src.ClientOnly,
			Grade:      src.Grade, IsMaster: src.Grade == entity.GradeMaster, Status: entity.HealthUnknown,
			Collected: collected[src.Id],
		}
		// 仅端侧源服务端不测速, 不展示为 unknown/down(避免误判), 给固定占位说明。
		if src.ClientOnly {
			v.Message = clientOnlyMessage
		}
		if h, ok := hm[src.Id]; ok {
			if h.Status != "" {
				v.Status = h.Status
			}
			v.Suppressed = h.Suppressed
			v.LatencyMs = h.LatencyMs
			v.Films = h.Films
			v.PageCount = h.PageCount
			v.Total = h.Total
			v.PlayLatencyMs = h.PlayLatencyMs
			v.OkCount = h.OkCount
			v.Probes = h.Probes
			v.ConsecutiveFails = h.ConsecutiveFails
			v.Message = h.Message
			v.CheckedAt = h.CheckedAt
		}
		out = append(out, v)
	}
	return out, nil
}

// recordHealth 把一次探测结果合并进健康度并持久化(健康设施故障只记日志, 不影响测速)。
func (s *ManageService) recordHealth(ctx context.Context, id string, res CollectTestResult) {
	if s.health == nil {
		return
	}
	prev, _ := s.health.Get(ctx, id) // 未命中 prev=nil
	h := applyHealth(prev, res, healthFailThreshold, time.Now().UnixMilli())
	h.SourceId = id
	if err := s.health.Upsert(ctx, h); err != nil {
		log.Printf("recordHealth %s: %v", id, err)
	}
}

// applyHealth 纯函数: 把本次探测结果合并进上次健康快照(便于单测)。
// ok → 失败计数归零 / healthy / 解除停采; fail → 计数+1, 达阈值 → down+停采, 未达 → degraded(沿用原停采态)。
func applyHealth(prev *entity.SourceHealth, res CollectTestResult, threshold int, now int64) *entity.SourceHealth {
	h := &entity.SourceHealth{}
	if prev != nil {
		*h = *prev
	}
	h.CheckedAt = now
	h.UpdatedAt = now
	h.LastOk = res.Ok
	h.LatencyMs = res.LatencyMs
	h.BestMs = res.BestMs
	h.Films = res.Films
	h.PageCount = res.PageCount
	h.OkCount = res.OkCount
	h.Probes = res.Probes
	h.Message = res.Message
	// 目录大小/播放延时: 成功探测才更新, 失败保留上次(避免抖动把目录量清 0)
	if res.Total > 0 {
		h.Total = res.Total
	}
	if res.PlayLatencyMs > 0 {
		h.PlayLatencyMs = res.PlayLatencyMs
	}
	if res.Ok {
		h.ConsecutiveFails = 0
		h.Status = entity.HealthHealthy
		h.Suppressed = false // 自动恢复
		return h
	}
	h.ConsecutiveFails++
	if h.ConsecutiveFails >= threshold {
		h.Status = entity.HealthDown
		h.Suppressed = true
	} else {
		h.Status = entity.HealthDegraded // 未达阈值: 沿用原 Suppressed(首次失败为 false)
	}
	return h
}

// StartHealthScheduler 启动健康检查定时任务: 延迟首检后每 healthCheckInterval 跑一轮全量测速(写健康度→驱动自动停采/恢复)。
func (s *ManageService) StartHealthScheduler(ctx context.Context) {
	if s.health == nil {
		return
	}
	go func() {
		timer := time.NewTimer(healthInitialDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.runHealthCheck(ctx)
		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runHealthCheck(ctx)
			}
		}
	}()
}

func (s *ManageService) runHealthCheck(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("health check panic: %v", r)
		}
	}()
	if _, err := s.TestAllSources(ctx); err != nil {
		log.Printf("scheduled health check: %v", err)
	}
}

// ---- cron 任务(仅持久化; 调度注册在引擎里程碑接入) ----

func (s *ManageService) ListCrons(ctx context.Context) ([]entity.CronTask, error) {
	return s.crons.List(ctx)
}
func (s *ManageService) GetCron(ctx context.Context, id int64) (*entity.CronTask, error) {
	return s.crons.Get(ctx, id)
}
func (s *ManageService) UpsertCron(ctx context.Context, t *entity.CronTask) error {
	return s.crons.Upsert(ctx, t)
}
func (s *ManageService) DeleteCron(ctx context.Context, id int64) error {
	return s.crons.Delete(ctx, id)
}

// ---- 分类 ----

// ListCategories 返回结构化分类树(顶级带 children), 与公开 /categories 同构。
// 修复: 原返回扁平列表, 但后台 FilmClassView 按 parent.children 渲染 → 子级永空、全平铺。
func (s *ManageService) ListCategories(ctx context.Context) ([]*entity.CategoryNode, error) {
	cats, err := s.category.All(ctx)
	if err != nil {
		return nil, err
	}
	return buildTree(cats), nil
}
func (s *ManageService) UpsertCategory(ctx context.Context, c *entity.Category) error {
	if err := s.category.Upsert(ctx, c); err != nil {
		return err
	}
	cache.InvalidateCategory(ctx)
	return nil
}
func (s *ManageService) DeleteCategory(ctx context.Context, id int64) error {
	if err := s.category.Delete(ctx, id); err != nil {
		return err
	}
	cache.InvalidateCategory(ctx)
	return nil
}

// ---- 文件 ----

func (s *ManageService) ListFiles(ctx context.Context, page repository.Page) ([]entity.FileInfo, int64, error) {
	return s.files.List(ctx, page.Normalize(39))
}
func (s *ManageService) DeleteFile(ctx context.Context, id int64) error {
	return s.files.Delete(ctx, id) // blob 清理 TODO(需 FileRepository.Get 拿 objectKey)
}

// ---- APK 版本 ----

func (s *ManageService) ListVersions(ctx context.Context, page repository.Page) ([]entity.AppVersion, int64, error) {
	return s.versions.List(ctx, page.Normalize(20))
}
func (s *ManageService) CreateVersion(ctx context.Context, v *entity.AppVersion) error {
	return s.versions.Create(ctx, v)
}
func (s *ManageService) DeleteVersion(ctx context.Context, id int64) error {
	return s.versions.Delete(ctx, id)
}

// ---- 用户(委派 UserService) ----

func (s *ManageService) CreateUser(ctx context.Context, name, password string, role int) (*entity.User, error) {
	return s.users.CreateUser(ctx, name, password, role)
}
func (s *ManageService) ListUsers(ctx context.Context, page repository.Page) ([]entity.User, int64, error) {
	return s.users.ListUsers(ctx, page)
}

// ---- 影片管理 ----

func (s *ManageService) SearchFilms(ctx context.Context, spec repository.FilterSpec, page repository.Page) (CardPage, error) {
	page = page.Normalize(20)
	// 有关键字 → 走 FULLTEXT 检索(与公开搜索同口径); 否则按分类/筛选条件过滤。
	var (
		list  []entity.MovieSearch
		total int64
		err   error
	)
	if kw := strings.TrimSpace(spec.Keyword); kw != "" {
		list, total, err = s.search.SearchKeyword(ctx, kw, page)
	} else {
		list, total, err = s.search.Filter(ctx, spec, page)
	}
	if err != nil {
		return CardPage{}, err
	}
	return CardPage{List: list, Total: total, Page: page}, nil
}

func (s *ManageService) GetFilm(ctx context.Context, mid int64) (*entity.Movie, error) {
	return s.movie.GetByMid(ctx, mid)
}

// ManageFilmSource 后台影片详情的单个播放源(按 siteId+playFrom 去重后的一条线路)。
type ManageFilmSource struct {
	SiteId   string           `json:"siteId"`
	SiteName string           `json:"siteName"` // 采集源维护名(缺省回退 siteId)
	PlayFrom string           `json:"playFrom"`
	Master   bool             `json:"master"` // 该源是否主站
	Episodes []entity.Episode `json:"episodes"`
}

// ManageFilmDetail 后台影片详情: 影片主体 + 全部源与集(实时读库, 不走公开缓存)。
type ManageFilmDetail struct {
	Movie   entity.Movie       `json:"movie"`
	Sources []ManageFilmSource `json:"sources"`
}

// FilmDetail 后台影片详情: 主站(按 mid) + 各补充源(按 match_key 命中)装配所有源与集, 实时读库。
// 与公开 assembleSources 同口径但不缓存、不排延时, 便于采集后立即查看。
func (s *ManageService) FilmDetail(ctx context.Context, mid int64) (*ManageFilmDetail, error) {
	m, err := s.movie.GetByMid(ctx, mid)
	if err != nil {
		return nil, err
	}
	rows, err := s.play.ListByMid(ctx, mid)
	if err != nil {
		return nil, err
	}
	keys := []string{domain.GenerateHashKey(m.Name)}
	if m.DbId > 0 {
		keys = append(keys, domain.GenerateHashKey(m.DbId))
	}
	matched, err := s.play.GetByMatchKeys(ctx, "", keys)
	if err != nil {
		return nil, err
	}
	// 采集源元信息: siteId → 维护名 + 是否主站
	srcMeta := map[string]entity.CollectSource{}
	if all, e := s.sources.List(ctx, false); e == nil {
		for _, c := range all {
			srcMeta[c.Id] = c
		}
	}
	seen := map[string]struct{}{}
	out := make([]ManageFilmSource, 0, len(rows)+len(matched))
	add := func(ps entity.MoviePlaySource) {
		dedup := ps.SiteId + "\x00" + ps.PlayFrom
		if _, ok := seen[dedup]; ok {
			return
		}
		seen[dedup] = struct{}{}
		meta, ok := srcMeta[ps.SiteId]
		name := ps.SiteId
		master := false
		if ok {
			if meta.Name != "" {
				name = meta.Name
			}
			master = meta.Grade == entity.GradeMaster
		}
		out = append(out, ManageFilmSource{
			SiteId: ps.SiteId, SiteName: name, PlayFrom: ps.PlayFrom, Master: master, Episodes: ps.Episodes,
		})
	}
	for _, r := range rows {
		add(r)
	}
	for _, r := range matched {
		add(r)
	}
	return &ManageFilmDetail{Movie: *m, Sources: out}, nil
}

// AddFilm 手动新增/编辑影片: 事务双写 movie + movie_search(mid 缺省用毫秒时间戳), 失效相关缓存。
func (s *ManageService) AddFilm(ctx context.Context, m *entity.Movie) (int64, error) {
	if m.Name == "" {
		return 0, domain.ErrInvalidArgument
	}
	if m.Mid == 0 {
		m.Mid = time.Now().UnixMilli()
	}
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.movie.Upsert(ctx, m); err != nil {
			return err
		}
		return s.search.Upsert(ctx, domain.ProjectMovieToSearch(m))
	})
	if err != nil {
		return 0, err
	}
	cache.InvalidateMovie(ctx, m.Mid)
	return m.Mid, nil
}

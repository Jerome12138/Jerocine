package service

import (
	"context"
	"fmt"
	"log"
	"path"
	"regexp"
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
	"server/internal/tmdb"
)

// collectSourceIDRe 采集源 id 命名规则: 小写字母开头, 仅小写字母/数字/下划线, 2~32 字符。
// id 是主键且被四处以字符串引用(无外键), 规则保证跨库可读、可迁移、永不冲突保留字大小写形态。
var collectSourceIDRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// siteURLRe 站点网址规则(选填): http/https 绝对 URL。
var siteURLRe = regexp.MustCompile(`^https?://\S+$`)

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
	tmdb     *tmdb.Client // TMDB key 保存前验真
	// OnTMDBKeyChange key 保存/清除后的回调(组合根接 backdropSvc.Kick): 新 key 立即参与下一轮回填。
	OnTMDBKeyChange func()

	// siteCollected 用的 30s 内存缓存(CountBySite 全表统计 ~800ms, 高频调用必缓存)。
	countCacheMu sync.Mutex
	countCacheAt time.Time
	countCache   map[string]int64
}

func NewManageService(
	sources repository.CollectSourceRepository, crons repository.CronTaskRepository,
	siteCfg repository.SiteConfigRepository, versions repository.AppVersionRepository,
	files repository.FileRepository, category repository.CategoryRepository,
	search repository.SearchRepository, movie repository.MovieRepository,
	play repository.PlaySourceRepository,
	health repository.SourceHealthRepository,
	tx repository.TxManager, users *UserService, blob blobstore.BlobStore,
	tmdbc *tmdb.Client,
) *ManageService {
	return &ManageService{
		sources: sources, crons: crons, siteCfg: siteCfg, versions: versions,
		files: files, category: category, search: search, movie: movie, play: play,
		health: health, tx: tx, users: users, blob: blob, tmdb: tmdbc,
		prober: spider.NewFetcherWithTimeout(probeTimeout),
	}
}

// siteCollected 各源已采集片数(30s 内存缓存)。
// CountBySite 是 movie_play_source 按 site_id 的 GROUP BY 全表统计(线上实测 ~800ms),
// 而源健康列表/健康检查会高频调用(曾观察到每 ~10s 一次), 每次都打全表会持续占用 MySQL。
// 30s 的陈旧度对"已采集片数"展示无感, 换来的 DB 压力下降是实打实的。
const siteCollectedCacheTTL = 30 * time.Second

func (s *ManageService) siteCollected(ctx context.Context) map[string]int64 {
	s.countCacheMu.Lock()
	defer s.countCacheMu.Unlock()
	if s.countCache != nil && time.Since(s.countCacheAt) < siteCollectedCacheTTL {
		return s.countCache
	}
	m, err := s.play.CountBySite(ctx)
	if err != nil {
		if s.countCache != nil {
			return s.countCache // 统计失败退回旧缓存, 展示不闪断
		}
		return nil
	}
	s.countCache, s.countCacheAt = m, time.Now()
	return m
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
	PendingFails int64 `json:"pendingFails"` // 待补采的失败页数(由 handler 从采集服务注入)
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
	// TmdbAPIKey 不走本入口(管理端 json:"-" 绑不进来, 恒为空串): 用库内现值回填,
	// 防止 UpdateAll upsert 把后台配好的 key 抹成空。
	if cur, err := s.siteCfg.Get(ctx); err == nil {
		c.TmdbAPIKey = cur.TmdbAPIKey
	}
	if err := s.siteCfg.Save(ctx, c); err != nil {
		return err
	}
	cache.InvalidateConfig(ctx)
	return nil
}

// SetTMDBKey 保存 TMDB 凭据(v3 key / v4 token): 保存前到 TMDB 验真, 存错 key 会让横图静默不回填。
// 成功后失效配置缓存并触发回调(横图 worker 下一轮立即用新 key)。
func (s *ManageService) SetTMDBKey(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return domain.ErrInvalidArgument
	}
	if err := s.tmdb.Verify(ctx, key); err != nil {
		return err
	}
	c, err := s.siteCfg.Get(ctx)
	if err != nil {
		return err
	}
	changed := c.TmdbAPIKey != key
	c.TmdbAPIKey = key
	if err := s.siteCfg.Save(ctx, c); err != nil {
		return err
	}
	cache.InvalidateConfig(ctx)
	if changed {
		safeNotify("tmdb key change", s.OnTMDBKeyChange)
	}
	return nil
}

// ClearTMDBKey 清除凭据: 下一轮横图 worker 探测到空 key 即整体停摆(本地已回填的图片不受影响)。
func (s *ManageService) ClearTMDBKey(ctx context.Context) error {
	c, err := s.siteCfg.Get(ctx)
	if err != nil {
		return err
	}
	if c.TmdbAPIKey == "" {
		return nil
	}
	c.TmdbAPIKey = ""
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

// UpsertSource 新增/编辑采集源(站点网址 siteUrl 可选, 填了须为 http/https; 由 handler 绑定 JSON → repo Upsert 全列写入)。
// id 必须符合 collectSourceIDRe —— 落库后不可改(改 id 等于新建, 播放源/失败台账/健康表的字符串引用全部悬挂)。
func (s *ManageService) UpsertSource(ctx context.Context, src *entity.CollectSource) error {
	if !collectSourceIDRe.MatchString(src.Id) {
		return domain.ErrInvalidSourceID
	}
	if src.SiteUrl != "" && !siteURLRe.MatchString(src.SiteUrl) {
		return domain.ErrInvalidSourceID // 站点网址选填, 填了必须是 http(s):// URL
	}
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
	PlayLatencyMs int64  `json:"playLatencyMs"` // 服务端抽样 m3u8 延时(0=未测); >0 即服务端可达(可代理过滤)
	SampleM3u8    string `json:"sampleM3u8,omitempty"` // 探测解析到的样本 m3u8(端侧播放测速用)
	AdFilterOk    *bool  `json:"adFilterOk,omitempty"` // 服务端 m3u8 可达性: nil=未测(无样本) true=可代理过滤 false=不可达
	Message       string `json:"message"`
}

// SourceTestResult 批量测速结果(带站点标识, 便于前端排序)。
type SourceTestResult struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	CollectTestResult
}

// TestSource 对单源连续打真实采集请求(ac=detail), 实测延时/成功率/可采集性, 并写入健康度。
// 顺带抽样服务端拉取样本 m3u8 的延时 → 判定服务端广告过滤代理链路可用性。
func (s *ManageService) TestSource(ctx context.Context, id string) (CollectTestResult, error) {
	src, err := s.sources.Get(ctx, id)
	if err != nil {
		return CollectTestResult{}, err
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
	// 服务端抽样 m3u8: 计时拉取一次。成功 → 服务端代理(广告过滤)链路可用; 失败(含非 200) → 不可达。
	// 该指标与"采集速度"同源产出, 随测采集一起刷新。
	if sampleM3u8 != "" {
		res.SampleM3u8 = sampleM3u8
		if pl, err := s.prober.MeasureURL(ctx, sampleM3u8); err == nil {
			res.PlayLatencyMs = pl
			ok := true
			res.AdFilterOk = &ok
		} else {
			ok := false
			res.AdFilterOk = &ok
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
	Grade            int8   `json:"grade"`
	IsMaster         bool   `json:"isMaster"` // 当前主站(grade=0)
	Status           string `json:"status"`
	Suppressed       bool   `json:"suppressed"`
	LatencyMs        int64  `json:"latencyMs"`
	Films            int    `json:"films"`
	PageCount        int    `json:"pageCount"`
	Collected        int64  `json:"collected"`      // 已采集片数(movie_play_source 该源行数, 实时)
	Total            int    `json:"total"`          // 目录总片数(资源最全)
	PlayLatencyMs    int64  `json:"playLatencyMs"`  // 服务端抽样 m3u8 延时(0=未测, 广告过滤可达性)
	OkCount          int    `json:"okCount"`
	Probes           int    `json:"probes"`
	ConsecutiveFails int    `json:"consecutiveFails"`
	Message          string `json:"message"`
	CheckedAt        int64  `json:"checkedAt"`
	// 测速拆分: 采集=服务端 API, 播放=浏览器端直连 CDN(回传落库)。
	ApiCheckedAt   int64  `json:"apiCheckedAt"`
	PlayLatencyWeb int64  `json:"playLatencyWeb"`
	PlayCheckedAt  int64  `json:"playCheckedAt"`
	SampleM3u8     string `json:"sampleM3u8"`   // 样本 m3u8(端侧播放测速用)
	AdFilterOk     *bool  `json:"adFilterOk"`   // nil=未测
	AdFilterAt     int64  `json:"adFilterAt"`   // 广告过滤可达性检测时间
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
	// 已采集片数: GROUP BY site_id 全量统计(实测 ~800ms), 30s 内存缓存兜住高频调用。
	var collected map[string]int64
	if s.play != nil {
		collected = s.siteCollected(ctx)
	}
	out := make([]SourceHealthView, 0, len(srcs))
	for _, src := range srcs {
		v := SourceHealthView{
			Id: src.Id, Name: src.Name, State: src.State == entity.StateEnabled,
			Grade: src.Grade, IsMaster: src.Grade == entity.GradeMaster, Status: entity.HealthUnknown,
			Collected: collected[src.Id],
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
			v.ApiCheckedAt = h.ApiCheckedAt
			v.PlayLatencyWeb = h.PlayLatencyWeb
			v.PlayCheckedAt = h.PlayCheckedAt
			v.SampleM3u8 = h.SampleM3u8
			v.AdFilterOk = h.AdFilterOk
			v.AdFilterAt = h.AdFilterCheckedAt
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
	h.ApiCheckedAt = now // 本轮是采集测速(服务端 API), 顺带产出服务端 m3u8 可达性
	h.LastOk = res.Ok
	h.LatencyMs = res.LatencyMs
	h.BestMs = res.BestMs
	h.Films = res.Films
	h.PageCount = res.PageCount
	h.OkCount = res.OkCount
	h.Probes = res.Probes
	h.Message = res.Message
	// 目录大小/播放延时/样本 m3u8: 成功探测才更新, 失败保留上次(避免抖动把目录量清 0)
	if res.Total > 0 {
		h.Total = res.Total
	}
	if res.PlayLatencyMs > 0 {
		h.PlayLatencyMs = res.PlayLatencyMs
	}
	if res.SampleM3u8 != "" {
		h.SampleM3u8 = res.SampleM3u8
	}
	// 广告过滤可达性: 有样本才判定(成功/失败都写, 失败同样有价值——提示代理链路不可用)
	if res.AdFilterOk != nil {
		h.AdFilterOk = res.AdFilterOk
		h.AdFilterCheckedAt = now
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

// SampleM3u8For 取一条源当前可用的样本 m3u8(端侧播放测速的输入)。
// 样本必须新鲜: 存量样本对应的影片可能已下线, 端侧解析必失败造成假阴性 —— 故每次实时探测;
// 探测失败(源临时不可达)再回退健康表存量样本。
func (s *ManageService) SampleM3u8For(ctx context.Context, id string) (string, error) {
	src, err := s.sources.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if pr, perr := s.prober.Probe(ctx, src); perr == nil && pr.SampleM3u8 != "" {
		// 顺手刷新健康表样本(best-effort, 失败不影响返回)
		if s.health != nil {
			if h, e := s.health.Get(ctx, id); e == nil && h != nil && h.SampleM3u8 != pr.SampleM3u8 {
				h.SampleM3u8 = pr.SampleM3u8
				h.UpdatedAt = time.Now().UnixMilli()
				_ = s.health.Upsert(ctx, h)
			}
		}
		return pr.SampleM3u8, nil
	}
	if s.health != nil {
		if h, err := s.health.Get(ctx, id); err == nil && h != nil && h.SampleM3u8 != "" {
			return h.SampleM3u8, nil
		}
	}
	return "", nil
}

// RecordPlayLatency 落库浏览器端播放测速结果(测播放回传): 只更新播放侧字段, 不动采集侧健康状态。
func (s *ManageService) RecordPlayLatency(ctx context.Context, id string, ms int64) error {
	if s.health == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	prev, _ := s.health.Get(ctx, id)
	h := &entity.SourceHealth{}
	if prev != nil {
		*h = *prev
	}
	h.SourceId = id
	h.PlayLatencyWeb = ms
	h.PlayCheckedAt = now
	h.UpdatedAt = now
	if err := s.health.Upsert(ctx, h); err != nil {
		return err
	}
	return nil
}

// RecordAdFilter 播放时兜底上报: 广告过滤链路实际失败 → 立即标不可达(不等下一轮定时测速)。
// ok=true 仅在定时测速里恢复; 运行时只上报失败, 避免单次成功误刷掉定时结论。
func (s *ManageService) RecordAdFilter(ctx context.Context, id string, ok bool) error {
	if ok {
		return nil
	}
	if s.health == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	prev, _ := s.health.Get(ctx, id)
	h := &entity.SourceHealth{}
	if prev != nil {
		*h = *prev
	}
	h.SourceId = id
	f := false
	h.AdFilterOk = &f
	h.AdFilterCheckedAt = now
	h.UpdatedAt = now
	if err := s.health.Upsert(ctx, h); err != nil {
		return err
	}
	return nil
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
func (s *ManageService) ListUsers(ctx context.Context, keyword string, page repository.Page) ([]entity.User, int64, error) {
	return s.users.ManageListUsers(ctx, keyword, page)
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
		list, total, err = s.search.SearchKeyword(ctx, kw, spec.Deleted, page)
	} else {
		list, total, err = s.search.Filter(ctx, spec, page)
	}
	if err != nil {
		return CardPage{}, err
	}
	return CardPage{List: list, Total: total, Page: page}, nil
}

// GetFilm 后台读取单部影片; 含已软删(后台要能看到并恢复它们)。
func (s *ManageService) GetFilm(ctx context.Context, mid int64) (*entity.Movie, error) {
	return s.movie.GetByMidIncludingDeleted(ctx, mid)
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
	m, err := s.movie.GetByMidIncludingDeleted(ctx, mid)
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

// SoftDeleteFilm 软删影片: 事务双写 movie + movie_search 的 deleted_at, 再失效内容缓存。
// 不做物理删除 —— 采集源随时会把同一部片再推一遍, 物理删早晚会被长回来;
// 打标记后公开读路径立刻看不到它, 后台可在回收站恢复。
func (s *ManageService) SoftDeleteFilm(ctx context.Context, mid int64) error {
	if mid <= 0 {
		return domain.ErrInvalidArgument
	}
	at := time.Now().UnixMilli()
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// 先判存在性。SoftDelete 对不存在的 mid 是"0 行受影响", 但 0 行同样可能来自
		// "本来就已经删过了" —— 光看影响行数分不清这两种情况(MySQL affected_rows 是
		// 实际变更行数), 于是以前对一笔都不存在的 mid 也返回成功, 前端照样弹"已删除"。
		if _, err := s.movie.GetByMidIncludingDeleted(ctx, mid); err != nil {
			return err // 不存在 → ErrMovieNotFound → 404
		}
		if err := s.movie.SoftDelete(ctx, mid, at); err != nil {
			return err
		}
		return s.search.SoftDelete(ctx, mid, at)
	})
	if err != nil {
		return err
	}
	cache.InvalidateMovie(ctx, mid)
	cache.InvalidateAfterCollect(ctx) // 列表/分类页/标签/推荐都受影响, 按变更片处理
	return nil
}

// RestoreFilm 恢复被软删的影片, 与 SoftDeleteFilm 对称。
func (s *ManageService) RestoreFilm(ctx context.Context, mid int64) error {
	if mid <= 0 {
		return domain.ErrInvalidArgument
	}
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// 同 SoftDeleteFilm: 先判存在, 否则对不存在的 mid 也报"恢复成功"。
		if _, err := s.movie.GetByMidIncludingDeleted(ctx, mid); err != nil {
			return err
		}
		if err := s.movie.Restore(ctx, mid); err != nil {
			return err
		}
		return s.search.Restore(ctx, mid)
	})
	if err != nil {
		return err
	}
	cache.InvalidateMovie(ctx, mid)
	cache.InvalidateAfterCollect(ctx)
	return nil
}

package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// TelemetryService 埋点入库(公开) + 分析查询/问题闭环(后台)。
type TelemetryService struct {
	repo      repository.TelemetryRepository
	search    repository.SearchRepository   // 关联片名/封面(热点视频/收藏最多)
	favorites repository.FavoriteRepository // 收藏计数
	allowed   []string                      // 埋点允许来源域的 env 兜底/补充(主来源 = 站点配置 domain)
	cfg       *ConfigService                // 取站点配置 domain(缓存)作允许域, 后台改即生效
}

func NewTelemetryService(repo repository.TelemetryRepository, search repository.SearchRepository, favorites repository.FavoriteRepository, allowedHosts []string, cfg *ConfigService) *TelemetryService {
	return &TelemetryService{repo: repo, search: search, favorites: favorites, allowed: allowedHosts, cfg: cfg}
}

const maxIngestBatch = 200

// 机器人/扫描器 UA 子串(小写匹配)。空 UA 也视为机器人。
var botUASubstrings = []string{
	"bot", "spider", "crawl", "slurp", "curl", "wget", "python", "go-http",
	"java/", "scrapy", "headless", "phantom", "masscan", "zgrab", "nmap",
	"censys", "semrush", "ahrefs", "mj12", "dotbot", "bytespider", "facebookexternalhit",
}

// 扫描器探测的非路由垃圾路径(历史清理用; ingest 已按来源域拦截)。
var junkPaths = []string{"/probe", "/x"}

// extractHost 从 URL 或 host[:port] 取纯主机名(小写)。
func extractHost(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil {
			s = u.Host
		}
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i] // 去端口
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// allowedHosts 当前允许的来源域 = 站点配置 domain(后台可改, 缓存)+ env 兜底/补充。
func (s *TelemetryService) allowedHosts(ctx context.Context) []string {
	hosts := append([]string{}, s.allowed...)
	if s.cfg != nil {
		if sc, err := s.cfg.SiteConfig(ctx); err == nil && sc != nil && sc.Domain != "" {
			if h := extractHost(sc.Domain); h != "" {
				hosts = append(hosts, h)
			}
		}
	}
	return hosts
}

// hostAllowedIn host 是否属允许名单(精确或 *.base)。
func hostAllowedIn(host string, allow []string) bool {
	if host == "" {
		return false
	}
	for _, base := range allow {
		b := strings.ToLower(strings.TrimSpace(base))
		if b != "" && (host == b || strings.HasSuffix(host, "."+b)) {
			return true
		}
	}
	return false
}

// isBotUA UA 是否机器人/扫描器(含空 UA)。
func isBotUA(ua string) bool {
	u := strings.ToLower(strings.TrimSpace(ua))
	if u == "" {
		return true
	}
	for _, s := range botUASubstrings {
		if strings.Contains(u, s) {
			return true
		}
	}
	return false
}

// SourceHost 解析一次上报的来源域: 优先 Origin, 次 Referer, 再退请求 Host。
func (s *TelemetryService) SourceHost(reqHost, origin, referer string) string {
	host := extractHost(origin)
	if host == "" {
		host = extractHost(referer)
	}
	if host == "" {
		host = extractHost(reqHost)
	}
	return host
}

// IsNoise 判定一次埋点上报是否为攻击/扫描噪声: bot UA, 或来源域不属允许名单(撞 IP/外域/脚本直POST)。
func (s *TelemetryService) IsNoise(ctx context.Context, reqHost, origin, referer, ua string) bool {
	if isBotUA(ua) {
		return true
	}
	return !hostAllowedIn(s.SourceHost(reqHost, origin, referer), s.allowedHosts(ctx))
}

// PurgeNoise 清理历史脏数据: bot/空 UA 或垃圾路径的行。返回删除条数。
// 注: 历史行未存 Host, 浏览器UA的爬虫行无法回溯识别, 只能清明确的机器人/探测噪声。
func (s *TelemetryService) PurgeNoise(ctx context.Context) (int64, error) {
	return s.repo.DeleteNoise(ctx, botUASubstrings, junkPaths)
}

// PurgeApiErrors 一次性清理误记的 API HTTP 错误历史(extra.type=error 且 category 以 api- 开头, 如 api-4xx/api-5xx)。
// 前端已停止把 HTTP 4xx/5xx 上报为 error 事件, 此处把历史脏行清掉(部署后手动调一次)。返回删除条数。
func (s *TelemetryService) PurgeApiErrors(ctx context.Context) (int64, error) {
	return s.repo.DeleteApiErrors(ctx)
}

// PurgeErrorsBefore 清理 days 天前的错误事件(按龄保留, days<=0 默认 5 天)。返回删除条数。
func (s *TelemetryService) PurgeErrorsBefore(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		days = 5
	}
	return s.repo.DeleteErrorsBefore(ctx, sinceUnixDays(days))
}

// PurgeChunkNoise 清理 chunk 加载失败类噪声错误(部署自愈现象)。返回删除条数。
func (s *TelemetryService) PurgeChunkNoise(ctx context.Context) (int64, error) {
	return s.repo.DeleteChunkNoise(ctx)
}

// IngestEvent 客户端上报的单条事件。
type IngestEvent struct {
	Category  string         `json:"category"`
	Path      string         `json:"path"`
	Label     string         `json:"label"`
	Value     float64        `json:"value"`
	Duration  int            `json:"duration"`
	ClientTs  int64          `json:"clientTs"`
	SessionId string         `json:"sessionId"`
	Extra     map[string]any `json:"extra"`
}

// TmOverview 看板概览 (前端 OverviewResp 契约)。
type TmOverview struct {
	PV          int64   `json:"pv"`
	UV          int64   `json:"uv"`
	UniqueUsers int64   `json:"uniqueUsers"`
	ErrorCount  int64   `json:"errorCount"`
	ApiCount    int64   `json:"apiCount"`
	AvgApiMs    float64 `json:"avgApiMs"`
	WindowDays  int     `json:"windowDays"`
}

// TmApiPerf 接口性能 (前端 ApiPerfItem 契约, 含百分位)。
type TmApiPerf struct {
	Action string  `json:"action"`
	Count  int64   `json:"count"`
	Avg    float64 `json:"avg"`
	P50    float64 `json:"p50"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
}

// TmEventRow 事件行 (前端 TelemetryEventRow 契约)。type/action/platform/appVersion 从 extra 取,
// extra 序列化为字符串(前端 JSON.parse), 并附问题闭环 issueStatus/resolution。
type TmEventRow struct {
	Id          int64                            `json:"id"`
	Ts          int64                            `json:"ts"`
	ServerTs    time.Time                        `json:"serverTs"`
	Type        string                           `json:"type"`
	Category    string                           `json:"category"`
	Action      string                           `json:"action"`
	Label       string                           `json:"label"`
	Value       float64                          `json:"value"`
	Path        string                           `json:"path"`
	UserId      int64                            `json:"userId"`
	SessionId   string                           `json:"sessionId"`
	Platform    string                           `json:"platform"`
	AppVersion  string                           `json:"appVersion"`
	Extra       string                           `json:"extra"`
	Resolution  *entity.TelemetryIssueResolution `json:"resolution,omitempty"`
	IssueStatus string                           `json:"issueStatus"`
}

// Ingest 批量入库。超过 200 条截断; 服务端补 server_ts / ip / ua / userId / extra.host(来源域)。
func (s *TelemetryService) Ingest(ctx context.Context, events []IngestEvent, userID *int64, ip, ua, srcHost string) error {
	if len(events) == 0 {
		return nil
	}
	if len(events) > maxIngestBatch {
		events = events[:maxIngestBatch]
	}
	now := time.Now()
	rows := make([]entity.TelemetryEvent, 0, len(events))
	for _, e := range events {
		extra := e.Extra
		if srcHost != "" { // 写入来源域, 便于看板按域筛选
			if extra == nil {
				extra = map[string]any{}
			}
			extra["host"] = srcHost
		}
		rows = append(rows, entity.TelemetryEvent{
			Category: e.Category, Path: e.Path, Label: e.Label, Value: e.Value,
			Duration: e.Duration, UserId: userID, SessionId: e.SessionId,
			Ip: ip, Ua: ua, Extra: entity.JSONMap(extra), ClientTs: e.ClientTs, ServerTs: now,
		})
	}
	return s.repo.BatchInsert(ctx, rows)
}

// Overview 看板概览, 时间窗按天。
func (s *TelemetryService) Overview(ctx context.Context, days int) (TmOverview, error) {
	if days <= 0 {
		days = 7
	}
	st, err := s.repo.OverviewStats(ctx, sinceUnixDays(days))
	if err != nil {
		return TmOverview{}, err
	}
	return TmOverview{
		PV: st.PV, UV: st.UV, UniqueUsers: st.UniqueUsers,
		ErrorCount: st.ErrorCount, ApiCount: st.ApiCount, AvgApiMs: st.AvgApiMs,
		WindowDays: days,
	}, nil
}

func (s *TelemetryService) TopPaths(ctx context.Context, sinceHours, limit int) ([]repository.PathCount, error) {
	return s.repo.TopPaths(ctx, sinceUnix(sinceHours), limit)
}

// ApiPerf 按接口路径(extra.action) 聚合 + 应用层近似百分位, 时间窗按天。
func (s *TelemetryService) ApiPerf(ctx context.Context, days, limit int) ([]TmApiPerf, error) {
	if days <= 0 {
		days = 7
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	since := sinceUnixDays(days)
	aggs, err := s.repo.ApiPerfByAction(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]TmApiPerf, 0, len(aggs))
	for _, a := range aggs {
		item := TmApiPerf{Action: a.Action, Count: a.Count, Avg: a.Avg}
		if vals, e := s.repo.ApiActionValues(ctx, since, a.Action, 1000); e == nil && len(vals) > 0 {
			sort.Float64s(vals)
			item.P50 = percentile(vals, 0.50)
			item.P95 = percentile(vals, 0.95)
			item.P99 = percentile(vals, 0.99)
		}
		out = append(out, item)
	}
	return out, nil
}

// ErrorEvents 看板错误列表: 按前端事件类型(extra.type)过滤 + 问题闭环 issueStatus + status 过滤。
// 返回的 total 为该 type 命中总数(状态过滤前), rows 为当前页过滤后行。
func (s *TelemetryService) ErrorEvents(ctx context.Context, days int, frontendType, status string, limit, offset int) ([]TmEventRow, int64, error) {
	if days <= 0 {
		days = 7
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	raw, total, err := s.repo.ListEventsByType(ctx, sinceUnixDays(days), frontendType, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	if len(raw) == 0 {
		return []TmEventRow{}, total, nil
	}
	hashes := make([]string, len(raw))
	for i, e := range raw {
		hashes[i] = IssueKey(e.Category, e.Path, e.Label)
	}
	resoMap, _ := s.repo.GetResolutions(ctx, hashes)
	now := time.Now()
	out := make([]TmEventRow, 0, len(raw))
	for i, e := range raw {
		res := resoMap[hashes[i]]
		st := classifyIssueStatus(res, e.ServerTs, now)
		if !statusMatches(status, st) {
			continue
		}
		out = append(out, toEventRow(e, res, st))
	}
	return out, total, nil
}

func (s *TelemetryService) Events(ctx context.Context, f repository.TelemetryFilter, page repository.Page) ([]entity.TelemetryEvent, int64, error) {
	return s.repo.ListEvents(ctx, f, page.Normalize(50))
}

// classifyIssueStatus 据 resolution(及其解决时间)与事件时间判定问题状态。
// 未解决/已撤销=unresolved; 解决>30天=expired; 解决后过24h灰度期又触发=reopened; 否则=resolved。
func classifyIssueStatus(res *entity.TelemetryIssueResolution, eventServerTs, now time.Time) string {
	if res == nil || !res.Resolved {
		return "unresolved"
	}
	resolvedAt := time.UnixMilli(res.UpdatedAt)
	const grace = 24 * time.Hour
	const expire = 30 * 24 * time.Hour
	switch {
	case now.Sub(resolvedAt) > expire:
		return "expired"
	case eventServerTs.After(resolvedAt.Add(grace)):
		return "reopened"
	default:
		return "resolved"
	}
}

// statusMatches 看板 status 过滤: 默认 active=未解决+重启; resolved/reopened/expired 各自; all=全部。
func statusMatches(filter, status string) bool {
	switch filter {
	case "", "active":
		return status == "unresolved" || status == "reopened"
	case "all":
		return true
	default:
		return filter == status
	}
}

// toEventRow 把存储行映射为前端契约行: type/action/platform/appVersion 从 extra 取, extra 序列化为字符串。
func toEventRow(e entity.TelemetryEvent, res *entity.TelemetryIssueResolution, status string) TmEventRow {
	extraStr := ""
	if len(e.Extra) > 0 {
		if b, err := json.Marshal(e.Extra); err == nil {
			extraStr = string(b)
		}
	}
	getStr := func(k string) string {
		if e.Extra == nil {
			return ""
		}
		if v, ok := e.Extra[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	uid := int64(0)
	if e.UserId != nil {
		uid = *e.UserId
	}
	return TmEventRow{
		Id: e.Id, Ts: e.ClientTs, ServerTs: e.ServerTs,
		Type: getStr("type"), Category: e.Category, Action: getStr("action"),
		Label: e.Label, Value: e.Value, Path: e.Path, UserId: uid, SessionId: e.SessionId,
		Platform: getStr("platform"), AppVersion: getStr("appVersion"),
		Extra: extraStr, Resolution: res, IssueStatus: status,
	}
}

// percentile 取已升序数组的近似分位值。
func percentile(sortedAsc []float64, p float64) float64 {
	if len(sortedAsc) == 0 {
		return 0
	}
	idx := int(float64(len(sortedAsc)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sortedAsc) {
		idx = len(sortedAsc) - 1
	}
	return sortedAsc[idx]
}

func sinceUnixDays(days int) int64 {
	if days <= 0 {
		return 0
	}
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
}

// ResolveIssue 标记问题已解决(key=sha1(category|path|label)), 关联 commitId。
func (s *TelemetryService) ResolveIssue(ctx context.Context, category, path, label, commitId string) error {
	key := IssueKey(category, path, label)
	now := time.Now().UnixMilli()
	res := &entity.TelemetryIssueResolution{
		KeyHash: key, Category: category, Path: path, Label: label,
		ResolvedCommitId: commitId, Resolved: true, CreatedAt: now, UpdatedAt: now,
	}
	return s.repo.UpsertResolution(ctx, res)
}

// UnresolveIssue 撤销解决标记, reopened_count +1。
func (s *TelemetryService) UnresolveIssue(ctx context.Context, category, path, label string) error {
	key := IssueKey(category, path, label)
	reopened := 1
	if cur, err := s.repo.GetResolution(ctx, key); err == nil && cur != nil {
		reopened = cur.ReopenedCount + 1
	}
	now := time.Now().UnixMilli()
	res := &entity.TelemetryIssueResolution{
		KeyHash: key, Category: category, Path: path, Label: label,
		Resolved: false, ReopenedCount: reopened, CreatedAt: now, UpdatedAt: now,
	}
	return s.repo.UpsertResolution(ctx, res)
}

// IssueKey = sha1(category|path|label)。
func IssueKey(category, path, label string) string {
	h := sha1.Sum([]byte(category + "|" + path + "|" + label))
	return hex.EncodeToString(h[:])
}

func sinceUnix(hours int) int64 {
	if hours <= 0 {
		return 0
	}
	return time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
}

// ---- 热点视频 / 收藏最多 ----

// FilmStat 热点视频 / 收藏最多 单项(含片名封面便于直接展示)。
type FilmStat struct {
	Mid   int64  `json:"mid"`
	Name  string `json:"name"`
	Cover string `json:"cover"`
	Count int64  `json:"count"`
}

// HotFilms 热点视频: pv 访问量统计。label(/filmDetail?link= 或 /play?id=) 解析 mid 聚合, 关联片名/封面。
func (s *TelemetryService) HotFilms(ctx context.Context, sinceHours, limit int) ([]FilmStat, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.repo.TopFilmViews(ctx, sinceUnix(sinceHours), 500)
	if err != nil {
		return nil, err
	}
	counts := make(map[int64]int64)
	for _, r := range rows {
		if mid := midFromLabel(r.Path); mid > 0 {
			counts[mid] += r.Count
		}
	}
	mids := make([]int64, 0, len(counts))
	for mid := range counts {
		mids = append(mids, mid)
	}
	sort.Slice(mids, func(i, j int) bool { return counts[mids[i]] > counts[mids[j]] })
	if len(mids) > limit {
		mids = mids[:limit]
	}
	return s.buildFilmStats(ctx, mids, counts), nil
}

// MostFavorited 收藏最多。
func (s *TelemetryService) MostFavorited(ctx context.Context, limit int) ([]FilmStat, error) {
	if limit <= 0 {
		limit = 10
	}
	if s.favorites == nil {
		return nil, nil
	}
	rows, err := s.favorites.TopFavorited(ctx, limit)
	if err != nil {
		return nil, err
	}
	counts := make(map[int64]int64, len(rows))
	mids := make([]int64, 0, len(rows))
	for _, r := range rows {
		counts[r.Mid] = r.Count
		mids = append(mids, r.Mid) // 已按 count DESC 排序
	}
	return s.buildFilmStats(ctx, mids, counts), nil
}

// buildFilmStats 按给定 mid 顺序关联片名/封面(search 缺失则只返回 mid+count)。
func (s *TelemetryService) buildFilmStats(ctx context.Context, mids []int64, counts map[int64]int64) []FilmStat {
	meta := map[int64]entity.MovieSearch{}
	if s.search != nil && len(mids) > 0 {
		if list, err := s.search.GetByMids(ctx, mids); err == nil {
			for _, m := range list {
				meta[m.Mid] = m
			}
		}
	}
	out := make([]FilmStat, 0, len(mids))
	for _, mid := range mids {
		fs := FilmStat{Mid: mid, Count: counts[mid]}
		if m, ok := meta[mid]; ok {
			fs.Name = m.Name
			fs.Cover = m.Cover
		}
		out = append(out, fs)
	}
	return out
}

// midFromLabel 从 pv label 解析影片 mid: /filmDetail?link=123 或 /play?id=123&...。
func midFromLabel(label string) int64 {
	u, err := url.Parse(label)
	if err != nil {
		return 0
	}
	q := u.Query()
	for _, key := range []string{"link", "id"} {
		if v := q.Get(key); v != "" {
			if n, e := strconv.ParseInt(v, 10, 64); e == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

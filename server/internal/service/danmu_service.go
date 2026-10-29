package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"server/internal/cache"
	"server/internal/danmu"
	"server/internal/domain"
	"server/internal/domain/repository"
)

// 弹幕缓存 TTL：正缓存 7d（db_id/season 资源 id/整集弹幕长期稳定）；负缓存短 TTL 2h
// （评审 7.6：db_id 空 / 无 vendors / 无匹配集 / danmu-service 无弹幕的低频片也要挡住重复打上游）。
const (
	danmuSourceTTL = 7 * 24 * time.Hour
	danmuEmptyTTL  = 2 * time.Hour
	// danmuServiceTimeout 路径 A（B站整集聚合）超时：29 段串行 + 段间退避 ≈ 6-10s，
	// 留 30s 大于最坏聚合耗时（8.2 约定）。danmu-service 不可用/超时 → 静默降级"暂无弹幕"。
	danmuServiceTimeout = 30 * time.Second
	// danmuSegmentTimeout 路径 B（腾讯等分段点播）单请求超时：3s（8.2 约定，小于整集聚合 30s）。
	danmuSegmentTimeout = 3 * time.Second
)

// danmuProviderAll 后台可选的弹幕平台全集（须与 danmu-service 壳 ID_PREFIX 同步：
// 平台要可用，两边都要启用 —— 壳白名单决定上游能否拉，这里决定 Go/前端是否开放）。
var danmuProviderAll = []string{"bilibili", "tencent", "iqiyi", "youku"}

// danmuEnabledProviders 后台启用的弹幕平台（总开关 + 白名单过滤）。
// 返回 nil 表示弹幕整体未启用（DanmuSource 空候选、路径 A/B 全 400）。
// 直接读 site_config 库现值（不经 ConfigService 缓存 —— v1:cfg:site 是 JSON 序列化
// 往返，而弹幕两列 json:"-" 会被裁剪成恒零值；同 MailConfig 范式读 repo）。
func (s *DanmuService) danmuEnabledProviders(ctx context.Context) []string {
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil || cfg.DanmuEnabled != 1 || len(cfg.DanmuProviders) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(danmuProviderAll))
	for _, p := range danmuProviderAll {
		allowed[p] = true
	}
	out := make([]string, 0, len(cfg.DanmuProviders))
	for _, p := range cfg.DanmuProviders {
		if allowed[p] {
			out = append(out, p)
		}
	}
	return out
}

// validatePathBProvider provider 未启用 → ErrInvalidArgument（dto.Fail 映射 400）。
// 必须在拼 URL 之前调用：provider 来自路由参数，未启用的值不得进请求路径。
// 白名单 = 后台配置（danmu_enabled + danmu_providers），非写死。
func (s *DanmuService) validatePathBProvider(ctx context.Context, provider string) error {
	enabled := s.danmuEnabledProviders(ctx)
	for _, p := range enabled {
		if p == provider {
			return nil
		}
	}
	return fmt.Errorf("unsupported danmu provider %q（启用：%s）: %w",
		provider, strings.Join(enabled, "/"), domain.ErrInvalidArgument)
}

// DanmuSourceResp 弹幕来源响应（评审 7.7：不暴露 dbId，只给平台候选）。
type DanmuSourceResp struct {
	Providers []DanmuProvider `json:"providers"`
}

// DanmuProvider 一个可拉弹幕的平台。
type DanmuProvider struct {
	Provider   string `json:"provider"`   // tencent / iqiyi / youku / bilibili
	ResourceID string `json:"resourceId"` // 平台侧整剧/整片资源 id
}

// DanmuComment 对外弹幕条目（对齐 danmu-service 规范化契约 {text,time,color,mode}，毫秒）。
type DanmuComment struct {
	Text  string `json:"text"`
	Time  int64  `json:"time"`
	Color uint32 `json:"color"`
	Mode  int32  `json:"mode"`
}

// DanmuService 弹幕服务（D2：vendors 候选 Go 自留；分集/弹幕取数走 danmu-service）。
type DanmuService struct {
	movie    repository.MovieRepository
	client   *danmu.Client
	dandan   *danmu.DandanClient // 聚合弹幕（dandanplay v2 兼容，按标题匹配）
	danmuURL string
	siteCfg  repository.SiteConfigRepository // 弹幕后台配置（总开关 + 平台白名单，直读库现值）
	hc       *http.Client                    // 路径 A：整集聚合 30s
	segHC    *http.Client                    // 路径 B：分段点播 3s
}

// NewDanmuService 构造弹幕服务。danmuURL 为 danmu-service 基址（config.Danmu.ServiceURL）；
// siteCfg 用于读取后台弹幕配置（默认不启用）。
func NewDanmuService(movie repository.MovieRepository, client *danmu.Client, dandan *danmu.DandanClient, danmuURL string, siteCfg repository.SiteConfigRepository) *DanmuService {
	return &DanmuService{
		movie:    movie,
		client:   client,
		dandan:   dandan,
		danmuURL: strings.TrimRight(danmuURL, "/"),
		siteCfg:  siteCfg,
		hc:       &http.Client{Timeout: danmuServiceTimeout},
		segHC:    &http.Client{Timeout: danmuSegmentTimeout},
	}
}

// DanmuSource 按 mid 取弹幕来源候选（全平台，前端按优先级/用户选择取一个）。
//
// 缓存：命中（解析出 ≥1 平台）→ 正缓存 7d；未命中（db_id 空 / vendors 空 / 全部解析不出）
// → 负缓存 2h（GetOrLoadTTL 的 negTTL），避免冷门片每次播放都打豆瓣。
func (s *DanmuService) DanmuSource(ctx context.Context, mid int64) (DanmuSourceResp, error) {
	resp, _, err := cache.GetOrLoadTTL(ctx, cache.KeyDanmuSource(mid), danmuSourceTTL, danmuEmptyTTL,
		func(ctx context.Context) (DanmuSourceResp, bool, error) {
			return s.loadDanmuSource(ctx, mid)
		})
	if err != nil {
		return DanmuSourceResp{}, err
	}
	return resp, nil
}

// loadDanmuSource 回源：mid → db_id → 豆瓣 vendors → 平台资源 id（按后台配置过滤）。
// 弹幕未启用 → 空候选负缓存 2h（配置保存时清 v1:danmu:src:*，切换立即生效）。
func (s *DanmuService) loadDanmuSource(ctx context.Context, mid int64) (DanmuSourceResp, bool, error) {
	enabled := s.danmuEnabledProviders(ctx)
	if enabled == nil {
		return DanmuSourceResp{}, false, nil // 未启用：负缓存 2h
	}
	enabledSet := make(map[string]bool, len(enabled))
	for _, p := range enabled {
		enabledSet[p] = true
	}
	m, err := s.movie.GetByMid(ctx, mid)
	if err != nil {
		if errors.Is(err, domain.ErrMovieNotFound) {
			return DanmuSourceResp{}, false, nil
		}
		return DanmuSourceResp{}, false, err
	}
	if m.DbId <= 0 {
		return DanmuSourceResp{}, false, nil // 负缓存 2h
	}
	vendors, err := s.client.DoubanVendors(ctx, fmt.Sprintf("%d", m.DbId))
	if err != nil {
		return DanmuSourceResp{}, false, err
	}
	providers := make([]DanmuProvider, 0, len(vendors))
	for _, v := range vendors {
		pr, ok := danmu.ParsePlatformResource(v.URI)
		if !ok || !enabledSet[pr.Provider] {
			continue
		}
		providers = append(providers, DanmuProvider{Provider: pr.Provider, ResourceID: pr.ResourceID})
	}
	if len(providers) == 0 {
		return DanmuSourceResp{}, false, nil // 负缓存 2h
	}
	return DanmuSourceResp{Providers: providers}, true, nil
}

// DanmuBilibili 按 seasonId + 集号取该集全部弹幕（路径 A：整集聚合）。
//
// D2 起：Go 不再自研 B站 season/seg.so 解析，全部交给 danmu-service——
//  1. episodes 接口（resourceId=seasonId&episode=ep）拿分集匹配结果 matched.episodeId；
//  2. comments 接口（episodeId 缺省 segmentId = 整集聚合）一次返回整集弹幕；
//  3. 结果缓存 v1:danmu:bili:cmt:{seasonID}:{ep}（正 7d / 负 2h，8.6）。
//
// 降级分轨（评审修正）：danmu-service 调用失败（不可用/超时）→ 返回 error，
// GetOrLoadTTL 对 error 不写缓存 —— 避免容器重启/发布窗口把"无弹幕"负缓存 2h；
// 业务无数据（无匹配集/壳 ok=false/空弹幕）→ 负缓存 2h（挡低频片重复回源）。
// handler 对 error 返回 500，前端 .catch 后静默（不阻塞播放，用户体验不变）。
func (s *DanmuService) DanmuBilibili(ctx context.Context, seasonID string, episode int) ([]DanmuComment, error) {
	// 受后台配置控制：未启用 bilibili → 400（与路径 B 同一白名单）。
	if err := s.validatePathBProvider(ctx, "bilibili"); err != nil {
		return nil, err
	}
	comments, _, err := cache.GetOrLoadTTL(ctx, cache.KeyDanmuBiliCmt(seasonID, episode),
		danmuSourceTTL, danmuEmptyTTL,
		func(ctx context.Context) ([]DanmuComment, bool, error) {
			return s.loadDanmuBilibili(ctx, seasonID, episode)
		})
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// loadDanmuBilibili 回源 danmu-service 拉整集弹幕。
func (s *DanmuService) loadDanmuBilibili(ctx context.Context, seasonID string, episode int) ([]DanmuComment, bool, error) {
	// 1) 分集匹配（episodes 接口）
	epsURL := fmt.Sprintf("%s/danmu/bilibili/episodes?resourceId=%s&episode=%d",
		s.danmuURL, url.QueryEscape(seasonID), episode)
	var epsResp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Matched *struct {
				EpisodeID string `json:"episodeId"`
			} `json:"matched"`
		} `json:"data"`
	}
	if err := s.danmuGet(ctx, epsURL, &epsResp); err != nil {
		log.Printf("[danmu] bilibili episodes 调用失败(不缓存, 下次重试): %v", err)
		return nil, false, err
	}
	if !epsResp.Ok || epsResp.Data.Matched == nil || epsResp.Data.Matched.EpisodeID == "" {
		return nil, false, nil // 无匹配集：负缓存 2h（评审 7.3 降级口径）
	}
	episodeID := epsResp.Data.Matched.EpisodeID

	// 2) 整集弹幕（comments 接口，episodeId 复合串必须 encodeURIComponent）
	cmtURL := fmt.Sprintf("%s/danmu/bilibili/comments?episodeId=%s",
		s.danmuURL, url.QueryEscape(episodeID))
	var cmtResp struct {
		Ok   bool           `json:"ok"`
		Data []DanmuComment `json:"data"`
	}
	if err := s.danmuGet(ctx, cmtURL, &cmtResp); err != nil {
		log.Printf("[danmu] bilibili comments 调用失败(不缓存, 下次重试): %v", err)
		return nil, false, err
	}
	if !cmtResp.Ok {
		return nil, false, nil
	}
	if len(cmtResp.Data) == 0 {
		return nil, false, nil // 负缓存 2h
	}
	return cmtResp.Data, true, nil
}

// danmuGet 请求 danmu-service 并解析 JSON。非 200 / JSON 损坏 → error。
func (s *DanmuService) danmuGet(ctx context.Context, endpoint string, out any) error {
	return s.danmuGetWith(ctx, s.hc, endpoint, out)
}

// danmuSegGet 路径 B 专用：3s 短超时（分段点播单请求，8.2 约定）。
func (s *DanmuService) danmuSegGet(ctx context.Context, endpoint string, out any) error {
	return s.danmuGetWith(ctx, s.segHC, endpoint, out)
}

func (s *DanmuService) danmuGetWith(ctx context.Context, hc *http.Client, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("danmu-service status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("danmu-service 响应非 JSON: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 路径 B：腾讯/爱奇艺/优酷 分段点播（8.2）。前端按 currentTime 预拉（当前段+后续 2 段）。
// 与 B站整集聚合的差异：单请求 3s 超时、episodes/segments 不缓存（进片一次）、
// 单段内容缓存 v1:danmu:seg:{provider}:{epKey}:{segmentId}（8.6，正 7d/负 2h）。
// ---------------------------------------------------------------------------

// DanmuMatched 分集匹配结果（matched.episodeId 为复合串，透传时须 encodeURIComponent）。
type DanmuMatched struct {
	EpisodeID string `json:"episodeId"`
}

// DanmuSegment 弹幕分段（startTime 秒，壳按段起始排序；segmentId 透传壳）。
type DanmuSegment struct {
	Provider  string  `json:"provider"`
	StartTime float64 `json:"startTime"`
	SegmentID string  `json:"segmentId"`
}

// DanmuEpisodes 路径 B 分集匹配（episodes 接口转发；provider 须在白名单）。
// 返回 found=false 表示无匹配集（前端静默无弹幕）；错误分两档：
// provider 未启用 → ErrInvalidArgument（400）；danmu-service 不可用 → 500。
func (s *DanmuService) DanmuEpisodes(ctx context.Context, provider, resourceID string, episode int) (DanmuMatched, bool, error) {
	if err := s.validatePathBProvider(ctx, provider); err != nil {
		return DanmuMatched{}, false, err
	}
	epsURL := fmt.Sprintf("%s/danmu/%s/episodes?resourceId=%s&episode=%d",
		s.danmuURL, provider, url.QueryEscape(resourceID), episode)
	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Matched *DanmuMatched `json:"matched"`
		} `json:"data"`
	}
	if err := s.danmuSegGet(ctx, epsURL, &resp); err != nil {
		return DanmuMatched{}, false, fmt.Errorf("%s episodes: %w", provider, err)
	}
	if !resp.Ok || resp.Data.Matched == nil || resp.Data.Matched.EpisodeID == "" {
		return DanmuMatched{}, false, nil
	}
	return *resp.Data.Matched, true, nil
}

// DanmuSegments 路径 B 分段索引（segments 接口转发；不缓存，进片一次）。
func (s *DanmuService) DanmuSegments(ctx context.Context, provider, episodeID string) ([]DanmuSegment, error) {
	if err := s.validatePathBProvider(ctx, provider); err != nil {
		return nil, err
	}
	segURL := fmt.Sprintf("%s/danmu/%s/segments?episodeId=%s", s.danmuURL, provider, url.QueryEscape(episodeID))
	var resp struct {
		Ok   bool           `json:"ok"`
		Data []DanmuSegment `json:"data"`
	}
	if err := s.danmuSegGet(ctx, segURL, &resp); err != nil {
		return nil, fmt.Errorf("%s segments: %w", provider, err)
	}
	if !resp.Ok {
		return nil, fmt.Errorf("%s segments: ok=false", provider)
	}
	return resp.Data, nil
}

// DanmuComments 路径 B 单段弹幕（comments 接口转发；缓存 v1:danmu:seg）。
// 降级分轨同路径 A：调用失败 → error（不缓存）；业务无数据 → 负缓存 2h。
func (s *DanmuService) DanmuComments(ctx context.Context, provider, episodeID, segmentID string) ([]DanmuComment, error) {
	if err := s.validatePathBProvider(ctx, provider); err != nil {
		return nil, err
	}
	comments, _, err := cache.GetOrLoadTTL(ctx,
		cache.KeyDanmuSeg(provider, cache.EpKeyOf(episodeID), segmentID),
		danmuSourceTTL, danmuEmptyTTL,
		func(ctx context.Context) ([]DanmuComment, bool, error) {
			return s.loadDanmuSegComments(ctx, provider, episodeID, segmentID)
		})
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// loadDanmuSegComments 回源 danmu-service 拉单段弹幕。
func (s *DanmuService) loadDanmuSegComments(ctx context.Context, provider, episodeID, segmentID string) ([]DanmuComment, bool, error) {
	cmtURL := fmt.Sprintf("%s/danmu/%s/comments?episodeId=%s&segmentId=%s",
		s.danmuURL, provider, url.QueryEscape(episodeID), url.QueryEscape(segmentID))
	var resp struct {
		Ok   bool           `json:"ok"`
		Data []DanmuComment `json:"data"`
	}
	if err := s.danmuSegGet(ctx, cmtURL, &resp); err != nil {
		log.Printf("[danmu] %s comments 调用失败(不缓存, 下次重试): %v", provider, err)
		return nil, false, err
	}
	if !resp.Ok {
		return nil, false, nil
	}
	if len(resp.Data) == 0 {
		return nil, false, nil // 负缓存 2h
	}
	return resp.Data, true, nil
}

// ---------------------------------------------------------------------------
// 聚合弹幕（dandanplay v2 兼容，按标题匹配，与平台 vendors 链路相互独立）。
// 三段缓存：关键词搜索 7d / 作品分集 7d / 整集内容 7d（负缓存均 2h）。
// 服务未配置（无基址/令牌）或后台总开关关闭 → 静默降级为空弹幕，不影响播放。
// ---------------------------------------------------------------------------

// DanmuAggregateResp 聚合弹幕响应（复用 DanmuComment 毫秒契约）。
type DanmuAggregateResp struct {
	Comments []DanmuComment `json:"comments"`
}

// danmuMasterEnabled 聚合链路总开关（与平台链路共用 danmu_enabled，不套平台白名单）。
func (s *DanmuService) danmuMasterEnabled(ctx context.Context) bool {
	cfg, err := s.siteCfg.Get(ctx)
	return err == nil && cfg.DanmuEnabled == 1
}

// DanmuAggregate 按标题 + 集号取整集弹幕。查无作品/无该集 → domain.ErrNotFound（404，
// 前端标"该片暂无弹幕"）；取数异常 → 原样上抛（前端标失败可重试）。
func (s *DanmuService) DanmuAggregate(ctx context.Context, title string, year, episode int) (DanmuAggregateResp, error) {
	if strings.TrimSpace(title) == "" {
		return DanmuAggregateResp{}, fmt.Errorf("missing title: %w", domain.ErrInvalidArgument)
	}
	empty := DanmuAggregateResp{Comments: []DanmuComment{}}
	// 未配置聚合服务或后台关闭：空弹幕（不报错，前端标"未找到弹幕"）
	if s.dandan == nil || !s.dandan.Enabled() || !s.danmuMasterEnabled(ctx) {
		return empty, nil
	}
	episodeID, err := s.resolveAggregateEpisode(ctx, title, year, episode)
	if err != nil {
		return DanmuAggregateResp{}, err
	}
	resp, _, err := cache.GetOrLoadTTL(ctx, cache.KeyDanmuDdCmt(episodeID), danmuSourceTTL, danmuEmptyTTL,
		func(ctx context.Context) (DanmuAggregateResp, bool, error) {
			raw, err := s.dandan.Comments(ctx, episodeID)
			if err != nil {
				return DanmuAggregateResp{}, false, err
			}
			parsed := danmu.ParseDandanComments(raw)
			out := make([]DanmuComment, len(parsed))
			for i := range parsed {
				out[i] = DanmuComment{Text: parsed[i].Text, Time: parsed[i].Time, Color: parsed[i].Color, Mode: parsed[i].Mode}
			}
			return DanmuAggregateResp{Comments: out}, len(out) > 0, nil
		})
	if err != nil {
		return DanmuAggregateResp{}, err
	}
	return resp, nil
}

// resolveAggregateEpisode 两跳定位集：关键词 → 作品（缓存 7d）→ 分集列表（缓存 7d）→ 集。
func (s *DanmuService) resolveAggregateEpisode(ctx context.Context, title string, year, episode int) (int64, error) {
	kwKey := cache.EpKeyOf(danmu.NormalizeTitle(title))
	anime, _, err := cache.GetOrLoadTTL(ctx, cache.KeyDanmuDdAnime(kwKey), danmuSourceTTL, danmuEmptyTTL,
		func(ctx context.Context) (*danmu.DandanAnime, bool, error) {
			animes, err := s.dandan.SearchAnime(ctx, title)
			if err != nil {
				return nil, false, err
			}
			picked := danmu.PickAnime(animes, title, year, episode)
			if picked == nil {
				return nil, false, nil // 查无作品 → 负缓存 2h
			}
			cp := *picked
			return &cp, true, nil
		})
	if err != nil {
		return 0, err
	}
	if anime == nil {
		return 0, fmt.Errorf("aggregate anime not found: %w", domain.ErrNotFound)
	}
	eps, _, err := cache.GetOrLoadTTL(ctx, cache.KeyDanmuDdEps(anime.AnimeID), danmuSourceTTL, danmuEmptyTTL,
		func(ctx context.Context) ([]danmu.DandanEpisode, bool, error) {
			list, err := s.dandan.Bangumi(ctx, anime.AnimeID)
			if err != nil {
				return nil, false, err
			}
			return list, len(list) > 0, nil
		})
	if err != nil {
		return 0, err
	}
	ep := danmu.PickEpisode(eps, episode)
	if ep == nil {
		return 0, fmt.Errorf("aggregate episode not found %s#%d: %w", anime.AnimeTitle, episode, domain.ErrNotFound)
	}
	return ep.EpisodeID, nil
}

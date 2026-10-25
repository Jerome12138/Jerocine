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
	danmuURL string
	hc       *http.Client // 路径 A：整集聚合 30s
	segHC    *http.Client // 路径 B：分段点播 3s
}

// NewDanmuService 构造弹幕服务。danmuURL 为 danmu-service 基址（config.Danmu.ServiceURL）。
func NewDanmuService(movie repository.MovieRepository, client *danmu.Client, danmuURL string) *DanmuService {
	return &DanmuService{
		movie:    movie,
		client:   client,
		danmuURL: strings.TrimRight(danmuURL, "/"),
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

// loadDanmuSource 回源：mid → db_id → 豆瓣 vendors → 平台资源 id。
func (s *DanmuService) loadDanmuSource(ctx context.Context, mid int64) (DanmuSourceResp, bool, error) {
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
		if !ok {
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
// 降级：danmu-service 不可用 / 超时 / 无匹配集 → 返回空弹幕 + 无错误（不阻塞播放）。
func (s *DanmuService) DanmuBilibili(ctx context.Context, seasonID string, episode int) ([]DanmuComment, error) {
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
		log.Printf("[danmu] bilibili episodes 调用失败, 静默降级: %v", err)
		return nil, false, nil // 负缓存 2h，避免持续打不可用服务
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
		log.Printf("[danmu] bilibili comments 调用失败, 静默降级: %v", err)
		return nil, false, nil
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

// TencentMatched 腾讯分集匹配结果（matched.episodeId 为复合串，透传时须 encodeURIComponent）。
type TencentMatched struct {
	EpisodeID string `json:"episodeId"`
}

// TencentSegment 腾讯弹幕分段（startTime 秒，壳按段起始排序；segmentId 透传壳）。
type TencentSegment struct {
	Provider  string  `json:"provider"`
	StartTime float64 `json:"startTime"`
	SegmentID string  `json:"segmentId"`
}

// DanmuTencentEpisodes 腾讯分集匹配（episodes 接口转发）。
// 返回 found=false 表示无匹配集（前端静默无弹幕）；错误仅在 danmu-service 不可用时出现。
func (s *DanmuService) DanmuTencentEpisodes(ctx context.Context, resourceID string, episode int) (TencentMatched, bool, error) {
	epsURL := fmt.Sprintf("%s/danmu/tencent/episodes?resourceId=%s&episode=%d",
		s.danmuURL, url.QueryEscape(resourceID), episode)
	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Matched *TencentMatched `json:"matched"`
		} `json:"data"`
	}
	if err := s.danmuSegGet(ctx, epsURL, &resp); err != nil {
		return TencentMatched{}, false, fmt.Errorf("tencent episodes: %w", err)
	}
	if !resp.Ok || resp.Data.Matched == nil || resp.Data.Matched.EpisodeID == "" {
		return TencentMatched{}, false, nil
	}
	return *resp.Data.Matched, true, nil
}

// DanmuTencentSegments 腾讯分段索引（segments 接口转发；不缓存）。
func (s *DanmuService) DanmuTencentSegments(ctx context.Context, episodeID string) ([]TencentSegment, error) {
	segURL := fmt.Sprintf("%s/danmu/tencent/segments?episodeId=%s", s.danmuURL, url.QueryEscape(episodeID))
	var resp struct {
		Ok   bool             `json:"ok"`
		Data []TencentSegment `json:"data"`
	}
	if err := s.danmuSegGet(ctx, segURL, &resp); err != nil {
		return nil, fmt.Errorf("tencent segments: %w", err)
	}
	if !resp.Ok {
		return nil, fmt.Errorf("tencent segments: ok=false")
	}
	return resp.Data, nil
}

// DanmuTencentComments 腾讯单段弹幕（comments 接口转发；缓存 v1:danmu:seg）。
// 降级：danmu-service 不可用/超时 → 空弹幕 + 无错误（不阻塞播放，负缓存 2h）。
func (s *DanmuService) DanmuTencentComments(ctx context.Context, episodeID, segmentID string) ([]DanmuComment, error) {
	comments, _, err := cache.GetOrLoadTTL(ctx,
		cache.KeyDanmuSeg("tencent", cache.EpKeyOf(episodeID), segmentID),
		danmuSourceTTL, danmuEmptyTTL,
		func(ctx context.Context) ([]DanmuComment, bool, error) {
			return s.loadDanmuTencentComments(ctx, episodeID, segmentID)
		})
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// loadDanmuTencentComments 回源 danmu-service 拉单段弹幕。
func (s *DanmuService) loadDanmuTencentComments(ctx context.Context, episodeID, segmentID string) ([]DanmuComment, bool, error) {
	cmtURL := fmt.Sprintf("%s/danmu/tencent/comments?episodeId=%s&segmentId=%s",
		s.danmuURL, url.QueryEscape(episodeID), url.QueryEscape(segmentID))
	var resp struct {
		Ok   bool           `json:"ok"`
		Data []DanmuComment `json:"data"`
	}
	if err := s.danmuSegGet(ctx, cmtURL, &resp); err != nil {
		log.Printf("[danmu] tencent comments 调用失败, 静默降级: %v", err)
		return nil, false, nil // 负缓存 2h
	}
	if !resp.Ok {
		return nil, false, nil
	}
	if len(resp.Data) == 0 {
		return nil, false, nil // 负缓存 2h
	}
	return resp.Data, true, nil
}

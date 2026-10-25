package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"server/internal/cache"
	"server/internal/danmu"
	"server/internal/domain"
	"server/internal/domain/repository"
)

// 弹幕缓存 TTL：正缓存 7d（db_id/season 资源 id 长期稳定）；负缓存短 TTL 2h
// （评审 7.6：db_id 空 / 无 vendors / B站无 season 的低频片也要挡住重复打豆瓣）。
const (
	danmuSourceTTL = 7 * 24 * time.Hour
	danmuEpsTTL    = 7 * 24 * time.Hour
	danmuEmptyTTL  = 2 * time.Hour
	// maxSegs 一集弹幕分段上限（45min ≈ 8 段；留余量防超长集）。
	maxSegs = 12
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

// DanmuComment 对外弹幕条目（对齐 danmupb.Comment 的 JSON 形态）。
type DanmuComment struct {
	Text  string `json:"text"`
	Time  int64  `json:"time"`
	Color uint32 `json:"color"`
	Mode  int32  `json:"mode"`
}

// DanmuService 弹幕服务（P0：vendors 候选 + B站弹幕）。
type DanmuService struct {
	movie  repository.MovieRepository
	client *danmu.Client
}

// NewDanmuService 构造弹幕服务。
func NewDanmuService(movie repository.MovieRepository, client *danmu.Client) *DanmuService {
	return &DanmuService{movie: movie, client: client}
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

// DanmuBilibili 按 seasonId + 集号取该集全部弹幕（评审 7.4：串行拉全部分段，
// 前端按 currentTime 过滤后渲染；segment 参数留给后续"按播放进度预拉"优化）。
//
// 分集匹配（评审 7.3）：B站剧集标题格式混乱（第3集/EP3/数字），统一走
// danmu.ParseEpisodeNo 提取；电影单集（无集号标题）默认第 1 集；越界/无匹配
// 静默降级为"暂无弹幕"（不报错、不串集）。
func (s *DanmuService) DanmuBilibili(ctx context.Context, seasonID string, episode int) ([]DanmuComment, error) {
	episodes, err := s.biliEpisodes(ctx, seasonID)
	if err != nil {
		return nil, err
	}
	target, ok := MatchEpisode(episodes, episode)
	if !ok {
		return nil, nil // 无对应集：静默降级
	}
	var out []DanmuComment
	for seg := 1; seg <= maxSegs; seg++ {
		comments, err := s.client.BiliSeg(ctx, target.Cid, seg)
		if err != nil {
			// 分段失败：若已拉到部分数据则截断返回（首版体验优先）；
			// 首段即失败视为整集无弹幕（接口异常，降级）。
			if len(out) > 0 {
				log.Printf("[danmu] season=%s ep=%d seg=%d 失败, 截断返回 %d 条: %v",
					seasonID, episode, seg, len(out), err)
				break
			}
			log.Printf("[danmu] season=%s ep=%d 首段失败, 视为无弹幕: %v", seasonID, episode, err)
			return nil, nil
		}
		if len(comments) == 0 {
			break // 空段 = 弹幕到底
		}
		for _, cm := range comments {
			out = append(out, DanmuComment{Text: cm.Content, Time: cm.TimeMS, Color: cm.Color, Mode: cm.Mode})
		}
		time.Sleep(200 * time.Millisecond) // 平台频率纪律：段间小幅退避
	}
	return out, nil
}

// biliEpisodes 取 B站剧集列表（正缓存 7d；season 无效 → 负缓存 2h）。
func (s *DanmuService) biliEpisodes(ctx context.Context, seasonID string) ([]danmu.Episode, error) {
	eps, _, err := cache.GetOrLoadTTL(ctx, cache.KeyDanmuBiliEps(seasonID), danmuEpsTTL, danmuEmptyTTL,
		func(ctx context.Context) ([]danmu.Episode, bool, error) {
			list, err := s.client.BiliEpisodes(ctx, seasonID)
			if err != nil {
				return nil, false, err
			}
			if len(list) == 0 {
				return nil, false, nil // 负缓存 2h
			}
			return list, true, nil
		})
	if err != nil {
		return nil, err
	}
	return eps, nil
}

// MatchEpisode 按集号匹配剧集（评审 7.3 的表驱动匹配核心，service 层纯函数，可单测）。
//
// 规则：
//  1. requested ≤ 0 视为"默认第 1 集"；
//  2. 全部标题无集号（电影/单集片，如"正片"）→ 取第一条（默认第 1 集）；
//  3. 标题集号 == requested → 命中；
//  4. 越界/无匹配 → ok=false（调用方降级为"暂无弹幕"，不串集）。
func MatchEpisode(episodes []danmu.Episode, requested int) (danmu.Episode, bool) {
	if len(episodes) == 0 {
		return danmu.Episode{}, false
	}
	// 单集片特例：所有标题都提不出集号 → 整片只算第 1 集。
	if noEpisodeNumbers(episodes) {
		return episodes[0], true
	}
	if requested <= 0 {
		requested = 1
	}
	for _, e := range episodes {
		if danmu.ParseEpisodeNo(e.Title) == requested {
			return e, true
		}
	}
	return danmu.Episode{}, false
}

// noEpisodeNumbers 判断是否"全部标题无集号"（电影/单集片）。
func noEpisodeNumbers(episodes []danmu.Episode) bool {
	for _, e := range episodes {
		if danmu.ParseEpisodeNo(e.Title) > 0 {
			return false
		}
	}
	return true
}

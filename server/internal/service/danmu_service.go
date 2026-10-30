package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
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

// DanmuService 弹幕服务（聚合链路: danmu_api dandanplay v2; 旧 vendors/分段链路已下线 2026-10-31）。
type DanmuService struct {
	movie   repository.MovieRepository
	dandan  *danmu.DandanClient             // 聚合弹幕（dandanplay v2 兼容，按标题匹配）
	siteCfg repository.SiteConfigRepository // 弹幕后台配置（总开关，直读库现值）

	// 弹幕启用配置进程内快照(30s TTL, 见 danmuCfgSnapshot): 热路径免每请求查库 + DB 故障容错
	cfgMu   sync.Mutex
	cfgSnap *danmuCfgSnapshot
	cfgAt   time.Time
}

// NewDanmuService 构造弹幕服务（聚合链路）。
// siteCfg 用于读取后台弹幕配置（默认不启用）。
func NewDanmuService(movie repository.MovieRepository, dandan *danmu.DandanClient, siteCfg repository.SiteConfigRepository) *DanmuService {
	return &DanmuService{
		movie:   movie,
		dandan:  dandan,
		siteCfg: siteCfg,
	}
}

// danmuProviderAll 后台可选的弹幕平台全集（须与 danmu-service 壳 ID_PREFIX 同步：
// 平台要可用，两边都要启用 —— 壳白名单决定上游能否拉，这里决定 Go/前端是否开放）。
var danmuProviderAll = []string{"bilibili", "tencent", "iqiyi", "youku"}

// danmuCfgSnapshot 弹幕启用配置快照（进程内 30s TTL）：
//   - 热路径（danmuMasterEnabled 每请求调用）不再每请求打 MySQL；
//   - 瞬时 DB 故障时沿用旧快照（无旧快照才上抛），不再把错误误判成"未启用"毒化负缓存 2h
//     （P1: site_config 查询出错曾被当 nil 处理，负缓存语义污染）。
//   - 后台改配置最长 30s 延迟生效（弹幕开关类变更可接受；如需立即生效可在保存路径加失效钩子）。
type danmuCfgSnapshot struct {
	masterOn  bool
	providers []string // nil = 总开关关（未启用）
}

const danmuCfgCacheTTL = 30 * time.Second

func (s *DanmuService) danmuCfgSnapshot(ctx context.Context) (danmuCfgSnapshot, error) {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	if s.cfgSnap != nil && time.Since(s.cfgAt) < danmuCfgCacheTTL {
		return *s.cfgSnap, nil
	}
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil {
		// 刷新失败: 有旧快照沿用(容错降级), 无旧快照上抛(调用方不得把它当"未启用"写负缓存)
		if s.cfgSnap != nil {
			log.Printf("danmu: site_config refresh failed, using stale snapshot: %v", err)
			return *s.cfgSnap, nil
		}
		return danmuCfgSnapshot{}, err
	}
	snap := danmuCfgSnapshot{masterOn: cfg.DanmuEnabled == 1}
	if snap.masterOn && len(cfg.DanmuProviders) > 0 {
		allowed := make(map[string]bool, len(danmuProviderAll))
		for _, p := range danmuProviderAll {
			allowed[p] = true
		}
		for _, p := range cfg.DanmuProviders {
			if allowed[p] {
				snap.providers = append(snap.providers, p)
			}
		}
	}
	s.cfgSnap = &snap
	s.cfgAt = time.Now()
	return snap, nil
}

// DanmuComment 对外弹幕条目（对齐 danmu-service 规范化契约 {text,time,color,mode}，毫秒）。
type DanmuComment struct {
	Text  string `json:"text"`
	Time  int64  `json:"time"`
	Color uint32 `json:"color"`
	Mode  int32  `json:"mode"`
}

func (s *DanmuService) danmuMasterEnabled(ctx context.Context) bool {
	snap, err := s.danmuCfgSnapshot(ctx)
	if err != nil {
		// 读取失败且无旧快照: 短暂降级视为关闭(不报错, 聚合返回空弹幕), 恢复后自愈
		return false
	}
	return snap.masterOn
}

// DanmuAggregateResp 聚合弹幕响应（复用 DanmuComment 毫秒契约）。
// Total = 整集总条数：from/to 分片过滤时供前端判断是否已拉完全集。
type DanmuAggregateResp struct {
	Comments []DanmuComment `json:"comments"`
	Total    int            `json:"total,omitempty"` // 整集总条数: from/to 分片过滤时供前端判断是否拉完
}

// DanmuAggregate 按标题 + 集号取整集弹幕。查无作品/无该集 → domain.ErrNotFound（404，
// 前端标"该片暂无弹幕"）；取数异常 → 原样上抛（前端标失败可重试）。
// from/to（秒, 可选 to=0 视为无穷）: 分片过滤 time∈[from*1000, to*1000) —— 前端懒加载
// 起播段+按进度预取；上游/整集缓存仍整集粒度（分片只在响应层过滤, 上游只打一次）。
// Total 恒为整集总条数（过滤后 Comments 条数与之对比可判断是否拉完）。
func (s *DanmuService) DanmuAggregate(ctx context.Context, title string, year, episode, from, to int) (DanmuAggregateResp, error) {
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
	// 负缓存命中/空结果回零值结构：JSON 输出 comments:null 违背契约, 归一为空数组
	if resp.Comments == nil {
		resp.Comments = []DanmuComment{}
	}
	resp.Total = len(resp.Comments)
	// 分片过滤(秒 → 毫秒半开区间): from<0 视为 0; to<=0 视为无穷; 非分片请求原样整集返回
	if from > 0 || to > 0 {
		lo, hi := int64(from)*1000, int64(to)*1000
		if lo < 0 {
			lo = 0
		}
		filtered := make([]DanmuComment, 0, len(resp.Comments))
		for _, c := range resp.Comments {
			if c.Time >= lo && (hi <= 0 || c.Time < hi) {
				filtered = append(filtered, c)
			}
		}
		resp.Comments = filtered
	}
	return resp, nil
}

// resolveAggregateEpisode 两跳定位集：关键词 → 作品（缓存 7d）→ 分集列表（缓存 7d）→ 集。
// 缓存键掺入 year + episode：同标题不同年份（多季）各自缓存，避免先到请求把后到年份
// 串到错误季；掺 episode 是因为 PickAnime 打分含集数项(EpisodeCount>=episode+1)，
// 同标题同年不同集号共享 pick 会把后到请求定位到先到请求按其集号选出的作品。
func (s *DanmuService) resolveAggregateEpisode(ctx context.Context, title string, year, episode int) (int64, error) {
	kwKey := cache.EpKeyOf(fmt.Sprintf("%s|%d|%d", danmu.NormalizeTitle(title), year, episode))
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

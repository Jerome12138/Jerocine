package service

import (
	"context"
	"log"
	"strconv"
	"time"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/platform/blobstore"
	"server/internal/tmdb"
)

// 回填节奏参数: 免费档 TMDB 建议 ≤50 req/s, 这里压到个位 —— 慢慢跑, 不抢限额也不惊动源站。
const (
	// backdropReqDelay 相邻两次 TMDB 检索的间隔(检索含 movie+tv 最多 4 次请求, 实际 RPS≈4-13)。
	backdropReqDelay = 300 * time.Millisecond
	// backdropSweepInterval 兜底扫描间隔。正常路径是事件驱动(采集完成/轮播变更 → Kick),
	// 这个低频扫描只兜三类没有事件的场景: 首页聚合缓存过期后兜底榜单自然换片、
	// 此前网络失败待重试的片、以及后台改轮播时 banner 缓存尚未到期的时间差。
	backdropSweepInterval = 20 * time.Minute
)

// BackdropService 首页轮播横图回填 worker: 周期性重算"轮播影片集合"
// (后台配置 banner 关联的影片 + 兜底 hot/latest 前 5, 与前端首页大图口径一致),
// 对其中缺横图的影片按片名(+年份)在 TMDB 检索, 图片下载到本地 blob
// (终端不直连被墙的 image.tmdb.org), 回填 movie + movie_search 双表。
//
// 范围刻意收窄为仅轮播影片 —— 不做全库回填, TMDB 用量与轮播规模成正比;
// 轮播(后台增删改 / 兜底榜单换片)一有变动, 下一轮扫描即增量补采新片。
type BackdropService struct {
	movie   repository.MovieRepository
	search  repository.SearchRepository
	banners *BannerService // 生效轮播位(缓存), 手动+自动补位同源
	blob    blobstore.BlobStore
	client  *tmdb.Client // nil = 未配置 API key, Start 直接返回
	kick    chan struct{} // 事件触发通道(容量 1, 多次触发自动合并)
}

func NewBackdropService(movie repository.MovieRepository, search repository.SearchRepository,
	banners *BannerService, blob blobstore.BlobStore, client *tmdb.Client) *BackdropService {
	return &BackdropService{movie: movie, search: search, banners: banners,
		blob: blob, client: client, kick: make(chan struct{}, 1)}
}

// Kick 事件触发一轮扫描(采集落库/轮播变更后调用)。非阻塞且幂等: 已有待处理触发时合并为一次。
// worker 未启动(未配置 API key)时触发被静默丢弃。
func (s *BackdropService) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Start 起后台回填循环(阻塞 goroutine, 由组合根 go 出去)。未配置 TMDB_API_KEY 时为 no-op。
func (s *BackdropService) Start(ctx context.Context) {
	if s.client == nil || s.blob == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[backdrop] worker panic: %v", r)
			}
		}()
		s.loop(ctx)
	}()
}

func (s *BackdropService) loop(ctx context.Context) {
	s.tick(ctx) // 启动即跑一轮: 重启/部署后立即补齐轮播横图, 不必等首个事件或兜底扫描
	sweep := time.NewTicker(backdropSweepInterval)
	defer sweep.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.kick: // 采集完成/轮播变更: 立即重算
		case <-sweep.C: // 兜底扫描
		}
		s.tick(ctx)
	}
}

// safeNotify 调用事件回调并隔离 panic —— 回调是 worker 的触发器, 绝不连累采集/轮播主流程。
func safeNotify(name string, fn func()) {
	if fn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[backdrop] %s notify panic: %v", name, r)
		}
	}()
	fn()
}

// tick 一轮扫描: 轮播集合 → 过滤缺图 → 逐片补采。每步失败都只记日志, 不影响下一轮。
func (s *BackdropService) tick(ctx context.Context) {
	mids := s.carouselMids(ctx)
	if len(mids) == 0 {
		return
	}
	movies, err := s.movie.ListMissingBackdropsByMids(ctx, mids)
	if err != nil {
		log.Printf("[backdrop] list missing err: %v", err)
		return
	}
	for _, m := range movies {
		if ctx.Err() != nil {
			return
		}
		s.fillOne(ctx, &m)
		if !sleepCtx(ctx, backdropReqDelay) {
			return
		}
	}
}

// carouselMids 汇总轮播影片集合(去重): 生效轮播位的 mid —— 手动配置位 + 自动补位,
// 与前台首页大图完全同源; 外链 banner(无 mid)不参与。
func (s *BackdropService) carouselMids(ctx context.Context) []int64 {
	var mids []int64
	seen := make(map[int64]bool, 16)
	add := func(mid int64) {
		if mid > 0 && !seen[mid] {
			seen[mid] = true
			mids = append(mids, mid)
		}
	}
	slides, err := s.banners.Public(ctx)
	if err != nil {
		log.Printf("[backdrop] list slides err: %v", err)
		return mids
	}
	for _, sl := range slides {
		add(sl.Mid)
	}
	return mids
}

// fillOne 单片回填: TMDB 检索 → 下载 → 双表回填。检索无果写 MissMark 哨兵防重查
// (轮播常驻片不会每分钟被重复检索); 网络类错误不写任何标记, 下一轮自然重试。
// 任何一步失败都不影响主流程(worker 与采集完全解耦)。
func (s *BackdropService) fillOne(ctx context.Context, m *entity.Movie) {
	path, err := s.client.SearchBackdrop(ctx, m.Name, m.Year)
	if err != nil {
		log.Printf("[backdrop] search %s(%d) mid=%d: %v", m.Name, m.Year, m.Mid, err)
		return
	}
	if path == tmdb.MissMark {
		if _, err := s.movie.UpdateBackdrop(ctx, m.Mid, tmdb.MissMark); err != nil {
			log.Printf("[backdrop] mark miss mid=%d: %v", m.Mid, err)
		}
		return
	}
	imageURL := s.client.ImageURL(path)
	key := "backdrop/" + strconv.FormatInt(m.Mid, 10) + ".jpg"
	localURL, err := s.blob.SaveFromURL(ctx, key, imageURL)
	if err != nil {
		log.Printf("[backdrop] download mid=%d %s: %v", m.Mid, imageURL, err)
		return
	}
	if _, err := s.movie.UpdateBackdrop(ctx, m.Mid, localURL); err != nil {
		log.Printf("[backdrop] update movie mid=%d: %v", m.Mid, err)
		return
	}
	if err := s.search.UpdateBackdrop(ctx, m.Mid, localURL); err != nil {
		log.Printf("[backdrop] update movie_search mid=%d: %v", m.Mid, err)
	}
	log.Printf("[backdrop] mid=%d %s → %s", m.Mid, m.Name, localURL)
}

// sleepCtx 可中断休眠; 返回 false 表示 ctx 已取消。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

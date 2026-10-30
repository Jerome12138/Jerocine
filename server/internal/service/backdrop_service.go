package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/deepteams/webp"

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
	client  *tmdb.Client                    // key 运行期热替换, 永不为 nil
	siteCfg repository.SiteConfigRepository // key 唯一权威: 后台「系统设置 → TMDB」(无 env 兜底)
	kick    chan struct{}                   // 事件触发通道(容量 1, 多次触发自动合并)
}

func NewBackdropService(movie repository.MovieRepository, search repository.SearchRepository,
	banners *BannerService, blob blobstore.BlobStore, client *tmdb.Client,
	siteCfg repository.SiteConfigRepository) *BackdropService {
	return &BackdropService{movie: movie, search: search, banners: banners,
		blob: blob, client: client, siteCfg: siteCfg,
		kick: make(chan struct{}, 1)}
}

// Kick 事件触发一轮扫描(采集落库/轮播变更后调用)。非阻塞且幂等: 已有待处理触发时合并为一次。
// worker 未启动(未配置 API key)时触发被静默丢弃。
func (s *BackdropService) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Start 起后台回填循环(阻塞 goroutine, 由组合根 go 出去)。未配置任何 key 时
// 每轮只做一次轻量 key 探测即返回, 等后台配好 key 后下一轮自动激活, 无需重启。
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
		// recover 下沉到每轮: 单轮 panic 只损失该轮, worker 常驻(顶层的 recover 只兜 loop 之外)
		runSafely("[backdrop]", func() { s.tick(ctx) })
	}
}

// runSafely 单轮执行包裹: worker 循环内逐轮 recover, 单轮 panic 不终结 worker。
func runSafely(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("%s tick panic: %v", name, r)
		}
	}()
	fn()
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

// tick 一轮扫描: 解析当前 key → 轮播集合 → 过滤缺图 → 逐片补采。每步失败都只记日志, 不影响下一轮。
func (s *BackdropService) tick(ctx context.Context) {
	key := s.currentKey(ctx)
	if key == "" {
		return // 未配置 key: 功能整体关闭(本轮静默跳过)
	}
	s.client.SetKey(key)
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

// currentKey 当前生效凭据: 只认后台 site_config(空 = 功能关闭, worker 轻量探测等待)。
func (s *BackdropService) currentKey(ctx context.Context) string {
	if s.siteCfg == nil {
		return ""
	}
	if c, err := s.siteCfg.Get(ctx); err == nil {
		return c.TmdbAPIKey
	}
	return ""
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
	localURL, err := s.saveBackdrop(ctx, m.Mid, imageURL)
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

// FillOne 单片回填横图(后台手动触发): 按 mid 查影片 → TMDB 检索 → 下载 → 双表回填。
// 复用 worker 的 fillOne 内联逻辑, 但主动回读确认结果并返回状态, 供管理端提示:
//
//	already = 原本就有横图, 未重查;  miss    = 命中 TMDB MissMark 哨兵(检索无果, 不回填);
//	filled  = 本次成功回填;   "" + err = 未生效(网络失败/无 key/检索无果), 详情见 [backdrop] 日志。
func (s *BackdropService) FillOne(ctx context.Context, mid int64) (string, error) {
	m, err := s.movie.GetByMid(ctx, mid)
	if err != nil {
		return "", err
	}
	switch m.Backdrop {
	case "":
		// 缺图: 走单片回填(内部打日志, 失败不抛)。
	case tmdb.MissMark:
		return "miss", nil // 此前检索无果, 不重复检索(与 worker 同策略)
	default:
		return "already", nil
	}
	s.fillOne(ctx, m)
	mm, err := s.movie.GetByMid(ctx, mid)
	if err != nil {
		return "", err
	}
	switch mm.Backdrop {
	case "":
		return "", fmt.Errorf("回填未生效(网络失败或无 TMDB key), 详情见服务端 [backdrop] 日志")
	case tmdb.MissMark:
		return "miss", nil
	default:
		return "filled", nil
	}
}

// backdropWebpQuality TMDB 横图统一重编码质量。w1280 原图 q100 体积常在
// 300KB-1.5MB; webp(有损)在同等观感下比 jpeg 再小 40-60%, 首页/详情下发与存储同步变轻。
const backdropWebpQuality float32 = 82

// backdropHTTPClient 图片下载专用 client: 大图 30s, 与 TMDB API 检索的 10s 分开。
// URL 全部由 tmdb.Client.ImageURL 拼装(固定 image.tmdb.org 域名 + API 返回的
// backdrop_path 作为路径, 无用户可控输入), 不存在 SSRF 逃逸面。
var backdropHTTPClient = &http.Client{Timeout: 30 * time.Second}

// saveBackdrop 下载 TMDB 横图并压缩落盘(WebP): 可解码 → 统一重编码为 WebP(质量
// backdropWebpQuality; w1280 不放大, 仅降体积), 键为 backdrop/<mid>.webp;
// 解码失败或压缩后反而更大 → 原样存储(键 backdrop/<mid>.jpg)。
// 错误语义与旧 SaveFromURL 一致, 不阻塞回填。
func (s *BackdropService) saveBackdrop(ctx context.Context, mid int64, imageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := backdropHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("backdrop fetch %s status %d", imageURL, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32MB 上限, 与 blobstore 一致
	if err != nil {
		return "", err
	}
	out := raw
	key := "backdrop/" + strconv.FormatInt(mid, 10) + ".jpg"
	// webp 包 self-register, image.Decode 可直接解 webp 原图(TMDB 若返回 webp 也能走压缩分支)
	if img, _, derr := image.Decode(bytes.NewReader(raw)); derr == nil {
		var buf bytes.Buffer
		if werr := webp.Encode(&buf, img, &webp.Options{Quality: backdropWebpQuality}); werr == nil && buf.Len() < len(raw) {
			out = buf.Bytes()
			key = "backdrop/" + strconv.FormatInt(mid, 10) + ".webp"
		}
	}
	return s.blob.Save(ctx, key, out)
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

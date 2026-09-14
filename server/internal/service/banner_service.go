package service

import (
	"context"
	"log"
	"time"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/douban"
)

// ttlBanners 轮播列表缓存时长。比站点配置短: 排期(起止时间)会随时间推移自然生效/失效,
// 缓存太久会出现"到点了但首页还没换"。
const ttlBanners = 3 * time.Minute

// heroSlideCount 首页轮播位总数: 手动配置位排前, 不足部分由热榜/最新榜自动补位。
const heroSlideCount = 5

// 轮播位来源。手动位 = banner 配置行; 自动位 = 榜单派生(无对应配置行)。
const (
	SlideSourceManual = "banner"
	SlideSourceAuto   = "fallback"
)

// BannerService 首页轮播: 前台读取(缓存) + 后台增删改 + 生效位排序/屏蔽。
//
// 生效语义(前 heroSlideCount 位, 前台首页大图与管理页共用同一口径):
//  1. 手动位 = 启用 + 生效窗口内 + 有横/竖图的配置行, 按 sort 升序排前;
//  2. 自动位 = 手动位不足 heroSlideCount 时, 按各区块 hot → latest 顺序补尾;
//  3. 铁律: mid 只要在 banner 表出现过(无论启用与否)就不再被自动补位选中 ——
//     "禁用自动位"即落地为一条禁用配置行(屏蔽该 mid)。
type BannerService struct {
	repo repository.BannerRepository
	// films 自动补位榜单数据源(与前台首页聚合同源同缓存)。
	films *FilmService
	// OnChange 轮播配置变更回调(横图 worker 重算用, 组合根注入, 可空)。panic 由 safeNotify 隔离。
	OnChange func()
}

func NewBannerService(repo repository.BannerRepository, films *FilmService) *BannerService {
	return &BannerService{repo: repo, films: films}
}

// EffectiveSlide 当前生效的一个轮播位(前台 Public 与后台管理列表共用)。
type EffectiveSlide struct {
	// Source 数据来源: SlideSourceManual=后台配置 / SlideSourceAuto=热榜自动补位。
	Source   string `json:"source"`
	BannerId int64  `json:"bannerId,omitempty"` // Source=手动 时的配置 id
	Mid      int64  `json:"mid,omitempty"`
	Name     string `json:"name"`
	Subtitle string `json:"subtitle,omitempty"`
	// Image 生效中的宽幅主视觉: 配置横图, 或自动位的 TMDB 回填横图(可能为空=尚未回填)。
	Image  string `json:"image,omitempty"`
	Poster string `json:"poster,omitempty"` // 竖图/封面
	Link   string `json:"link,omitempty"`
	Sort   int    `json:"sort,omitempty"`
	State  int8   `json:"state,omitempty"`
	// 以下 4 项为按 mid 补齐的影片元信息(见 enrichMeta): 首屏大图描述行展示
	// 「评分 · 类型标签 · 豆瓣·热门电影 No.1」。纯自定义位(无 mid)或影片已删时缺省。
	DbScore  float64 `json:"dbScore,omitempty"`
	ClassTag string  `json:"classTag,omitempty"`
	HotRank  int     `json:"hotRank,omitempty"`
	HotBoard string  `json:"hotBoard,omitempty"`
	// Banner 仅后台 Board 填充: 手动位对应的完整配置行(编辑表单回填用), 前台不返回。
	Banner *entity.Banner `json:"banner,omitempty"`
}

// InactiveRow 未生效的配置行(管理页折叠区), 不参与展示与自动补位。
type InactiveRow struct {
	entity.Banner
	// Reason 未生效原因: disabled=已停用 / noimage=缺横竖图 / pending=未开始 / expired=已过期 / overflow=超出前5位。
	Reason string `json:"reason"`
}

// Board 后台管理视图: 生效位(前 heroSlideCount) + 未生效配置行。
type Board struct {
	Active   []EffectiveSlide `json:"active"`
	Inactive []InactiveRow    `json:"inactive"`
}

// activeManual 从全量行里挑出当前生效的手动行, 保持 sort/id 升序。
func activeManual(rows []entity.Banner, now int64) []entity.Banner {
	var out []entity.Banner
	for _, b := range rows {
		if b.State == entity.BannerEnabled &&
			(b.StartAt == 0 || b.StartAt <= now) &&
			(b.EndAt == 0 || b.EndAt >= now) &&
			(b.Image != "" || b.Poster != "") {
			out = append(out, b)
		}
	}
	return out
}

// compose 组装生效轮播位(不缓存)。手动位在前, 不足 heroSlideCount 时自动补位;
// 手动位超过 heroSlideCount 时截断, 超出部分在 Board 里以 overflow 呈现。
func (s *BannerService) compose(ctx context.Context) ([]EffectiveSlide, error) {
	now := time.Now().UnixMilli()
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	// excluded: banner 表里出现过的 mid 永不由自动补位产生(禁用行 = 屏蔽)。
	excluded := make(map[int64]bool, len(rows))
	for _, b := range rows {
		if b.Mid > 0 {
			excluded[b.Mid] = true
		}
	}
	manualRows := activeManual(rows, now)
	slides := make([]EffectiveSlide, 0, heroSlideCount)
	for _, b := range manualRows {
		slides = append(slides, EffectiveSlide{
			Source: SlideSourceManual, BannerId: b.Id, Mid: b.Mid,
			Name: b.Title, Subtitle: b.Subtitle, Image: b.Image, Poster: b.Poster,
			Link: b.Link, Sort: b.Sort, State: b.State,
		})
	}
	if len(slides) < heroSlideCount {
		home, herr := s.films.Home(ctx)
		if herr != nil {
			// 已有手动位就照常返回(自动补位失败不连累前台); 一个都没有才向上报错。
			if len(slides) > 0 {
				log.Printf("[banner] home agg err, skip auto fill: %v", herr)
				return slides, nil
			}
			return nil, herr
		}
		seen := excluded // 复用同一张表: 既排除配置过的 mid, 也对候选去重
		fill := func(items []entity.MovieSearch) {
			for _, it := range items {
				if len(slides) >= heroSlideCount {
					return
				}
				if it.Mid <= 0 || seen[it.Mid] {
					continue
				}
				seen[it.Mid] = true
				slides = append(slides, EffectiveSlide{
					Source: SlideSourceAuto, Mid: it.Mid, Name: it.Name,
					Subtitle: it.Remarks, Image: it.Backdrop, Poster: it.Cover,
				})
			}
		}
		for _, row := range home.Rows {
			fill(row.Hot)
		}
		for _, row := range home.Rows {
			fill(row.Latest)
		}
	}
	if len(slides) > heroSlideCount {
		slides = slides[:heroSlideCount]
	}
	return slides, nil
}

// Public 前台轮播: 生效位列表(手动 + 自动补位, 前 heroSlideCount 位), 带缓存。
func (s *BannerService) Public(ctx context.Context) ([]EffectiveSlide, error) {
	slides, _, err := cache.GetOrLoad(ctx, cache.KeyBanners, ttlBanners, func(ctx context.Context) ([]EffectiveSlide, bool, error) {
		slides, err := s.compose(ctx)
		// 元信息补齐放在缓存装载里: 一次 IN 查询(≤5 个 mid), 结果随轮播缓存复用 3min。
		// 失败只记日志不报错 —— 缺评分/标签不影响轮播可用性, 没必要连累首页。
		s.enrichMeta(ctx, slides)
		return slides, true, err
	})
	return slides, err
}

// enrichMeta 给生效位补影片元信息(评分 / 类型标签 / 豆瓣榜位), 供首屏大图描述行展示。
//
// 轮播位本身只有 mid + 图 + 标题(自动位从榜单派生后也只留这几项), 而评分、类型标签、
// 榜位都在影片读模型里 —— 一次 GetByMids 覆盖手动位与自动位, 避免逐条查详情。
// 取不到的位(纯自定义无 mid / 影片已删)保持零值, 由 omitempty 从 JSON 里省掉。
func (s *BannerService) enrichMeta(ctx context.Context, slides []EffectiveSlide) {
	if s.films == nil || len(slides) == 0 {
		return
	}
	mids := make([]int64, 0, len(slides))
	for _, sl := range slides {
		if sl.Mid > 0 {
			mids = append(mids, sl.Mid)
		}
	}
	if len(mids) == 0 {
		return
	}
	cards, err := s.films.SearchByMids(ctx, mids)
	if err != nil {
		log.Printf("[banner] enrich meta err: %v", err)
		return
	}
	byMid := make(map[int64]entity.MovieSearch, len(cards))
	for _, c := range cards {
		byMid[c.Mid] = c
	}
	for i := range slides {
		c, ok := byMid[slides[i].Mid]
		if !ok {
			continue
		}
		slides[i].DbScore = c.DbScore
		slides[i].ClassTag = c.ClassTag
		slides[i].HotRank = c.HotRank
		// 与详情页 hotBadge 同口径: 落库的是集合名(movie_hot_gaia), 对外给中文榜单名;
		// hot_board 缺失时按分类热榜兜底, 避免前台只剩"热门"两个字(用户要求显示全)。
		slides[i].HotBoard = douban.HotBoardLabel(c.Pid, c.HotBoard, c.HotRank)
	}
}

// Board 后台管理视图: 生效位 + 未生效配置行(带原因, 管理页折叠区展示)。
func (s *BannerService) Board(ctx context.Context) (*Board, error) {
	now := time.Now().UnixMilli()
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	active, err := s.Public(ctx)
	if err != nil {
		return nil, err
	}
	used := make(map[int64]bool, len(active))
	for _, sl := range active {
		if sl.BannerId > 0 {
			used[sl.BannerId] = true
		}
	}
	board := &Board{Active: active, Inactive: []InactiveRow{}}
	for _, b := range rows {
		if used[b.Id] {
			continue
		}
		reason := "disabled"
		if b.State == entity.BannerEnabled {
			switch {
			case b.Image == "" && b.Poster == "":
				reason = "noimage"
			case b.StartAt > 0 && b.StartAt > now:
				reason = "pending"
			case b.EndAt > 0 && b.EndAt < now:
				reason = "expired"
			default:
				reason = "overflow"
			}
		}
		board.Inactive = append(board.Inactive, InactiveRow{Banner: b, Reason: reason})
	}
	return board, nil
}

// Save 新建(id=0)或按 id 更新。新建时 slot 非空表示钉到生效列表第 slot 位(采纳自动位用),
// 否则追加到手动位末尾; sort 由排序操作统一维护, 请求值仅作兼容保留。
func (s *BannerService) Save(ctx context.Context, b *entity.Banner, slot *int) error {
	var err error
	if b.Id > 0 {
		err = s.repo.Update(ctx, b)
	} else {
		maxSort, rerr := s.maxSort(ctx)
		if rerr != nil {
			return rerr
		}
		b.Sort = maxSort + 1
		if err = s.repo.Create(ctx, b); err == nil && slot != nil && *slot >= 0 {
			err = s.placeAt(ctx, b, *slot)
		}
	}
	if err != nil {
		return err
	}
	cache.InvalidateBanners(ctx)
	safeNotify("banner", s.OnChange)
	return nil
}

// Move 生效位排序。手动位在手动区内换位; 自动位仅支持上移 = 转为手动位钉在目标位置
// (自动位是榜单派生, 无行可删, "下移/删除"由 禁用/屏蔽 语义承担)。
func (s *BannerService) Move(ctx context.Context, slot int, dir string) error {
	if dir != "up" && dir != "down" {
		return domain.ErrInvalidMove
	}
	slides, err := s.Public(ctx)
	if err != nil {
		return err
	}
	if slot < 0 || slot >= len(slides) {
		return domain.ErrInvalidMove
	}
	target := slot - 1
	if dir == "down" {
		target = slot + 1
	}
	if target < 0 || target >= len(slides) {
		return domain.ErrInvalidMove
	}
	manuals, err := s.manualRows(ctx)
	if err != nil {
		return err
	}
	if slides[slot].Source == SlideSourceManual {
		i := -1
		for k := range manuals {
			if manuals[k].Id == slides[slot].BannerId {
				i = k
				break
			}
		}
		if i < 0 || target >= len(manuals) {
			return domain.ErrInvalidMove
		}
		b := manuals[i]
		manuals = append(manuals[:i], manuals[i+1:]...)
		manuals = append(manuals[:target], append([]entity.Banner{b}, manuals[target:]...)...)
	} else {
		if dir == "down" {
			return domain.ErrInvalidMove // 自动位恒在尾部, 无"更后"可去
		}
		// 采纳为手动位: 复制当前生效图 pin 住, 钉在目标位置(target 允许落在自动区内 → 收敛为手动末位)。
		sl := slides[slot]
		nb := entity.Banner{
			Title: sl.Name, Subtitle: sl.Subtitle, Image: sl.Image, Poster: sl.Poster,
			Mid: sl.Mid, State: entity.BannerEnabled,
		}
		if err := s.repo.Create(ctx, &nb); err != nil {
			return err
		}
		if target > len(manuals) {
			target = len(manuals)
		}
		manuals = append(manuals[:target], append([]entity.Banner{nb}, manuals[target:]...)...)
	}
	if err := s.reindex(ctx, manuals); err != nil {
		return err
	}
	cache.InvalidateBanners(ctx)
	safeNotify("banner", s.OnChange)
	return nil
}

// Delete 删除一条并失效前台缓存。
func (s *BannerService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	cache.InvalidateBanners(ctx)
	safeNotify("banner", s.OnChange)
	return nil
}

// manualRows 当前生效手动行(sort/id 升序)。
func (s *BannerService) manualRows(ctx context.Context) ([]entity.Banner, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return activeManual(rows, time.Now().UnixMilli()), nil
}

// reindex 把手动行顺序落库为 sort=下标(0 起)。
func (s *BannerService) reindex(ctx context.Context, manuals []entity.Banner) error {
	for i := range manuals {
		if manuals[i].Sort == i {
			continue
		}
		if err := s.repo.UpdateSort(ctx, manuals[i].Id, i); err != nil {
			return err
		}
	}
	return nil
}

// maxSort 当前最大 sort 值(新建追加到末尾用)。
func (s *BannerService) maxSort(ctx context.Context) (int, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return 0, err
	}
	max := 0
	for _, r := range rows {
		if r.Sort > max {
			max = r.Sort
		}
	}
	return max, nil
}

// placeAt 把刚创建的手动行从手动末位移到第 slot 位(仅当 slot 在其前; 采纳自动位场景)。
func (s *BannerService) placeAt(ctx context.Context, nb *entity.Banner, slot int) error {
	manuals, err := s.manualRows(ctx)
	if err != nil {
		return err
	}
	i := len(manuals) - 1
	if i < 0 || manuals[i].Id != nb.Id {
		return nil // 行不在末位(并发或状态异常), 保守不动
	}
	if slot >= i {
		return nil
	}
	manuals = append(manuals[:i], manuals[i+1:]...)
	if slot > len(manuals) {
		slot = len(manuals)
	}
	manuals = append(manuals[:slot], append([]entity.Banner{*nb}, manuals[slot:]...)...)
	return s.reindex(ctx, manuals)
}

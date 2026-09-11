package service

import (
	"context"
	"time"

	"server/internal/cache"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// ttlBanners 轮播列表缓存时长。比站点配置短: 排期(起止时间)会随时间推移自然生效/失效,
// 缓存太久会出现"到点了但首页还没换"。
const ttlBanners = 3 * time.Minute

// BannerService 首页轮播: 前台读取(缓存) + 后台增删改 + 生效位实时查询。
type BannerService struct {
	repo repository.BannerRepository
	// films 兜底榜单数据源(Effective 用; 与前台首页聚合同源同缓存)。
	films *FilmService
	// OnChange 轮播配置变更回调(横图 worker 重算用, 组合根注入, 可空)。panic 由 safeNotify 隔离。
	OnChange func()
}

func NewBannerService(repo repository.BannerRepository, films *FilmService) *BannerService {
	return &BannerService{repo: repo, films: films}
}

// Public 前台轮播: 只返回启用且在生效窗口内的, 按 sort 升序。
func (s *BannerService) Public(ctx context.Context) ([]entity.Banner, error) {
	list, _, err := cache.GetOrLoad(ctx, cache.KeyBanners, ttlBanners, func(ctx context.Context) ([]entity.Banner, bool, error) {
		bs, err := s.repo.ListEnabled(ctx, time.Now().UnixMilli())
		if err != nil {
			return nil, false, err
		}
		return bs, true, nil
	})
	return list, err
}

// ManageList 后台全量列表(含停用/未到期的)。
func (s *BannerService) ManageList(ctx context.Context) ([]entity.Banner, error) {
	return s.repo.ListAll(ctx)
}

// Save 新建(id=0)或按 id 更新, 成功后失效前台缓存。
func (s *BannerService) Save(ctx context.Context, b *entity.Banner) error {
	var err error
	if b.Id > 0 {
		err = s.repo.Update(ctx, b)
	} else {
		err = s.repo.Create(ctx, b)
	}
	if err != nil {
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

// EffectiveSlide 当前实际生效的一个轮播位(后台"当前列表"展示用)。
type EffectiveSlide struct {
	// source 数据来源: "banner"=后台配置 / "fallback"=热门兜底(首页无可用配置时前端自动回退)。
	Source   string `json:"source"`
	BannerId int64  `json:"bannerId,omitempty"` // source=banner 时的配置 id, 据此跳编辑
	Mid      int64  `json:"mid,omitempty"`
	Name     string `json:"name"`
	Subtitle string `json:"subtitle,omitempty"`
	// image 生效中的宽幅主视觉: 配置横图, 或兜底片的 TMDB 回填横图(可能为空=尚未回填)。
	Image  string `json:"image,omitempty"`
	Poster string `json:"poster,omitempty"` // 竖图/封面
	Link   string `json:"link,omitempty"`
	Sort   int    `json:"sort,omitempty"`
	State  int8   `json:"state,omitempty"`
}

// Effective 后台实时生效列表 —— 首页此刻真正会展示的轮播位, 与前端 HomeView.heroSlides 同口径:
// 存在带图的启用配置 → 只展示配置(与前台 Public 同源); 否则回退首个 hot/latest 行的前 5 部。
// 兜底位的 image 带出该片已回填的 TMDB 横图, 管理页可直接看到横图采集效果。
func (s *BannerService) Effective(ctx context.Context) ([]EffectiveSlide, error) {
	bs, err := s.Public(ctx)
	if err != nil {
		return nil, err
	}
	slides := make([]EffectiveSlide, 0, len(bs)+heroFallbackCount)
	for _, b := range bs {
		if b.Image == "" && b.Poster == "" {
			continue // 与前端一致: 无图配置不参与展示
		}
		slides = append(slides, EffectiveSlide{
			Source: "banner", BannerId: b.Id, Mid: b.Mid,
			Name: b.Title, Subtitle: b.Subtitle, Image: b.Image, Poster: b.Poster,
			Link: b.Link, Sort: b.Sort, State: b.State,
		})
	}
	if len(slides) > 0 {
		return slides, nil
	}
	home, err := s.films.Home(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range home.Rows {
		src := row.Hot
		if len(src) == 0 {
			src = row.Latest
		}
		if len(src) == 0 {
			continue
		}
		if len(src) > heroFallbackCount {
			src = src[:heroFallbackCount]
		}
		for _, it := range src {
			slides = append(slides, EffectiveSlide{
				Source: "fallback", Mid: it.Mid, Name: it.Name,
				Subtitle: it.Remarks, Image: it.Backdrop, Poster: it.Cover,
			})
		}
		break
	}
	return slides, nil
}

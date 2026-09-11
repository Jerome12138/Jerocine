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

// BannerService 首页轮播: 前台读取(缓存) + 后台增删改。
type BannerService struct {
	repo repository.BannerRepository
	// OnChange 轮播配置变更回调(横图 worker 重算用, 组合根注入, 可空)。panic 由 safeNotify 隔离。
	OnChange func()
}

func NewBannerService(repo repository.BannerRepository) *BannerService {
	return &BannerService{repo: repo}
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

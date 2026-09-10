package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

type bannerRepo struct{ db *gorm.DB }

// NewBannerRepository 构造首页轮播仓储。
func NewBannerRepository(db *gorm.DB) repository.BannerRepository {
	return &bannerRepo{db: db}
}

// ListEnabled 生效 = 启用(state=0) 且 落在 [start_at, end_at] 窗口内(0 表示该端不限)。
func (r *bannerRepo) ListEnabled(ctx context.Context, now int64) ([]entity.Banner, error) {
	var out []entity.Banner
	err := dbFrom(ctx, r.db).
		Where("state = ?", entity.BannerEnabled).
		Where("(start_at = 0 OR start_at <= ?)", now).
		Where("(end_at = 0 OR end_at >= ?)", now).
		Order("sort ASC, id ASC").
		Find(&out).Error
	return out, err
}

func (r *bannerRepo) ListAll(ctx context.Context) ([]entity.Banner, error) {
	var out []entity.Banner
	err := dbFrom(ctx, r.db).Order("sort ASC, id ASC").Find(&out).Error
	return out, err
}

func (r *bannerRepo) Get(ctx context.Context, id int64) (*entity.Banner, error) {
	var b entity.Banner
	err := dbFrom(ctx, r.db).Where("id = ?", id).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *bannerRepo) Create(ctx context.Context, b *entity.Banner) error {
	return dbFrom(ctx, r.db).Create(b).Error
}

func (r *bannerRepo) Update(ctx context.Context, b *entity.Banner) error {
	res := dbFrom(ctx, r.db).Model(&entity.Banner{}).Where("id = ?", b.Id).
		Updates(map[string]any{
			"title":      b.Title,
			"subtitle":   b.Subtitle,
			"image":      b.Image,
			"poster":     b.Poster,
			"mid":        b.Mid,
			"link":       b.Link,
			"sort":       b.Sort,
			"state":      b.State,
			"start_at":   b.StartAt,
			"end_at":     b.EndAt,
			"updated_at": b.UpdatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *bannerRepo) Delete(ctx context.Context, id int64) error {
	res := dbFrom(ctx, r.db).Where("id = ?", id).Delete(&entity.Banner{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

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

// Update 按 id 整体覆盖。
//
// 存在性单独判一次而不是看 RowsAffected: MySQL 默认 affected_rows 是**实际变更**的行数
// (不是匹配行数, DSN 未设 clientFoundRows), 所以"所有列写入值与库里完全一致"时
// RowsAffected 也是 0 —— 用 0 判"不存在"会把"原样保存一次"误报成 404。
//
// updated_at 不在这里写: entity.Banner 上有 autoUpdateTime:milli, 交给 gorm 在 UPDATE 时
// 填当前时间, 避免调用方(HTTP body)传 0 把审计时间写成 0。
func (r *bannerRepo) Update(ctx context.Context, b *entity.Banner) error {
	db := dbFrom(ctx, r.db)
	var n int64
	if err := db.Model(&entity.Banner{}).Where("id = ?", b.Id).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return db.Model(&entity.Banner{}).Where("id = ?", b.Id).
		Updates(map[string]any{
			"title":    b.Title,
			"subtitle": b.Subtitle,
			"image":    b.Image,
			"poster":   b.Poster,
			"mid":      b.Mid,
			"link":     b.Link,
			"sort":     b.Sort,
			"state":    b.State,
			"start_at": b.StartAt,
			"end_at":   b.EndAt,
		}).Error
}

// UpdateSort 只更新排序值(排序重排专用, 不碰其余字段)。
func (r *bannerRepo) UpdateSort(ctx context.Context, id int64, sort int) error {
	return dbFrom(ctx, r.db).Model(&entity.Banner{}).Where("id = ?", id).
		Update("sort", sort).Error
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

package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

type liveSourceRepo struct{ db *gorm.DB }

// NewLiveSourceRepository 构造直播源仓储。
func NewLiveSourceRepository(db *gorm.DB) repository.LiveSourceRepository {
	return &liveSourceRepo{db: db}
}

func (r *liveSourceRepo) ListEnabled(ctx context.Context) ([]entity.LiveSource, error) {
	var out []entity.LiveSource
	err := dbFrom(ctx, r.db).
		Where("state = ?", entity.LiveSourceEnabled).
		Order("sort ASC, id ASC").
		Find(&out).Error
	return out, err
}

func (r *liveSourceRepo) ListAll(ctx context.Context) ([]entity.LiveSource, error) {
	var out []entity.LiveSource
	err := dbFrom(ctx, r.db).Order("sort ASC, id ASC").Find(&out).Error
	return out, err
}

func (r *liveSourceRepo) Get(ctx context.Context, id int64) (*entity.LiveSource, error) {
	var s entity.LiveSource
	err := dbFrom(ctx, r.db).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *liveSourceRepo) Create(ctx context.Context, s *entity.LiveSource) error {
	return dbFrom(ctx, r.db).Create(s).Error
}

// Update 按 id 整体覆盖（存在性单独 Count —— RowsAffected 语义坑见 banner_repo 同款注释）。
func (r *liveSourceRepo) Update(ctx context.Context, s *entity.LiveSource) error {
	db := dbFrom(ctx, r.db)
	var n int64
	if err := db.Model(&entity.LiveSource{}).Where("id = ?", s.Id).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return db.Model(&entity.LiveSource{}).Where("id = ?", s.Id).
		Updates(map[string]any{
			"name":   s.Name,
			"url":    s.URL,
			"remark": s.Remark,
			"sort":   s.Sort,
			"state":  s.State,
		}).Error
}

func (r *liveSourceRepo) Delete(ctx context.Context, id int64) error {
	res := dbFrom(ctx, r.db).Where("id = ?", id).Delete(&entity.LiveSource{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

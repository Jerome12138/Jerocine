package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

type categoryRepo struct{ db *gorm.DB }

// NewCategoryRepository 构造分类仓储。
func NewCategoryRepository(db *gorm.DB) repository.CategoryRepository { return &categoryRepo{db: db} }

func (r *categoryRepo) All(ctx context.Context) ([]entity.Category, error) {
	var out []entity.Category
	err := dbFrom(ctx, r.db).Order("sort ASC, id ASC").Find(&out).Error
	return out, err
}

func (r *categoryRepo) Get(ctx context.Context, id int64) (*entity.Category, error) {
	var c entity.Category
	err := dbFrom(ctx, r.db).Where("id = ?", id).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *categoryRepo) Upsert(ctx context.Context, c *entity.Category) error {
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(c).Error
}

func (r *categoryRepo) BatchUpsert(ctx context.Context, list []entity.Category) error {
	if len(list) == 0 {
		return nil
	}
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).CreateInBatches(list, 200).Error
}

func (r *categoryRepo) Delete(ctx context.Context, id int64) error {
	return dbFrom(ctx, r.db).Where("id = ?", id).Delete(&entity.Category{}).Error
}

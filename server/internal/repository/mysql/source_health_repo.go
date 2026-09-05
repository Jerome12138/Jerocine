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

type sourceHealthRepo struct{ db *gorm.DB }

// NewSourceHealthRepository 构造采集源健康度仓储。
func NewSourceHealthRepository(db *gorm.DB) repository.SourceHealthRepository {
	return &sourceHealthRepo{db: db}
}

func (r *sourceHealthRepo) Get(ctx context.Context, sourceId string) (*entity.SourceHealth, error) {
	var h entity.SourceHealth
	err := dbFrom(ctx, r.db).Where("source_id = ?", sourceId).First(&h).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *sourceHealthRepo) List(ctx context.Context) ([]entity.SourceHealth, error) {
	var out []entity.SourceHealth
	err := dbFrom(ctx, r.db).Find(&out).Error
	return out, err
}

func (r *sourceHealthRepo) Upsert(ctx context.Context, h *entity.SourceHealth) error {
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "source_id"}},
		UpdateAll: true,
	}).Create(h).Error
}

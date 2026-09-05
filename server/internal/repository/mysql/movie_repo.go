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

type movieRepo struct{ db *gorm.DB }

// NewMovieRepository 构造影片详情主表仓储。
func NewMovieRepository(db *gorm.DB) repository.MovieRepository { return &movieRepo{db: db} }

func (r *movieRepo) GetByMid(ctx context.Context, mid int64) (*entity.Movie, error) {
	var m entity.Movie
	err := dbFrom(ctx, r.db).Where("mid = ?", mid).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrMovieNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *movieRepo) Upsert(ctx context.Context, m *entity.Movie) error {
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "mid"}},
		UpdateAll: true,
	}).Create(m).Error
}

func (r *movieRepo) BatchUpsert(ctx context.Context, list []entity.Movie) error {
	if len(list) == 0 {
		return nil
	}
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "mid"}},
		UpdateAll: true,
	}).CreateInBatches(list, 200).Error
}

func (r *movieRepo) Delete(ctx context.Context, mid int64) error {
	return dbFrom(ctx, r.db).Where("mid = ?", mid).Delete(&entity.Movie{}).Error
}

func (r *movieRepo) Truncate(ctx context.Context) error {
	return dbFrom(ctx, r.db).Exec("TRUNCATE TABLE movie").Error
}

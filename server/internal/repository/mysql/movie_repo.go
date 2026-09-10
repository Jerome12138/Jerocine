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

// movieUpsertCols 采集回写时参与 ON DUPLICATE KEY UPDATE 的列。
// 刻意剔除三列, 让 upsert 只覆盖"内容", 不碰"生命周期":
//   - mid:        主键, 冲突判定依据, 本就不该出现在更新集里;
//   - created_at: 首次入库时间, 重采不应把它刷成现在(否则"今日新增"会虚高);
//   - deleted_at: 软删标记, 若跟着更新, 源站把已删影片再推一次就会自动复活。
//
// 清单与 entity.Movie 的同步由 TestMovieUpsertColsCoverEntity 反射校验, 漏改会直接测试失败。
var movieUpsertCols = []string{
	"cid", "pid", "name", "sub_title", "c_name", "en_name", "initial", "class_tag",
	"area", "language", "year", "actor", "director", "writer", "content",
	"db_id", "db_score", "hits", "state", "remarks", "cover",
	"play_from", "down_from", "release_stamp", "update_stamp", "updated_at",
}

// movieUpsertExclude 内容列之外的例外列(mid 是冲突键, 另两列见 movieUpsertCols 注释)。
var movieUpsertExclude = map[string]bool{"mid": true, "created_at": true, "deleted_at": true}

// movieUpsertClause 冲突时按内容列更新。MySQL 侧会编译成 `col = VALUES(col)`。
func movieUpsertClause() clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "mid"}},
		DoUpdates: clause.AssignmentColumns(movieUpsertCols),
	}
}

func (r *movieRepo) GetByMid(ctx context.Context, mid int64) (*entity.Movie, error) {
	return r.getByMid(ctx, mid, false)
}

func (r *movieRepo) GetByMidIncludingDeleted(ctx context.Context, mid int64) (*entity.Movie, error) {
	return r.getByMid(ctx, mid, true)
}

// getByMid 读单部影片; includeDeleted=false 时把已软删的当作不存在(公开读路径)。
func (r *movieRepo) getByMid(ctx context.Context, mid int64, includeDeleted bool) (*entity.Movie, error) {
	q := dbFrom(ctx, r.db).Where("mid = ?", mid)
	if !includeDeleted {
		q = q.Where("deleted_at = 0")
	}
	var m entity.Movie
	err := q.First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrMovieNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *movieRepo) Upsert(ctx context.Context, m *entity.Movie) error {
	return dbFrom(ctx, r.db).Clauses(movieUpsertClause()).Create(m).Error
}

func (r *movieRepo) BatchUpsert(ctx context.Context, list []entity.Movie) error {
	if len(list) == 0 {
		return nil
	}
	return dbFrom(ctx, r.db).Clauses(movieUpsertClause()).CreateInBatches(list, 200).Error
}

func (r *movieRepo) Delete(ctx context.Context, mid int64) error {
	return dbFrom(ctx, r.db).Where("mid = ?", mid).Delete(&entity.Movie{}).Error
}

func (r *movieRepo) SoftDelete(ctx context.Context, mid, deletedAt int64) error {
	if deletedAt <= 0 {
		deletedAt = nowMilli()
	}
	return dbFrom(ctx, r.db).Model(&entity.Movie{}).
		Where("mid = ?", mid).
		Update("deleted_at", deletedAt).Error
}

func (r *movieRepo) Restore(ctx context.Context, mid int64) error {
	return dbFrom(ctx, r.db).Model(&entity.Movie{}).
		Where("mid = ?", mid).
		Update("deleted_at", 0).Error
}

func (r *movieRepo) Truncate(ctx context.Context) error {
	return dbFrom(ctx, r.db).Exec("TRUNCATE TABLE movie").Error
}

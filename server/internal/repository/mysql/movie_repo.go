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

// movieUpsertCols 采集回写时参与 ON DUPLICATE KEY UPDATE 的列。分两类处置:
//
//	B 类 条件更新 —— db_id / db_score / year / pub_date(见 base.go 的 conditionalUpsert):
//	  这四列源站与本地都会写, 本地只在源站给不出时才补, 冲突时按 IF(VALUES(x) 有效, ...) 取舍,
//	  保证"新采集为空/0 不覆盖已有值"。
//	其余 常规覆盖 `col = VALUES(col)` —— name / cover / remarks 这类纯内容列以源站为权威。
//
// 另有 A 类列完全排除在更新集之外, 见 movieUpsertExclude:
//   - mid:        主键, 冲突判定依据, 本就不该出现在更新集里;
//   - created_at: 首次入库时间, 重采不应把它刷成现在(否则"今日新增"会虚高);
//   - deleted_at: 软删标记, 若跟着更新, 源站把已删影片再推一次就会自动复活;
//   - backdrop:   TMDB 横图由后台 worker 下载回填, 源站没有该数据, 跟着更新只会把已回填的抹成空;
//   - hot_rank / hot_rank_at / hot_score / db_id_src:
//     豆瓣榜单计算 / 回填列, 源站完全没有对应数据, 跟着更新会把算好的热度抹成 0。
//
// 清单与 entity.Movie 的同步由 TestMovieUpsertColsCoverEntity 反射校验, 漏改会直接测试失败。
var movieUpsertCols = []string{
	"cid", "pid", "name", "sub_title", "c_name", "en_name", "initial", "class_tag",
	"area", "language", "year", "pub_date", "actor", "director", "writer", "content",
	"db_id", "db_score", "hits", "state", "remarks", "cover",
	"play_from", "down_from", "release_stamp", "update_stamp", "updated_at",
}

// movieUpsertExclude 内容列之外的例外列(A 类, 各列理由见 movieUpsertCols 注释)。
var movieUpsertExclude = map[string]bool{
	"mid": true, "created_at": true, "deleted_at": true, "backdrop": true,
	"hot_rank": true, "hot_rank_at": true, "hot_score": true, "db_id_src": true,
}

// movieUpsertClause 冲突时按内容列更新(常规列 VALUES(col) + B 类条件表达式)。
func movieUpsertClause() clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "mid"}},
		DoUpdates: upsertAssignments(movieUpsertCols),
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

// ListMissingBackdropsByMids 取指定影片中尚未回填横图的行(未软删、非空片名、backdrop 为空)。
// 入参是首页轮播集合(配置 banner 关联片 + 兜底 hot/latest), 范围刻意收窄 —— 不做全库回填。
func (r *movieRepo) ListMissingBackdropsByMids(ctx context.Context, mids []int64) ([]entity.Movie, error) {
	if len(mids) == 0 {
		return nil, nil
	}
	var list []entity.Movie
	err := dbFrom(ctx, r.db).
		Where("mid IN ? AND deleted_at = 0 AND name != '' AND backdrop = ''", mids).
		Order("mid ASC").
		Find(&list).Error
	return list, err
}

// UpdateBackdrop 回填横图(全量写, url 可为 MissMark 哨兵)。返回是否确有行被更新。
func (r *movieRepo) UpdateBackdrop(ctx context.Context, mid int64, url string) (bool, error) {
	res := dbFrom(ctx, r.db).Model(&entity.Movie{}).
		Where("mid = ?", mid).
		Update("backdrop", url)
	return res.RowsAffected > 0, res.Error
}

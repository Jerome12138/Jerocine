package mysql

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

type playRepo struct{ db *gorm.DB }

// NewPlaySourceRepository 构造多源播放仓储。
func NewPlaySourceRepository(db *gorm.DB) repository.PlaySourceRepository { return &playRepo{db: db} }

func (r *playRepo) UpsertBatch(ctx context.Context, list []entity.MoviePlaySource) error {
	if len(list) == 0 {
		return nil
	}
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "match_key"}, {Name: "play_from"}},
		// source_vod_id 纳入更新列: 重采时回填旧行的源站影片ID(供去重统计 CountBySite)。
		DoUpdates: clause.AssignmentColumns([]string{"mid", "source_vod_id", "episodes_json", "updated_at"}),
	}).CreateInBatches(list, 200).Error
}

func (r *playRepo) ListByMid(ctx context.Context, mid int64) ([]entity.MoviePlaySource, error) {
	var out []entity.MoviePlaySource
	err := dbFrom(ctx, r.db).Where("mid = ?", mid).Find(&out).Error
	return out, err
}

func (r *playRepo) GetByMatchKeys(ctx context.Context, siteId string, matchKeys []string) ([]entity.MoviePlaySource, error) {
	if len(matchKeys) == 0 {
		return nil, nil
	}
	var out []entity.MoviePlaySource
	q := dbFrom(ctx, r.db).Where("match_key IN ?", matchKeys)
	if siteId != "" {
		q = q.Where("site_id = ?", siteId)
	}
	err := q.Find(&out).Error
	return out, err
}

func (r *playRepo) Truncate(ctx context.Context) error {
	return dbFrom(ctx, r.db).Exec("TRUNCATE TABLE movie_play_source").Error
}

// CountBySite 一次 GROUP BY site_id 统计各源已采集"去重片数"。
// 补充源同一部片按 hash(片名)/hash(豆瓣ID) 双键各存一行, 故不能直接 COUNT(*)(会把一片记多行,
// sn 实测 1.43x)。两段不相交求和去重: 新行按源站 source_vod_id 去重(同片双键共享同一 vod_id → 计1),
// 旧行(vod_id=0)回退按 match_key 计; 全量重采一轮后旧行清零、自然收敛到去重片数。
// 仅引用 site_id/source_vod_id/match_key 三列, 命中覆盖索引 idx_mps_site_vod_match → index-only,
// 不回表读取巨大的 episodes_json 列(否则全表扫描 30~60s 超时)。
func (r *playRepo) CountBySite(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		SiteId string
		Count  int64
	}
	if err := dbFrom(ctx, r.db).Model(&entity.MoviePlaySource{}).
		Select("site_id, " +
			"COUNT(DISTINCT IF(source_vod_id <> 0, source_vod_id, NULL)) + " +
			"COUNT(DISTINCT IF(source_vod_id = 0, match_key, NULL)) AS count").
		Group("site_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]int64, len(rows))
	for _, x := range rows {
		m[x.SiteId] = x.Count
	}
	return m, nil
}

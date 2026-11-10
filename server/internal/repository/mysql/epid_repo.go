package mysql

import (
	"context"

	"gorm.io/gorm"

	"server/internal/domain/entity"
)

// EpidRepo 派生表 movie_episode_epid 的读写（P1-1, 2026-11-10）。
// **单向派生**：本表只由派生任务写；`episodes_json` 是唯一真相源。
// 刻意导出**具体类型**（而非走 ports.go 的公共端口）：服务层只依赖它，降低回归面。
type EpidRepo struct{ db *gorm.DB }

func NewEpidRepository(db *gorm.DB) *EpidRepo { return &EpidRepo{db: db} }

// ReplaceForSourceRow **整源行重算**：事务内清旧 + 插新（P1-1 §5.4）。
//
// ⚠️ 绝不按 `ep_index` 打补丁 —— 集序会漂（重排/插集），按位置补丁必然错位。
// 同事务 ⇒ 读侧要么见旧、要么见新，不会读到半截。
func (r *EpidRepo) ReplaceForSourceRow(ctx context.Context, srcRowId int64, rows []entity.MovieEpisodeEpid) error {
	return dbFrom(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("src_row_id = ?", srcRowId).Delete(&entity.MovieEpisodeEpid{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(rows, 200).Error
	})
}

// Anchor 源行当前的派生状态锚。
type Anchor struct {
	N         int    // 现有派生行数（`0` = 从未派生 ⇒ F-07 的"首次派生"分支）
	SrcEpHash string // 派生时的 ep_hash
}

// AnchorsBySrcRowIds 批量取锚（走 `uk_row_ep` 前缀 ⇒ index-only，不碰巨表）。
func (r *EpidRepo) AnchorsBySrcRowIds(ctx context.Context, ids []int64) (map[int64]Anchor, error) {
	out := make(map[int64]Anchor, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		SrcRowId  int64
		N         int
		SrcEpHash string
	}
	err := dbFrom(ctx, r.db).Model(&entity.MovieEpisodeEpid{}).
		Select("src_row_id, COUNT(*) AS n, MIN(src_ep_hash) AS src_ep_hash").
		Where("src_row_id IN ?", ids).
		Group("src_row_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.SrcRowId] = Anchor{N: x.N, SrcEpHash: x.SrcEpHash}
	}
	return out, nil
}

// ExistsRawName 旧派生行里是否存在这些集名之一（**抖动守卫**用它判断"新旧集名是否完全不相交"）。
func (r *EpidRepo) ExistsRawName(ctx context.Context, srcRowId int64, names []string) (bool, error) {
	if len(names) == 0 {
		return false, nil
	}
	var n int64
	err := dbFrom(ctx, r.db).Model(&entity.MovieEpisodeEpid{}).
		Where("src_row_id = ? AND raw_name IN ?", srcRowId, names).
		Limit(1).Count(&n).Error
	return n > 0, err
}

// DeleteBySrcRowIds 按源行批量删（级联清理 / 抖动回滚）。
func (r *EpidRepo) DeleteBySrcRowIds(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx := dbFrom(ctx, r.db).Where("src_row_id IN ?", ids).Delete(&entity.MovieEpisodeEpid{})
	return tx.RowsAffected, tx.Error
}

// OrphanSrcRowIds 找**孤儿**：派生表里有、但源表已不存在（整片下架/站点下线/Truncate）—— F-06。
// 用 NOT EXISTS + 主键子查询（index-only），可离线慢慢跑。
func (r *EpidRepo) OrphanSrcRowIds(ctx context.Context, limit int) ([]int64, error) {
	var ids []int64
	err := dbFrom(ctx, r.db).Raw(
		"SELECT DISTINCT e.src_row_id FROM movie_episode_epid e "+
			"WHERE NOT EXISTS (SELECT 1 FROM movie_play_source s WHERE s.id = e.src_row_id) LIMIT ?", limit).
		Scan(&ids).Error
	return ids, err
}

// FromSrcStat 来源分布（P1-1 验收：校验归一链假设 L1~L6）。
type FromSrcStat struct {
	FromSrc string
	N       int64
}

func (r *EpidRepo) FromSrcStats(ctx context.Context) ([]FromSrcStat, error) {
	var out []FromSrcStat
	err := dbFrom(ctx, r.db).Model(&entity.MovieEpisodeEpid{}).
		Select("from_src, COUNT(*) AS n").Group("from_src").Order("n DESC").Scan(&out).Error
	return out, err
}

// Coverage 派生覆盖率：(派生行数, 空 epid 行数)。
func (r *EpidRepo) Coverage(ctx context.Context) (total int64, empty int64, err error) {
	if err = dbFrom(ctx, r.db).Model(&entity.MovieEpisodeEpid{}).Count(&total).Error; err != nil {
		return
	}
	err = dbFrom(ctx, r.db).Model(&entity.MovieEpisodeEpid{}).Where("epid = ''").Count(&empty).Error
	return
}

// AlignmentRate 跨源对齐率（P1-1 验收的**主指标**）：同一部片（`match_key`）里同一个 `epid`
// 出现在 **≥2 个不同 site** 的比例。仅统计**非空 epid**（空串 = 不产身份，按 §4.7 豁免）。
func (r *EpidRepo) AlignmentRate(ctx context.Context) (aligned int64, total int64, err error) {
	var row struct {
		Aligned int64
		Total   int64
	}
	err = dbFrom(ctx, r.db).Raw(`
		SELECT
		  SUM(CASE WHEN sites >= 2 THEN 1 ELSE 0 END) AS aligned,
		  COUNT(*) AS total
		FROM (
		  SELECT match_key, epid, COUNT(DISTINCT site_id) AS sites
		  FROM movie_episode_epid
		  WHERE epid <> ''
		  GROUP BY match_key, epid
		) t`).Scan(&row).Error
	return row.Aligned, row.Total, err
}

// ListByMatchKeys 一次批量取整片所有源的 epid（**性能铁律 3：避免 N+1**）——
// 供 P1-1b 出参扩展使用。只取必要列（不碰巨表）。
func (r *EpidRepo) ListByMatchKeys(ctx context.Context, keys []string) ([]entity.MovieEpisodeEpid, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	var out []entity.MovieEpisodeEpid
	err := dbFrom(ctx, r.db).
		Select("src_row_id, site_id, play_from, ep_index, epid, from_src").
		Where("match_key IN ?", keys).
		Find(&out).Error
	return out, err
}

// ===== 派生任务用（P1-1 §5.4）=====
//
// 这两支刻意放在 EpidRepo 上（而非 playRepo 的端口接口）：服务层只需依赖本具体类型，
// 不必改动 `domain/repository/ports.go` 的公共端口，降低回归面。

// DirtySourceRows keyset 分页取"**待派生 / 需重算**"的源行（P1-1 §5.4）。
//
// 判据：`ep_hash` 非空（没哨兵的行跳过）且（派生侧无锚 **或** 锚与本行当前 `ep_hash` 不等）。
// 只对脏行读 `episodes_json`（派生必须读它）——派生锚子查询被 `src_row_id > ?` 限制在同一
// keyset 窗口内，避免每次全表 GROUP BY 647 万行。
func (r *EpidRepo) DirtySourceRows(ctx context.Context, afterId int64, limit int) ([]entity.MoviePlaySource, error) {
	var out []entity.MoviePlaySource
	err := dbFrom(ctx, r.db).Raw(`
		SELECT s.id, s.mid, s.match_key, s.site_id, s.play_from, s.episodes_json, s.ep_hash
		FROM movie_play_source s
		LEFT JOIN (
		  SELECT src_row_id, MIN(src_ep_hash) AS h
		  FROM movie_episode_epid
		  WHERE src_row_id > ?
		  GROUP BY src_row_id
		) d ON d.src_row_id = s.id
		WHERE s.id > ? AND s.ep_hash <> '' AND (d.src_row_id IS NULL OR d.h <> s.ep_hash)
		ORDER BY s.id ASC
		LIMIT ?`, afterId, afterId, limit).Scan(&out).Error
	return out, err
}

// ExistingSourceRowIds 批量判断源行是否仍存在（孤儿清理用；只取主键 ⇒ index-only，不碰巨表）。
func (r *EpidRepo) ExistingSourceRowIds(ctx context.Context, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	var got []int64
	if err := dbFrom(ctx, r.db).Model(&entity.MoviePlaySource{}).
		Where("id IN ?", ids).Pluck("id", &got).Error; err != nil {
		return nil, err
	}
	for _, id := range got {
		out[id] = true
	}
	return out, nil
}

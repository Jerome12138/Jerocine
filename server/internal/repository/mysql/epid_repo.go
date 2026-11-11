package mysql

import (
	"context"
	"sync"

	"gorm.io/gorm"

	"server/internal/domain/entity"
)

// EpidRepo 派生表 movie_episode_epid 的读写（P1-1, 2026-11-10）。
// **单向派生**：本表只由派生任务写；`episodes_json` 是唯一真相源。
// 刻意导出**具体类型**（而非走 ports.go 的公共端口）：服务层只依赖它，降低回归面。
type EpidRepo struct{ db *gorm.DB }

func NewEpidRepository(db *gorm.DB) *EpidRepo { return &EpidRepo{db: db} }

// ---- 包级便捷查询（P1-1b，2026-11-10）----
//
// 取向：给 `FilmService` 提供 epid 而**不改其构造签名**（其签名被多处测试使用，加参会扩大回归面）；
// 装配在 main 里一行完成，**未装配时本函数返回 (nil, nil)** ⇒ 调用方当作"无 epid" ⇒ 行为与改动前一致（安全降级）。
var (
	defaultEpidRepoMu sync.RWMutex
	defaultEpidRepo   *EpidRepo
)

func SetDefaultEpidRepo(r *EpidRepo) {
	defaultEpidRepoMu.Lock()
	defer defaultEpidRepoMu.Unlock()
	defaultEpidRepo = r
}

func currentEpidRepo() *EpidRepo {
	defaultEpidRepoMu.RLock()
	defer defaultEpidRepoMu.RUnlock()
	return defaultEpidRepo
}

// LookupEpidsByMatchKeys 一次批量取"整片所有源"的 epid（**性能铁律 3：避免 N+1**）。
//
// 返回 `map[Id]map[epIndex]epid`，其中 `Id = site_id + ":" + play_from` —— 与 `PlaySourceView.Id`
// 的口径一致（调用方据此把它挂回对应源）。
func LookupEpidsByMatchKeys(ctx context.Context, keys []string) (map[string]map[int]string, error) {
	r := currentEpidRepo()
	if r == nil || len(keys) == 0 {
		return nil, nil
	}
	rows, err := r.ListByMatchKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[int]string, len(rows))
	for _, x := range rows {
		id := x.SiteId + ":" + x.PlayFrom
		m, ok := out[id]
		if !ok {
			m = map[int]string{}
			out[id] = m
		}
		m[x.EpIndex] = x.Epid
	}
	return out, nil
}

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
	N              int    // 现有派生行数（`0` = 从未派生 ⇒ F-07 的"首次派生"分支）
	SrcEpHash      string // 派生时的 ep_hash
	LastComputedAt int64  // 上次重建时间（L4 宽限期用）
}

// AnchorsBySrcRowIds 批量取锚（走 `uk_row_ep` 前缀 ⇒ index-only，不碰巨表）。
func (r *EpidRepo) AnchorsBySrcRowIds(ctx context.Context, ids []int64) (map[int64]Anchor, error) {
	out := make(map[int64]Anchor, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		SrcRowId       int64
		N              int
		SrcEpHash      string
		LastComputedAt int64
	}
	err := dbFrom(ctx, r.db).Model(&entity.MovieEpisodeEpid{}).
		Select("src_row_id, COUNT(*) AS n, MIN(src_ep_hash) AS src_ep_hash, MAX(computed_at) AS last_computed_at").
		Where("src_row_id IN ?", ids).
		Group("src_row_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.SrcRowId] = Anchor{N: x.N, SrcEpHash: x.SrcEpHash, LastComputedAt: x.LastComputedAt}
	}
	return out, nil
}

// EpHashBy 批量取源行当前 `ep_hash`（只取两列 ⇒ **不碰巨列**）。
func (r *EpidRepo) EpHashBy(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		Id     int64
		EpHash string
	}
	if err := dbFrom(ctx, r.db).Model(&entity.MoviePlaySource{}).
		Select("id, ep_hash").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.Id] = x.EpHash
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
// **M3 修法（2026-11-11 评审）**：原实现用「剩余派生表全表 GROUP BY」子查询判脏（注释声称"限制在
// 同一窗口"并不成立）⇒ 一轮总扫描 ≈ 647.7 万 × 批数（数十亿行）✗。现改为**三步、全程 index-only**：
//  1. 取候选 id 页（`SELECT id ... ORDER BY id LIMIT n` ⇒ 覆盖主键）
//  2. 对本页 ids 取派生态锚（`AnchorsBySrcRowIds` ⇒ `uk_row_ep` 前缀）
//  3. 只对**脏的** id 取全字段（含 `episodes_json` 巨列）
// ⇒ 每批成本与**表规模无关**；且不再需要 `ep_hash <> ''` 过滤（L7：空 hash 也当候选，首次派生一次即收敛）
func (r *EpidRepo) DirtySourceRows(ctx context.Context, afterId int64, limit int) ([]entity.MoviePlaySource, error) {
	var ids []int64
	if err := dbFrom(ctx, r.db).Model(&entity.MoviePlaySource{}).
		Where("id > ?", afterId).Order("id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	anchors, err := r.AnchorsBySrcRowIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	hashes, err := r.EpHashBy(ctx, ids)
	if err != nil {
		return nil, err
	}
	dirty := make([]int64, 0, len(ids))
	for _, id := range ids {
		a, ok := anchors[id]
		if !ok || a.SrcEpHash != hashes[id] {
			dirty = append(dirty, id)
		}
	}
	if len(dirty) == 0 {
		return nil, nil
	}
	var out []entity.MoviePlaySource
	if err := dbFrom(ctx, r.db).Where("id IN ?", dirty).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// SourceTitlesByMatchKeys 取"**同片其它源行**"的集名（M2, 2026-11-11 评审）——
// 供派生任务组成"本片所有源的所有集名"池（web `allEpisodeTitles` 口径）。
// 只对**本批脏行涉及的片**发起一次批量查询（无 N+1）；`excludeIds` 排除本批自身。
func (r *EpidRepo) SourceTitlesByMatchKeys(ctx context.Context, keys []string, excludeIds []int64) (map[string][]string, error) {
	out := make(map[string][]string, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	var rows []entity.MoviePlaySource
	q := dbFrom(ctx, r.db).Select("match_key, episodes_json").Where("match_key IN ?", keys)
	if len(excludeIds) > 0 {
		q = q.Where("id NOT IN ?", excludeIds)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, x := range rows {
		for _, e := range x.Episodes {
			if e.Episode != "" {
				out[x.MatchKey] = append(out[x.MatchKey], e.Episode)
			}
		}
	}
	return out, nil
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

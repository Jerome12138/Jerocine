package mysql

import (
	"context"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"server/internal/danmu"
	"server/internal/domain/entity"
)

// EpidRepo 派生表 movie_episode_epid 的读写（P1-1, 2026-11-10）。
// **单向派生**：本表只由派生任务写；`episodes_json` 是唯一真相源。
// 刻意导出**具体类型**（而非走 ports.go 的公共端口）：服务层只依赖它，降低回归面。
// EpidRepo 集身份派生仓储：**两套连接（2026-11-13）**。
//
//   - `db`：主连接（正常写 binlog）—— 所有**读**、以及**真相源/计数器**的写（`movie_play_source`、
//     `epid_derive_stat`）都必须走它，保住 PITR 能力；
//   - `deriveDB`：**派生写**专用（会话级 `sql_log_bin=0`，见 `platform/db.InitMySQLNoBinlog`）——
//     只用于 `movie_episode_epid` 的 整源行 DELETE+INSERT / 孤儿清理 / override 覆盖 / `TRUNCATE` 重建。
//     派生表是**单向派生产物**（可由 `movie_play_source` 重算回来）⇒ 不记 binlog 是安全的，
//     而它的写入量最大（全量 rebuild 一轮就是 650 万行级 DELETE+INSERT），是 binlog 的主要来源之一。
//
// ⚠️ **绝不可**把真相源的写放到 `deriveDB` 上。
type EpidRepo struct {
	db       *gorm.DB
	deriveDB *gorm.DB
}

// NewEpidRepository 主连接同时充当派生写连接（照常写 binlog；旧行为，供测试与兼容装配）。
func NewEpidRepository(db *gorm.DB) *EpidRepo { return &EpidRepo{db: db, deriveDB: db} }

// NewEpidRepositoryWithDeriveDB 生产装配：派生**写**走 `deriveDB`（`sql_log_bin=0`）。
// `deriveDB` 为 nil ⇒ 降级为主连接 —— 该特性只省 binlog 体积，绝不阻断派生。
func NewEpidRepositoryWithDeriveDB(db, deriveDB *gorm.DB) *EpidRepo {
	if deriveDB == nil {
		deriveDB = db
	}
	return &EpidRepo{db: db, deriveDB: deriveDB}
}

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

// epidInsertChunk 插批大小（对照采集侧 `UpsertBatch` 的 200 行/语句；派生行更宽 ⇒ 500 行/句）。
const epidInsertChunk = 500

// ReplaceForSourceRows **批级整源行重算**：一个事务内「清旧 + 插新」，一次覆盖**多个源行**（P1-1 §5.4）。
//
// ⚠️ 绝不按 `ep_index` 打补丁 —— 集序会漂（重排/插集），按位置补丁必然错位。
// 同事务 ⇒ 读侧要么见旧、要么见新，不会读到半截。
//
// **2026-11-11 优化（借采集侧批量思路）**：原实现是**每源行一个事务** ⇒ 单批 2000 个事务
// （DELETE + `CreateInBatches`），redo/binlog 与 4 个索引的维护被放大 2000 倍 —— 与采集侧
// `playRepo.UpsertBatch`（200 行/语句、一次调用处理整页）相比效率差距明显。批级后事务数降到个位数/批。
//
// **语义变化（可接受）**：原子性从"单源行"放宽到"**一次 flush 的一组源行**" ⇒ 失败时这组一起回滚，
// 下次按 `src_ep_hash` 判脏重新派生 ✓（幂等）；窗口内读侧看到"没有 epid" ⇒ 端侧回退实时解析（§4.6）✓。
//
// **调用方约束**：`ids` 必须是**本次 flush 内真要重建**的全部源行（被守卫拦下的行**不要**传进来，
// 否则会被白删 —— 那些行的旧派生要保留）。
func (r *EpidRepo) ReplaceForSourceRows(ctx context.Context, ids []int64, rows []entity.MovieEpisodeEpid) error {
	if len(ids) == 0 {
		return nil
	}
	return dbFrom(ctx, r.deriveDB).Transaction(func(tx *gorm.DB) error {
		// ⚠️ **binlog 取舍（2026-11-13 评审 F2）**：本事务走 `deriveDB`（`sql_log_bin=0`）⇒ **计数器
		// `epid_derive_stat` 的同步增减也不进 binlog**。这是**有意**的：
		//   · 拆到主连接会破坏"派生行与计数在同一事务"的**原子性**（跨连接无法回滚一致）；
		//   · 而 `epid_derive_stat` 是**可重算的读数**（`RecountDeriveStat` 一次 COUNT 即可校准）。
		// ⇒ PITR 后若读数失真，重算即可，**不影响派生结果本身**。
		// ---- 进度计数器（迁移 000048, 2026-11-11）----
		// 必须在 DELETE **之前**取"这批源行里已有派生的源行数"，才能算出 `derived_src_rows` 的**净增减**
		// （它是全局计数：只加不减会越滚越大 ✗）。成本 = 一次 `WHERE src_row_id IN (≤500 个 id)`，
		// 走 `uk_row_ep` 前缀 ⇒ 与表规模无关 ✓。
		var before struct{ OldSrcRows int64 }
		if err := tx.Raw(
			"SELECT COUNT(DISTINCT src_row_id) AS old_src_rows FROM movie_episode_epid WHERE src_row_id IN ?",
			ids).Scan(&before).Error; err != nil {
			return err
		}
		res := tx.Where("src_row_id IN ?", ids).Delete(&entity.MovieEpisodeEpid{})
		if res.Error != nil {
			return res.Error
		}
		if len(rows) == 0 {
			return bumpDeriveStat(tx, -res.RowsAffected, -before.OldSrcRows)
		}
		if err := tx.CreateInBatches(rows, epidInsertChunk).Error; err != nil {
			return err
		}
		// ⚠️ 新增源行数**不能**用 `len(ids)`：源行的 `episodes` 可能为空 ⇒ 它进了 ids 却**不产任何派生行** ✗
		// ⇒ 按 `rows` 里的 distinct src_row_id 数（rows ≤ 5000，内存算即可）。
		return bumpDeriveStat(tx, int64(len(rows))-res.RowsAffected, distinctSrcRows(rows)-before.OldSrcRows)
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
	// 删也要**同步扣减计数器**（000048）：否则孤儿清理后读数会虚高 ✗（同事务，原子）。
	// 派生表写 ⇒ 走 deriveDB（不记 binlog）。
	var deleted int64
	err := dbFrom(ctx, r.deriveDB).Transaction(func(tx *gorm.DB) error {
		var before struct{ OldSrcRows int64 }
		if err := tx.Raw(
			"SELECT COUNT(DISTINCT src_row_id) AS old_src_rows FROM movie_episode_epid WHERE src_row_id IN ?",
			ids).Scan(&before).Error; err != nil {
			return err
		}
		res := tx.Where("src_row_id IN ?", ids).Delete(&entity.MovieEpisodeEpid{})
		if res.Error != nil {
			return res.Error
		}
		deleted = res.RowsAffected
		return bumpDeriveStat(tx, -res.RowsAffected, -before.OldSrcRows)
	})
	return deleted, err
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
//
// **口径排除（2026-11-13）**：`dup_copy = 0` —— 排除「双写副本」（补充源同片按
// `hash(片名)/hash(豆瓣ID)` 双键各存一行，见 `spider.parse.go` 与
// docs/epid-集身份/分析-采集侧键口径与匹配度-2026-11-13.md §10.5/§11）。
// 实测：含副本 35.16% vs 排除副本 **32.83%**（子集同口径）⇒ 不排除会让读数**虚高 ≈2.3pp**
// （豆瓣键副本天然跨多站、更易满足 site≥2）。
// 走 `idx_dup_match_epid_site (dup_copy, match_key, epid, site_id)` ⇒ 仍是 **Using index**（不建临时表）。
func (r *EpidRepo) AlignmentRate(ctx context.Context) (aligned int64, total int64, err error) {
	var row struct {
		Aligned int64
		Total   int64
	}
	err = dbFrom(ctx, r.db).Raw(`
		SELECT
		  COALESCE(SUM(CASE WHEN sites >= 2 THEN 1 ELSE 0 END), 0) AS aligned,
		  COUNT(*) AS total
		FROM (
		  SELECT match_key, epid, COUNT(DISTINCT site_id) AS sites
		  FROM movie_episode_epid
		  WHERE epid <> ''
		    AND dup_copy = 0
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
//
// ⇒ 每批成本与**表规模无关**；且不再需要 `ep_hash <> ”` 过滤（L7：空 hash 也当候选，首次派生一次即收敛）
// 返回三元组：`rows`=**脏行**（已过滤）、`scanLast`=本批 keyset 窗口的**末端 id**、`scanCount`=本批**扫过**的源行数。
//
// ⚠️ **2026-11-11 实测 bug 的修法**：只返回"脏行"是不够的 —— 一段**没有脏行**的区间会返回空，
// 调用方就无法区分"到表尾"与"这段恰好不脏"，只能拿脏行数判表尾 ⇒ 会把游标错误归零、原地空转 ✗✗
// （线上表现：重启后"补缺"每批 `扫=0` 秒结束、派生表不再增长）。
// ⇒ 游标必须按 `scanLast` 推进、表尾必须按 `scanCount < limit` 判定。
func (r *EpidRepo) DirtySourceRows(ctx context.Context, afterId int64, limit int) ([]entity.MoviePlaySource, int64, int, error) {
	return r.dirtySourceRows(ctx, afterId, limit, false)
}

// DirtySourceRowsHot **热门趟**：只取 `mid` 非空的源行（主站行 / 已关联行）—— H6（2026-11-11 评审）。
//
// §5.5 铁律 5 要求"限速 + **热门优先**（`mid` 命中的片优先、长尾延后）"。注意：**只给 `ORDER BY` 加一句
// `mid IS NULL` 是无效的** —— keyset 分页（`id > after` + `LIMIT n`）**先定候选集合**，页内排序不改变
// 集合 ✗。因此拆**两趟**：先热门趟（`mid IS NOT NULL`），跑到底再进长尾趟（全表）；由服务层
// `deriveHotPhase` 切换。收益：长尾占 74.4%（470,587 / 632,539）⇒ 不拆的话**热门片会被排在长尾之后**，
// 而单轮只推进 2000 行 ⇒ 头部内容拿到 epid 的时间会被推后数十倍。
func (r *EpidRepo) DirtySourceRowsHot(ctx context.Context, afterId int64, limit int) ([]entity.MoviePlaySource, int64, int, error) {
	return r.dirtySourceRows(ctx, afterId, limit, true)
}

func (r *EpidRepo) dirtySourceRows(ctx context.Context, afterId int64, limit int, hotOnly bool) ([]entity.MoviePlaySource, int64, int, error) {
	var ids []int64
	q := dbFrom(ctx, r.db).Model(&entity.MoviePlaySource{}).Where("id > ?", afterId)
	if hotOnly {
		q = q.Where("mid IS NOT NULL")
	}
	if err := q.Order("id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return nil, afterId, 0, err
	}
	scanCount := len(ids)
	scanLast := afterId
	if scanCount > 0 {
		scanLast = ids[scanCount-1]
	}
	if scanCount == 0 {
		return nil, scanLast, 0, nil
	}
	anchors, err := r.AnchorsBySrcRowIds(ctx, ids)
	if err != nil {
		return nil, scanLast, scanCount, err
	}
	hashes, err := r.EpHashBy(ctx, ids)
	if err != nil {
		return nil, scanLast, scanCount, err
	}
	dirty := make([]int64, 0, len(ids))
	for _, id := range ids {
		a, ok := anchors[id]
		if !ok || a.SrcEpHash != hashes[id] {
			dirty = append(dirty, id)
		}
	}
	if len(dirty) == 0 {
		return nil, scanLast, scanCount, nil
	}
	var out []entity.MoviePlaySource
	if err := dbFrom(ctx, r.db).Where("id IN ?", dirty).Find(&out).Error; err != nil {
		return nil, scanLast, scanCount, err
	}
	return out, scanLast, scanCount, nil
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

// ===== epid 源头校正层 · T3（2026-11-12）=====
//
// B/C 两点（方案 §8.2）对派生表的**点访问**支撑。红线①：movie_episode_epid **零新增列**，
// B 点只点改既有 epid 一列；红线②：raw_name 等真相列不碰；红线④：全部按 uk_row_ep 前缀
// /主键键查，**绝不全表扫**（按 match_key 分批的要求针对全表聚合，这里都是键点访问）。

// EpidRowPatch B 点（手动快速应用）的单行点改：把 (src_row_id, ep_index) 的 epid 改为目标值。
type EpidRowPatch struct {
	SrcRowId int64
	EpIndex  int
	Epid     string
}

// UpdateEpidByRowKeys B 点用：按 (src_row_id, ep_index) 逐行**点改** epid —— 只改 epid 一列，
// 不碰 raw_name / from_src / src_ep_hash / computed_at（computed_at 语义是"派生时间"，
// B 不是派生；src_ep_hash 保持不变 ⇒ 该行不会被判脏误重算）。
//
// 一个事务打包（读侧要么全见旧要么全见新）；走 uk_row_ep 唯一键点改，无扫描。
// 幂等：重复把 epid 更新为相同值 ⇒ MySQL 不实际改行，结果一致（RowsAffected 会为 0，
// 调用方以自己的计数为准，勿依赖本返回值）。
func (r *EpidRepo) UpdateEpidByRowKeys(ctx context.Context, patches []EpidRowPatch) (int64, error) {
	if len(patches) == 0 {
		return 0, nil
	}
	var n int64
	err := dbFrom(ctx, r.deriveDB).Transaction(func(tx *gorm.DB) error {
		for _, p := range patches {
			res := tx.Model(&entity.MovieEpisodeEpid{}).
				Where("src_row_id = ? AND ep_index = ?", p.SrcRowId, p.EpIndex).
				Update("epid", p.Epid)
			if res.Error != nil {
				return res.Error
			}
			n += res.RowsAffected
		}
		return nil
	})
	return n, err
}

// EpidRowsBySrcRowIds B 点用：按源行批量取派生行的 (ep_index, epid, raw_name) 现值 ——
// 三元组校验 (src_row_id, key_raw_name, key_epid) 的**比对面**。走 uk_row_ep 前缀、只取四列，
// 行数 = 命中源行的集数（有界），不碰巨表。
func (r *EpidRepo) EpidRowsBySrcRowIds(ctx context.Context, srcRowIds []int64) ([]entity.MovieEpisodeEpid, error) {
	if len(srcRowIds) == 0 {
		return nil, nil
	}
	var out []entity.MovieEpisodeEpid
	err := dbFrom(ctx, r.db).Model(&entity.MovieEpisodeEpid{}).
		Select("src_row_id, ep_index, epid, raw_name").
		Where("src_row_id IN ?", srcRowIds).
		Find(&out).Error
	return out, err
}

// SourceRowsByIds C 点（定向重算）用：按主键批量取源行**全字段**（含 episodes_json 巨列），
// 供派生链路重算整源行（与 dirtySourceRows 末步同款取法）。
//
// ⚠️ 只给**显式 id 小批量**（管理端定向重算，个位数源行）调用；主键 IN 键查，无全表扫（红线④）。
func (r *EpidRepo) SourceRowsByIds(ctx context.Context, ids []int64) ([]entity.MoviePlaySource, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []entity.MoviePlaySource
	err := dbFrom(ctx, r.db).Where("id IN ?", ids).Find(&out).Error
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

// ---- `mode=scope`（2026-11-14）：按 `from_src` 档圈「要重算的源行」----
//
// 口径来源 = `danmu.DecisionRuleMetas()` 的 `Scope`/`InFromSrc`（改规则 ⇒ 重算该规则的**输入档**）。
// 三条纪律：
//  ① **刻意不加 `dup_copy=0`** —— 副本行也要拿到新规则（否则永远补不上 ✗）；
//  ② **宁宽勿漏** —— 档级是**安全上界**，多算的行重算后结果相同（只多写一遍）；
//  ③ 读派生表走 `r.db`（`deriveDB` 只用于派生**写**）。

// SourceRowIdsByFromSrc 取「产出于这些档」的全部源行 id（升序、去重、keyset 分页）。
func (r *EpidRepo) SourceRowIdsByFromSrc(ctx context.Context, fromSrc []string) ([]int64, error) {
	if len(fromSrc) == 0 {
		return nil, nil
	}
	const page = 50000
	out := make([]int64, 0, 1024)
	var after int64
	for {
		var chunk []int64
		err := dbFrom(ctx, r.db).Model(&entity.MovieEpisodeEpid{}).
			Where("from_src IN ? AND src_row_id > ?", fromSrc, after).
			Distinct("src_row_id").Order("src_row_id").Limit(page).
			Pluck("src_row_id", &chunk).Error
		if err != nil {
			return nil, err
		}
		if len(chunk) == 0 {
			return out, nil
		}
		out = append(out, chunk...)
		if len(chunk) < page {
			return out, nil
		}
		after = chunk[len(chunk)-1]
	}
}

// SourceRowIdsInMoviesByFromSrc **池扇出**：取「由 `anchorFromSrc` 档的行所属的片（`match_key`）
// 之下、产出于 `expandFromSrc` 档」的源行 id。
//
// 用途 = `FansOutMovie` 规则（`D4`/`D5`/`D6`，以及"让新日期进池"的 R11 类）：
// 池一变，**同片里读池的行**（`bridge`/`anchor`/`none`）的 `n` 可能跟着变 ⇒ 必须一起重算。
// 同片里的 `self`/`single` **不进范围**（读池前就 return ✓，见 §3.1.1 步 3）。
func (r *EpidRepo) SourceRowIdsInMoviesByFromSrc(ctx context.Context, anchorFromSrc, expandFromSrc []string) ([]int64, error) {
	if len(anchorFromSrc) == 0 || len(expandFromSrc) == 0 {
		return nil, nil
	}
	var ids []int64
	err := dbFrom(ctx, r.db).Raw(`
		SELECT DISTINCT e2.src_row_id
		FROM movie_episode_epid e1
		JOIN movie_episode_epid e2 ON e2.match_key = e1.match_key
		WHERE e1.from_src IN ? AND e2.from_src IN ?`, anchorFromSrc, expandFromSrc).Scan(&ids).Error
	return ids, err
}

// SourceRowTitlesByFromSrc 取「候选源行（= 产出于这些档的行）**各自全部集**的集名」——供 `mode=scope`
// 的**选行谓词**（`danmu.RuleSelectsRow`）用。
//
// 两条口径（都实测过，别再凭直觉改）：
//
//	① 必须是"该行的**全部**集名"，**不是**"该行里属于这些档的那几集" ✗ —— 规则守卫看的是**整行**
//	   （`D7` 数"整行只有 1 集"、`D8` 数"行内 distinct 裸数字 ≥2"）；只给子集 ⇒ `D7` 少算集数**误选**、
//	   `D8` 漏掉非 `skip` 集里的裸数字**漏选** ✗。
//	② **不加任何 `dup_copy` 过滤** —— `dup_copy` 是**源行级**标记（双写 = 主份/副本**两行**，派生行继承它），
//	   派生表里**没有**人工重复；过滤掉 `dup_copy=1` 会把 43,026 个**副本源行**踢出紧范围 ⇒ 副本行永远
//	   拿不到新规则 ✗。行内确有重名时（实测仅 1,408 行）那是 `episodes_json` 本身就重名 ⇒ 应当照数 ✓。
func (r *EpidRepo) SourceRowTitlesByFromSrc(ctx context.Context, fromSrc []string) (map[int64][]string, error) {
	out := map[int64][]string{}
	if len(fromSrc) == 0 {
		return out, nil
	}
	var rows []struct {
		SrcRowID int64
		RawName  string
	}
	// 半连接（semi-join）：候选行集合用子查询圈，外层取**这些行的全部集名** —— 无巨列表 `IN`。
	err := dbFrom(ctx, r.db).Raw(`
		SELECT e.src_row_id, e.raw_name
		FROM movie_episode_epid e
		WHERE e.src_row_id IN (SELECT DISTINCT c.src_row_id FROM movie_episode_epid c
		                       WHERE c.from_src IN ?)`, fromSrc).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.SrcRowID] = append(out[x.SrcRowID], x.RawName)
	}
	return out, nil
}

// CoverageFull **完整覆盖率**（§6.1 口径，H7 2026-11-11 评审）：
//
//	派生覆盖率 = 派生表行数 / Σ JSON_LENGTH(episodes_json)  ⇒ 应 ≈100%（**空 epid 也算行**）
//
// ⚠️ **只作离线/低频校验用**：分母要扫 `movie_play_source` 的巨列（实测 632,539 行 / 集数 6,477,048），
// 不是热点路径（性能铁律 1 约束的是"派生与出参"路径，不含这种人工触发的对账）。调用点只放在
// `AlignmentReport` 的**每整轮一次**分支里（见 `EpidService.DeriveEpidInBackground`）。
func (r *EpidRepo) CoverageFull(ctx context.Context) (derivedRows int64, sourceEpisodes int64, err error) {
	var row struct {
		Derived  int64
		SourceEp int64
	}
	err = dbFrom(ctx, r.db).Raw(`
		SELECT
		  (SELECT COUNT(*) FROM movie_episode_epid) AS derived,
		  (SELECT COALESCE(SUM(JSON_LENGTH(episodes_json)), 0) FROM movie_play_source) AS source_ep`).
		Scan(&row).Error
	return row.Derived, row.SourceEp, err
}

// TruncateAll **清空整张派生表**（一键「清空重建」用；H2 运维入口, 2026-11-11）。
//
// ⚠️ **TRUNCATE 不可回滚且隐式提交** —— 只在管理端显式传 `mode=rebuild` 时调用（调用方记日志）。
// 清空期间出参没有 epid ⇒ 端侧自然回退实时解析（方案 §4.6）⇒ **不影响站点可用性** ✓
// （派生表是单向派生物：`episodes_json` 才是真相源，重建结果确定 ⇒ 可随时重算）。
func (r *EpidRepo) TruncateAll(ctx context.Context) error {
	// ⚠️ TRUNCATE **隐式提交**（不可回滚）⇒ 不能用事务包住；计数器重置只能放在其后。
	if err := dbFrom(ctx, r.deriveDB).Exec("TRUNCATE TABLE movie_episode_epid").Error; err != nil {
		return err
	}
	// 清空 ⇒ 计数必然归零（000048）。不重置的话读侧会继续显示清空前的大数 ✗（如 590 万行）。
	// 重置失败不让任务失败（派生正确性不受影响，只是读数偏大）—— 但必须留痕。
	if err := r.resetDeriveStat(ctx); err != nil {
		log.Printf("[epid] 计数器归零失败（读数可能偏大，不影响派生正确性）: %v", err)
	}
	return nil
}

// CountDerivedRows 派生表行数（轻量，给状态/报告用）。
func (r *EpidRepo) CountDerivedRows(ctx context.Context) (int64, error) {
	var n int64
	err := dbFrom(ctx, r.db).Model(&entity.MovieEpisodeEpid{}).Count(&n).Error
	return n, err
}

// DerivedSourceRowCount 已派生过的**源行数**（`COUNT(DISTINCT src_row_id)`，走 `uk_row_ep` 覆盖索引）。
//
// 2026-11-11 实测：196 万派生行上 **763ms** ⇒ 重建期的"进度分母"也**不需要近似值**（精确即可）✓
func (r *EpidRepo) DerivedSourceRowCount(ctx context.Context) (int64, error) {
	var n int64
	err := dbFrom(ctx, r.db).Raw(`SELECT COUNT(DISTINCT src_row_id) FROM movie_episode_epid`).Scan(&n).Error
	return n, err
}

// SourceRowCount 真相源源行总数（进度分母；实测 ≈300ms）。
//
// ⚠️ 不要用 `information_schema.TABLES.TABLE_ROWS`（实测 411,808 vs 真实 633,794，**低估 35%** ✗）。
func (r *EpidRepo) SourceRowCount(ctx context.Context) (int64, error) {
	var n int64
	err := dbFrom(ctx, r.db).Model(&entity.MoviePlaySource{}).Count(&n).Error
	return n, err
}

// ===== 指标口径层（2026-11-11 用户拍板）=====
//
// 三条纪律（来自今天两次口径事故）：
//  1. **"是否多源"必须查真相源 `movie_play_source`** —— 派生表只覆盖一部分，且多源片常**只派生了一侧**
//     ⇒ 拿派生表判"多源"会把它们当成单源，分母被无谓撑大、指标必然虚低 ✗；
//  2. **只统计"成熟片"**：该 `match_key` 的**全部源行都已派生**。否则跑批期间分母里混着"还没跑到"的片，
//     读数不可解释（对齐率会随进度横跳）✗；
//  3. 全库口径（`AlignmentRate`）保留作对照，**不作门槛判定**。

// PidUnresolvedWithMid 合成 sentinel：**有 mid 但归类不出来**（movie/category 查不到）。
// 与 `-1`（无主源：组内零 mid 行）分开统计 —— M2（2026-11-16）把"未关联"拆两档，
// 否则两块语义完全不同的数据会混在一个 `未关联` 里（实测：无主源 135,440 个 mk 混着
// "有 mid 但 pid=0/无 movie" 的 45,137 个）。
const PidUnresolvedWithMid int64 = -2

// PidUnresolvedSource 合成 sentinel：**无主源 mk**（组内全部行 mid 为 NULL，没有对应影片）。
// 与 PidUnresolvedWithMid 区分：-2 是"有 mid 但归不到类"，-3 是"根本没有 mid"。
// 不用 -1 的原因：漏斗快照把全库合计行写成 pid=-1，分类若也用 -1 会在 Load 时被当成合计行吞掉。
const PidUnresolvedSource int64 = -3

// ClassAlignmentStat 按分类的**跨源对齐**（**槽位口径**，2026-11-15 用户定稿；双粒度）：
//   - **槽位 = (match_key, ep_index)**：各源行的"第 N 集"对位到同一槽。
//     槽对齐 ⇔ 槽内非空 epid **只有一种**：
//     槽内 1 行（单源集）⇒ 默认对齐；槽内 ≥2 行同 epid ⇒ 多源全部对齐；
//     槽内 ≥2 种 epid ⇒ **分歧**（不同源对"第 N 集"给出不同身份——P-1/修饰词分裂类）。
//   - **集级对齐率（SlotPct）** = 对齐槽位 / 全部非空槽位。
//   - **片级对齐率（MkPct）** = 集级口径 100% 的片（零分歧片）/ 全部有身份片。
//     单源片全部槽位单行 ⇒ 自动集级 100% ⇒ 自动对齐（无需特判——判定单位是片，用户拍板）。
//
// 旧组级口径（组内 site≥2 的**组占比**）把单源片算成"未对齐"⇒ 51.3% 单源片永久压制比率（33.92%），
// 且 R12 上线反而稀释（反向激励）——已废弃作主指标（组级 `AlignmentRate` 保留仅作对照）。
// ⚠️ 三个已知边界：
//  ① ep_index 跨源**错位**（头部插集/中间缺集）产出伪分歧 —— 容忍口径见 `TolerantAlignmentStat`；
//  ② 分类归属 = `movie.pid`，**pid=0 的"直挂影片"回退 `movie.cid`**（M1，2026-11-16；`cid` 若为
//     二级类目再回退其父类 = R4），解析在 Go 侧 `resolveCategory`（不在 560 万行上挂 join）；
//  ③ "未关联"已拆两档（M2）：`-1` 无主源 ／ `PidUnresolvedWithMid` 有 mid 未归类。
type ClassAlignmentStat struct {
	Pid          int64   `json:"pid"`
	Category     string  `json:"category"`     // category.name（一级分类名）；`pid=-1` → 未关联片（无主源）、`pid=-2` → 未关联片（有 mid 未归类）
	TotalSlots   int64   `json:"totalSlots"`   // 集级分母：非空 (mk, ep_index) 槽位数
	AlignedSlots int64   `json:"alignedSlots"` // 集级分子：对齐槽位（单源槽 + 同 epid 多源槽）
	SlotPct      float64 `json:"slotPct"`      // 集级对齐率
	TotalMks     int64   `json:"totalMks"`     // 片级分母：有身份的片数
	AlignedMks   int64   `json:"alignedMks"`   // 片级分子：集级 100% 的片（零分歧）
	MkPct        float64 `json:"mkPct"`        // 片级对齐率
}

// TolerantAlignmentStat 容忍口径（M4，2026-11-16）的**片级**读数：
// 判定 = `danmu.TolerantMkAligned`（骨架取最长源 + 仅插删 + 主源必备相容）。
// 与严格口径的差别：跨源 `ep_index` 错位（头部多一集 / 中间缺集）在严格口径下产出伪分歧，
// 这里按"多出 / 缺失"容忍 ⇒ 能回答"把错位治好后能收编多少片"。
// ⚠️ 单源片**不背书**（非空源 < 2 ⇒ 判不齐）—— 与严格口径的"单源槽默认对齐"有意不同，
// 所以 `TotalMks` 只统计"≥2 源"的片，两个口径的片级**不可直接相减**。
type TolerantAlignmentStat struct {
	Pid           int64   `json:"pid"`
	Category      string  `json:"category"`
	TotalMks      int64   `json:"totalMks"`      // 分母：≥2 个源的片
	AlignedMks    int64   `json:"alignedMks"`    // 分子：容忍口径判齐的片
	TolerantMkPct float64 `json:"tolerantMkPct"` // 容忍片级
	StrictAligned int64   `json:"strictAligned"` // 同一分母下、严格口径就整齐的片（对照）
}

// ClassAlignmentTolerant 容忍口径（M4）按分类的片级读数。
//
// **按需 / 离线专用**：它是"体检"指标（不是进度指标），且比严格口径多一次按源取序列的开销，
// 所以**不进** `heavyMetricsSnapshot` 的 600s 快照、也**不进**请求路径 ——
// 只由管理端 `GET /manage/epid/alignment/tolerant` 显式触发（用户 2026-11-16 定："算法时间长，离线算即可"）。
//
// 批量策略与 `ClassAlignment` 一致（keyset 圈 10 万 mk/批）；每批按 (mk, src_row_id, ep_index) 取
// 身份序列后在 Go 里判 —— 不依赖 SQL 侧的排序（避免在 560 万行上落盘 filesort）。
func (r *EpidRepo) ClassAlignmentTolerant(ctx context.Context) ([]TolerantAlignmentStat, TolerantAlignmentStat, error) {
	catPID := map[int64]int64{}
	catName := map[int64]string{}
	{
		var rows []struct {
			Id   int64
			Pid  int64
			Name string
		}
		if err := dbFrom(ctx, r.db).Raw("SELECT id, pid, name FROM category").Scan(&rows).Error; err != nil {
			return nil, TolerantAlignmentStat{}, err
		}
		for _, c := range rows {
			catPID[c.Id], catName[c.Id] = c.Pid, c.Name
		}
	}
	resolveCategory := func(pid, cid int64) int64 {
		out := pid
		if out == 0 {
			out = cid
		}
		if p, ok := catPID[out]; ok && p > 0 {
			out = p
		}
		return out
	}

	type mkAcc struct{ total, aligned, strictOK int64 }
	accs := map[int64]*mkAcc{}
	var all TolerantAlignmentStat
	const page = 100000
	after := int64(math.MinInt64)
	for i := 0; i < 32; i++ {
		var next int64
		if err := dbFrom(ctx, r.db).Raw(
			"SELECT COALESCE(MAX(mk),0) FROM (SELECT match_key AS mk FROM movie_play_source WHERE match_key > ? ORDER BY match_key LIMIT ?) t",
			after, page).Scan(&next).Error; err != nil {
			return nil, all, err
		}
		if next == 0 {
			break
		}
		var rows []struct {
			Mk     string
			RowID  int64
			EpIdx  int
			Epid   string
			Pid    int64
			Cid    int64
			HasMid int64
		}
		err := dbFrom(ctx, r.db).Raw(`
			SELECT e.match_key AS mk, e.src_row_id AS row_id, e.ep_index AS ep_idx, e.epid,
			       MAX(m.pid) AS pid, MAX(m.cid) AS cid, MAX(e.mid IS NOT NULL) AS has_mid
			FROM movie_episode_epid e
			LEFT JOIN movie_play_source src ON src.id = e.src_row_id
			LEFT JOIN movie m ON m.mid = src.mid
			WHERE e.epid <> '' AND e.dup_copy = 0 AND e.match_key > ? AND e.match_key <= ?
			GROUP BY e.match_key, e.src_row_id, e.ep_index`, after, next).Scan(&rows).Error
		if err != nil {
			return nil, all, err
		}
		// 组装：mk → (src_row_id → {ep_index: epid})
		type mkSeq struct {
			pid, cid, hasMid int64
			bySrc            map[int64]map[int]string
		}
		perMK := make(map[string]*mkSeq, len(rows)/4+1)
		for j := range rows {
			rr := rows[j]
			m := perMK[rr.Mk]
			if m == nil {
				m = &mkSeq{bySrc: map[int64]map[int]string{}}
				perMK[rr.Mk] = m
			}
			if rr.Pid != 0 {
				m.pid = rr.Pid
			}
			if rr.Cid != 0 {
				m.cid = rr.Cid
			}
			if rr.HasMid == 1 {
				m.hasMid = 1
			}
			sm := m.bySrc[rr.RowID]
			if sm == nil {
				sm = map[int]string{}
				m.bySrc[rr.RowID] = sm
			}
			sm[rr.EpIdx] = rr.Epid
		}
		for _, m := range perMK {
			seqs := make([][]string, 0, len(m.bySrc))
			for _, sm := range m.bySrc {
				idx := make([]int, 0, len(sm))
				for k := range sm {
					idx = append(idx, k)
				}
				sort.Ints(idx)
				seq := make([]string, 0, len(idx))
				for _, k := range idx {
					seq = append(seq, sm[k])
				}
				seqs = append(seqs, seq)
			}
			ok, _, _ := danmu.TolerantMkAligned(seqs, -1)
			cat := resolveCategory(m.pid, m.cid)
			if cat <= 0 {
				if m.hasMid == 1 {
					cat = PidUnresolvedWithMid
				} else {
					cat = PidUnresolvedSource
				}
			}
			a := accs[cat]
			if a == nil {
				a = &mkAcc{}
				accs[cat] = a
			}
			a.total++
			if ok {
				a.aligned++
			}
			if strictMkAligned(m.bySrc) {
				a.strictOK++
			}
		}
		after = next
	}

	out := make([]TolerantAlignmentStat, 0, len(accs))
	for pid, a := range accs {
		name := "(未知分类)"
		switch {
		case pid == PidUnresolvedSource, pid == -1:
			name = "未关联片（无主源）"
		case pid == PidUnresolvedWithMid:
			name = "未关联片（有 mid 未归类）"
		case pid == 0:
			name = "(未分类)"
		default:
			if n, ok := catName[pid]; ok && n != "" {
				name = n
			}
		}
		st := TolerantAlignmentStat{Pid: pid, Category: name, TotalMks: a.total, AlignedMks: a.aligned, StrictAligned: a.strictOK}
		if a.total > 0 {
			st.TolerantMkPct = float64(a.aligned) * 100 / float64(a.total)
		}
		out = append(out, st)
		all.TotalMks += a.total
		all.AlignedMks += a.aligned
		all.StrictAligned += a.strictOK
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalMks > out[j].TotalMks })
	all.Pid, all.Category = -1, "全部"
	if all.TotalMks > 0 {
		all.TolerantMkPct = float64(all.AlignedMks) * 100 / float64(all.TotalMks)
	}
	return out, all, nil
}

// ===== 综艺「日期对齐」口径（S 口径，2026-11-17）=====
//
// 与 M4 容忍口径同构：都是**按需/离线**的体检口径 —— 不进 `heavyMetricsSnapshot` 的 600s 快照、
// 不进请求路径，只由管理端 `GET /manage/epid/alignment/date` 显式触发。
// 差别只在**身份取什么**：M4 吃 epid 值序列（位置对齐），本口径吃 `(日期, part)` 复合键（值对齐）。
//
// 背景与实测（2026-11-17，生产库只读）：
//   - 综艺占全库「首段差异 = d」分歧槽 75.9%（19,936 / 26,256）；
//   - 综艺 47.1% 的行是「纯日期型」（s/v/n 空、d 非空）⇒ 这些集现在没有可用身份。
//
// 零迁移：**只读派生表现有 `epid` 列**，不新增列、不改写入、不重建派生表。
type DateAlignmentStat struct {
	Pid           int64   `json:"pid"`
	Category      string  `json:"category"`
	TotalMks      int64   `json:"totalMks"`      // 分母：≥2 个「纯日期型源」且通过守卫的片
	AlignedMks    int64   `json:"alignedMks"`    // 分子：日期(+part) 值语义判齐的片
	DateMkPct     float64 `json:"dateMkPct"`     // 片级日期对齐率
	StrictAligned int64   `json:"strictAligned"` // 对照：**同一分母、同一源集合（纯日期型源）**下严格口径就整齐的片
	SkippedMks    int64   `json:"skippedMks"`    // 因「同源同日同 part 重复」放弃判定的片
}

// ClassAlignmentDate 日期对齐口径的按分类片级读数。
//
// 口径要点（缺一不可）：
//   - 只认**纯日期型源**：该源全部行都满足 `s/v/n` 空、`d` 非空（混用期号/栏目的源不参与 —— 宁缺勿错）；
//   - 需 **≥2 个**这样的源（单源不背书，与 M4 一致）；
//   - 守卫：每源 `(日期, part)` 复合键唯一；**同日多行但 part 不同属合法**（一天两集，P2 把当日集序落 `p` 段）。
//
// `StrictAligned` 对照值也**只在该 mk 的纯日期型源子集内**计算（审查 C3）⇒ 与分子/分母同域，
// 差异才可归因为"日期口径本身多收编了多少"，而不是"对照多看了几个源"。
//
// 判定本体在 `danmu.DateAlignVerdictItems`（值语义子集相容、两两判定、不做无 part 通配）。
func (r *EpidRepo) ClassAlignmentDate(ctx context.Context) ([]DateAlignmentStat, DateAlignmentStat, error) {
	catPID := map[int64]int64{}
	catName := map[int64]string{}
	{
		var rows []struct {
			Id   int64
			Pid  int64
			Name string
		}
		if err := dbFrom(ctx, r.db).Raw("SELECT id, pid, name FROM category").Scan(&rows).Error; err != nil {
			return nil, DateAlignmentStat{}, err
		}
		for _, c := range rows {
			catPID[c.Id], catName[c.Id] = c.Pid, c.Name
		}
	}
	resolveCategory := func(pid, cid int64) int64 {
		out := pid
		if out == 0 {
			out = cid
		}
		if p, ok := catPID[out]; ok && p > 0 {
			out = p
		}
		return out
	}

	// ⚠️ 每个 mk 只维护**一张** per-源映射（审查 2026-11-17）：原先 `bySrc` + `srcs` 两张 map
	// 会让批内内存翻倍（100k mk × ~10 源 × 2 张 map），小机器上属真实放大风险。
	type srcInfo struct {
		eps     map[int]string // ep_index → epid（严格口径对照用）
		allPure bool           // 该源**全部行**都是纯日期型
		items   []danmu.DateAlignItem
	}
	type mkSeq struct {
		pid, cid, hasMid int64
		bySrc            map[int64]*srcInfo
	}

	type catAcc struct{ total, aligned, strictOK, skipped int64 }
	accs := map[int64]*catAcc{}
	var all DateAlignmentStat
	const page = 100000
	after := int64(math.MinInt64)
	for i := 0; i < 32; i++ {
		var next int64
		if err := dbFrom(ctx, r.db).Raw(
			"SELECT COALESCE(MAX(mk),0) FROM (SELECT match_key AS mk FROM movie_play_source WHERE match_key > ? ORDER BY match_key LIMIT ?) t",
			after, page).Scan(&next).Error; err != nil {
			return nil, all, err
		}
		if next == 0 {
			break
		}
		var rows []struct {
			Mk     string
			RowID  int64
			EpIdx  int
			Epid   string
			Pid    int64
			Cid    int64
			HasMid int64
		}
		err := dbFrom(ctx, r.db).Raw(`
			SELECT e.match_key AS mk, e.src_row_id AS row_id, e.ep_index AS ep_idx, e.epid,
			       MAX(m.pid) AS pid, MAX(m.cid) AS cid, MAX(e.mid IS NOT NULL) AS has_mid
			FROM movie_episode_epid e
			LEFT JOIN movie_play_source src ON src.id = e.src_row_id
			LEFT JOIN movie m ON m.mid = src.mid
			WHERE e.epid <> '' AND e.dup_copy = 0 AND e.match_key > ? AND e.match_key <= ?
			GROUP BY e.match_key, e.src_row_id, e.ep_index`, after, next).Scan(&rows).Error
		if err != nil {
			return nil, all, err
		}
		perMK := make(map[string]*mkSeq, len(rows)/4+1)
		for j := range rows {
			rr := rows[j]
			m := perMK[rr.Mk]
			if m == nil {
				m = &mkSeq{bySrc: map[int64]*srcInfo{}}
				perMK[rr.Mk] = m
			}
			if rr.Pid != 0 {
				m.pid = rr.Pid
			}
			if rr.Cid != 0 {
				m.cid = rr.Cid
			}
			if rr.HasMid == 1 {
				m.hasMid = 1
			}
			si := m.bySrc[rr.RowID]
			if si == nil {
				si = &srcInfo{eps: map[int]string{}, allPure: true}
				m.bySrc[rr.RowID] = si
			}
			si.eps[rr.EpIdx] = rr.Epid

			k, ok := danmu.ParseEpidString(rr.Epid)
			// 纯日期型：s/v/n 全空、d 非空（v 段的 `#序号` 也算 v 非空）
			pure := ok && k.Date != "" && k.Num == 0 && k.ENum == 0 &&
				k.Series == "" && k.Variant == "" && k.VariantNum == 0
			if !pure {
				si.allPure = false
				continue
			}
			si.items = append(si.items, danmu.DateAlignItem{Date: k.Date, Part: k.Part})
		}
		for _, m := range perMK {
			// 只把**纯日期型源**喂给两个口径 ⇒ 分子/分母与对照**同域**（审查 C3，2026-11-17）：
			// 若对照仍按"该 mk 全部源"算，混用片里那个带期号/栏目的源会把它判成不齐，
			// 于是差异既含"日期口径多收编"也含"对照看的源更多"⇒ 不可归因。
			var perSrc [][]danmu.DateAlignItem
			inner := make([]map[int]string, 0, len(m.bySrc))
			for _, si := range m.bySrc {
				if !si.allPure || len(si.items) == 0 {
					continue
				}
				perSrc = append(perSrc, si.items)
				inner = append(inner, si.eps)
			}
			if len(perSrc) < 2 {
				continue // 单源不背书 / 非纯日期片：不进分母
			}
			cat := resolveCategory(m.pid, m.cid)
			if cat <= 0 {
				if m.hasMid == 1 {
					cat = PidUnresolvedWithMid
				} else {
					cat = PidUnresolvedSource
				}
			}
			a := accs[cat]
			if a == nil {
				a = &catAcc{}
				accs[cat] = a
			}
			if !danmu.DateAlignUsableItems(perSrc) {
				a.skipped++ // 守卫否决（同源同日同 part 重复）
				continue
			}
			a.total++
			if danmu.DateAlignVerdictItems(perSrc) {
				a.aligned++
			}
			if strictMkAlignedFrom(inner) {
				a.strictOK++
			}
		}
		after = next
	}

	out := make([]DateAlignmentStat, 0, len(accs))
	for pid, a := range accs {
		name := categoryDisplayName(pid, catName)
		st := DateAlignmentStat{
			Pid: pid, Category: name,
			TotalMks: a.total, AlignedMks: a.aligned,
			StrictAligned: a.strictOK, SkippedMks: a.skipped,
		}
		if a.total > 0 {
			st.DateMkPct = float64(a.aligned) * 100 / float64(a.total)
		}
		out = append(out, st)
		all.TotalMks += a.total
		all.AlignedMks += a.aligned
		all.StrictAligned += a.strictOK
		all.SkippedMks += a.skipped
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalMks > out[j].TotalMks })
	all.Pid, all.Category = -1, "全部"
	if all.TotalMks > 0 {
		all.DateMkPct = float64(all.AlignedMks) * 100 / float64(all.TotalMks)
	}
	return out, all, nil
}

// categoryDisplayName 分类显示名（-1 / 未归类 sentinel / 空名统一兜底）。
func categoryDisplayName(pid int64, names map[int64]string) string {
	switch {
	case pid == PidUnresolvedSource, pid == -1:
		return "未关联片（无主源）"
	case pid == PidUnresolvedWithMid:
		return "未关联片（有 mid 未归类）"
	case pid == 0:
		return "(未分类)"
	}
	if n, ok := names[pid]; ok && n != "" {
		return n
	}
	return "(未知分类)"
}

// strictMkAligned 严格口径的"片级"判定（与 `ClassAlignment` 同义）：
// 该片**每个 ep_index 槽位**跨源 epid 只有一种 ⇒ 零分歧片。分母口径由调用方统一为"≥2 源"。
func strictMkAligned(bySrc map[int64]map[int]string) bool {
	inner := make([]map[int]string, 0, len(bySrc))
	for _, sm := range bySrc {
		inner = append(inner, sm)
	}
	return strictMkAlignedFrom(inner)
}

// strictMkAlignedFrom 同上，但直接吃"每源一份 `ep_index → epid`"的切片 ——
// 供只维护单张 per-mk 映射的调用方复用（日期对齐口径，2026-11-17），避免为对照再造一张 map。
func strictMkAlignedFrom(inner []map[int]string) bool {
	seen := map[int]map[string]bool{}
	for _, sm := range inner {
		for idx, epid := range sm {
			if seen[idx] == nil {
				seen[idx] = map[string]bool{}
			}
			seen[idx][epid] = true
		}
	}
	for _, set := range seen {
		if len(set) > 1 {
			return false
		}
	}
	return len(seen) > 0
}

// ClassAlignment 槽位口径双粒度按分类聚合。**mk 键集分批**（Errno 28 纪律：全表 GROUP BY 表达式
// 有打挂小机器的前科），批内两层（slots = (mk,ep_index) 槽级 → mkstat = 片级对齐计数），
// 批间 Go 归并；负数 mk（uint32 hash 存进有符号列）从 MinInt64 起圈、一并覆盖。
// category 名最后批量查出补齐。
func (r *EpidRepo) ClassAlignment(ctx context.Context) ([]ClassAlignmentStat, ClassAlignmentStat, error) {
	type acc struct {
		alignedSlots int64
		totalSlots   int64
		alignedMks   int64
		totalMks     int64
	}
	accs := map[int64]*acc{}
	var all ClassAlignmentStat

	// 分类表（44 行）一次读全：① 名字 ② id→pid（**Go 侧解析归属**，不在 5.6M 行上多挂一个 join ——
	// R4，2026-11-16：`pid=0` 的"直挂影片"回退 `cid`；若 `cid` 本身是**二级**类目则再回退到它的父类）。
	catPID := map[int64]int64{}
	catName := map[int64]string{}
	{
		var rows []struct {
			Id   int64
			Pid  int64
			Name string
		}
		if err := dbFrom(ctx, r.db).Raw("SELECT id, pid, name FROM category").Scan(&rows).Error; err != nil {
			return nil, all, err
		}
		for _, c := range rows {
			catPID[c.Id] = c.Pid
			catName[c.Id] = c.Name
		}
	}
	// resolveCategory 归属解析（M1 + R4）：0 ⇒ 回退 `cid`；仍是二级类目 ⇒ 再回退父类。
	resolveCategory := func(pid, cid int64) int64 {
		out := pid
		if out == 0 {
			out = cid
		}
		if p, ok := catPID[out]; ok && p > 0 {
			out = p
		}
		return out
	}

	const page = 100000
	after := int64(math.MinInt64)
	for i := 0; i < 32; i++ {
		var next int64
		if err := dbFrom(ctx, r.db).Raw(
			"SELECT COALESCE(MAX(mk),0) FROM (SELECT match_key AS mk FROM movie_play_source WHERE match_key > ? ORDER BY match_key LIMIT ?) t",
			after, page).Scan(&next).Error; err != nil {
			return nil, all, err
		}
		if next == 0 {
			break
		}
		var rows []struct {
			Pid          int64
			Cid          int64
			HasMid       int64
			AlignedSlots int64
			TotalSlots   int64
			AlignedMks   int64
			TotalMks     int64
		}
		err := dbFrom(ctx, r.db).Raw(`
			SELECT IFNULL(MAX(mkstat.pid), -3) AS pid,
			       IFNULL(MAX(mkstat.cid), 0) AS cid,
			       IFNULL(MAX(mkstat.has_mid), 0) AS has_mid,
			       SUM(mkstat.aligned_slots) AS aligned_slots,
			       SUM(mkstat.total_slots) AS total_slots,
			       SUM(mkstat.aligned_slots = mkstat.total_slots) AS aligned_mks,
			       COUNT(*) AS total_mks
			FROM (
			  SELECT slots.mk, MAX(slots.pid) AS pid, MAX(slots.cid) AS cid, MAX(slots.has_mid) AS has_mid,
			         SUM(slots.kinds = 1) AS aligned_slots, COUNT(*) AS total_slots
			  FROM (
			    SELECT e.match_key AS mk, e.ep_index,
			           -- M1(2026-11-16): 分类归属 = movie.pid，pid=0 的"直挂影片"回退 movie.cid。
			           -- 依据: spider/parse.go 直接取上游 type_id_1（无本地映射表）；上游 type_id_1=0 的
			           -- 写法里直挂影片只有 cid=顶级类目id（实测 45,830 部全挂在 电影解说/AI漫剧/短剧
			           -- 三个顶级类目上）。查询侧早已用 (pid=? OR cid=?) 兼容（search_repo.go）；
			           -- 指标侧此前只按 pid ⇒ 这 4.5 万部被错算进"未关联"，与"真·无主源"混装。
			           -- ⚠️ pid/cid 只**取出**、解析在 Go 侧（resolveCategory）——避免在 560 万行上再挂 join。
			           MAX(m.pid) AS pid, MAX(m.cid) AS cid,
			           MAX(e.mid IS NOT NULL) AS has_mid,
			           COUNT(DISTINCT e.epid) AS kinds
			    FROM movie_episode_epid e
			    LEFT JOIN movie_play_source src ON src.id = e.src_row_id
			    LEFT JOIN movie m ON m.mid = src.mid
			    WHERE e.epid <> '' AND e.dup_copy = 0 AND e.match_key > ? AND e.match_key <= ?
			    GROUP BY e.match_key, e.ep_index
			  ) slots
			  GROUP BY slots.mk
			) mkstat
			GROUP BY mkstat.pid, mkstat.cid, mkstat.has_mid`, after, next).Scan(&rows).Error
		if err != nil {
			return nil, all, err
		}
		for i := range rows {
			// M1 + R4：Go 侧解析归属（0 ⇒ 回退 cid；cid 是二级类目 ⇒ 再回退父类）。
			pid := resolveCategory(rows[i].Pid, rows[i].Cid)
			// M2（2026-11-16）：把"未关联"拆两档 —— ① 无主源（组内零 mid 行）② 有 mid 但归类不出来。
			if pid <= 0 {
				if rows[i].HasMid == 1 {
					pid = PidUnresolvedWithMid
				} else {
					pid = PidUnresolvedSource
				}
			}
			a := accs[pid]
			if a == nil {
				a = &acc{}
				accs[pid] = a
			}
			a.alignedSlots += rows[i].AlignedSlots
			a.totalSlots += rows[i].TotalSlots
			a.alignedMks += rows[i].AlignedMks
			a.totalMks += rows[i].TotalMks
			all.AlignedSlots += rows[i].AlignedSlots
			all.TotalSlots += rows[i].TotalSlots
			all.AlignedMks += rows[i].AlignedMks
			all.TotalMks += rows[i].TotalMks
		}
		after = next
	}
	// 分类名：本函数入口已把 category 全表读进 `catName`（44 行），此处不再二次查询。
	out := make([]ClassAlignmentStat, 0, len(accs))
	for pid, a := range accs {
		name := "(未知分类)"
		switch {
		case pid == PidUnresolvedSource, pid == -1:
			name = "未关联片（无主源）"
		case pid == PidUnresolvedWithMid:
			name = "未关联片（有 mid 未归类）"
		case pid == 0:
			name = "(未分类)"
		default:
			if n, ok := catName[pid]; ok && n != "" {
				name = n
			}
		}
		st := ClassAlignmentStat{
			Pid: pid, Category: name,
			AlignedSlots: a.alignedSlots, TotalSlots: a.totalSlots,
			AlignedMks: a.alignedMks, TotalMks: a.totalMks,
		}
		if a.totalSlots > 0 {
			st.SlotPct = float64(a.alignedSlots) * 100 / float64(a.totalSlots)
		}
		if a.totalMks > 0 {
			st.MkPct = float64(a.alignedMks) * 100 / float64(a.totalMks)
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalSlots > out[j].TotalSlots })
	if all.TotalSlots > 0 {
		all.SlotPct = float64(all.AlignedSlots) * 100 / float64(all.TotalSlots)
	}
	if all.TotalMks > 0 {
		all.MkPct = float64(all.AlignedMks) * 100 / float64(all.TotalMks)
	}
	all.Category = "全部"
	return out, all, nil
}

// ShapeStat 空 epid 的**集名形态**分布 —— 回答"为什么空"。
type ShapeStat struct {
	Shape  string `json:"shape"`
	RowCnt int64  `json:"rowCnt"`
}

// EmptyEpidByShape 空 epid 按集名形态分档（**未触发的近似口径**）：
//
//	has_date  含 8 位日期 ⇒ 理论上应触发（若为空 ⇒ 属"触发但推不出期号"，即 §4.7 L6 的空串豁免）
//	e_num     E/EP 圆号 ⇒ 按口径**不触发**（epid 串不含 e 段）
//	ordinal   第N集/期/话… ⇒ 2026-11-11 放宽后**应触发**（rebuild 前仍为空 ⇒ 说明这批还没重算）
//	other     其它（含片名/无规律）
//
// ⚠️ 局限：`from_src` 目前把"未触发"与"触发但推不出"都记成 `none`，无法直接拆开
// ⇒ 这里用形态近似（后续可在写入侧细分来源值再精确化）。
func (r *EpidRepo) EmptyEpidByShape(ctx context.Context) ([]ShapeStat, error) {
	var out []ShapeStat
	err := dbFrom(ctx, r.db).Raw(`
		SELECT CASE
		    WHEN raw_name REGEXP '[0-9]{8}' THEN 'has_date'
		    WHEN raw_name REGEXP '^[Ee][Pp]?[0-9]+' THEN 'e_num'
		    WHEN raw_name REGEXP '第[0-9]+[集期话回季部]' THEN 'ordinal'
		    ELSE 'other' END AS shape,
		  COUNT(*) AS row_cnt
		FROM movie_episode_epid
		WHERE epid = ''
		GROUP BY shape
		ORDER BY row_cnt DESC`).Scan(&out).Error
	return out, err
}

// ===== 进度读数层（2026-11-11 线上"卡片读数全是 0"事故的修法）=====
//
// 事故：派生表涨到 **590 万行**后，短路期（任务进行中）用的三个"便宜"计数全部撞穿预算：
//
//	`COUNT(*) FROM movie_episode_epid`              → **19.2s**（预算 3s）✗
//	`COUNT(DISTINCT src_row_id) FROM movie_episode_epid` → **8.6s**（预算 5s）✗
//	`COUNT(*) FROM movie_play_source`               → **5.3s**（表里挂着 1.1GB 的 episodes_json 巨列）
//
// ⇒ 被"每项独立超时 + 全项容错"逐个降级成 0 ⇒ 管理员看到 `0 / 0`，误以为数据丢了 ✗。
// 修法分三条（本文件给前两条的实现）：
//  1. **派生行数 / 已派生源行 → 写侧计数器**（单行表 `epid_derive_stat`，读侧主键点查，O(1)）；
//  2. **影片完整度**（`MovieCompleteness`）—— 新口径，代价 10~14s ⇒ 只走**后台缓存刷新**；
//  3. 源行总数 → 进程内缓存（见 service 层）。

// EpidDeriveStat 派生进度计数器读数（迁移 000048 的单行表）。
type EpidDeriveStat struct {
	DerivedRows    int64 `json:"derivedRows"`
	DerivedSrcRows int64 `json:"derivedSrcRows"`
	UpdatedAt      int64 `json:"updatedAt"`
}

// DeriveStatFast 读计数器：**一次主键点查**，成本与表规模无关（590 万行实测 <1ms）。
//
// ⚠️ `UpdatedAt == 0` 表示**尚未初始化**（迁移没跑 / 表被手工清过）⇒ 调用方必须退回精确计数，
// 不能把 0 当成"真的 0 行"。
func (r *EpidRepo) DeriveStatFast(ctx context.Context) (EpidDeriveStat, error) {
	var out EpidDeriveStat
	err := dbFrom(ctx, r.db).Raw(
		"SELECT derived_rows, derived_src_rows, updated_at FROM epid_derive_stat WHERE id = 1").Scan(&out).Error
	return out, err
}

// bumpDeriveStat 在**写事务内**增减计数（原子；`GREATEST(...,0)` 兜底防负漂）。
func bumpDeriveStat(tx *gorm.DB, dRows, dSrcRows int64) error {
	if dRows == 0 && dSrcRows == 0 {
		return nil
	}
	// INSERT IGNORE 兜底：计数器行缺失（迁移未跑）时也不让写路径失败。
	if err := tx.Exec(
		"INSERT IGNORE INTO epid_derive_stat (id, derived_rows, derived_src_rows, updated_at) VALUES (1, 0, 0, ?)",
		time.Now().UnixMilli()).Error; err != nil {
		return err
	}
	return tx.Exec(`UPDATE epid_derive_stat
	                SET derived_rows     = GREATEST(derived_rows + ?, 0),
	                    derived_src_rows = GREATEST(derived_src_rows + ?, 0),
	                    updated_at       = ?
	                WHERE id = 1`, dRows, dSrcRows, time.Now().UnixMilli()).Error
}

// resetDeriveStat 计数器归零（清空重建后调用）。
func (r *EpidRepo) resetDeriveStat(ctx context.Context) error {
	now := time.Now().UnixMilli()
	return dbFrom(ctx, r.db).Exec(`INSERT INTO epid_derive_stat (id, derived_rows, derived_src_rows, updated_at)
	                               VALUES (1, 0, 0, ?)
	                               ON DUPLICATE KEY UPDATE derived_rows = 0, derived_src_rows = 0, updated_at = ?`,
		now, now).Error
}

// distinctSrcRows `rows` 里出现过的不同源行数（内存算，rows ≤ 5000）。
func distinctSrcRows(rows []entity.MovieEpisodeEpid) int64 {
	seen := make(map[int64]struct{}, len(rows))
	for i := range rows {
		seen[rows[i].SrcRowId] = struct{}{}
	}
	return int64(len(seen))
}

// MovieCompletenessStat 影片级完整度。
type MovieCompletenessStat struct {
	Movies         int64 `json:"movies"`         // 影片总数（按真相源 match_key 分组）
	CompleteMovies int64 `json:"completeMovies"` // 其**全部源行都已派生**的影片数
	MissingSrcRows int64 `json:"missingSrcRows"` // 不完整影片里一共还缺多少个源行 ⇒ 直接回答"还缺多少"
}

// MovieCompleteness 影片完整度（**进度主口径**，2026-11-11 用户拍板取代"覆盖率当进度条"）。
//
// 为什么用它而不是 §6.1 覆盖率当进度：
//   - **完全不碰 `episodes_json` 巨列**（覆盖率要 `SUM(JSON_LENGTH(...))`，随集数膨胀，实测 13s+）；
//   - 它与 §4.8 门槛"可比口径"里的**成熟判据是同一个定义**（该片全部源行都已派生）
//     ⇒ 看它就能**预判门槛能不能过**，而不只是"跑了多少"；
//   - 天然回答"还缺多少"：剩余影片数 + 剩余源行数。
//
// 代价（2026-11-11 真库实测，590 万派生行）：**10.1s / 13.1s** ⇒ 只允许在**后台缓存刷新**里调用。
//
// 实现取舍（同数据 A/B 实测）：
//
//	V1（本实现）`LEFT JOIN ... ON d.src_row_id = s.id AND d.ep_index = 0`  → **10.1s / 13.1s** ✓
//	V2（双侧聚合再比对 `COUNT(DISTINCT src_row_id)`）                      → **142s / 73s** ✗
//
// V2 慢是因为派生侧 `GROUP BY match_key` 要回表取 `src_row_id`（500 万次回表）；而 V1 的
// `(src_row_id, ep_index=0)` 是 `uk_row_ep` 的**唯一键点查** ⇒ 既快又不会"一源多集"扇出。
//
// 前置条件（均已线上实测，不满足时本查询结果不可信）：
//   - `movie_play_source.match_key` 非空（空值 0 行 ⇒ `GROUP BY match_key` 即"影片"）；
//   - 每个源行的派生行都从 `ep_index = 0` 起编号（"MIN(ep_index) <> 0 的源行数" = 0）—— 写侧
//     `DeriveBatch` 以 `EpIndex: i` 从 0 递增，结构上保证。
func (r *EpidRepo) MovieCompleteness(ctx context.Context) (MovieCompletenessStat, error) {
	var out MovieCompletenessStat
	err := dbFrom(ctx, r.db).Raw(`
		SELECT COUNT(*)                     AS movies,
		       COALESCE(SUM(c_d = c_s), 0)  AS complete_movies,
		       COALESCE(SUM(c_s - c_d), 0)  AS missing_src_rows
		FROM (
		  SELECT s.match_key AS mk, COUNT(*) AS c_s, COUNT(d.src_row_id) AS c_d
		  FROM movie_play_source s
		  LEFT JOIN movie_episode_epid d ON d.src_row_id = s.id AND d.ep_index = 0
		  GROUP BY s.match_key
		) t`).Scan(&out).Error
	return out, err
}

// AnalyzeDerived 重建收尾刷新派生表的优化器统计信息。
// 大批量 TRUNCATE + INSERT 之后统计会明显失真 ⇒ 计划可能退化（尤其按 match_key/epid 分组的指标查询）。
// 调用方给**独立上下文**（任务本身可能已到超时）。
func (r *EpidRepo) AnalyzeDerived(ctx context.Context) error {
	return dbFrom(ctx, r.db).Exec("ANALYZE TABLE movie_episode_epid").Error
}

// RecountDeriveStat 计数器**自愈**：用一次精确计数把计数器覆盖成真值。
//
// ⚠️ **慢**（590 万行实测 8.6s + 19.2s ⇒ 合计约 20~30s）⇒ 只能在**后台**或迁移里调用，
// 绝不进请求路径（那正是本次事故的根因）。触发条件：计数器 `updated_at = 0`（迁移没跑 / 被手工清过）。
//
// 与写侧增量的相容性：写侧是"事务内增减差额" ⇒ 本函数给出的绝对值 + 之后的所有差额 = 一致 ✓。
// （计数与写并发时理论上会差几行，下一轮 rebuild 或下一次自愈即可收敛，不影响派生正确性。）
func (r *EpidRepo) RecountDeriveStat(ctx context.Context) (EpidDeriveStat, error) {
	var out EpidDeriveStat
	if err := dbFrom(ctx, r.db).Raw(`
		SELECT COUNT(*) AS derived_rows, COUNT(DISTINCT src_row_id) AS derived_src_rows
		FROM movie_episode_epid`).Scan(&out).Error; err != nil {
		return out, err
	}
	out.UpdatedAt = time.Now().UnixMilli()
	if err := r.SetDeriveStat(ctx, out.DerivedRows, out.DerivedSrcRows); err != nil {
		return out, err
	}
	return out, nil
}

// SetDeriveStat 直接把计数器写成给定值（自愈 / 基线校准用）。
func (r *EpidRepo) SetDeriveStat(ctx context.Context, derivedRows, derivedSrcRows int64) error {
	now := time.Now().UnixMilli()
	return dbFrom(ctx, r.db).Exec(`INSERT INTO epid_derive_stat (id, derived_rows, derived_src_rows, updated_at)
	                               VALUES (1, ?, ?, ?)
	                               ON DUPLICATE KEY UPDATE derived_rows = ?, derived_src_rows = ?, updated_at = ?`,
		derivedRows, derivedSrcRows, now, derivedRows, derivedSrcRows, now).Error
}

// MovieNamesByMids 批量取影片名（派生层"S 以片名为前缀则去前缀"判定用，2026-11-19）。
func (r *EpidRepo) MovieNamesByMids(ctx context.Context, mids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(mids))
	if len(mids) == 0 {
		return out, nil
	}
	var rows []struct {
		Mid  int64
		Name string
	}
	if err := r.db.WithContext(ctx).Table("movie").Select("mid, name").Where("mid IN ?", mids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.Mid] = x.Name
	}
	return out, nil
}

package mysql

import (
	"context"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

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

// AlignmentRateComparable 可比口径对齐率：`match_key` 在**真相源**里有 ≥2 个 site **且该片已成熟**；
// 仅统计非空 epid 组。§4.8 门槛（≥85%）以本口径为准。
//
// ⚠️ **必须显式 `COLLATE`**（2026-11-11 审查实测）：迁移 `000044` 建派生表时未指定 collation ⇒ 用的是
// 服务器默认 `utf8mb4_0900_ai_ci`，而 `movie_play_source` 的列是 `utf8mb4_unicode_ci`
// ⇒ **跨表列比较直接报 `Illegal mix of collations`** ✗（只有"列 vs 字面量/参数"的比较才不报）。
// 这里把派生侧统一到 `utf8mb4_unicode_ci`（只影响比较、不影响语义：`match_key` 是数字串）。
func (r *EpidRepo) AlignmentRateComparable(ctx context.Context) (aligned, total int64, err error) {
	var row struct{ Aligned, Total int64 }
	err = dbFrom(ctx, r.db).Raw(`
		SELECT
		  COALESCE(SUM(CASE WHEN t.sites >= 2 THEN 1 ELSE 0 END), 0) AS aligned,
		  COUNT(*) AS total
		FROM (
		  SELECT e.match_key, e.epid, COUNT(DISTINCT e.site_id) AS sites
		  FROM movie_episode_epid e
		  WHERE e.epid <> ''
		    AND e.dup_copy = 0
		  GROUP BY e.match_key, e.epid
		) t
		WHERE t.match_key COLLATE utf8mb4_unicode_ci IN (
		    SELECT s.match_key FROM movie_play_source s GROUP BY s.match_key HAVING COUNT(DISTINCT s.site_id) >= 2
		  )
		  AND t.match_key COLLATE utf8mb4_unicode_ci IN (
		    SELECT s.match_key FROM movie_play_source s
		    LEFT JOIN movie_episode_epid d ON d.src_row_id = s.id
		    GROUP BY s.match_key
		    HAVING COUNT(*) = COUNT(DISTINCT d.src_row_id)
		  )`).Scan(&row).Error
	return row.Aligned, row.Total, err
}

// CategoryStat 按分类分层的"有身份率"（`movie.pid` 指向 `category.id`）。
type CategoryStat struct {
	Pid        int64   `json:"pid"`
	Category   string  `json:"category"` // category.name：电影片 / 连续剧 / 综艺片 / 动漫片 …
	RowCnt     int64   `json:"rowCnt"`
	NonEmpty   int64   `json:"nonEmpty"`
	NonEmptyPc float64 `json:"nonEmptyPct"`
}

// ByCategory 分层"有身份率"（**按需调用**，不进每轮日志：要 join movie/category）。
func (r *EpidRepo) ByCategory(ctx context.Context) ([]CategoryStat, error) {
	var out []CategoryStat
	err := dbFrom(ctx, r.db).Raw(`
		SELECT
		  IFNULL(m.pid, -1) AS pid,
		  IFNULL(c.name, '(未知)') AS category,
		  COUNT(*) AS row_cnt,
		  SUM(IF(e.epid <> '', 1, 0)) AS non_empty
		FROM movie_episode_epid e
		LEFT JOIN movie m ON m.mid = e.mid
		LEFT JOIN category c ON c.id = m.pid
		GROUP BY IFNULL(m.pid, -1), IFNULL(c.name, '(未知)')
		ORDER BY row_cnt DESC`).Scan(&out).Error
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].RowCnt > 0 {
			out[i].NonEmptyPc = float64(out[i].NonEmpty) * 100 / float64(out[i].RowCnt)
		}
	}
	return out, nil
}

// MaturityStat 源行口径的派生成熟度（按分类）。
//
// 含义：**分母 = 真相源 `movie_play_source` 的全部源行**，分子 = 其中**已存在派生行**的源行数
// ⇒ 衡量"派生任务跑到哪了"（覆盖进度），**不是** epid 质量。
// 为什么必须与 `AlignmentRateComparable` 配对看：可比口径只统计"全源行都已派生"的片 ⇒
// 成熟度低时样本极小（实测只有 6 个组），任何对齐率读数都不具判定意义 ✗
type MaturityStat struct {
	Pid         int64  `json:"pid"`
	Category    string `json:"category"` // category.name；`pid=-1`（`mid` 为空的附属站行）显示 未关联片
	SrcRows     int64  `json:"srcRows"`
	DerivedRows int64  `json:"derivedSrcRows"`
}

// SourceRowMaturity 源行口径成熟度：**真相源**每个源行是否已有派生行（按分类）。
// 与 `ByCategory` 同规：**必须 join `category` 给出分类名**（不再暴露裸 pid ✗）。
func (r *EpidRepo) SourceRowMaturity(ctx context.Context) ([]MaturityStat, error) {
	var out []MaturityStat
	err := dbFrom(ctx, r.db).Raw(`
		SELECT IFNULL(m.pid, -1) AS pid,
		       CASE
		         WHEN m.pid IS NULL THEN '未关联片（mid 为空）'
		         WHEN m.pid = 0 THEN '(未分类)'
		         ELSE IFNULL(c.name, '(未知分类)')
		       END AS category,
		       COUNT(*) AS src_rows,
		       SUM(IF(d.src_row_id IS NULL, 0, 1)) AS derived_rows
		FROM movie_play_source s
		LEFT JOIN movie m ON m.mid = s.mid
		LEFT JOIN category c ON c.id = m.pid
		LEFT JOIN (SELECT DISTINCT src_row_id FROM movie_episode_epid) d ON d.src_row_id = s.id
		GROUP BY IFNULL(m.pid, -1),
		         CASE
		           WHEN m.pid IS NULL THEN '未关联片（mid 为空）'
		           WHEN m.pid = 0 THEN '(未分类)'
		           ELSE IFNULL(c.name, '(未知分类)')
		         END
		ORDER BY src_rows DESC`).Scan(&out).Error
	return out, err
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

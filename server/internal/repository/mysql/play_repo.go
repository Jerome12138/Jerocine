package mysql

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync/atomic"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

type playRepo struct{ db *gorm.DB }

// 「内容未变不写」的累计计数（F5 日志降噪：每 20 批汇总一行）。
// 采集有多个 worker 并发 upsert ⇒ 用 `atomic.Int64`（**不混用** int64 + 包级函数，见评审纪律）。
var (
	upsertSkipBatches atomic.Int64
	upsertSkipRows    atomic.Int64
)

// NewPlaySourceRepository 构造多源播放仓储。
func NewPlaySourceRepository(db *gorm.DB) repository.PlaySourceRepository { return &playRepo{db: db} }

func (r *playRepo) UpsertBatch(ctx context.Context, list []entity.MoviePlaySource) error {
	if len(list) == 0 {
		return nil
	}
	// P1-1 / F-05（2026-11-10）：**写侧统一回填 `ep_hash`** —— 派生的唯一入口就是本方法，
	// 放在这里算可以确保**任何构造点都不会漏**（否则重采不回填 ⇒ 派生任务的变更检测静默失效）。
	for i := range list {
		list[i].EpHash = EpisodesHash(list[i].Episodes)
		list[i].ContentHash = EpisodesContentHash(list[i].Episodes)
		// updated_at 写入闸：ODKU 是 `updated_at = VALUES(updated_at)`，值来自本字段；
		// 一旦有人把 YYYYMMDDHHMMSS/秒级/0 写进来，会污染所有按时间窗的审计（2026-11-23 事故）。
		list[i].UpdatedAt = guardMilli(list[i].UpdatedAt)
	}
	sortPlaySourcesForKey(list)
	// 「内容未变不写」（2026-11-13）：全站重采会把**每一行整行**重写（含 `episodes_json` 巨列）⇒ binlog
	// 暴涨（实测撑到 12GB+、磁盘 89%）。判重口径 = **mid / source_vod_id / dup_copy / content_hash 四项全等**
	// ⇒ 跳过（不发 UPDATE）。⚠️ 只看 content_hash 不够：`source_vod_id` 由 0 被回填（vod0 修复）、
	// `dup_copy` 从存量脚本的**近似值**变回写侧的**确定值**，这类**修复型写入**都必须放行；
	// `content_hash` 为存量空串时也必写（自愈回填）。
	list, skipped := r.dropUnchanged(ctx, list)
	if skipped > 0 {
		// 日志降噪（F5，2026-11-13 评审）：全站重采时**每批**都会跳过 ⇒ 逐批一行会刷屏。
		// 改为**每 20 批汇总一行**（累计批数 + 累计行数），首个批次必打（便于确认特性生效）。
		total := upsertSkipRows.Add(int64(skipped))
		if n := upsertSkipBatches.Add(1); n%20 == 1 {
			log.Printf("[spider] upsert 跳过内容未变：本批 %d 行（累计 %d 批 / %d 行）", skipped, n, total)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "match_key"}, {Name: "play_from"}},
		// source_vod_id 纳入更新列: 重采时回填旧行的源站影片ID(供去重统计 CountBySite)。
		// dup_copy 同理纳入: 重采时把"双写副本标记"一并刷新（存量由脚本**近似**补齐，重采会用**正确值**覆盖）✓
		// content_hash 同理: 判重列本身也要落库(否则下次仍判为"变了" ⇒ 白写, 见 000051)。
		// 更新列见 playSourceUpsertUpdates()：有序；dup_copy / 000052 两列各有刻意设计（见该函数注释）。
		DoUpdates: playSourceUpsertUpdates(),
	}).CreateInBatches(list, 200).Error
}

// playSourceUpsertUpdates 组装 upsert 的更新列（**顺序即 MySQL 的求值顺序**，从左到右）。
//
// 三处刻意设计（改动前先读）：
//  1. `dup_copy` 纳入 —— 重采时把双写副本标记一并刷新（存量由回填脚本近似补齐，重采会用**正确值**覆盖）；
//  2. `source_name` / `source_douban_id`（000052，2026-11-14）用 **`COALESCE(VALUES(col), col)`**：
//     上游有值才覆盖、**空值不冲掉已有值**（宁缺勿错）。这两列是"双写副本判定做到 100%"的唯一依据
//     ⇒ 被冲成 NULL 会静默退回 ≈69.8% 且不可自愈；故**不能**退化成裸 `VALUES(col)`（有防呆用例钉住）。
//     另：两列**不进**判重的**相等性**比较（不算"变了"）；但 `dropUnchanged`/`shouldSkipWrite` 里有一条
//     **单向自愈**放行：存量空而新行有值 ⇒ 必写一次（定向/自然补采的落地条件，见 `shouldSkipWrite`）。
//  3. `content_hash` 纳入 —— 判重列本身要落库，否则下次仍判为"变了" ⇒ 白写（见 000051）。
func playSourceUpsertUpdates() clause.Set {
	return clause.Set{
		{Column: clause.Column{Name: "mid"}, Value: gorm.Expr("VALUES(mid)")},
		{Column: clause.Column{Name: "source_vod_id"}, Value: gorm.Expr("VALUES(source_vod_id)")},
		{Column: clause.Column{Name: "episodes_json"}, Value: gorm.Expr("VALUES(episodes_json)")},
		{Column: clause.Column{Name: "ep_hash"}, Value: gorm.Expr("VALUES(ep_hash)")},
		{Column: clause.Column{Name: "content_hash"}, Value: gorm.Expr("VALUES(content_hash)")},
		{Column: clause.Column{Name: "dup_copy"}, Value: gorm.Expr("VALUES(dup_copy)")},
		{Column: clause.Column{Name: "source_name"}, Value: gorm.Expr("COALESCE(VALUES(source_name), source_name)")},
		{Column: clause.Column{Name: "source_douban_id"}, Value: gorm.Expr("COALESCE(VALUES(source_douban_id), source_douban_id)")},
		{Column: clause.Column{Name: "updated_at"}, Value: gorm.Expr("VALUES(updated_at)")},
	}
}

// dropUnchanged 返回"确实需要写"的子集与**被跳过**的行数（2026-11-13，「内容未变不写」）。
//
// 判据见 `shouldSkipWrite`：`mid / source_vod_id / dup_copy / content_hash` **四项全等** ⇒ 跳过，
// 但两处**自愈**必须放行：① `content_hash` 为存量空串（000051 前）⇒ 必写一次填上；
// ② `source_name`/`source_douban_id` 存量空而新行有值（000052）⇒ 必写一次——这是"定向补采/自然补采
// 能把上游原文补进来"的落地条件（只在缺失时放行，填上即止，不会反复重写）。
//   - 只比 `content_hash` 不够：`source_vod_id` 从 0 被回填（vod0 修复）、`dup_copy` 由**存量脚本近似值**
//     变为**写侧的确定值** —— 这类**修复型写入**必须放行（否则永远停在近似值上 ✗）。
//
// 性能：只 SELECT 唯一键三列 + 四个比较列 + 000052 两列 ⇒ **不碰 `episodes_json` 巨列**（性能铁律 1）。
// 容错：查询失败时**整体不跳过**（宁多写不误跳）——本特性只省体积，不能因它丢写。
func (r *playRepo) dropUnchanged(ctx context.Context, list []entity.MoviePlaySource) ([]entity.MoviePlaySource, int) {
	ph := make([]string, 0, len(list))
	args := make([]any, 0, len(list)*3)
	for _, x := range list {
		ph = append(ph, "(?,?,?)")
		args = append(args, x.SiteId, x.MatchKey, x.PlayFrom)
	}
	var cur []entity.MoviePlaySource
	err := dbFrom(ctx, r.db).Model(&entity.MoviePlaySource{}).
		Select("site_id, match_key, play_from, mid, source_vod_id, dup_copy, content_hash, source_name, source_douban_id").
		Where("(site_id, match_key, play_from) IN ("+strings.Join(ph, ",")+")", args...).
		Find(&cur).Error
	if err != nil {
		log.Printf("[spider] upsert 判重查询失败（本轮不跳过，照常写）: %v", err)
		return list, 0
	}
	now := make(map[[3]string]entity.MoviePlaySource, len(cur))
	for _, x := range cur {
		now[[3]string{x.SiteId, x.MatchKey, x.PlayFrom}] = x
	}
	out := make([]entity.MoviePlaySource, 0, len(list))
	skipped := 0
	for _, x := range list {
		old, ok := now[[3]string{x.SiteId, x.MatchKey, x.PlayFrom}]
		if ok && shouldSkipWrite(old, x) {
			skipped++
			continue
		}
		out = append(out, x)
	}
	return out, skipped
}

// shouldSkipWrite 判重口径（2026-11-13「内容未变不写」+ 000052 自愈）：返回 true = 这行写下去也是同样的值。
//
// 四个**相等性**判据：`mid`（比指针值）/ `source_vod_id` / `dup_copy` / `content_hash`（且非空）。
// 两条**单向往**的自愈放行（必须放行，否则修复/补采永远到不了库）：
//   - `content_hash` 存量空串（000051 前）⇒ 写一次填上；
//   - `source_name` / `source_douban_id` 存量**空（NULL）而新行有值** ⇒ 写一次填上。
//     ⚠️ 反向不算"变了"：存量有值、新行 NULL 时**跳过**（upsert 用 `COALESCE(VALUES(col), col)` 本来也保旧值）
//     ⇒ 既补齐、又不因上游偶发缺字段而无谓重写。
func shouldSkipWrite(old, x entity.MoviePlaySource) bool {
	if old.SourceName == nil && x.SourceName != nil {
		return false
	}
	if old.SourceDoubanID == nil && x.SourceDoubanID != nil {
		return false
	}
	return sameMidPtr(old.Mid, x.Mid) && old.SourceVodID == x.SourceVodID &&
		old.DupCopy == x.DupCopy && old.ContentHash != "" && old.ContentHash == x.ContentHash
}

// sameMidPtr 比较两个可空 mid 的**值**（指针直接比较会比较地址，错）。
func sameMidPtr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// sortPlaySourcesForKey 按 upsert 唯一键 `(site_id, match_key, play_from)` 升序排序。
//
// 死锁防护（2026-11-13）：`UpsertBatch` 走 `INSERT … ON DUPLICATE KEY UPDATE`，会在唯一键上
// 加**间隙锁/下一键锁**。多个采集 worker 并发提交时，若各自批内的键顺序不同 ⇒ **取锁顺序相反**
// ⇒ 环形等待（MySQL **Error 1213 Deadlock found**）。实测集中在 `src_lz` 大批量重采期间
// （`collect_failure` 台账 47 条 1213，其余站均为网络超时），日志形如：
//
//	spider page src=src_lz pg=8085 deadlock retry 0/2
//	play_repo.go:31 Error 1213 (40001): Deadlock found when trying to get lock
//
// **先按唯一键排序再分批** ⇒ 所有事务以一致顺序取锁，环即消失（比加锁/降并发更根本，且不损吞吐）。
// 排序在写侧做（列表本就是待写批次，顺序无语义），对调用方无可见影响。
func sortPlaySourcesForKey(list []entity.MoviePlaySource) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].SiteId != list[j].SiteId {
			return list[i].SiteId < list[j].SiteId
		}
		if list[i].MatchKey != list[j].MatchKey {
			return list[i].MatchKey < list[j].MatchKey
		}
		return list[i].PlayFrom < list[j].PlayFrom
	})
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

// PlayIDsBySite 取该源全部源行 id + 最大 id（走 idx_mps_site*，不读 episodes_json）。
// maxID 用作删除上界：删除期间新采的行 id 更大，不会被误删。
func (r *playRepo) PlayIDsBySite(ctx context.Context, siteId string) ([]int64, int64, error) {
	if siteId == "" {
		return nil, 0, nil
	}
	var ids []int64
	if err := dbFrom(ctx, r.db).Model(&entity.MoviePlaySource{}).
		Where("site_id = ?", siteId).Pluck("id", &ids).Error; err != nil {
		return nil, 0, err
	}
	var maxID int64
	for _, v := range ids {
		if v > maxID {
			maxID = v
		}
	}
	return ids, maxID, nil
}

// DeleteEpidByRowIDs 按源行 id 分批删派生表（每批 2000，走 uk_row_ep(src_row_id, ep_index)）。
//
// ⚠️ 为什么不能用 `WHERE site_id = ?`：movie_episode_epid 没有任何以 site_id 为前缀的索引
// （idx_match_epid_site / idx_dup_match_epid_site 里 site_id 都在第 3/4 列），
// 按 site_id 删会每批全表扫 683 万行。
func (r *playRepo) DeleteEpidByRowIDs(ctx context.Context, rowIDs []int64) (int64, error) {
	const chunk = 2000
	var total int64
	for i := 0; i < len(rowIDs); i += chunk {
		end := i + chunk
		if end > len(rowIDs) {
			end = len(rowIDs)
		}
		res := dbFrom(ctx, r.db).Exec("DELETE FROM movie_episode_epid WHERE src_row_id IN (?)", rowIDs[i:end])
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
	}
	return total, nil
}

// DeletePlayBySiteUpTo 删该源 <= maxID 的源行（同步，走 site 索引）。maxID<=0 表示不限。
func (r *playRepo) DeletePlayBySiteUpTo(ctx context.Context, siteId string, maxID int64) (int64, error) {
	if siteId == "" {
		return 0, nil
	}
	q := dbFrom(ctx, r.db).Where("site_id = ?", siteId)
	if maxID > 0 {
		q = q.Where("id <= ?", maxID)
	}
	res := q.Delete(&entity.MoviePlaySource{})
	return res.RowsAffected, res.Error
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

// ListSlimByMidsOrKeys 一批影片的源行瘦身投影(管理列表"源数"列的数据面)。
// 只引用 mid/match_key/site_id/play_from 四列, 不触碰巨大的 episodes_json 列(否则全表扫描超时,
// 见 CountBySite 注释); mid 与 match_key 两段独立索引扫描, OR 合并交给调用方。
func (r *playRepo) ListSlimByMidsOrKeys(ctx context.Context, mids []int64, matchKeys []string) ([]repository.SlimSourceRow, error) {
	out := make([]repository.SlimSourceRow, 0, len(mids)+len(matchKeys))
	if len(mids) > 0 {
		var byMid []repository.SlimSourceRow
		if err := dbFrom(ctx, r.db).Model(&entity.MoviePlaySource{}).
			Select("mid, match_key, site_id, play_from").
			Where("mid IN ?", mids).
			Scan(&byMid).Error; err != nil {
			return nil, err
		}
		out = append(out, byMid...)
	}
	if len(matchKeys) > 0 {
		var byKey []repository.SlimSourceRow
		if err := dbFrom(ctx, r.db).Model(&entity.MoviePlaySource{}).
			Select("mid, match_key, site_id, play_from").
			Where("match_key IN ?", matchKeys).
			Scan(&byKey).Error; err != nil {
			return nil, err
		}
		out = append(out, byKey...)
	}
	return out, nil
}

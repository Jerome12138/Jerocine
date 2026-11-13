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
		DoUpdates: clause.AssignmentColumns([]string{"mid", "source_vod_id", "episodes_json", "ep_hash", "content_hash", "dup_copy", "updated_at"}),
	}).CreateInBatches(list, 200).Error
}

// dropUnchanged 返回"确实需要写"的子集与**被跳过**的行数（2026-11-13，「内容未变不写」）。
//
// 判据：库中同 `(site_id, match_key, play_from)` 的行存在，且
// `mid / source_vod_id / dup_copy / content_hash` **四项全等** ⇒ 跳过（这行写下去也是同样的值）。
//   - 只比 `content_hash` 不够：`source_vod_id` 从 0 被回填（vod0 修复）、`dup_copy` 由**存量脚本近似值**
//     变为**写侧的确定值** —— 这类**修复型写入**必须放行（否则永远停在近似值上 ✗）；
//   - `content_hash` 为空串（000051 前的存量行）⇒ 必写一次并填上 ⇒ 自愈 ✓。
//
// 性能：只 SELECT 唯一键三列 + 四个比较列 ⇒ **不碰 `episodes_json` 巨列**（性能铁律 1）。
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
		Select("site_id, match_key, play_from, mid, source_vod_id, dup_copy, content_hash").
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
		if ok && sameMidPtr(old.Mid, x.Mid) && old.SourceVodID == x.SourceVodID &&
			old.DupCopy == x.DupCopy && old.ContentHash != "" && old.ContentHash == x.ContentHash {
			skipped++
			continue
		}
		out = append(out, x)
	}
	return out, skipped
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

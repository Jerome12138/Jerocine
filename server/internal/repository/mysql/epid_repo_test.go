package mysql

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// R2 验证（2026-11-11，评审「最终报告」§一 R2）—— **M3 修法后更新**：
//
// M3 把 `DirtySourceRows` 从「`Raw()` + 全表 GROUP BY 子查询」改成三步（候选 id 页 → 取锚判脏 → 只对
// 脏 id 取全字段）。**第③步改用 `Find()`（走 schema）** ⇒ 原先"`serializer:json` 在 `Raw().Scan`
// 路径是否生效"的**不确定性结构性消失** ✓；本用例保留为守卫：钉住 `episodes_json` **必须**被反序列化
// （否则派生池为空 ⇒ 派生零产出且静默 ✗）。
func TestDirtySourceRows_EpisodesJSON反序列化必须生效(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewEpidRepository(gdb)

	const jsonPayload = `[{"episode":"第1期上","link":"https://x/1.m3u8"},{"episode":"第2期上","link":"https://x/2.m3u8"}]`

	// ① 候选 id 页（keyset；只取主键 ⇒ index-only）
	mock.ExpectQuery(regexp.QuoteMeta("ORDER BY id ASC LIMIT")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	// ② 取锚：本行**无锚** ⇒ 判脏（首次派生）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT src_row_id, COUNT(*) AS n, MIN(src_ep_hash) AS src_ep_hash, MAX(computed_at) AS last_computed_at")).
		WillReturnRows(sqlmock.NewRows([]string{"src_row_id", "n", "src_ep_hash", "last_computed_at"}))
	// ③ 取源行当前 ep_hash（两列 ⇒ 不碰巨列）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, ep_hash FROM `movie_play_source`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ep_hash"}).AddRow(9, "0123456789abcdef"))
	// ④ 只对脏 id 取全字段（含巨列）；**Find ⇒ serializer:json 生效**
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `movie_play_source`")).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "mid", "match_key", "site_id", "play_from", "episodes_json", "ep_hash",
		}).AddRow(9, 100, "mk-1", "src_lz", "lzm3u8", jsonPayload, "0123456789abcdef"))

	rows, scanLast, scanCount, err := repo.DirtySourceRows(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("DirtySourceRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("应返回 1 行, got %d", len(rows))
	}
	// 关键断言（2026-11-11 游标 bug 的回归守卫）：只有"脏行"是不够的 ——
	// 必须同时拿回"扫过的窗口规模"与"窗口末端 id"，调用方才能正确判表尾与推进游标
	// （否则一段没有脏行的区间会被误判为表尾、游标归零 ⇒ 原地空转）。
	if scanCount != 1 || scanLast != 9 {
		t.Fatalf("扫描窗口信息不对: scanCount=%d scanLast=%d（want 1 / 9）", scanCount, scanLast)
	}
	r := rows[0]
	// ① 关键断言：episodes_json 必须被反序列化（否则派生池为空 ⇒ 零产出且静默）
	if len(r.Episodes) != 2 {
		t.Fatalf("`episodes_json` 未被反序列化 —— 派生会零产出且静默。got Episodes=%#v", r.Episodes)
	}
	if r.Episodes[0].Episode != "第1期上" || r.Episodes[0].Link != "https://x/1.m3u8" {
		t.Fatalf("第 1 集反序列化内容不对: %#v", r.Episodes[0])
	}
	if r.Episodes[1].Episode != "第2期上" {
		t.Fatalf("第 2 集反序列化内容不对: %#v", r.Episodes[1])
	}
	// ② 其余列也必须映射正确（派生要用它们做受控冗余）
	if r.Id != 9 || r.MatchKey != "mk-1" || r.SiteId != "src_lz" || r.PlayFrom != "lzm3u8" || r.EpHash != "0123456789abcdef" {
		t.Fatalf("其它列映射不对: %#v", r)
	}
	if r.Mid == nil || *r.Mid != 100 {
		t.Fatalf("mid 映射不对: %v", r.Mid)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// M3 回归守卫：`DirtySourceRows` 不得再出现「对派生表做全表 GROUP BY」的判脏子查询
// （原实现一轮总扫描 ≈ 647.7 万 × 批数 ✗）。此处以"锚只按本页 ids 查"为断言口径：
// 见 `AnchorsBySrcRowIds` 用 `src_row_id IN ?`（而非 `src_row_id > ?` 的窗口式全表扫）。
func TestAnchorsBySrcRowIds_按In查询而非窗口全表(t *testing.T) {
	gdb, mock, lastSQL := newMockDBCapture(t)
	repo := NewEpidRepository(gdb)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT src_row_id, COUNT(*) AS n, MIN(src_ep_hash) AS src_ep_hash, MAX(computed_at) AS last_computed_at")).
		WillReturnRows(sqlmock.NewRows([]string{"src_row_id", "n", "src_ep_hash", "last_computed_at"}).
			AddRow(9, 3, "abc", 1700000000000))

	got, err := repo.AnchorsBySrcRowIds(context.Background(), []int64{9})
	if err != nil {
		t.Fatalf("AnchorsBySrcRowIds: %v", err)
	}
	if a := got[9]; a.N != 3 || a.SrcEpHash != "abc" || a.LastComputedAt != 1700000000000 {
		t.Fatalf("锚解出不对: %#v", a)
	}
	if !regexp.MustCompile("src_row_id IN").MatchString(*lastSQL) {
		t.Fatalf("锚查询必须按 IN 收窄（M3），实际 SQL: %s", *lastSQL)
	}
}

// ClassAlignment（双粒度）三重钉：
//
//	① keyset 分页子查询**必须带表别名** —— 生产曾因缺 `) t` 报 Error 1248，后台快照静默空跑（2026-11-15）；
//	② 槽位口径：集级 = (mk, ep_index) 槽内非空 epid 唯一（单源槽默认对齐 + 多源同 epid 槽）；
//	   片级 = 该片**全部槽位对齐**（集级 100% 的片，零分歧）。判定单位：槽/片。
//	③ 归并与百分比（SlotPct/MkPct）。
func TestClassAlignment_双粒度归并(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewEpidRepository(gdb)

	// ① keyset 圈批：一批即完（MAX=100），随后一轮返回 0 ⇒ 循环结束。
	//    expected 正则里显式含 `) t` —— 实际 SQL 缺别名时匹配失败 ⇒ 用例红（Error 1248 回归守卫）。
	//    执行顺序：keyset → 槽位聚合 → keyset（返回 0 终止）→ category 名回查。
	keyset := regexp.QuoteMeta("SELECT COALESCE(MAX(mk),0) FROM (SELECT match_key AS mk FROM movie_play_source WHERE match_key > ? ORDER BY match_key LIMIT ?) t")
	mock.ExpectQuery(keyset).WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow(100))

	// ② 槽位聚合：连续剧 3 槽全对齐（单源槽 + 同 epid 多源槽）⇒ 全对齐片；
	//    电影 1 槽分歧（多源多 epid）⇒ 非全对齐片
	mock.ExpectQuery(regexp.QuoteMeta("SUM(mkstat.aligned_slots = mkstat.total_slots) AS aligned_mks")).
		WillReturnRows(sqlmock.NewRows([]string{"pid", "aligned_slots", "total_slots", "aligned_mks", "total_mks"}).
			AddRow(2, 3, 3, 3, 3).AddRow(1, 0, 1, 0, 1))

	mock.ExpectQuery(keyset).WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow(0))

	// ③ category 名回查（ids=[1,2]）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name FROM category WHERE id IN")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "电影片").AddRow(2, "连续剧"))

	out, all, err := repo.ClassAlignment(context.Background())
	if err != nil {
		t.Fatalf("ClassAlignment: %v", err)
	}
	if len(out) != 2 || all.TotalSlots != 4 || all.AlignedSlots != 3 || all.TotalMks != 4 || all.AlignedMks != 3 {
		t.Fatalf("归并不符: out=%+v all=%+v", out, all)
	}
	for _, st := range out {
		if st.Pid == 2 && (st.TotalSlots != 3 || st.AlignedSlots != 3 || st.SlotPct != 100 || st.MkPct != 100 || st.Category != "连续剧") {
			t.Fatalf("连续剧聚合不符: %+v", st)
		}
		if st.Pid == 1 && (st.AlignedSlots != 0 || st.SlotPct != 0 || st.MkPct != 0 || st.Category != "电影片") {
			t.Fatalf("电影（分歧槽）应为 0%%: %+v", st)
		}
	}
	if all.SlotPct != 75 {
		t.Fatalf("集级 SlotPct 应 75%%（3/4 槽）, got %v", all.SlotPct)
	}
	if all.MkPct != 75 {
		t.Fatalf("片级 MkPct 应 75%%（3/4 片全对齐）, got %v", all.MkPct)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

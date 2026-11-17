package mysql

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"server/internal/domain/entity"
)

// T2 单测（epid 源头校正层 · 第②段，2026-11-12）。
//
// 风格对齐 epid_repo_test.go（go-sqlmock + newMockDBCapture）。三例**必测**：
// ① 重复 import 拒 ② 唯一键 (src_row_id,ep_index) 冲突拒 ③ 坏 key_epid（对不上当前规则值）拒。
//
// ⚠️ 红线⑥：本文件只用**合成数据**（mk-9 / src_row_id=9 等），真实候选（12 条台账、真实
// src_row_id/片名/evidence）只进 Harness 私有仓，绝不进本公开仓。
//
// 派生表 epid 串格式对齐 danmu.EpidOf：`s=<series>|v=<variant>|p=<part>|n=<num>|d=<date>`。
var (
	epidKeyCur  = "s=-|v=-|p=上|n=3|d=20260101" // 合成：当前规则 epid（与 new_epid 同构）
	epidKeyNew  = "s=-|v=-|p=上|n=5|d=20260101" // 合成：裁定目标 epid
	overrideRow = entity.MovieEpidOverride{
		SrcRowId: 9, EpIndex: 2,
		KeyRawName: "第3期上", KeyEpid: epidKeyCur,
		Verdict: entity.OverrideVerdictCorrected, NewEpid: epidKeyNew,
		Provenance: "agent", Confidence: "verified",
		Reason: "单测合成数据（非真实候选）",
	}
)

// 台账冲突预检（UpsertPending 第③步）的期望。
func expectOverrideExisting(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, src_row_id, ep_index, status FROM `movie_epid_override`")).
		WillReturnRows(rows)
}

// 派生表 (src_row_id,ep_index) 点查（UpsertPending 第④步）的期望。
func expectDerivPointQuery(mock sqlmock.Sqlmock, epid string, mid any) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT epid, match_key, mid FROM `movie_episode_epid` WHERE src_row_id = ")).
		WillReturnRows(sqlmock.NewRows([]string{"epid", "match_key", "mid"}).AddRow(epid, "mk-9", mid))
}

func emptyOverrideExisting() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "src_row_id", "ep_index", "status"})
}

// rejectOf 取 (srcRowId,epIndex) 的拒收原因；没有即失败。
func rejectOf(t *testing.T, res OverrideImportResult, srcRowId int64, epIndex int) string {
	t.Helper()
	for _, r := range res.Rejected {
		if r.SrcRowId == srcRowId && r.EpIndex == epIndex {
			return r.Reason
		}
	}
	t.Fatalf("未找到 (%d,%d) 的拒收明细: %#v", srcRowId, epIndex, res.Rejected)
	return ""
}

// ① 重复 import 拒：同一条先成功入库，再原样 import 第二次 ⇒ 撞唯一键 (src_row_id,ep_index)，
// repo 预检给出可读原因（既有 id/status），且**不再发起点查与 INSERT**。
func TestUpsertPending_重复import拒(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewEpidOverrideRepository(gdb)
	ctx := context.Background()

	// 第一次 import：无既有条 → 点查 epid 一致 → 插入
	expectOverrideExisting(mock, emptyOverrideExisting())
	expectDerivPointQuery(mock, epidKeyCur, 100)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `movie_epid_override`")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	res1, err := repo.UpsertPending(ctx, []entity.MovieEpidOverride{overrideRow})
	if err != nil {
		t.Fatalf("首次 import 不应报 DB 错误: %v", err)
	}
	if res1.Inserted != 1 || len(res1.Rejected) != 0 {
		t.Fatalf("首次 import 应插入 1 行, got %+v", res1)
	}

	// 第二次（重复 import）：预检命中既有条 → 拒收
	expectOverrideExisting(mock, sqlmock.NewRows([]string{"id", "src_row_id", "ep_index", "status"}).
		AddRow(7, 9, 2, "pending"))
	res2, err := repo.UpsertPending(ctx, []entity.MovieEpidOverride{overrideRow})
	if err != nil {
		t.Fatalf("拒收走返回值明细, 不应报 DB 错误: %v", err)
	}
	if res2.Inserted != 0 {
		t.Fatalf("重复 import 不得再插入, got %+v", res2)
	}
	reason := rejectOf(t, res2, 9, 2)
	if !strings.Contains(reason, "id=7") || !strings.Contains(reason, "status=pending") {
		t.Fatalf("拒收原因应给出既有条 id/status, got: %s", reason)
	}
	// 上面只登记了"冲突预检"一个期望：若第二次还发了点查/INSERT，sqlmock 会因
	// 期望耗尽而报错 ⇒ UpsertPending 返回 err ⇒ 上面的 err 断言已把它拦下；
	// 这里再确认全部期望恰好被消费（无漏发的调用）。
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// ② 唯一键冲突拒：同一 (src_row_id,ep_index) 已有**另一条**裁定（内容不同、且已 active）⇒ 拒收。
// uk_row_ep 一集一条裁定 —— 换裁定必须先显式 revoked 既有条，绝不静默覆盖。
func TestUpsertPending_键冲突拒(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewEpidOverrideRepository(gdb)
	ctx := context.Background()

	expectOverrideExisting(mock, sqlmock.NewRows([]string{"id", "src_row_id", "ep_index", "status"}).
		AddRow(11, 9, 2, "active"))

	res, err := repo.UpsertPending(ctx, []entity.MovieEpidOverride{overrideRow})
	if err != nil {
		t.Fatalf("拒收走返回值明细, 不应报 DB 错误: %v", err)
	}
	if res.Inserted != 0 {
		t.Fatalf("键冲突不得插入, got %+v", res)
	}
	reason := rejectOf(t, res, 9, 2)
	if !strings.Contains(reason, "id=11") || !strings.Contains(reason, "status=active") {
		t.Fatalf("拒收原因应给出既有条 id/status, got: %s", reason)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// ③ 坏 key_epid 拒：key_epid 对不上派生表当前规则 epid ⇒ 拒收并给出两边的值（宁缺勿错）。
// 这类候选来自"源行集序已漂移/已被重算"的过期清单，套用必然错位。
func TestUpsertPending_坏keyEpid拒(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewEpidOverrideRepository(gdb)
	ctx := context.Background()

	stale := overrideRow
	stale.KeyEpid = "s=-|v=-|p=上|n=999|d=20260101" // 合成：过期的 key_epid

	expectOverrideExisting(mock, emptyOverrideExisting())
	expectDerivPointQuery(mock, epidKeyCur, 100) // 当前规则值 = n=3，与 stale.KeyEpid 对不上

	res, err := repo.UpsertPending(ctx, []entity.MovieEpidOverride{stale})
	if err != nil {
		t.Fatalf("拒收走返回值明细, 不应报 DB 错误: %v", err)
	}
	if res.Inserted != 0 {
		t.Fatalf("坏 key_epid 不得插入, got %+v", res)
	}
	reason := rejectOf(t, res, 9, 2)
	if !strings.Contains(reason, stale.KeyEpid) || !strings.Contains(reason, epidKeyCur) {
		t.Fatalf("拒收原因应给出 key_epid 与当前值两边, got: %s", reason)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// ③' 派生表无此 (src_row_id,ep_index) 行 ⇒ key_epid 无法校验 ⇒ 同样拒收。
func TestUpsertPending_派生表无行拒(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewEpidOverrideRepository(gdb)
	ctx := context.Background()

	expectOverrideExisting(mock, emptyOverrideExisting())
	// 点查无行 → Take 返回 ErrRecordNotFound
	mock.ExpectQuery(regexp.QuoteMeta("SELECT epid, match_key, mid FROM `movie_episode_epid` WHERE src_row_id = ")).
		WillReturnRows(sqlmock.NewRows([]string{"epid", "match_key", "mid"}))

	res, err := repo.UpsertPending(ctx, []entity.MovieEpidOverride{overrideRow})
	if err != nil {
		t.Fatalf("拒收走返回值明细, 不应报 DB 错误: %v", err)
	}
	reason := rejectOf(t, res, 9, 2)
	if !strings.Contains(reason, "派生表无") {
		t.Fatalf("拒收原因应说明派生表无行, got: %s", reason)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 去重幂等 + 成功导入：批内同 (src_row_id,ep_index) 只入库一条（去重幂等）；
// status 强制 pending（即使调用方传了别的值）；受控冗余 match_key/mid 以派生表点查值回填。
func TestUpsertPending_批内去重与强制pending(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewEpidOverrideRepository(gdb)
	ctx := context.Background()

	dup := overrideRow
	dup.Status = "active" // 调用方乱传的值，必须被强制回 pending
	batch := []entity.MovieEpidOverride{overrideRow, dup}

	expectOverrideExisting(mock, emptyOverrideExisting())
	expectDerivPointQuery(mock, epidKeyCur, 100) // 只对去重后的 1 行点查一次
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `movie_epid_override`")).
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), // src_row_id, ep_index
			"mk-9", int64(100), // match_key/mid = 派生表回填值（覆盖调用方）
			sqlmock.AnyArg(), sqlmock.AnyArg(), // key_raw_name, key_epid
			sqlmock.AnyArg(), sqlmock.AnyArg(), // verdict, new_epid
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), // provenance, confidence, reason
			sqlmock.AnyArg(),                                                       // evidence
			"pending",                                                              // status 强制 pending
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), // rule_version_at/created_at/verified_at/verified_by
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	res, err := repo.UpsertPending(ctx, batch)
	if err != nil {
		t.Fatalf("UpsertPending: %v", err)
	}
	if res.Deduped != 1 {
		t.Fatalf("批内重复应去重 1 行, got Deduped=%d (%+v)", res.Deduped, res)
	}
	if res.Inserted != 1 || len(res.Rejected) != 0 {
		t.Fatalf("应插入 1 行且无拒收, got %+v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// SetStatus：非法状态（pending/orphan 不走本入口）直接报错不发 SQL；
// 置 active 即人工确认，UPDATE 里必须补 verified_at；stale 等旁支状态不动审计列。
func TestSetStatus_状态门禁与verifiedAt(t *testing.T) {
	ctx := context.Background()

	// 非法状态：不产生任何 SQL
	{
		gdb, mock, _ := newMockDBCapture(t)
		repo := NewEpidOverrideRepository(gdb)
		if _, err := repo.SetStatus(ctx, []int64{7}, entity.OverrideStatusPending); err == nil {
			t.Fatalf("pending 不应被 SetStatus 接受（它由 import 写入）")
		}
		if _, err := repo.SetStatus(ctx, []int64{7}, entity.OverrideStatusOrphan); err == nil {
			t.Fatalf("orphan 不应被 SetStatus 接受（它由孤儿清扫写）")
		}
		if _, err := repo.SetStatus(ctx, []int64{7}, "activated"); err == nil {
			t.Fatalf("未知状态不应被接受")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("非法状态不应发任何 SQL, unmet: %v", err)
		}
	}
	// stale：只改 status，不带 verified_at
	{
		gdb, mock, lastSQL := newMockDBCapture(t)
		repo := NewEpidOverrideRepository(gdb)
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE `movie_epid_override` SET")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		n, err := repo.SetStatus(ctx, []int64{7}, entity.OverrideStatusStale)
		if err != nil || n != 1 {
			t.Fatalf("SetStatus(stale): n=%d err=%v", n, err)
		}
		if strings.Contains(*lastSQL, "verified_at") {
			t.Fatalf("stale 不应动 verified_at: %s", *lastSQL)
		}
	}
	// active：UPDATE 里必须带 verified_at（人工确认时间）
	{
		gdb, mock, lastSQL := newMockDBCapture(t)
		repo := NewEpidOverrideRepository(gdb)
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE `movie_epid_override` SET")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		n, err := repo.SetStatus(ctx, []int64{7}, entity.OverrideStatusActive)
		if err != nil || n != 1 {
			t.Fatalf("SetStatus(active): n=%d err=%v", n, err)
		}
		if !strings.Contains(*lastSQL, "verified_at") {
			t.Fatalf("置 active 应补 verified_at（人工确认审计）: %s", *lastSQL)
		}
	}
}

// ListActive：整表取 status=active，供派生前一次性预加载（方案 §8.2 A 点）；
// 同时守卫 evidence JSON 列必须被反序列化（JSONMap Scan）—— 丢了证据 T5 复核就断档。
func TestListActive_evidence反序列化与列映射(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewEpidOverrideRepository(gdb)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `movie_epid_override` WHERE status = ")).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "src_row_id", "ep_index", "match_key", "mid", "key_raw_name", "key_epid",
			"verdict", "new_epid", "provenance", "confidence", "reason", "evidence",
			"status", "rule_version_at", "created_at", "verified_at", "verified_by",
		}).AddRow(
			7, 9, 2, "mk-9", 100, "第3期上", epidKeyCur,
			"corrected", epidKeyNew, "agent", "verified", "单测合成数据",
			`{"url":"https://example.com/x","grabbedAt":"2026-11-12"}`,
			"active", "", 1731380000000, 1731390000000, "tester",
		))

	out, err := repo.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("应返回 1 行, got %d", len(out))
	}
	r := out[0]
	if r.Id != 7 || r.SrcRowId != 9 || r.EpIndex != 2 || r.MatchKey != "mk-9" || r.Mid != 100 {
		t.Fatalf("列映射不对: %#v", r)
	}
	if r.KeyRawName != "第3期上" || r.KeyEpid != epidKeyCur || r.NewEpid != epidKeyNew ||
		r.Verdict != "corrected" || r.Status != "active" || r.VerifiedBy != "tester" {
		t.Fatalf("字段映射不对: %#v", r)
	}
	if r.Evidence == nil || r.Evidence["url"] != "https://example.com/x" {
		t.Fatalf("evidence JSON 列未被反序列化: %#v", r.Evidence)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// Stats：按 verdict/provenance/status 三维分组计数（T11 管理端指标的数据源）。
func TestStats_三维分组计数(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewEpidOverrideRepository(gdb)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT verdict AS name, COUNT(*) AS n FROM `movie_epid_override`")).
		WillReturnRows(sqlmock.NewRows([]string{"name", "n"}).AddRow("corrected", 10).AddRow("kept", 2))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT provenance AS name, COUNT(*) AS n FROM `movie_epid_override`")).
		WillReturnRows(sqlmock.NewRows([]string{"name", "n"}).AddRow("agent", 8).AddRow("human", 4))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status AS name, COUNT(*) AS n FROM `movie_epid_override`")).
		WillReturnRows(sqlmock.NewRows([]string{"name", "n"}).AddRow("active", 9).AddRow("pending", 3))

	stats, err := repo.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(stats.ByVerdict) != 2 || stats.ByVerdict[0].Name != "corrected" || stats.ByVerdict[0].N != 10 {
		t.Fatalf("ByVerdict 不对: %#v", stats.ByVerdict)
	}
	if len(stats.ByProvenance) != 2 || stats.ByProvenance[1].Name != "human" || stats.ByProvenance[1].N != 4 {
		t.Fatalf("ByProvenance 不对: %#v", stats.ByProvenance)
	}
	if len(stats.ByStatus) != 2 || stats.ByStatus[0].Name != "active" || stats.ByStatus[0].N != 9 {
		t.Fatalf("ByStatus 不对: %#v", stats.ByStatus)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// M5 取证批（2026-11-17）：key_epid="" 的静态校验放宽 —— 空串 = "裁定时的规则 epid 为空（skip 行）"，
// ④ 的逐字符对账对空串成立（当前 epid 也是空）⇒ skip 行修正可走台账且 rebuild-durable
// （A/C 点的 `Epid != KeyEpid` 对空串同样成立）。漂移防护不减弱：规则重新给身份 ⇒ 与空 key 不等 ⇒ stale。
func TestValidateOverrideRow_KeyEpid空串允许skip行裁定(t *testing.T) {
	skip := entity.MovieEpidOverride{
		SrcRowId: 9602, EpIndex: 53,
		KeyRawName: "0240514", KeyEpid: "",
		Verdict: entity.OverrideVerdictCorrected, NewEpid: "s=-|v=-|p=-|n=-|d=20240514",
		Provenance: "agent", Confidence: "inferred",
	}
	if reason := validateOverrideRow(skip); reason != "" {
		t.Fatalf("skip 行裁定（key_epid 空串）应通过静态校验, 拒因: %s", reason)
	}
	// 其它校验不受放宽影响：key_raw_name 仍必填
	bad := skip
	bad.KeyRawName = ""
	if reason := validateOverrideRow(bad); reason == "" {
		t.Fatal("key_raw_name 为空仍应拒收")
	}
}

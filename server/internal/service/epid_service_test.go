package service

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"server/internal/domain/entity"
	repomysql "server/internal/repository/mysql"
)

// T3 单测（epid 源头校正层 · 第②段，2026-11-12）。
//
// 覆盖任务验收的两条**必测**：
// ① A 点幂等（同批重算前后一致）+ 三元组校验失败 ⇒ 标 stale 且不改值；
// ② B 点三元组校验失败 ⇒ 标 stale 且**不发任何派生表 UPDATE**（值不变，宁缺勿错）。
// 另补：B 命中生效（只点改 epid 一列）+ 重复 apply 幂等；未装配台账 repo 时 B/C 显式报错不发 SQL。
//
// ⚠️ 红线⑥：本文件只用**合成数据**（mk-9 / src_row_id=9 / 假 epid 串），真实 12 条候选
// （真实 src_row_id/片名/evidence）只进 Harness 私有仓，绝不进本公开仓。
// epid 串格式对齐 danmu.EpidOf：`s=<series>|v=<variant>|p=<part>|n=<num>|d=<date>` 五段。
var (
	epidKeyN3   = "s=-|v=-|p=上|n=3|d=20260101"   // 合成：裁定时的规则 epid（corrected 条目）
	epidKeyN4   = "s=-|v=-|p=上|n=4|d=20260101"   // 合成：kept 条目对应的规则值
	epidKeyN5   = "s=-|v=-|p=上|n=5|d=20260101"   // 合成：与台账无关的第 3 集规则值
	epidKeyN10  = "s=-|v=-|p=上|n=10|d=20260101"  // 合成：corrected 的目标 epid
	epidKeyN999 = "s=-|v=-|p=上|n=999|d=20260101" // 合成：漂移后的现值（三元组对不上）
)

// newEpidSvcMockDB service 层最小测试基建：go-sqlmock + gorm（对齐 mysql 包 newMockDBCapture
// 的做法，包不同 ⇒ 只能复制），顺带把最近一次实际执行的 SQL 记进 *string，
// 用于断言 gorm 编译出的语句形状（如"B 只点改 epid 一列"）。
// 匹配器在记录之后仍委托给 sqlmock 默认的正则匹配，Expect 语义不变。
func newEpidSvcMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *string) {
	t.Helper()
	last := new(string)
	inner := sqlmock.QueryMatcherRegexp
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(
		sqlmock.QueryMatcherFunc(func(expected, actual string) error {
			*last = actual
			return inner.Match(expected, actual)
		}),
	))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	return gdb, mock, last
}

// expectSvcListActive 台账 ListActive（B 点第一步 / A 点预加载）的期望。
func expectSvcListActive(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `movie_epid_override` WHERE status = ")).
		WillReturnRows(rows)
}

// expectSvcPromotePending P1 激活（apply 第一步：pending → active）的期望（2026-11-18）。
func expectSvcPromotePending(mock sqlmock.Sqlmock, affected int64) {
	mock.ExpectExec(regexp.QuoteMeta("UPDATE movie_epid_override SET status = 'active' WHERE status = 'pending'")).
		WillReturnResult(sqlmock.NewResult(0, affected))
}

// overrideListRows 台账查询的列骨架（列序对齐迁移 000049）。
func overrideListRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "src_row_id", "ep_index", "match_key", "mid", "key_raw_name", "key_epid",
		"verdict", "new_epid", "provenance", "confidence", "reason", "evidence",
		"status", "rule_version_at", "created_at", "verified_at", "verified_by",
	})
}

// expectSvcEpidRows 派生表 (src_row_id,ep_index) 现值批量键查（B 点三元组比对面）的期望。
func expectSvcEpidRows(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT src_row_id, ep_index, epid, raw_name FROM `movie_episode_epid` WHERE src_row_id IN ")).
		WillReturnRows(rows)
}

// ① A 点核心（方案 §8.2 "派生写库前"的内存命中替换，applyToPending 纯函数）：
//   - corrected 命中 ⇒ epid 换成 new_epid；kept 命中 ⇒ 不改值只登记；
//   - 三元组对不上 ⇒ 标 stale 且值不变（宁缺勿错）；集序缩短（ep_index 不在重建行集）⇒ 同 stale；
//   - 台账条目的源行不在本批重建集合 ⇒ 完全不碰（无漂移证据，不下判）；
//   - **幂等**：同一批"重新派生后再套用"结果逐字段一致（派生与套用都是确定性的）。
func TestApplyToPending_套用幂等与三元组校验(t *testing.T) {
	ledger := &overrideLedger{byKey: map[ovKey]*entity.MovieEpidOverride{
		{9, 0}: {Id: 71, SrcRowId: 9, EpIndex: 0, KeyRawName: "第3期上", KeyEpid: epidKeyN3,
			Verdict: entity.OverrideVerdictCorrected, NewEpid: epidKeyN10, Status: entity.OverrideStatusActive},
		{9, 1}: {Id: 72, SrcRowId: 9, EpIndex: 1, KeyRawName: "第4期上", KeyEpid: epidKeyN4,
			Verdict: entity.OverrideVerdictKept, Status: entity.OverrideStatusActive},
		{9, 2}: {Id: 73, SrcRowId: 9, EpIndex: 2, KeyRawName: "第5期上", KeyEpid: epidKeyN999,
			Verdict: entity.OverrideVerdictCorrected, NewEpid: epidKeyN10, Status: entity.OverrideStatusActive},
		{9, 9}: {Id: 74, SrcRowId: 9, EpIndex: 9, KeyRawName: "第10期上", KeyEpid: epidKeyN3,
			Verdict: entity.OverrideVerdictCorrected, NewEpid: epidKeyN10, Status: entity.OverrideStatusActive},
		{8, 0}: {Id: 75, SrcRowId: 8, EpIndex: 0, KeyRawName: "第3期上", KeyEpid: epidKeyN3,
			Verdict: entity.OverrideVerdictCorrected, NewEpid: epidKeyN10, Status: entity.OverrideStatusActive},
	}}
	freshPending := func() []entity.MovieEpisodeEpid { // 模拟"刚派生完"的 pendingRows（DeriveBatch 构造口径）
		return []entity.MovieEpisodeEpid{
			{SrcRowId: 9, EpIndex: 0, MatchKey: "mk-9", Epid: epidKeyN3, RawName: "第3期上"},
			{SrcRowId: 9, EpIndex: 1, MatchKey: "mk-9", Epid: epidKeyN4, RawName: "第4期上"},
			{SrcRowId: 9, EpIndex: 2, MatchKey: "mk-9", Epid: epidKeyN5, RawName: "第5期上"},
		}
	}

	rows1 := freshPending()
	applied1, kept1, stale1 := ledger.applyToPending(rows1)
	if applied1 != 1 || kept1 != 1 {
		t.Fatalf("应套用 1 条 corrected、登记 1 条 kept, got applied=%d kept=%d", applied1, kept1)
	}
	if rows1[0].Epid != epidKeyN10 {
		t.Fatalf("corrected 命中应把 epid 换成 new_epid, got %q", rows1[0].Epid)
	}
	if rows1[1].Epid != epidKeyN4 {
		t.Fatalf("kept 命中不得改值, got %q", rows1[1].Epid)
	}
	if rows1[2].Epid != epidKeyN5 {
		t.Fatalf("三元组校验失败（key_epid 对不上）不得改值, got %q", rows1[2].Epid)
	}
	// 73=三元组对不上；74=集序缩短（ep_index=9 不在本批重建行集）；75=源行不在本批 ⇒ 不动
	if !reflect.DeepEqual(stale1, []int64{73, 74}) {
		t.Fatalf("stale 应为 [73 74]（75 的源行不在本批，无漂移证据不下判）, got %v", stale1)
	}

	// 幂等：同一批重算（fresh copy 再套用一次）⇒ 结果与第一次逐字段一致
	rows2 := freshPending()
	applied2, kept2, stale2 := ledger.applyToPending(rows2)
	if applied2 != applied1 || kept2 != kept1 || !reflect.DeepEqual(stale1, stale2) {
		t.Fatalf("重算套用结果不一致: applied %d/%d kept %d/%d stale %v/%v",
			applied1, applied2, kept1, kept2, stale1, stale2)
	}
	if !reflect.DeepEqual(rows1, rows2) {
		t.Fatalf("同批重算前后派生行不一致:\n rows1=%+v\n rows2=%+v", rows1, rows2)
	}

	// 台账未装配（nil ledger）⇒ 纯空操作（A 点安全降级，不 panic）
	nilLedger := (*overrideLedger)(nil)
	a, k, s := nilLedger.applyToPending(freshPending())
	if a != 0 || k != 0 || s != nil {
		t.Fatalf("nil ledger 应为纯空操作, got applied=%d kept=%d stale=%v", a, k, s)
	}
}

// ② B 点三元组校验失败 ⇒ 标 stale 且**不发任何派生表 UPDATE**（值不变，宁缺勿错，方案 §5 不变式）。
func TestApplyOverrides_B_三元组校验失败标stale且值不变(t *testing.T) {
	gdb, mock, _ := newEpidSvcMockDB(t)
	svc := NewEpidService(repomysql.NewEpidRepository(gdb))
	svc.SetEpidOverrideRepo(repomysql.NewEpidOverrideRepository(gdb))
	// T4 的 D3 生效开关默认关 ⇒ B/C 入口拒绝；本组测的是开关打开后的应用语义，测毕还原。
	SetEpidOverrideEnabled(true)
	t.Cleanup(func() { SetEpidOverrideEnabled(false) })
	ctx := context.Background()

	expectSvcPromotePending(mock, 0) // P1 激活（apply 第一步；无 pending ⇒ 0 行）
	expectSvcListActive(mock, overrideListRows().AddRow(
		7, 9, 2, "mk-9", 100, "第3期上", epidKeyN3,
		"corrected", epidKeyN10, "agent", "verified", "单测合成数据（非真实候选）",
		nil, "active", "", 1731380000000, 1731390000000, "tester",
	))
	// 派生表现值已漂移：epid 既不是 key_epid（规则值已变）也不是 new_epid ⇒ 三元组对不上
	expectSvcEpidRows(mock, sqlmock.NewRows([]string{"src_row_id", "ep_index", "epid", "raw_name"}).
		AddRow(9, 2, epidKeyN999, "第3期上"))
	// 只应发"标 stale"的台账 UPDATE；若实现误发派生表 UPDATE，期望耗尽 ⇒ ApplyOverrides 返回 err ⇒ 下面拦下
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `movie_epid_override` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	res, err := svc.ApplyOverrides(ctx, nil) // nil targets ⇒ 应用全部 active
	if err != nil {
		t.Fatalf("ApplyOverrides: %v", err)
	}
	if res.Considered != 1 || res.Applied != 0 || res.InEffect != 0 || res.Kept != 0 || res.Stale != 1 || res.Missing != 0 {
		t.Fatalf("三元组校验失败应只标 stale、不改值: %+v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations（是否多发了派生表 UPDATE？）: %v", err)
	}
}

// ③ B 点命中生效 + 幂等：现值=key_epid ⇒ 点改生效（只改 epid 一列，红线②）；
// 重复调用（现值已是 new_epid）⇒ InEffect 幂等命中，不再写库。
func TestApplyOverrides_B_命中生效与幂等(t *testing.T) {
	gdb, mock, lastSQL := newEpidSvcMockDB(t)
	svc := NewEpidService(repomysql.NewEpidRepository(gdb))
	svc.SetEpidOverrideRepo(repomysql.NewEpidOverrideRepository(gdb))
	// T4 的 D3 生效开关默认关 ⇒ B/C 入口拒绝；本组测的是开关打开后的应用语义，测毕还原。
	SetEpidOverrideEnabled(true)
	t.Cleanup(func() { SetEpidOverrideEnabled(false) })
	ctx := context.Background()

	targets := []OverrideTarget{{SrcRowId: 9, EpIndex: 2}}

	// 第一遍：台账 2 条 active（targets 只命中 (9,2)，(9,5) 被过滤 ⇒ Considered=1），现值 = key_epid ⇒ 点改
	expectSvcPromotePending(mock, 0) // P1 激活（apply 第一步；无 pending ⇒ 0 行）
	expectSvcListActive(mock, overrideListRows().
		AddRow(7, 9, 2, "mk-9", 100, "第3期上", epidKeyN3,
			"corrected", epidKeyN10, "agent", "verified", "单测合成数据（非真实候选）",
			nil, "active", "", 1731380000000, 1731390000000, "tester").
		AddRow(8, 9, 5, "mk-9", 100, "第8期", epidKeyN4,
			"corrected", epidKeyN10, "agent", "verified", "单测合成数据（非真实候选）",
			nil, "active", "", 1731380000000, 1731390000000, "tester"))
	expectSvcEpidRows(mock, sqlmock.NewRows([]string{"src_row_id", "ep_index", "epid", "raw_name"}).
		AddRow(9, 2, epidKeyN3, "第3期上"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `movie_episode_epid` SET `epid`=")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	res, err := svc.ApplyOverrides(ctx, targets)
	if err != nil {
		t.Fatalf("ApplyOverrides: %v", err)
	}
	if res.Considered != 1 || res.Applied != 1 || res.Stale != 0 || res.Missing != 0 {
		t.Fatalf("命中应点改生效: %+v", res)
	}
	if !strings.Contains(*lastSQL, "UPDATE `movie_episode_epid` SET `epid`=") || strings.Contains(*lastSQL, "raw_name") {
		t.Fatalf("B 只点改 epid 一列（红线②，raw_name 是真相不碰）: %s", *lastSQL)
	}

	// 第二遍（幂等）：现值已是 new_epid ⇒ InEffect 命中，不再写库（无 UPDATE 期望 ⇒ 多发即报错）
	expectSvcPromotePending(mock, 0) // P1 激活（幂等遍同样先跑激活 SQL）
	expectSvcListActive(mock, overrideListRows().
		AddRow(7, 9, 2, "mk-9", 100, "第3期上", epidKeyN3,
			"corrected", epidKeyN10, "agent", "verified", "单测合成数据（非真实候选）",
			nil, "active", "", 1731380000000, 1731390000000, "tester").
		AddRow(8, 9, 5, "mk-9", 100, "第8期", epidKeyN4,
			"corrected", epidKeyN10, "agent", "verified", "单测合成数据（非真实候选）",
			nil, "active", "", 1731380000000, 1731390000000, "tester"))
	expectSvcEpidRows(mock, sqlmock.NewRows([]string{"src_row_id", "ep_index", "epid", "raw_name"}).
		AddRow(9, 2, epidKeyN10, "第3期上"))

	res2, err := svc.ApplyOverrides(ctx, targets)
	if err != nil {
		t.Fatalf("重复 ApplyOverrides 应幂等: %v", err)
	}
	if res2.Applied != 0 || res2.InEffect != 1 {
		t.Fatalf("重复 apply 应命中 InEffect 不再写库: %+v", res2)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// ④ 未装配台账 repo ⇒ B/C 显式报错、不发任何 SQL（与 A 点"跳过"同属安全降级语义）。
func TestApplyOverrides_未装配报错(t *testing.T) {
	gdb, mock, _ := newEpidSvcMockDB(t)
	svc := NewEpidService(repomysql.NewEpidRepository(gdb))
	ctx := context.Background()
	if _, err := svc.ApplyOverrides(ctx, nil); err == nil {
		t.Fatalf("未装配台账 repo，ApplyOverrides 应报错")
	}
	if _, err := svc.RecomputeForOverrides(ctx, []OverrideTarget{{SrcRowId: 9, EpIndex: 2}}); err == nil {
		t.Fatalf("未装配台账 repo，RecomputeForOverrides 应报错")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未装配不应发任何 SQL: %v", err)
	}
}

// ⑤ T4 · D3 生效开关（默认关）：开关关着 ⇒
//   - B/C 入口**显式报错**且零 SQL（fail-closed，管理端 apply/recompute 接口拒绝）；
//   - A 点预加载 loadOverrideLedger 返回 (nil, nil) ⇒ 派生照常、不套用裁定（"A 替换不生效"）。
func TestApplyOverrides_开关关拒绝(t *testing.T) {
	gdb, mock, _ := newEpidSvcMockDB(t)
	svc := NewEpidService(repomysql.NewEpidRepository(gdb))
	svc.SetEpidOverrideRepo(repomysql.NewEpidOverrideRepository(gdb))
	t.Cleanup(func() { SetEpidOverrideEnabled(false) }) // 防前序测试残留（本测试全程保持默认关）

	if EpidOverrideEnabled() {
		t.Fatalf("D3 默认应为关")
	}
	if _, err := svc.ApplyOverrides(context.Background(), nil); err == nil {
		t.Fatalf("开关关着，ApplyOverrides 应显式拒绝")
	}
	if _, err := svc.RecomputeForOverrides(context.Background(), []OverrideTarget{{SrcRowId: 9, EpIndex: 2}}); err == nil {
		t.Fatalf("开关关着，RecomputeForOverrides 应显式拒绝（C 会整源行重写，A 缺席会把校正冲掉）")
	}
	ledger, err := svc.loadOverrideLedger(context.Background())
	if err != nil || ledger != nil {
		t.Fatalf("开关关着，loadOverrideLedger 应 (nil, nil)（A 点跳过）, got %v, %v", ledger, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("开关关着不应发任何 SQL: %v", err)
	}

	// 整源行重写路径（mode=scope）**绕过开关**：开关关着也必须装出台账（保持已生效校正；
	// 2026-11-14 mode=fill 曾在开关关态把 34 条冲回 key_epid）。期望一次 ListActive。
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `movie_epid_override` WHERE status = ")).
		WillReturnRows(overrideListRows().
			AddRow(7, 9, 2, "mk-9", 100, "第3期上", epidKeyN3,
				"corrected", epidKeyN10, "agent", "verified", "单测合成数据（非真实候选）",
				nil, "active", "", 1731380000000, 1731390000000, "tester"))
	rewLedger, err := svc.loadOverrideLedgerForRewrite(context.Background())
	if err != nil {
		t.Fatalf("开关关着，loadOverrideLedgerForRewrite 也应装载（保持已生效校正）: %v", err)
	}
	if rewLedger == nil || rewLedger.byKey[ovKey{src: 9, ep: 2}] == nil {
		t.Fatalf("开关关着，ForRewrite 应回放 active 条目 (9,2)，got %+v", rewLedger)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ForRewrite 的 ListActive 期望未满足: %v", err)
	}
}

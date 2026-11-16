package mysql

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"server/internal/domain/entity"
)

// TestUserCreate_EmptyEmailOmitsColumn 锁住 uk_users_email 与空串的坑:
// MySQL 唯一索引允许多个 NULL 但只允许一个 ”。后台建号(email 可选)若把零值 ”
// 显式写进 email 列, **第二个**无邮箱用户就会撞唯一键 → 500。
// 空邮箱必须整列省略, 让它落 NULL。
func TestUserCreate_EmptyEmailOmitsColumn(t *testing.T) {
	gdb, mock, lastSQL := newMockDBCapture(t)
	repo := NewUserRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `users`")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := repo.Create(context.Background(), &entity.User{UserName: "u1"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if strings.Contains(*lastSQL, "`email`") {
		t.Fatalf("空邮箱不应进 INSERT 列(应落 NULL): %s", *lastSQL)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestUserCreate_WithEmailIncludesColumn 有邮箱时照常写入(不能被上一条的省略逻辑带偏)。
func TestUserCreate_WithEmailIncludesColumn(t *testing.T) {
	gdb, mock, lastSQL := newMockDBCapture(t)
	repo := NewUserRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `users`")).WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	if err := repo.Create(context.Background(), &entity.User{UserName: "u2", Email: "u2@qq.com"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(*lastSQL, "`email`") {
		t.Fatalf("有邮箱时应写入 email 列: %s", *lastSQL)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestHistoryUpsert_EpidRule（P0-3, 2026-11-10；H4 修订, 2026-11-11）
// 锁住 epid 的三条覆盖规则（H4 拍板后由"两条分支"改为**单条 CASE**）：
//
//	CASE WHEN episode <> ? THEN ''      -- ① 换集 ⇒ 作废（清空）
//	     WHEN ? <> '' THEN ?            -- ② 同集且携非空 ⇒ 覆盖
//	     ELSE epid END                  -- ③ 同集且为空 ⇒ 保留（老客户端/未触发的集）
//
// 为什么必须有 ①：客户端切集时把 epid 置空是在表达"旧身份作废"；服务端若保留，就会留下
// (新集号, 旧身份) 的脏组合 ⇒ 续播按旧身份**静默命中上一集** ✗。
// 为什么必须有 ③：老客户端 / 未触发的集（裸「第N集」）不带 epid ⇒ 不得把已存身份清掉。
// ⚠️ 还要锁住**赋值顺序**：CASE 必须出现在 `episode` 之前（MySQL 从左到右求值，
// 否则 CASE 里的 `episode` 已是新值 ⇒ 分支 ① 恒假）。
func TestHistoryUpsert_EpidRule(t *testing.T) {
	gdb, mock, lastSQL := newMockDBCapture(t)
	repo := NewHistoryRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_history`")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	h := &entity.UserHistory{UserId: 1, Mid: 100, PlayFrom: "lz", Episode: 3, Progress: 10}
	if err := repo.Upsert(context.Background(), h); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	sql := *lastSQL
	i := strings.Index(sql, "ON DUPLICATE KEY UPDATE")
	if i < 0 {
		t.Fatalf("未见 ON DUPLICATE KEY UPDATE: %s", sql)
	}
	upd := sql[i:]
	// ① 三分支齐全（换集清空 / 非空覆盖 / 空保留旧值）
	for _, frag := range []string{"`epid`", "CASE WHEN episode <> ?", "ELSE epid END"} {
		if !strings.Contains(upd, frag) {
			t.Fatalf("更新子句缺片段 %q\nSQL: %s", frag, sql)
		}
	}
	// ② 赋值顺序：epid 必须早于 episode
	if strings.Index(upd, "`epid`") > strings.Index(upd, "`episode`") {
		t.Fatalf("epid 必须排在 episode 之前（ON DUPLICATE KEY UPDATE 从左到右求值）\nSQL: %s", sql)
	}
	// ③ 其余列照旧更新（不得把既有行为带偏）
	for _, col := range []string{"`play_from`", "`episode`", "`progress`", "`duration`", "`updated_at`"} {
		if !strings.Contains(upd, col) {
			t.Fatalf("更新列表缺列 %s\nSQL: %s", col, sql)
		}
	}
	// ④ INSERT 侧始终带 epid 列（列默认空串，落库即空）
	if !strings.Contains(sql[:i], "`epid`") {
		t.Fatalf("INSERT 列应含 epid\nSQL: %s", sql)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

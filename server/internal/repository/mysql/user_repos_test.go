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

// TestHistoryUpsert_EpidOnlyOverwrittenWhenNonEmpty（P0-3, 2026-11-10）
// 锁住"空 epid 不得覆盖已存身份"：
// 老客户端 / 未触发的集（裸「第N集」/E圆号）上报时不带 epid（= 空串）。
// 若把它一并放进 ON DUPLICATE KEY UPDATE，就会把该条历史已存的 epid 清空
// ⇒ 续播从"按 epid 找目标源同集"**悄悄退回** play_from + episode 下标（无声劣化）。
func TestHistoryUpsert_EpidOnlyOverwrittenWhenNonEmpty(t *testing.T) {
	cases := []struct {
		name    string
		epid    string
		wantUpd bool
	}{
		{"epid 为空 ⇒ 不进更新列表（保留已存身份）", "", false},
		{"epid 非空 ⇒ 进更新列表", "s=-|v=-|p=中上|n=3|d=-", true},
	}
	for _, c := range cases {
		gdb, mock, lastSQL := newMockDBCapture(t)
		repo := NewHistoryRepository(gdb)

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_history`")).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		h := &entity.UserHistory{UserId: 1, Mid: 100, PlayFrom: "lz", Episode: 3, Epid: c.epid, Progress: 10}
		if err := repo.Upsert(context.Background(), h); err != nil {
			t.Fatalf("%s: upsert: %v", c.name, err)
		}
		sql := *lastSQL
		i := strings.Index(sql, "ON DUPLICATE KEY UPDATE")
		if i < 0 {
			t.Fatalf("%s: 未见 ON DUPLICATE KEY UPDATE: %s", c.name, sql)
		}
		upd := sql[i:]
		if got := strings.Contains(upd, "`epid`"); got != c.wantUpd {
			t.Fatalf("%s: 更新列表含 epid=%v, want %v\nSQL: %s", c.name, got, c.wantUpd, sql)
		}
		// 其余列照旧必须更新（分支不得把既有行为带偏）
		for _, col := range []string{"`play_from`", "`episode`", "`progress`", "`duration`", "`updated_at`"} {
			if !strings.Contains(upd, col) {
				t.Fatalf("%s: 更新列表缺列 %s\nSQL: %s", c.name, col, sql)
			}
		}
		// INSERT 侧始终带 epid 列（列默认空串，落库即空）
		if !strings.Contains(sql[:i], "`epid`") {
			t.Fatalf("%s: INSERT 列应含 epid\nSQL: %s", c.name, sql)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("%s: unmet expectations: %v", c.name, err)
		}
	}
}

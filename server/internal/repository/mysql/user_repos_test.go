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

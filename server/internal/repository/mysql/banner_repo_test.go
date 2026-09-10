package mysql

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"server/internal/domain"
	"server/internal/domain/entity"
)

var bannerCountRe = regexp.QuoteMeta("SELECT count(*) FROM `banner`")

// TestBannerUpdate_SameValuesNotTreatedAsNotFound 锁住一个真实踩过的坑:
// MySQL 默认 affected_rows 是**实际变更**的行数(不是匹配行数), 所以"管理员打开编辑框、
// 一个字段都不改直接保存"(UPDATE 影响 0 行)不能被当成"记录不存在"而返回 404。
// 存在性必须单独判, 不能看 RowsAffected。
func TestBannerUpdate_SameValuesNotTreatedAsNotFound(t *testing.T) {
	gdb, mock, lastSQL := newMockDBCapture(t)
	repo := NewBannerRepository(gdb)

	mock.ExpectQuery(bannerCountRe).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectBegin() // 写操作走 gorm 默认事务
	mock.ExpectExec("UPDATE `banner`").
		WillReturnResult(sqlmock.NewResult(0, 0)) // 行存在, 但没有任何列发生变化
	mock.ExpectCommit()

	err := repo.Update(context.Background(), &entity.Banner{Id: 1, Title: "t"})
	if err != nil {
		t.Fatalf("原样保存(0 行变更)不应报错, got %v", err)
	}
	// updated_at 由 gorm 的 autoUpdateTime 自动注入, 不依赖调用方传值:
	// 早先把它放进 Updates map, 而 HTTP body 里没有该字段 → 恒为 0 落库。
	if !strings.Contains(*lastSQL, "updated_at") {
		t.Fatalf("UPDATE 应由 gorm 自动带上 updated_at(autoUpdateTime), 实际 SQL: %s", *lastSQL)
	}
	if strings.Contains(*lastSQL, "`updated_at`=0") {
		t.Fatalf("updated_at 被写成 0, autoUpdateTime 未生效: %s", *lastSQL)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestBannerUpdate_NotFound 存在性判定仍然有效: id 不存在时返回 ErrNotFound(→404),
// 且不应再发一条 UPDATE。
func TestBannerUpdate_NotFound(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewBannerRepository(gdb)

	mock.ExpectQuery(bannerCountRe).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err := repo.Update(context.Background(), &entity.Banner{Id: 99, Title: "t"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestBannerDelete_NotFound 删除走的是"实际删除行数", 0 行 = 真的不存在(与 Update 语义不同,
// RowsAffected 在 DELETE 上是可信的), 保留 404 语义。
func TestBannerDelete_NotFound(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewBannerRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `banner`").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := repo.Delete(context.Background(), 99); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

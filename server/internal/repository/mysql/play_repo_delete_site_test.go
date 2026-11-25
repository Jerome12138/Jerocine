package mysql

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMockPlayRepo(t *testing.T) (*playRepo, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm: %v", err)
	}
	return &playRepo{db: gdb}, mock
}

// 取源行 id + 最大 id（最大 id 用作删除上界，防"删完又重采"竞态）。
func TestPlayRepo_PlayIDsBySite(t *testing.T) {
	repo, mock := newMockPlayRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id` FROM `movie_play_source` WHERE site_id = ?")).
		WithArgs("src_bf").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(3)).AddRow(int64(9)).AddRow(int64(7)))

	ids, maxID, err := repo.PlayIDsBySite(context.Background(), "src_bf")
	if err != nil {
		t.Fatalf("PlayIDsBySite: %v", err)
	}
	if len(ids) != 3 || maxID != 9 {
		t.Fatalf("want 3 ids & maxID 9, got %d & %d", len(ids), maxID)
	}
}

// 派生表按 src_row_id 分块删（走 uk_row_ep），不是按 site_id 全表扫。
func TestPlayRepo_DeleteEpidByRowIDs(t *testing.T) {
	repo, mock := newMockPlayRepo(t)
	mock.ExpectExec(`DELETE FROM movie_episode_epid WHERE src_row_id IN \(\?(\,\?)*\)`).
		WithArgs(int64(1), int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 5))

	n, err := repo.DeleteEpidByRowIDs(context.Background(), []int64{1, 2})
	if err != nil {
		t.Fatalf("DeleteEpidByRowIDs: %v", err)
	}
	if n != 5 {
		t.Fatalf("want 5, got %d", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// 源表删除带上界 id <= maxID；空 siteId 不发 SQL。
func TestPlayRepo_DeletePlayBySiteUpTo(t *testing.T) {
	repo, mock := newMockPlayRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `movie_play_source` WHERE site_id = ? AND id <= ?")).
		WithArgs("src_bf", int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 123))
	mock.ExpectCommit()

	n, err := repo.DeletePlayBySiteUpTo(context.Background(), "src_bf", 9)
	if err != nil {
		t.Fatalf("DeletePlayBySiteUpTo: %v", err)
	}
	if n != 123 {
		t.Fatalf("want 123, got %d", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPlayRepo_DeletePlayBySiteUpTo_EmptySite(t *testing.T) {
	repo, mock := newMockPlayRepo(t)
	n, err := repo.DeletePlayBySiteUpTo(context.Background(), "", 0)
	if err != nil || n != 0 {
		t.Fatalf("want 0/nil, got %d/%v", n, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("不应发起 SQL: %v", err)
	}
}

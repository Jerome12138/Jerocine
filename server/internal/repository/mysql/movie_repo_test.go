package mysql

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"server/internal/domain"
)

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	return gdb, mock
}

func TestMovieGetByMid_NotFound(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewMovieRepository(gdb)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `movie`")).
		WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"mid", "name"})) // 空结果

	_, err := repo.GetByMid(context.Background(), 42)
	if !errors.Is(err, domain.ErrMovieNotFound) {
		t.Fatalf("want ErrMovieNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMovieGetByMid_Found(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewMovieRepository(gdb)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `movie`")).
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"mid", "name", "db_score"}).AddRow(7, "复仇者联盟", 8.5))

	m, err := repo.GetByMid(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m.Mid != 7 || m.Name != "复仇者联盟" || m.DbScore != 8.5 {
		t.Fatalf("scan mismatch: %+v", m)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

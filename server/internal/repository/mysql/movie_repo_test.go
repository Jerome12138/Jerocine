package mysql

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"server/internal/domain"
	"server/internal/domain/entity"
)

// newMockDBCapture 建一个 sqlmock 支撑的 gorm 句柄, 并把最近一次实际执行的 SQL
// 记进返回的 *string —— 用于断言 gorm 编译出来的语句形状, 而不只是断言我们传进去的入参。
//
// 匹配器在记录之后仍委托给 sqlmock 默认的正则匹配, 所以 ExpectQuery 里写的期望
// 依然会被校验, 只是额外多了一份"真实 SQL"可用于文本断言。
func newMockDBCapture(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *string) {
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

// assertMovieGetByMidSQL 断言 GetByMid 编译出的语句形状: 命中主键 + 过滤软删标记。
// 只查 SQL 文本、不约束参数个数 —— First() 的 LIMIT 在不同 gorm 版本下会编译成
// 字面量或占位符, 参数个数随之变化, 断言个数会让测试和 gorm 版本绑死(升级时误报)。
func assertMovieGetByMidSQL(t *testing.T, sql string) {
	t.Helper()
	for _, frag := range []string{"SELECT * FROM `movie`", "mid = ?", "deleted_at = 0"} {
		if !strings.Contains(sql, frag) {
			t.Fatalf("GetByMid SQL 缺少 %q: %s", frag, sql)
		}
	}
}

func TestMovieGetByMid_NotFound(t *testing.T) {
	gdb, mock, lastSQL := newMockDBCapture(t)
	repo := NewMovieRepository(gdb)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `movie`")).
		WillReturnRows(sqlmock.NewRows([]string{"mid", "name"})) // 空结果

	_, err := repo.GetByMid(context.Background(), 42)
	if !errors.Is(err, domain.ErrMovieNotFound) {
		t.Fatalf("want ErrMovieNotFound, got %v", err)
	}
	assertMovieGetByMidSQL(t, *lastSQL)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestMovieUpsertColsCoverEntity 保证 movieUpsertCols 与 entity.Movie 的列严格同步。
// 实体加了字段却忘了补进更新集 → 该字段永远写不进库, 是典型的静默数据错误;
// 反之更新集里残留了已删字段 → upsert 直接报未知列。两种情况都要靠测试兜住, 不能靠人眼。
func TestMovieUpsertColsCoverEntity(t *testing.T) {
	inList := make(map[string]bool, len(movieUpsertCols))
	for _, c := range movieUpsertCols {
		if inList[c] {
			t.Fatalf("movieUpsertCols 有重复列: %s", c)
		}
		inList[c] = true
	}
	typ := reflect.TypeOf(entity.Movie{})
	for i := 0; i < typ.NumField(); i++ {
		col := strings.TrimPrefix(strings.Split(typ.Field(i).Tag.Get("gorm"), ";")[0], "column:")
		if col == "" {
			t.Fatalf("entity.Movie 字段 %s 缺少 gorm column 标签", typ.Field(i).Name)
		}
		want := !movieUpsertExclude[col]
		if got := inList[col]; got != want {
			t.Fatalf("列 %s 在 movieUpsertCols 中=%v, 期望=%v (请同步 movieUpsertCols / movieUpsertExclude)", col, got, want)
		}
	}
}

func TestMovieGetByMid_Found(t *testing.T) {
	gdb, mock, lastSQL := newMockDBCapture(t)
	repo := NewMovieRepository(gdb)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `movie`")).
		WillReturnRows(sqlmock.NewRows([]string{"mid", "name", "db_score"}).AddRow(7, "复仇者联盟", 8.5))

	m, err := repo.GetByMid(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m.Mid != 7 || m.Name != "复仇者联盟" || m.DbScore != 8.5 {
		t.Fatalf("scan mismatch: %+v", m)
	}
	assertMovieGetByMidSQL(t, *lastSQL)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestMovieBatchUpsert_SQLShape 锁定采集回写的最终 SQL 形状。
// 断言的是 gorm 编译出来的语句, 而不是我们传的入参 —— 因为"已删影片不会被重推复活"
// 这件事取决于 UPDATE 子句里到底列了哪些列, gorm 升级改了 upsert 的编译方式时这里会先炸。
func TestMovieBatchUpsert_SQLShape(t *testing.T) {
	gdb, mock, lastSQL := newMockDBCapture(t)
	repo := NewMovieRepository(gdb)
	mock.ExpectBegin() // CreateInBatches 走事务
	mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.BatchUpsert(context.Background(), []entity.Movie{{Mid: 1, Name: "x"}}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	sql := *lastSQL
	idx := strings.Index(sql, "ON DUPLICATE KEY UPDATE")
	if idx < 0 {
		t.Fatalf("期望冲突时走 ON DUPLICATE KEY UPDATE: %s", sql)
	}
	update := sql[idx:]
	t.Logf("upsert 更新子句: %s", update)
	// INSERT 的列清单里必然有 deleted_at/created_at(新行还是要写入的), 所以只在 UPDATE 子句里查它们的出现。
	for _, banned := range []string{"deleted_at", "created_at"} {
		if strings.Contains(update, banned) {
			t.Fatalf("upsert 不应更新 %s(会破坏软删标记/首存时间): %s", banned, update)
		}
	}
	if !strings.Contains(update, "name") {
		t.Fatalf("内容列应当参与更新: %s", update)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

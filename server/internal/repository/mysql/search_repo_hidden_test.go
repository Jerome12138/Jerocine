package mysql

import (
	"context"
	"database/sql/driver"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// expectHiddenCats 预置 hiddenCats 的 category 现查: rows 为 (id, pid, show)。
func expectHiddenCats(mock sqlmock.Sqlmock, rows ...[]driver.Value) {
	rs := sqlmock.NewRows([]string{"id", "pid", "show"})
	for _, r := range rows {
		rs.AddRow(r...)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, pid, `show` FROM `category`")).WillReturnRows(rs)
}

// TestHiddenCatFilter_TopHotAll 隐藏分类(show=0)的影片必须从全站热榜剔除:
// SQL 注入 pid NOT IN(顶级) + cid NOT IN(子分类) 双条件。
func TestHiddenCatFilter_TopHotAll(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	expectHiddenCats(mock,
		[]driver.Value{int64(5), int64(0), false},  // 顶级隐藏
		[]driver.Value{int64(9), int64(5), false},  // 子分类隐藏
		[]driver.Value{int64(3), int64(0), true},   // 正常顶级
	)
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	if _, err := repo.TopHotAll(context.Background(), 10); err != nil {
		t.Fatalf("TopHotAll: %v", err)
	}
	for _, frag := range []string{"pid NOT IN", "cid NOT IN"} {
		if !strings.Contains(*last, frag) {
			t.Fatalf("全站热榜 SQL 缺少 %q(隐藏分类影片会漏出): %s", frag, *last)
		}
	}
}

// TestHiddenCatFilter_NoHidden 无隐藏分类时不得引入多余条件(零开销路径)。
func TestHiddenCatFilter_NoHidden(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	expectHiddenCats(mock,
		[]driver.Value{int64(3), int64(0), true},
		[]driver.Value{int64(7), int64(3), true},
	)
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	if _, err := repo.TopHotAll(context.Background(), 10); err != nil {
		t.Fatalf("TopHotAll: %v", err)
	}
	if strings.Contains(*last, "NOT IN") {
		t.Fatalf("无隐藏分类不应出现 NOT IN: %s", *last)
	}
}

// TestHiddenCatFilter_AdminBypass 后台口径(回收站/全部)必须看到隐藏分类影片:
// 不发 category 现查, 也不注入 NOT IN。
func TestHiddenCatFilter_AdminBypass(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	mock.ExpectQuery("SELECT (.+) FROM `movie_search`"). // 仅此两条: 无 category 现查
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	if _, _, err := repo.Filter(context.Background(), repository.FilterSpec{Deleted: repository.DeletedInclude}, repository.Page{}); err != nil {
		t.Fatalf("Filter(全部): %v", err)
	}
	if strings.Contains(*last, "NOT IN") {
		t.Fatalf("后台口径不应注入隐藏过滤: %s", *last)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("后台口径不应现查 category(出现多余查询): %v", err)
	}
}

// TestHiddenCatFilter_SearchKeyword 公开搜索同样剔除隐藏分类(搜索不能成为漏出通道)。
func TestHiddenCatFilter_SearchKeyword(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	expectHiddenCats(mock, []driver.Value{int64(5), int64(0), false})
	mock.ExpectQuery("SELECT count(.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	if _, _, err := repo.SearchKeyword(context.Background(), "abc", repository.DeletedExclude, repository.Page{}); err != nil {
		t.Fatalf("SearchKeyword: %v", err)
	}
	if !strings.Contains(*last, "pid NOT IN") {
		t.Fatalf("搜索 SQL 缺少隐藏过滤: %s", *last)
	}
}

// TestCategoryBatchUpsertPreservesShow 采集覆盖/重采回写分类时**不得覆盖本地 show**:
// 源站 parse 写死 Show:true, 全字段覆盖曾把管理员手工隐藏的分类重置回显示。
// 锁 SQL 形状: ON DUPLICATE KEY UPDATE 只含结构列, 不含 show。
func TestCategoryBatchUpsertPreservesShow(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewCategoryRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `category`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	list := []entity.Category{{Id: 5, Pid: 0, Name: "电影", Show: true, Sort: 1}}
	if err := repo.BatchUpsert(context.Background(), list); err != nil {
		t.Fatalf("BatchUpsert: %v", err)
	}
	sql := *last
	if !strings.Contains(sql, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("应为 upsert 语句: %s", sql)
	}
	for _, frag := range []string{"pid", "name", "sort"} {
		idx := strings.Index(sql, "ON DUPLICATE KEY UPDATE")
		if !strings.Contains(sql[idx:], frag) {
			t.Fatalf("冲突更新缺结构列 %q: %s", frag, sql)
		}
	}
	if idx := strings.Index(sql, "ON DUPLICATE KEY UPDATE"); strings.Contains(sql[idx:], "`show`") {
		t.Fatalf("冲突更新不得覆盖 show 列: %s", sql)
	}
}

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

// expectHiddenCats 预置 hiddenCats 的 category 现查: rows 为 (id, pid, film_visible)。
func expectHiddenCats(mock sqlmock.Sqlmock, rows ...[]driver.Value) {
	rs := sqlmock.NewRows([]string{"id", "pid", "film_visible"})
	for _, r := range rows {
		rs.AddRow(r...)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, pid, film_visible FROM `category`")).WillReturnRows(rs)
}

// TestHiddenCatFilter_TopHotAll 影片不可见分类(film_visible=0)的影片必须从全站热榜剔除:
// SQL 注入 pid NOT IN(顶级) + cid NOT IN(子分类) 双条件。
func TestHiddenCatFilter_TopHotAll(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	expectHiddenCats(mock,
		[]driver.Value{int64(5), int64(0), false},  // 顶级影片不可见
		[]driver.Value{int64(9), int64(5), false},  // 子分类影片不可见
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

// TestHiddenCatFilter_MenuShowOnly 菜单隐藏(show=0)但影片可见(film_visible=1)的分类:
// 影片照常出现在公开列表 —— 菜单显隐与影片可见性是两个独立开关, 不得再联动。
func TestHiddenCatFilter_MenuShowOnly(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	expectHiddenCats(mock,
		[]driver.Value{int64(3), int64(0), true},  // 菜单藏了(show=0 由管理端另行处理)但影片可见
		[]driver.Value{int64(7), int64(3), true},
	)
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	if _, err := repo.TopHotAll(context.Background(), 10); err != nil {
		t.Fatalf("TopHotAll: %v", err)
	}
	if strings.Contains(*last, "NOT IN") {
		t.Fatalf("影片可见的分类不应被剔除(菜单显隐不联动影片): %s", *last)
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
		t.Fatalf("无影片不可见分类不应出现 NOT IN: %s", *last)
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

	if _, _, err := repo.SearchKeyword(context.Background(), "abc", repository.DeletedExclude, false, repository.Page{}); err != nil {
		t.Fatalf("SearchKeyword: %v", err)
	}
	if !strings.Contains(*last, "pid NOT IN") {
		t.Fatalf("搜索 SQL 缺少隐藏过滤: %s", *last)
	}
}

// TestHiddenCatFilter_SearchKeyword_ManageBypass 管理端口径(includeHidden=true)的关键字搜索:
// 不发 category 现查, 也不注入 NOT IN —— 后台要能搜到所有影片(含隐藏分类的)。
func TestHiddenCatFilter_SearchKeyword_ManageBypass(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	mock.ExpectQuery("SELECT count(.+) FROM `movie_search`"). // 仅此两条: 无 category 现查
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	if _, _, err := repo.SearchKeyword(context.Background(), "abc", repository.DeletedExclude, true, repository.Page{}); err != nil {
		t.Fatalf("SearchKeyword(manage): %v", err)
	}
	if strings.Contains(*last, "NOT IN") {
		t.Fatalf("管理口径搜索不应注入隐藏过滤: %s", *last)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("管理口径搜索不应现查 category: %v", err)
	}
}

// TestHiddenCatFilter_ManageIncludeHidden 管理端口径(IncludeHidden=true, 在架口径)的列表:
// DeletedExclude 本会走 hiddenCats, 但 IncludeHidden 放开 —— 不现查 category、不注入 NOT IN,
// 后台「在架」tab 必须能看到/搜到影片不可见分类下的影片(管理后台不受可见性影响)。
func TestHiddenCatFilter_ManageIncludeHidden(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	mock.ExpectQuery("SELECT (.+) FROM `movie_search`"). // 仅此两条: 无 category 现查
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	spec := repository.FilterSpec{Deleted: repository.DeletedExclude, IncludeHidden: true}
	if _, _, err := repo.Filter(context.Background(), spec, repository.Page{}); err != nil {
		t.Fatalf("Filter(manage): %v", err)
	}
	if strings.Contains(*last, "NOT IN") {
		t.Fatalf("管理口径在架列表不应注入隐藏过滤: %s", *last)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("管理口径不应现查 category(出现多余查询): %v", err)
	}
}

// TestFilter_CatIDs 分类多选(后台级联筛选)的 SQL 形状: (pid IN ids OR cid IN ids) ——
// 勾顶级命中子分类影片(pid=父顶级), 勾子级/直挂命中 cid, 单条件覆盖两种归属。
func TestFilter_CatIDs(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	mock.ExpectQuery("SELECT count(.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	spec := repository.FilterSpec{Deleted: repository.DeletedExclude, IncludeHidden: true, CatIDs: []int64{5, 9}}
	if _, _, err := repo.Filter(context.Background(), spec, repository.Page{}); err != nil {
		t.Fatalf("Filter(catIds): %v", err)
	}
	for _, frag := range []string{"pid IN", "OR cid IN"} {
		if !strings.Contains(*last, frag) {
			t.Fatalf("分类多选 SQL 缺少 %q: %s", frag, *last)
		}
	}
}

// TestFilter_CatIDs_IntersectHidden 公开口径下分类多选与隐藏过滤**叠加**:
// excludeHidden 的 NOT IN 与 CatIDs 的 IN 共存 —— 勾中隐藏分类时影片仍被剔除
// (公开端不存在"借多选把隐藏分类影片翻出来"的漏出通道)。
func TestFilter_CatIDs_IntersectHidden(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	expectHiddenCats(mock, []driver.Value{int64(5), int64(0), false})
	mock.ExpectQuery("SELECT count(.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	spec := repository.FilterSpec{Deleted: repository.DeletedExclude, CatIDs: []int64{5, 3}}
	if _, _, err := repo.Filter(context.Background(), spec, repository.Page{}); err != nil {
		t.Fatalf("Filter(catIds+hidden): %v", err)
	}
	if !strings.Contains(*last, "pid NOT IN") || !strings.Contains(*last, "pid IN") {
		t.Fatalf("公开口径多选应同时含隐藏剔除与分类匹配: %s", *last)
	}
}

// TestCategoryBatchUpsertPreservesShow 采集覆盖/重采回写分类时**不得覆盖本地 show / film_visible**:
// 源站 parse 写死 Show/FilmVisible:true, 全字段覆盖曾把管理员手工隐藏的分类重置回显示。
// 锁 SQL 形状: ON DUPLICATE KEY UPDATE 只含结构列, 不含 show / film_visible。
func TestCategoryBatchUpsertPreservesShow(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewCategoryRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `category`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	list := []entity.Category{{Id: 5, Pid: 0, Name: "电影", Show: true, FilmVisible: true, Sort: 1}}
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
	idx := strings.Index(sql, "ON DUPLICATE KEY UPDATE")
	for _, col := range []string{"`show`", "film_visible"} {
		if strings.Contains(sql[idx:], col) {
			t.Fatalf("冲突更新不得覆盖 %s 列: %s", col, sql)
		}
	}
}

// TestCountFilmsByCategory 分类影片计数: pid/cid 两查 GROUP BY, 软删排除由 applyDeleted 保证。
func TestCountFilmsByCategory(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	mock.ExpectQuery("SELECT pid AS grp, COUNT\\(\\*\\) AS cnt FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"grp", "cnt"}).AddRow(int64(3), int64(12)))
	mock.ExpectQuery("SELECT cid AS grp, COUNT\\(\\*\\) AS cnt FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"grp", "cnt"}).AddRow(int64(9), int64(7)))

	pids, cids, err := repo.CountFilmsByCategory(context.Background())
	if err != nil {
		t.Fatalf("CountFilmsByCategory: %v", err)
	}
	if pids[3] != 12 || cids[9] != 7 {
		t.Fatalf("计数不符: pids=%v cids=%v", pids, cids)
	}
}

// TestHiddenCats_CidCoversTopHidden 不可见顶级分类的 id 必须同时进 cid 排除集:
// XML 源/maccms type_id_1=0 写法下直挂影片 pid=0、只有 cid=顶级id, 若 cid 集只含
// 子分类, 这些影片会从隐藏顶级分类漏出(pid=0 不命中 pid NOT IN)。
func TestHiddenCats_CidCoversTopHidden(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	expectHiddenCats(mock,
		[]driver.Value{int64(5), int64(0), false}, // 顶级影片不可见
		[]driver.Value{int64(9), int64(5), false}, // 子分类影片不可见
		[]driver.Value{int64(3), int64(0), true},  // 正常顶级
	)

	pids, cids := repo.(*searchRepo).hiddenCats(context.Background())
	if len(pids) != 1 || pids[0] != 5 {
		t.Fatalf("pid 排除集应只含顶级不可见 id [5], got %v", pids)
	}
	if len(cids) != 2 || cids[0] != 5 || cids[1] != 9 {
		t.Fatalf("cid 排除集应含全部不可见 id(顶级5+子级9), got %v", cids)
	}
}

// TestPidFilter_DirectFilmORCid 一级分类列表口径 = (pid=X OR cid=X):
// 直挂影片(pid=0, cid=X)只按 pid 查会整类漏光 —— AI漫剧计数 0 但搜得到的根因之一。
func TestPidFilter_DirectFilmORCid(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	expectHiddenCats(mock)
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	if _, err := repo.TopByPidSorted(context.Background(), 5, repository.SortRecent, 10); err != nil {
		t.Fatalf("TopByPidSorted: %v", err)
	}
	if !strings.Contains(*last, "pid = ? OR cid = ?") && !strings.Contains(*last, "pid = ? OR cid=?") {
		t.Fatalf("TopByPidSorted 缺少 (pid=X OR cid=X) 口径: %s", *last)
	}

	expectHiddenCats(mock)
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))
	if _, err := repo.TopScoreByPid(context.Background(), 5, 10); err != nil {
		t.Fatalf("TopScoreByPid: %v", err)
	}
	if !strings.Contains(*last, "OR cid") {
		t.Fatalf("TopScoreByPid 缺少 (pid=X OR cid=X) 口径: %s", *last)
	}

	expectHiddenCats(mock)
	mock.ExpectQuery("SELECT count(.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT (.+) FROM `movie_search`").
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))
	if _, _, err := repo.Filter(context.Background(), repository.FilterSpec{Pid: 5}, repository.Page{}); err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if !strings.Contains(*last, "OR cid") {
		t.Fatalf("Filter 缺少 (pid=X OR cid=X) 口径: %s", *last)
	}
}

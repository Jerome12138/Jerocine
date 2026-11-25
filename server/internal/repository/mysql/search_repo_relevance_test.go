package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"server/internal/domain/repository"
)

// 回归保护（2026-11-25 线上缺陷）：相关性排序必须**真的渲染进最终 SQL**。
//
// 背景：原先写成 `fq.Order(clause.Expr{...})` —— GORM 的 Order() 只接受字符串或
// clause.OrderBy，直接给 clause.Expr 会被**静默忽略**（不报错、SQL 里没有 ORDER BY 片段）。
// 当时的单测 `TestRelevanceOrderExprVarsOrder` 只校验了 Expr 对象的 Vars 顺序，
// **没有校验它进入最终 SQL**，所以线上"搜超人首条不是《超人》"无人发现。
//
// 本用例直接断言捕获到的真实 SQL 含 CASE —— 一旦有人改回 Order()，立刻红。
func TestSearchKeyword_RelevanceOrderRendered(t *testing.T) {
	gdb, mock, last := newMockDBCapture(t)
	repo := NewSearchRepository(gdb)

	// 顺序：① 先查影片不可见分类 ② 再 count ③ 最后取数据（后者只关心 SQL 文本，返回空集）
	mock.ExpectQuery(`(?s)^SELECT id, pid, film_visible FROM .category.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "pid", "film_visible"}))
	mock.ExpectQuery(`(?s)^SELECT count\(.*`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(`(?s)^SELECT .* FROM .movie_search.`).
		WillReturnRows(sqlmock.NewRows([]string{"mid"}))

	// sort="" ⇒ orderForSpec 返回 (\"\", false) ⇒ 必须走相关性排序分支
	if _, _, err := repo.SearchKeyword(
		context.Background(), "超人", repository.DeletedExclude, false, "", repository.Page{},
	); err != nil {
		t.Fatalf("SearchKeyword: %v", err)
	}

	sql := *last
	if !strings.Contains(sql, "CASE") || !strings.Contains(sql, "name = ") {
		t.Fatalf("相关性排序未渲染进 SQL（GORM Order() 会静默忽略 clause.Expr）\n实际 SQL: %s", sql)
	}
}

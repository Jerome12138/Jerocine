package mysql

import (
	"fmt"
	"strings"
	"testing"
)

// 钉住相关性排序表达式的**占位符 ↔ Vars 顺序**。
//
// 背景（2026-11-23 审查 P1-1）：`clause.Expr` 的 Vars 顺序一旦与 SQL 里的 `?` 错位，
// MySQL 不会报错，只会静默按错误的字段比较 —— 表现为"排序莫名不对"，极难排查。
func TestRelevanceOrderExprVarsOrder(t *testing.T) {
	const kw = "cx"
	e := relevanceOrderExpr(kw)

	if n := strings.Count(e.SQL, "?"); n != len(e.Vars) {
		t.Fatalf("占位符数量 %d ≠ Vars 数量 %d（必然错位）", n, len(e.Vars))
	}
	if len(e.Vars) != 5 {
		t.Fatalf("want 5 vars, got %d", len(e.Vars))
	}

	// 逐位断言：精确名 → 缩写全等 → 名前缀 → 名包含 → 缩写前缀
	want := []interface{}{"cx", "CX", "cx%", "%cx%", "CX%"}
	for i, w := range want {
		if e.Vars[i] != w {
			t.Errorf("vars[%d] = %v, want %v", i, e.Vars[i], w)
		}
	}

	// 分支语义：精确名与缩写全等必须存在
	for _, frag := range []string{"name = ?", "name_pinyin = ?", "name LIKE ?", "name_pinyin LIKE ?"} {
		if !strings.Contains(e.SQL, frag) {
			t.Errorf("SQL 缺少分支片段 %q", frag)
		}
	}

	// 权重：THEN 0..4 各一次 + 末尾 ELSE 5（不是 THEN 5）
	for i := 0; i <= 4; i++ {
		marker := fmt.Sprintf("THEN %d", i)
		if c := strings.Count(e.SQL, marker); c != 1 {
			t.Errorf("%q 出现 %d 次，应为 1 次", marker, c)
		}
	}
	if c := strings.Count(e.SQL, "ELSE 5"); c != 1 {
		t.Errorf("\"ELSE 5\" 出现 %d 次，应为 1 次", c)
	}
}

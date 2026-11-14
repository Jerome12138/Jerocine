package mysql

import (
	"testing"

	"gorm.io/gorm/clause"
)

// TestPlaySourceUpsertUpdates_SourceMetaGuard 防呆（000052，2026-11-14）：
// 上游元数据两列在 upsert 更新集里必须是 **`COALESCE(VALUES(col), col)`** ——"有值才覆盖、空值不冲掉旧值"。
//
// 为什么必须钉住：一旦退化成裸 `VALUES(col)`，上游偶发返回空片名/无豆瓣 id 就会把**已采到的值冲成 NULL**，
// 而这两列正是"双写副本判定做到 100%"的唯一依据 ⇒ 判定静默退回 ≈69.8% 且**不可自愈**（存量已不回填）。
//
// 同时钉住：① 两列都进了更新集（漏了 ⇒ 重采永不回填）；② 顺序 `dup_copy < source_name < source_douban_id < updated_at`
// （MySQL `ON DUPLICATE KEY UPDATE` 从左到右求值 ⇒ 顺序要稳定可读，且不能用 map 版丢失顺序）；
// ③ 原有列（含判重列 `content_hash`）没被顺手删掉（丢了 ⇒ 下次重采仍判"变了" ⇒ 白写，见 000051）。
func TestPlaySourceUpsertUpdates_SourceMetaGuard(t *testing.T) {
	set := playSourceUpsertUpdates()
	expr := make(map[string]string, len(set))
	var order []string
	for _, a := range set {
		order = append(order, a.Column.Name)
		if v, ok := a.Value.(clause.Expr); ok {
			expr[a.Column.Name] = v.SQL
		} else {
			expr[a.Column.Name] = "?"
		}
	}

	// ① 两列都在，且带 COALESCE 守卫
	for _, c := range []string{"source_name", "source_douban_id"} {
		got, ok := expr[c]
		if !ok {
			t.Fatalf("更新集缺少 %s（000052 两列必须纳入，否则重采不回填）", c)
		}
		want := "COALESCE(VALUES(" + c + "), " + c + ")"
		if got != want {
			t.Errorf("%s 的更新表达式 = %q，want %q（裸 VALUES 会把已知值冲成 NULL）", c, got, want)
		}
	}

	// ② 顺序：dup_copy < source_name < source_douban_id < updated_at
	idx := func(name string) int {
		for i, n := range order {
			if n == name {
				return i
			}
		}
		return -1
	}
	if !(idx("dup_copy") < idx("source_name") && idx("source_name") < idx("source_douban_id") && idx("source_douban_id") < idx("updated_at")) {
		t.Errorf("更新列顺序不对（应 dup_copy < source_name < source_douban_id < updated_at）：%v", order)
	}

	// ③ 原有列齐全
	for _, c := range []string{"mid", "source_vod_id", "episodes_json", "ep_hash", "content_hash", "updated_at"} {
		if _, ok := expr[c]; !ok {
			t.Errorf("更新集丢了原有列 %s", c)
		}
	}
}

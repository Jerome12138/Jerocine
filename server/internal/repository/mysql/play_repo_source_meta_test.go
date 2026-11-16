package mysql

import (
	"testing"

	"gorm.io/gorm/clause"

	"server/internal/domain/entity"
)

// TestPlaySourceUpsertUpdates_SourceMetaGuard 防呆（000052，2026-11-14）：
// 上游元数据两列在 upsert 更新集里必须是 **`COALESCE(VALUES(col), col)`** ——"有值才覆盖、空值不冲掉旧值"。
//
// 为什么必须钉住：一旦退化成裸 `VALUES(col)`，上游偶发返回空片名/无豆瓣 id 就会把**已采到的值冲成 NULL**，
// 而这两列正是"双写副本判定做到 100%"的唯一依据 ⇒ 判定静默退回 ≈69.8% 且**不可自愈**。
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

	for _, c := range []string{"mid", "source_vod_id", "episodes_json", "ep_hash", "content_hash", "updated_at"} {
		if _, ok := expr[c]; !ok {
			t.Errorf("更新集丢了原有列 %s", c)
		}
	}
}

// TestShouldSkipWrite_SourceMetaSelfHeal 防呆（000052）：判重口径的**两条单向自愈**必须放行，
// 且反向不能算"变了"（否则上游偶发缺字段会造成无谓整行重写）。
//
//   - 存量空、新行有值 ⇒ **必写**（定向补采 / 自然补采能把上游原文补进来；填上即止）
//   - 存量有值、新行 NULL ⇒ 跳过（upsert 的 COALESCE 本来也保旧值，写下去没意义）
//   - 其余四个相等性判据（mid / source_vod_id / dup_copy / content_hash 且非空）任一不同 ⇒ 必写
func TestShouldSkipWrite_SourceMetaSelfHeal(t *testing.T) {
	name, dbid, mid := "某某剧", int64(456780), int64(9)
	withMeta := entity.MoviePlaySource{
		SiteId: "src_sn", MatchKey: "1048241256", PlayFrom: "线路1",
		Mid: &mid, SourceVodID: 123, DupCopy: 0, ContentHash: "aabbccddeeff0011",
		SourceName: &name, SourceDoubanID: &dbid,
	}
	noMeta := withMeta
	noMeta.SourceName, noMeta.SourceDoubanID = nil, nil

	// 基线：完全一致 ⇒ 跳过
	if !shouldSkipWrite(withMeta, withMeta) {
		t.Error("四判据全等且两列已有值 ⇒ 应跳过")
	}
	// ① 存量空、新行有值 ⇒ 必写（自愈；只补其中一列也算）
	if shouldSkipWrite(noMeta, withMeta) {
		t.Error("存量 NULL + 新行有值 ⇒ 必须写（000052 自愈，是定向补采的落地条件）")
	}
	half := noMeta
	half.SourceName = &name
	if shouldSkipWrite(noMeta, half) {
		t.Error("只补了 source_name 也必须写")
	}
	if shouldSkipWrite(noMeta, func() entity.MoviePlaySource { x := noMeta; x.SourceDoubanID = &dbid; return x }()) {
		t.Error("只补了 source_douban_id 也必须写")
	}
	// ② 存量有值、新行 NULL ⇒ 跳过（反向不算"变了"）
	if !shouldSkipWrite(withMeta, noMeta) {
		t.Error("存量有值 + 新行 NULL ⇒ 应跳过（COALESCE 本来也保旧值）")
	}
	// ③ content_hash 存量空 ⇒ 必写（000051 自愈）
	chEmpty := withMeta
	chEmpty.ContentHash = ""
	if shouldSkipWrite(chEmpty, withMeta) {
		t.Error("存量 content_hash 为空 ⇒ 必须写一次自愈")
	}
	// ④ 四个相等性判据任一不同 ⇒ 必写
	other := withMeta
	other.ContentHash = "ffffffffffffffff"
	if shouldSkipWrite(withMeta, other) {
		t.Error("content_hash 变了 ⇒ 必须写")
	}
	dup := withMeta
	dup.DupCopy = 1
	if shouldSkipWrite(withMeta, dup) {
		t.Error("dup_copy 变了 ⇒ 必须写（写侧确定值要覆盖存量脚本的近似值）")
	}
	vod := withMeta
	vod.SourceVodID = 456
	if shouldSkipWrite(withMeta, vod) {
		t.Error("source_vod_id 变了 ⇒ 必须写（vod0 修复型写入）")
	}
	mid2 := int64(10)
	other2 := withMeta
	other2.Mid = &mid2
	if shouldSkipWrite(withMeta, other2) {
		t.Error("mid 变了 ⇒ 必须写")
	}
}

package danmu

import "testing"

// TestEpidsForSourceRow_BareNumeric（R13，2026-11-14 服务端口径）：
// 裸数字集名纳入 `n`（`1`/`02`/`001` ⇒ 1/2/1），来源记 `self`（数字本来就写在集名里）。
//
// ⚠️ 关键回归：守卫是「行内 **distinct** 裸数字名 ≥2」——**不能用"行内 ≥2 条"**：
// 双写（片名键 + 豆瓣键两份）会让同一集名在同一行出现两次 ⇒ 该条件恒成立 ✗（实测踩过）。
func TestEpidsForSourceRow_BareNumeric(t *testing.T) {
	// ① 成串裸数字 ⇒ 各自 n（来源 self）
	got := EpidsForSourceRow([]string{"1", "2", "3"})
	want := []string{"s=-|v=-|p=-|n=1|d=-", "s=-|v=-|p=-|n=2|d=-", "s=-|v=-|p=-|n=3|d=-"}
	for i, w := range want {
		if got[i][0] != w || got[i][1] != EpKeySourceSelf {
			t.Errorf("成串裸数字第 %d 条 = %v, want %s/self", i, got[i], w)
		}
	}
	// ② 带空白 / 补零（按十进制）
	got = EpidsForSourceRow([]string{" 02", " 03"})
	if got[0][0] != "s=-|v=-|p=-|n=2|d=-" || got[1][0] != "s=-|v=-|p=-|n=3|d=-" {
		t.Errorf("带空白裸数字应解析为 n=2/3，得 %v", got)
	}
	got = EpidsForSourceRow([]string{"001", "012"})
	if got[0][0] != "s=-|v=-|p=-|n=1|d=-" || got[1][0] != "s=-|v=-|p=-|n=12|d=-" {
		t.Errorf("补零裸数字应按十进制，得 %v", got)
	}
	// ③ **双写重复**（同一集名出现两次）⇒ distinct=1 ⇒ 不补（这条是防呆重点）
	got = EpidsForSourceRow([]string{"1", "1"})
	for i, g := range got {
		if g[0] != "" {
			t.Errorf("双写重复（distinct=1）第 %d 条不该补身份，得 %v", i, g)
		}
	}
	// ④ 孤立单条 ⇒ 不补（可能是线路/版本号）
	if got := EpidsForSourceRow([]string{"1"}); got[0][0] != "" {
		t.Errorf("孤立裸数字不该补身份，得 %v", got[0])
	}
	// ⑤ 5 位以上 ⇒ 不补（6 位归 R11 日期、5 位含义不明）
	got = EpidsForSourceRow([]string{"12345", "12346"})
	for i, g := range got {
		if g[0] != "" {
			t.Errorf("5 位数字第 %d 条不该补身份，得 %v", i, g)
		}
	}
	// ⑥ 8 位日期串 ⇒ 走 R11（d 段），不算裸数字
	got = EpidsForSourceRow([]string{"20240103", "20240104"})
	if got[0][0] != "s=-|v=-|p=-|n=-|d=20240103" || got[1][0] != "s=-|v=-|p=-|n=-|d=20240104" {
		t.Errorf("8 位日期应走 R11 的 d 段，得 %v", got)
	}
	// ⑦ 与其它形态混排：成串裸数字仍补（distinct ≥2 ✓），非数字那条不受影响
	got = EpidsForSourceRow([]string{"1", "2", "正片"})
	if got[0][0] == "" || got[1][0] == "" {
		t.Errorf("与其它形态混排时，成串裸数字仍应补，得 %v", got)
	}
	// ⑧ 单集裸数字**不**走 R12（含数字 ⇒ 被 hasASCIIDigit 排除）⇒ 仍无身份（宁缺勿错）
	if got := EpidsForSourceRow([]string{"1"}); got[0][1] == EpKeySourceSingle {
		t.Errorf("单集裸数字不该走 R12，得 %v", got[0])
	}
}

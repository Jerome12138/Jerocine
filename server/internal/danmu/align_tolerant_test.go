package danmu

import (
	"fmt"
	"testing"
)

// ===== M4 错位容忍判据的三种典型行为（用户 2026-11-16 拍板口径）=====

func seq(n int) []string {
	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, fmt.Sprintf("n%d", i))
	}
	return out
}

func TestTolerantAligned_Archetypes(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"完全相同", seq(5), seq(5), true},
		{"整体偏移：头部多一集", seq(4), append([]string{"n0"}, seq(4)...), true},
		{"整体偏移：尾部多一集", seq(4), append(seq(4), "n5"), true},
		{"整体偏移：B 比 A 晚一期起步", seq(4), []string{"n2", "n3", "n4", "n5"}, true},
		{"中间缺失（B 缺第3集）", []string{"n1", "n2", "n3", "n4"}, []string{"n1", "n2", "n4"}, true},
		{"中间缺失（A 缺第3集）", []string{"n1", "n2", "n4"}, []string{"n1", "n2", "n3", "n4"}, true},
		// 用户明确要挡的误判：两边"各缺一集"但互不重合 ⇒ 同位置替换 ⇒ 不齐
		{"A缺x/B缺y（同位置替换）", []string{"n1", "n2", "n3"}, []string{"n1", "n2", "n4"}, false},
		// ⚠️ 已知取舍：**同一集合、顺序不同** 判为"已对齐"（集全在、只是顺序不同；
		// 差异步数 = 2，正好在 len=3 的额度内）。要更严就得收紧额度，但会误伤"头部多一集 + 尾部多一集"
		// 这种真实偏移形态（它天然产生 2 步差异）—— 按用户"多出/缺失算对齐"的口径，取宽松侧。
		{"同集合顺序不同（取舍：判对齐）", []string{"n1", "n2", "n3"}, []string{"n1", "n3", "n2"}, true},
		{"完全不同", []string{"n1", "n2", "n3"}, []string{"x1", "x2", "x3"}, false},
		{"空序列不可判", nil, seq(3), false},
		{"空序列不可判2", seq(3), nil, false},
		{"单条相同", []string{"n1"}, []string{"n1"}, true},
		{"单条不同", []string{"n1"}, []string{"n2"}, false},
	}
	for _, tc := range cases {
		if got := TolerantAligned(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: TolerantAligned = %v, want %v", tc.name, got, tc.want)
		}
		// 对称性：判据不该依赖入参顺序
		if got := TolerantAligned(tc.b, tc.a); got != tc.want {
			t.Errorf("%s: 对称调用 = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// 额度守卫：差异超过 `max(2, ceil(5%×len))` 就不该判对齐（防"几乎完全不同但恰好相容"）。
func TestTolerantAligned_Budget(t *testing.T) {
	// 30 条里缺 1 条 ⇒ 容得下（额度 = max(2, 2) = 2）
	a := seq(30)
	b := make([]string, 0, 29)
	for i, v := range a {
		if i == 10 {
			continue
		}
		b = append(b, v)
	}
	if !TolerantAligned(a, b) {
		t.Error("30 缺 1 应在额度内")
	}
	// 位移 10 条 ⇒ 超出 maxTolerantShift / 额度 ⇒ 不判对齐
	long := seq(40)
	if TolerantAligned(long, append([]string{"h1", "h2", "h3", "h4", "h5", "h6", "h7", "h8", "h9", "h10"}, long...)) {
		t.Error("头部多 10 条超出位移上限，不该判对齐")
	}
}

// 与严格口径的关系：完全相同必然两者都判对齐；"同位置替换"两口径都判不齐。
func TestTolerantAligned_AgainstStrict(t *testing.T) {
	same := seq(6)
	if !TolerantAligned(same, same) {
		t.Error("相同序列必须判对齐")
	}
	if TolerantAligned([]string{"n1", "n2", "n9"}, []string{"n1", "n2", "n9"}) == false {
		t.Error("相同序列（含尾部同值）必须判对齐")
	}
}

// ===== M4 指标侧：TolerantMkAligned（骨架取最长源 · 主源必备相容 · 单源不背书）=====

func TestTolerantMkAligned(t *testing.T) {
	head := append([]string{"n0"}, seq(4)...) // 头部多一集
	cases := []struct {
		name    string
		seqs    [][]string
		mainIdx int
		want    bool
	}{
		{"两源完全一致", [][]string{seq(5), seq(5)}, 0, true},
		{"骨架取最长源：短源(主源)与长源相容", [][]string{seq(3), head}, 0, true},
		{"主源为空 + 另一源独立 ⇒ 单源不背书 ⇒ 不齐", [][]string{nil, seq(5)}, 0, false},
		{"三源：两源偏移相容、一源同位置替换 ⇒ 不齐", [][]string{seq(6), append([]string{"x1"}, seq(6)...), []string{"n1", "n2", "n9", "n4", "n5", "n6"}}, 0, false},
		{"空序列被忽略（仍满足 ≥2 非空）", [][]string{seq(5), nil, seq(5)}, -1, true},
		{"无主源(-1) 也可判（调用方自行决定是否统计）", [][]string{seq(5), seq(5)}, -1, true},
	}
	for _, tc := range cases {
		got, sk, bad := TolerantMkAligned(tc.seqs, tc.mainIdx)
		if got != tc.want {
			t.Errorf("%s: TolerantMkAligned = %v (骨架 %d, 未过 %v), want %v", tc.name, got, sk, bad, tc.want)
		}
	}
	// 骨架必须是最长源（不是主源）
	_, sk, _ := TolerantMkAligned([][]string{seq(3), head}, 0)
	if sk != 1 {
		t.Errorf("骨架应取最长源（下标 1），得 %d", sk)
	}
	// 并列时取下标最小者 ⇒ 结果确定（可复现）
	if _, sk2, _ := TolerantMkAligned([][]string{seq(4), seq(4)}, 1); sk2 != 0 {
		t.Errorf("并列时应取下标最小者，得 %d", sk2)
	}
}

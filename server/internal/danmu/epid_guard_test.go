package danmu

import "testing"

// P1-1 / F-07：抖动守卫 —— 尤其钉住"**首次派生必须直接采纳**"（否则整套机制静默空转）。
func TestShouldReplaceDerived(t *testing.T) {
	cases := []struct {
		name       string
		oldN, newN int
		overlap    bool
		want       bool
		wantWhy    string
	}{
		{"F-07 首次派生（oldN=0）⇒ 直接采纳，不看阈值/交集", 0, 1, false, true, "first-derive"},
		{"F-07 首次派生且新集为空 ⇒ 仍采纳（由调用方落「空派生」）", 0, 0, false, true, "first-derive"},
		{"正常增长 ⇒ 采纳", 10, 12, true, true, "ok"},
		{"持平 ⇒ 采纳", 10, 10, true, true, "ok"},
		{"减少但未超阈值（10→7，恰在 0.6 之上）⇒ 采纳", 10, 7, true, true, "ok"},
		{"骤减超阈值（10→5 < 6）⇒ 疑似抖动，不采纳", 10, 5, true, false, "suspect-shrink"},
		{"骤减超阈值（100→59 < 60）⇒ 不采纳", 100, 59, true, false, "suspect-shrink"},
		{"集名完全不相交 ⇒ 不采纳", 10, 10, false, false, "suspect-disjoint"},
		{"1→0（整源被清空）⇒ 疑似抖动，不采纳", 1, 0, false, false, "suspect-shrink"},
	}
	for _, c := range cases {
		got, why := ShouldReplaceDerived(c.oldN, c.newN, c.overlap)
		if got != c.want || why != c.wantWhy {
			t.Errorf("%s: ShouldReplaceDerived(%d,%d,%v) = (%v,%q), want (%v,%q)",
				c.name, c.oldN, c.newN, c.overlap, got, why, c.want, c.wantWhy)
		}
	}
}

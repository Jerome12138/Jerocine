package danmu

import "testing"

// P1-1 / F-07：抖动守卫 —— 尤其钉住"**首次派生必须直接采纳**"（否则整套机制静默空转）。
func TestShouldReplaceDerived(t *testing.T) {
	const now = int64(1_700_000_000_000)
	recent := now - 60_000            // 1 分钟前（未过宽限期）
	stale := now - 25*60*60*1000      // 25 小时前（过宽限期）
	cases := []struct {
		name       string
		oldN, newN int
		overlap    bool
		lastAt     int64
		want       bool
		wantWhy    string
	}{
		{"F-07 首次派生（oldN=0）⇒ 直接采纳，不看阈值/交集", 0, 1, false, 0, true, "first-derive"},
		{"F-07 首次派生且新集为空 ⇒ 仍采纳（由调用方落「空派生」）", 0, 0, false, 0, true, "first-derive"},
		{"正常增长 ⇒ 采纳", 10, 12, true, recent, true, "ok"},
		{"持平 ⇒ 采纳", 10, 10, true, recent, true, "ok"},
		{"减少但未超阈值（10→7，恰在 0.6 之上）⇒ 采纳", 10, 7, true, recent, true, "ok"},
		{"骤减（10→5 < 6）⇒ 疑似抖动，未过宽限期 ⇒ 不采纳", 10, 5, true, recent, false, "suspect-shrink"},
		{"骤减（100→59 < 60）⇒ 不采纳", 100, 59, true, recent, false, "suspect-shrink"},
		{"集名完全不相交 ⇒ 不采纳", 10, 10, false, recent, false, "suspect-disjoint"},
		{"1→0（整源被清空）⇒ 不采纳", 1, 0, false, recent, false, "suspect-shrink"},
		// L4（2026-11-11 评审）：**已等过宽限期（24h）仍判疑似 ⇒ 认定真变化，采纳** ✓
		{"骤减但已过宽限期 ⇒ 采纳（L4，避免合法改名/下架半季永久停更）", 10, 5, true, stale, true, "suspect-shrink-grace"},
		{"不相交但已过宽限期 ⇒ 采纳（L4）", 10, 10, false, stale, true, "suspect-disjoint-grace"},
	}
	for _, c := range cases {
		got, why := ShouldReplaceDerived(c.oldN, c.newN, c.overlap, c.lastAt, now)
		if got != c.want || why != c.wantWhy {
			t.Errorf("%s: ShouldReplaceDerived(%d,%d,%v,%d) = (%v,%q), want (%v,%q)",
				c.name, c.oldN, c.newN, c.overlap, c.lastAt, got, why, c.want, c.wantWhy)
		}
	}
}

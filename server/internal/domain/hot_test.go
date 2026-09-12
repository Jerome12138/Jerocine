package domain

import (
	"testing"
	"time"
)

var hotNow = time.Date(2026, 9, 12, 4, 0, 0, 0, time.UTC)

// TestHotScore_BoardRankDominant 榜内**位次严格主导**: 无论评分/在播/年份怎么组合,
// 高位次片必须排在低位次片之前 —— 这是 2026-09-12 调整的核心诉求
// ("无评分的 No.2 被有评分的 No.30 反超"就是旧模型的病)。
func TestHotScore_BoardRankDominant(t *testing.T) {
	// No.2 无任何信号 vs No.30 满信号(新片+在播+9.9 分)
	no2 := HotScore(2, 0, "", 0, hotNow)
	no30 := HotScore(30, 2026, "更新至20260911期", 9.9, hotNow)
	if no2 <= no30 {
		t.Fatalf("位次必须主导: No.2(%d) 必须 > No.30 满信号(%d)", no2, no30)
	}
	// 相邻位次同样不可反转(第 1 名取最差信号 vs 第 2 名取最好信号)
	worst1 := HotScore(1, 0, "", 0, hotNow)
	best2 := HotScore(2, 2026, "更新至20260911期", 9.9, hotNow)
	if worst1 <= best2 {
		t.Fatalf("相邻位次不可反转: No.1 最差(%d) 必须 > No.2 最好(%d)", worst1, best2)
	}
	// 末位同样成立
	worst308 := HotScore(308, 0, "", 0, hotNow)
	best309 := HotScore(309, 2026, "更新至20260911期", 9.9, hotNow)
	if worst308 <= best309 {
		t.Fatalf("深榜末段不可反转: No.308(%d) 必须 > No.309(%d)", worst308, best309)
	}
}

// TestHotScore_BoardBeatsFallback 榜内下限必须严格高于榜外上限 ——
// "榜内片必然排在榜外片之前"的形式化保证, 一旦有人调基数/档差就会被这条测试拦住。
func TestHotScore_BoardBeatsFallback(t *testing.T) {
	worstBoard := HotScore(308, 0, "", 0, hotNow)
	bestOutside := HotScore(0, 2026, "更新至20260911期", 9.9, hotNow)
	if worstBoard <= bestOutside {
		t.Fatalf("榜内下限 %d 必须 > 榜外上限 %d", worstBoard, bestOutside)
	}
}

// TestHotScore_TiebreakWithinRank 同一位次内, 评分/在播/年份决定先后, 但上限不超过一档。
func TestHotScore_TiebreakWithinRank(t *testing.T) {
	// 同位次: 有评分/在播的排前
	if a, b := HotScore(5, 2024, "更新至第20集", 9.4, hotNow), HotScore(5, 0, "", 0, hotNow); a <= b {
		t.Fatalf("同位次下有信号的(%d)必须排前(%d)", a, b)
	}
	// tiebreak 最大值也跨不过一个档差(防越级)
	maxAt5 := HotScore(5, 2026, "更新至20260911期", 9.9, hotNow)
	minAt4 := HotScore(4, 0, "", 0, hotNow)
	if maxAt5 >= minAt4 {
		t.Fatalf("tiebreak 越级: No.5 满信号(%d) 必须 < No.4 最差(%d)", maxAt5, minAt4)
	}
	if got := HotBoardTiebreak(2026, "更新至20260911期", 9.9, hotNow); got != HotTiebreakMax {
		t.Fatalf("tiebreak 满值=%d, want %d", got, HotTiebreakMax)
	}
}

// TestHotScore_Values 具体数值锚点(防无意改公式)。
func TestHotScore_Values(t *testing.T) {
	// 榜首: 100000 + 口碑 9×4 + 年份 17/3=5 = 100041
	if got := HotScore(1, 2023, "已完结", 9.4, hotNow); got != 100041 {
		t.Fatalf("榜首合成分=%d, want 100041", got)
	}
	// 末位无信号: 100000 - 307×100 = 69300
	if got := HotScore(308, 0, "", 0, hotNow); got != 69300 {
		t.Fatalf("榜末合成分=%d, want 69300", got)
	}
	// 榜外(0.1 分单位 ×10): 2004 年在播日更无评分 → 在播 14 → 140
	if got := HotScore(0, 2004, "更新至20260911期", 0, hotNow); got != 140 {
		t.Fatalf("榜外合成分=%d, want 140", got)
	}
	// 榜外全空 → 0
	if got := HotScore(0, 0, "", 0, hotNow); got != 0 {
		t.Fatalf("榜外空合成分=%d, want 0", got)
	}
}

func TestHotYearFresh(t *testing.T) {
	cases := []struct {
		year int
		want int
	}{
		{2026, 20}, {2027, 20}, {2025, 19}, {2006, 0}, {2005, 0}, {0, 0}, {-1, 0},
	}
	for _, c := range cases {
		if got := HotYearFresh(c.year, hotNow); got != c.want {
			t.Errorf("HotYearFresh(%d)=%d, want %d", c.year, got, c.want)
		}
	}
}

func TestHotOnAirBonus(t *testing.T) {
	cases := []struct {
		remarks string
		want    int
	}{
		{"更新至20260911期", HotOnAirDaily}, // 日更/周更
		{"更新至第20集", HotOnAirOther},
		{"更新至20260911", HotOnAirOther},
		{"已完结", 0},
		{"更新至", HotOnAirOther}, // 边界: 前缀命中即算在播
		{"", 0},
		{" 更新至20260911期 ", HotOnAirDaily}, // 去空白后判定
	}
	for _, c := range cases {
		if got := HotOnAirBonus(c.remarks); got != c.want {
			t.Errorf("HotOnAirBonus(%q)=%d, want %d", c.remarks, got, c.want)
		}
	}
}

func TestHotPraiseScore(t *testing.T) {
	cases := []struct {
		score float64
		want  int
	}{
		{0, 0}, {8.4, 8}, {8.5, 9}, {9.4, 9}, {9.9, 10}, {12, 10},
	}
	for _, c := range cases {
		if got := HotPraiseScore(c.score); got != c.want {
			t.Errorf("HotPraiseScore(%v)=%d, want %d", c.score, got, c.want)
		}
	}
}

// TestHotFallbackScore_OldButOnAir 方案 3.10 的核心场景: 年份老的连载片靠"在播加分"顶上,
// 不能因为年份新鲜度归零而掉队(取大而非相加)。
func TestHotFallbackScore_OldButOnAir(t *testing.T) {
	// 2004 年的港台综艺, 日更中, 无评分: 年份新鲜度 0, 在播 14
	if got := HotFallbackScore(2004, "更新至20260911期", 0, hotNow); got != HotOnAirDaily {
		t.Fatalf("老片在播兜底分=%d, want %d", got, HotOnAirDaily)
	}
	// 新片 + 高分: 年份 20 + 口碑 9
	if got := HotFallbackScore(2023, "已完结", 9.4, hotNow); got != 17+9 {
		t.Fatalf("新片兜底分=%d, want 26", got)
	}
	// 全空: 0
	if got := HotFallbackScore(0, "", 0, hotNow); got != 0 {
		t.Fatalf("空兜底分=%d, want 0", got)
	}
	// 上限: 不超 HotFallbackMax
	if got := HotFallbackScore(2026, "更新至20260911期", 9.9, hotNow); got != HotFallbackMax {
		t.Fatalf("兜底分上限=%d, want %d", got, HotFallbackMax)
	}
}

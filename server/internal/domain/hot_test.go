package domain

import (
	"testing"
	"time"
)

var hotNow = time.Date(2026, 9, 12, 4, 0, 0, 0, time.UTC)

func TestHotRankScore(t *testing.T) {
	cases := []struct {
		name       string
		rank, size int
		want       int
	}{
		{"第 1 名满分", 1, 308, HotRankTop},
		{"末位 60", 308, 308, HotRankFloor},
		{"超过深度收敛到末位", 500, 308, HotRankFloor},
		{"深度 1 视为满分", 1, 1, HotRankTop},
		{"榜外(rank=0)不给分", 0, 308, 0},
		{"中位落在区间内", 20, 40, 81}, // 100 - 40*19/39 = 100 - 19(整数截断) = 81
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HotRankScore(c.rank, c.size); got != c.want {
				t.Fatalf("HotRankScore(%d,%d)=%d, want %d", c.rank, c.size, got, c.want)
			}
		})
	}
}

// TestHotRankScoreInsideBeatsOutside 榜内下限(60)必须严格高于榜外上限(30) ——
// 这是"榜内片必然排在榜外片之前"的形式化保证, 一旦有人调分项权重就会被这条测试拦住。
func TestHotRankScoreInsideBeatsOutside(t *testing.T) {
	lonely := HotRankScore(308, 308)
	if HotFallbackMax >= lonely {
		t.Fatalf("榜外上限 %d 不得 >= 榜内下限 %d", HotFallbackMax, lonely)
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

// TestHotScore 合成分的存储单位是 0.1 分: 榜内 600–1300 / 榜外 0–300。
// 这一层的意义在于**位次分辨率** —— 见 hot.go 里 hotScoreScale 的注释。
func TestHotScore(t *testing.T) {
	// 第 1 名 + 新片 + 豆瓣 9.4 → (100 + 17 + 9) × 10
	if got := HotScore(1, 308, 2023, "已完结", 9.4, hotNow); got != 1260 {
		t.Fatalf("榜首合成分=%d, want 1260", got)
	}
	// 末位 + 无兜底 → 600
	if got := HotScore(308, 308, 0, "", 0, hotNow); got != 600 {
		t.Fatalf("榜末合成分=%d, want 600", got)
	}
	// 榜外(rank=0)只算兜底
	if got := HotScore(0, 308, 2004, "更新至20260911期", 0, hotNow); got != 140 {
		t.Fatalf("榜外合成分=%d, want 140", got)
	}
	// 相邻位次必须可区分(1 分制下前 8 名会同分, 这正是放大 10 倍的理由)
	if a, b := HotScore(1, 308, 2024, "", 0, hotNow), HotScore(2, 308, 2024, "", 0, hotNow); a <= b {
		t.Fatalf("第 1 名(%d) 必须高于第 2 名(%d)", a, b)
	}
	// 榜内最差 > 榜外最好(存储单位下的不变式)
	worstBoard := HotScore(308, 308, 0, "", 0, hotNow)
	bestOutside := HotScore(0, 308, 2026, "更新至20260911期", 9.9, hotNow)
	if worstBoard <= bestOutside {
		t.Fatalf("榜内下限 %d 必须 > 榜外上限 %d", worstBoard, bestOutside)
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

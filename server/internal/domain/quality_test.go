package domain

import "testing"

// TestIsLowQualityTelesync 枪版识别: 只认 remarks 独立 token, 排除英文巧合与正常高清标识。
func TestIsLowQualityTelesync(t *testing.T) {
	positive := []string{
		"HDTC", "TC", "TS", "HDTC版", "TS版", "TC版", "CAM", "CAMRIP",
		"高清 TC", "HD CAM 抢先", "枪版", "抢先版", "抢先看", "TC抢先",
	}
	for _, r := range positive {
		if !IsLowQualityTelesync(r) {
			t.Errorf("IsLowQualityTelesync(%q) = false, want true", r)
		}
	}
	negative := []string{
		"", "HD", "HD高清", "BD", "蓝光", "1080P", "更新至20260911期", "已完结",
		// 线上 name 字段实测的英文巧合, 不能误判:
		"BTS幕后", "Switch", "ATCS", "TSCAM", "Heart Catch", "CRISIS",
	}
	for _, r := range negative {
		if IsLowQualityTelesync(r) {
			t.Errorf("IsLowQualityTelesync(%q) = true, want false", r)
		}
	}
}

// TestHotBoardTCMultiBoardBonusScore 多榜回血: 每多一榜 +60, 封顶 120。
func TestHotBoardTCMultiBoardBonusScore(t *testing.T) {
	cases := []struct{ count, want int }{
		{0, 0}, {1, 0}, {2, 60}, {3, 120}, {4, 120}, {8, 120},
	}
	for _, c := range cases {
		if got := HotBoardTCMultiBoardBonusScore(c.count); got != c.want {
			t.Errorf("bonus(%d)=%d, want %d", c.count, got, c.want)
		}
	}
}

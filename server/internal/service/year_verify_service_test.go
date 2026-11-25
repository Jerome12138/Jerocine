package service

import "testing"

// 钉住「同名采信」的两条守卫（2026-11-25 审查 P1-2）：
//   - 相等 → 采信
//   - 包含关系但长度差过大 → **不采信**（挡跨作品误命中）
//   - 包含关系且长度接近 → 采信（同一作品的不同副标题）
func TestYearVerifySameName(t *testing.T) {
	cases := []struct {
		local, remote string
		want          bool
	}{
		{"流浪地球3[预告片]", "流浪地球3（上）", true}, // 剥后缀后长度 5 vs 7，比例 0.71 ≥ 0.7
		{"流浪地球3", "流浪地球3（上）", true},
		{"超人", "超人总动员", false},        // 2 vs 5 = 0.4 < 0.7 ⇒ 挡住
		{"勇者时代", "勇敢者的预算超支时代", false}, // 字不同
		{"恋爱朝九晚五", "恋爱朝九晚五", true},    // 全等
		{"", "超人", false},
		{"超人", "", false},
	}
	for _, c := range cases {
		if got := yearVerifySameName(c.local, c.remote); got != c.want {
			t.Errorf("sameName(%q, %q) = %v, want %v", c.local, c.remote, got, c.want)
		}
	}
}

// 剥预告/解说等方括号后缀，用于提高 TMDB 命中率
func TestYearVerifyQueryName(t *testing.T) {
	cases := map[string]string{
		"流浪地球3[预告片]":    "流浪地球3",
		"一拳超人第三季[电影解说]": "一拳超人第三季",
		"普通片名":          "普通片名",
		"":              "",
	}
	for in, want := range cases {
		if got := yearVerifyQueryName(in); got != want {
			t.Errorf("queryName(%q) = %q, want %q", in, got, want)
		}
	}
}

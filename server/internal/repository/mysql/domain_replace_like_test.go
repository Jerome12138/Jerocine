package mysql

import "testing"

// 搜索关键词里的 LIKE 通配符必须转义，否则 "100%" 会变成任意匹配。
func TestLikeContains(t *testing.T) {
	cases := map[string]string{
		"abc":  "%abc%",
		"100%": `%100\%%`,
		"a_b":  `%a\_b%`,
		`a\b`:  `%a\\b%`,
		"中文":   "%中文%",
	}
	for in, want := range cases {
		if got := likeContains(in); got != want {
			t.Fatalf("likeContains(%q) = %q, want %q", in, got, want)
		}
	}
}

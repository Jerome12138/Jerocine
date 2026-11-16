package danmu

import "testing"

// 中文数字期号（2026-11-16）：`第一期/第十二期/第二十三集/第一百期` 必须与阿拉伯数字同身份。
// 背景：reDiN 只吃阿拉伯数字 ⇒「第二期」落进 s 段（实测 155 行 `s=第X期`）⇒ 与其它源的
// 「第2期」永不相等（伪分歧）。

func TestCnNumeral(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"一", 1, true}, {"二", 2, true}, {"两", 2, true}, {"九", 9, true}, {"零", 0, true},
		{"十", 10, true}, {"十一", 11, true}, {"二十", 20, true}, {"二十三", 23, true},
		{"三十", 30, true}, {"九十九", 99, true}, {"一百", 100, true}, {"一百二十三", 123, true},
		{"两百", 200, true}, {"一千", 1000, true}, {"〇", 0, true},
		{"", 0, false}, {"期", 0, false}, {"2", 0, false}, {"二十X", 0, false},
	}
	for _, tc := range cases {
		got, ok := cnNumeral(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("cnNumeral(%q) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestParseEpisodeKey_ChineseNumeral(t *testing.T) {
	cases := []struct {
		title   string
		wantNum int
		wantS   string
	}{
		{"第一期", 1, ""},
		{"第2期", 2, ""},
		{"第十二期", 12, ""},
		{"第二十三集", 23, ""},
		{"第一百期", 100, ""},
		{"第二期未播完整版", 2, ""},        // 变体 + 噪声词 + 中文数字 同时出现
		{"20250128年度特辑", 0, "年度"}, // 不受影响（"特辑"走变体、无期号）
		{"正片", 0, ""},
		{"第x期", 0, ""}, // 非数字字符 ⇒ 不产期号（宁缺勿错）
	}
	for _, tc := range cases {
		k := ParseEpisodeKey(tc.title)
		if k.Num != tc.wantNum || k.Series != tc.wantS {
			t.Errorf("%q: Num=%d Series=%q, want Num=%d Series=%q", tc.title, k.Num, k.Series, tc.wantNum, tc.wantS)
		}
	}
	// 与阿拉伯数字同身份（这是补中文数字的**目的**）
	a := EpidOf(ParseEpisodeKey("第2期"))
	b := EpidOf(ParseEpisodeKey("第二期"))
	if a != b {
		t.Errorf("中文数字与阿拉伯数字应产出同一身份：%q vs %q", a, b)
	}
}

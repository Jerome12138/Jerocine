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
		{"第二期未播完整版", 2, ""},       // 变体 + 噪声词 + 中文数字 同时出现
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

// 季/部 **不是集序**（2026-11-16 修正 reDiN 单位类）：
//   - `第7季` / `第七季` / `第3部` 一律不给 Num（季/部是分季/分部标记）；
//   - 关键回归：`第7季第5集` 的 Num 必须是 **5** —— 修正前 reDiN 的第一个匹配落在"第7季"⇒ 集号被抢占成 7。
func TestParseEpisodeKey_SeasonPartNotEpisodeNum(t *testing.T) {
	cases := []struct {
		title   string
		wantNum int
	}{
		{"第7季", 0}, {"第七季", 0}, {"第3部", 0}, {"第三部", 0},
		{"第7季第5集", 5}, {"王牌对王牌第七季第5期", 5},
		{"第7季20260101", 0}, // 季号 + 日期：n 该由日期链路（bridge/anchor）解决，不该取 7
		{"第7季第5集完整版", 5},   // 噪声词不影响
	}
	for _, tc := range cases {
		if k := ParseEpisodeKey(tc.title); k.Num != tc.wantNum {
			t.Errorf("%q: Num=%d, want %d", tc.title, k.Num, tc.wantNum)
		}
	}
}

// 方案 A（2026-11-16）：季/部标记**归一化后并入 Series**（`#S<n>` / `#B<n>`）——
// 分季/分部标记必须进身份（否则同片多季的 S1E1 与 S2E1 会得到同一身份）；
// **归一化**是为了让各源写法差异（第七季/第7季/7）不制造新的伪分歧。
func TestSeriesSeasonMark(t *testing.T) {
	cases := []struct {
		title string
		wantS string
	}{
		{"王牌对王牌第七季", "王牌对王牌#S7"},
		{"王牌对王牌第7季", "王牌对王牌#S7"},
		{"王牌对王牌第七季第5期", "王牌对王牌#S7"}, // Num=5 另由 reDiN 给（测试见上一条）
		{"斗罗大陆第三部", "斗罗大陆#B3"},
		{"斗罗大陆第3部第12集", "斗罗大陆#B3"},
		{"第七季", "#S7"}, // 无栏目名也保留标记（比"无身份"好）
		{"第7季第5集", "#S7"},
		{"风味人间", "风味人间"}, // 无季/部 ⇒ 不加标记（Series 仍是栏目名本身）
		// 「季」是**分季区分位**：「四季的味道」在「季」处被截断 ⇒ 得"四"(1字) ⇒ 不足 2 字被丢 ⇒ Series=""
		// （既有行为，与季号标记无关；也印证了"季不该混进栏目名"）
		{"四季的味道", ""},
		{"王牌对王牌第七季2022.05.01期-会员版", "王牌对王牌#S7"},
	}
	for _, tc := range cases {
		if got := ParseEpisodeKey(tc.title).Series; got != tc.wantS {
			t.Errorf("%q: Series=%q, want %q", tc.title, got, tc.wantS)
		}
	}
	// 归一化的意义：两种写法产出**同一身份**
	a := EpidOf(ParseEpisodeKey("王牌对王牌第七季第1期"))
	b := EpidOf(ParseEpisodeKey("王牌对王牌第7季第1期"))
	if a != b {
		t.Errorf("季号写法不同应同身份：%q vs %q", a, b)
	}
}

package danmu

import "testing"

// 用例全部取自**现网真实集名**（2026-11-06 抽样：随机 87 部 + 综艺定向 126 部）
func TestParseEpisodeKey_RealForms(t *testing.T) {
	cases := []struct {
		title string
		want  EpisodeKey
	}{
		{"20260803(第1期上)", EpisodeKey{Date: "20260803", Num: 1, Part: "上"}},
		{"20260803(第1期中)", EpisodeKey{Date: "20260803", Num: 1, Part: "中"}},
		{"20260803(第1期上纯享)", EpisodeKey{Date: "20260803", Num: 1, Part: "上", Variant: "纯享"}},
		{"20260806(第1期加更上)", EpisodeKey{Date: "20260806", Num: 1, Part: "上", Variant: "加更"}},
		{"20260807(第1期陪看上)", EpisodeKey{Date: "20260807", Num: 1, Part: "上", Variant: "陪看"}},
		{"20260731(先导片上)", EpisodeKey{Date: "20260731", Part: "上", Variant: "先导"}},
		{"20260805(超前彩蛋)", EpisodeKey{Date: "20260805", Variant: "彩蛋|超前"}},
		{"20260128未公开特辑", EpisodeKey{Date: "20260128", Variant: "未公开|特辑"}},
		{"20260921上", EpisodeKey{Date: "20260921", Part: "上"}},
		{"20250803", EpisodeKey{Date: "20250803"}},
		{"第9期上", EpisodeKey{Num: 9, Part: "上"}},
		{"第1集", EpisodeKey{Num: 1}},
		{"第1期加更版1", EpisodeKey{Num: 1, Variant: "加更"}},
		{"E01.220705", EpisodeKey{ENum: 1}},
		{"正片", EpisodeKey{}},
		{"国语", EpisodeKey{}},
		{"", EpisodeKey{}},
	}
	for _, c := range cases {
		if got := ParseEpisodeKey(c.title); got != c.want {
			t.Errorf("ParseEpisodeKey(%q) = %+v, want %+v", c.title, got, c.want)
		}
	}
}

func TestSameEpisode_KeyRules(t *testing.T) {
	app := ParseEpisodeKey("20260803(第1期上)")
	appGeng := ParseEpisodeKey("20260806(第1期加更上)")
	appXia := ParseEpisodeKey("20260804(第1期下)")

	cases := []struct {
		name string
		a, b EpisodeKey
		want bool
	}{
		{"App「第1期上」 vs 库「第1期上」(库侧无日期也应同集)", app, ParseEpisodeKey("第1期上"), true},
		{"App「第1期上」 vs 库「第1期」(库未分上下, 宽容)", app, ParseEpisodeKey("第1期"), true},
		{"App「第1期加更上」 vs 库「第1期上」→ 变体不同, 必须判不同", appGeng, ParseEpisodeKey("第1期上"), false},
		{"App「第1期加更上」 vs 库「第1期加更上」", appGeng, ParseEpisodeKey("第1期加更上"), true},
		{"App「第1期上」 vs 库「第1期下」→ 上下不同, 判不同", app, appXia, false},
		{"纯日期同 → 同集", ParseEpisodeKey("20250803"), ParseEpisodeKey("20250803"), true},
		{"纯日期不同 → 不同集", ParseEpisodeKey("20250803"), ParseEpisodeKey("20250810"), false},
		{"E圆号同 → 同集", ParseEpisodeKey("E01.220705"), ParseEpisodeKey("E01"), true},
		{"无特征键之间不判同(避免误配)", ParseEpisodeKey("正片"), ParseEpisodeKey("正片"), false},
		{"App 有变体、库无变体 → 判不同(宁缺勿错)", ParseEpisodeKey("20250712(加更篇)"), ParseEpisodeKey("第7期"), false},
	}
	for _, c := range cases {
		if got := SameEpisode(c.a, c.b); got != c.want {
			t.Errorf("%s: SameEpisode = %v, want %v (a=%+v b=%+v)", c.name, got, c.want, c.a, c.b)
		}
	}
}

func TestPickEpisode_ByKey(t *testing.T) {
	lib := []DandanEpisode{
		{EpisodeID: 1, EpisodeNumber: "1", EpisodeTitle: "第1期"},
		{EpisodeID: 2, EpisodeNumber: "2", EpisodeTitle: "第2期"},
		{EpisodeID: 3, EpisodeNumber: "3", EpisodeTitle: "第1期加更"},
	}
	// 综艺错位场景：App 侧第 120 个视频 = 第2期上 → 应命中库里的「第2期」
	if ep := PickEpisode(lib, 120, "20260810(第2期上)"); ep == nil || ep.EpisodeID != 2 {
		if ep == nil {
			t.Fatal("期望命中「第2期」, 实际 nil")
		}
		t.Fatalf("期望命中 EpisodeID=2, 实际=%d", ep.EpisodeID)
	}
	// 变体不同 → 不应错配到「第1期」；同变体则应命中「第1期加更」
	if ep := PickEpisode(lib, 120, "20260806(第1期加更上)"); ep == nil || ep.EpisodeID != 3 {
		if ep == nil {
			t.Fatal("期望命中「第1期加更」, 实际 nil")
		}
		t.Fatalf("期望命中 EpisodeID=3, 实际=%d", ep.EpisodeID)
	}
	// 未提供集名 → 走旧策略（集号精确相等）
	if ep := PickEpisode(lib, 2); ep == nil || ep.EpisodeID != 2 {
		t.Fatal("未提供集名时应走旧策略命中 EpisodeNumber=2")
	}
	// 键明确但库里找不到同键 → 不落到"位置回退"（宁缺勿错）
	if ep := PickEpisode(lib, 3, "20260801(第9期上)"); ep != nil {
		t.Fatalf("键明确但无同键时应返回 nil, 实际命中 %d", ep.EpisodeID)
	}
}

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
		// 无意义后缀（会员/未公开/未播/直拍/完整版/未删减）解析前剔除 → 等价于「20260128特辑」
		{"20260128未公开特辑", EpisodeKey{Date: "20260128", Variant: "特辑"}},
		{"20260803完整版", EpisodeKey{Date: "20260803"}},
		{"20260805超前彩蛋未播", EpisodeKey{Date: "20260805", Variant: "彩蛋|超前"}},
		{"会员版", EpisodeKey{}},
		// 主/次部分细分（2026-11-06）：中（上）/ 中（下）不再塌成同一键
		{"20260818中（下）", EpisodeKey{Date: "20260818", Part: "中", Sub: "下"}},
		{"20260831中上", EpisodeKey{Date: "20260831", Part: "中", Sub: "上"}},
		{"第3期中（上）", EpisodeKey{Num: 3, Part: "中", Sub: "上"}},
		{"第3期中（下）", EpisodeKey{Num: 3, Part: "中", Sub: "下"}},
		{"第3期中", EpisodeKey{Num: 3, Part: "中"}},
		// 摘要正文里的 上/下 不算次部分（必须紧邻、最多隔一个左括号）
		{"第1期中：上集回顾", EpisodeKey{Num: 1, Part: "中"}},
		// 栏目/系列名（Series）：正片为空，衍生栏目单独成身份（≥2 中文字才算数）
		{"20261012心动的周末上", EpisodeKey{Date: "20261012", Part: "上", Series: "心动的周末"}},
		{"【qq】 第1期上：又争又抢，心动了就要打明牌！", EpisodeKey{Num: 1, Part: "上"}},
		{"心动的周末第1期上：高甜游戏CP亲密贴贴～", EpisodeKey{Num: 1, Part: "上", Series: "心动的周末"}},
		{"心动的旅行上：草原团建！", EpisodeKey{Part: "上", Series: "心动的旅行"}},
		{"20260921上", EpisodeKey{Date: "20260921", Part: "上"}},
		{"20250803", EpisodeKey{Date: "20250803"}},
		{"第9期上", EpisodeKey{Num: 9, Part: "上"}},
		{"第1集", EpisodeKey{Num: 1}},
		{"第1期加更版1", EpisodeKey{Num: 1, Variant: "加更"}},
		{"E01.220705", EpisodeKey{ENum: 1}},
		{"正片", EpisodeKey{}},
		{"国语", EpisodeKey{}},
		{"", EpisodeKey{}},
		// ---- 2026-11-07 v2: 日期三格式归一(8位/破折/点分) ----
		{"2026-10-10期", EpisodeKey{Date: "20261010"}},
		{"第20261010期", EpisodeKey{Date: "20261010"}},
		{"2022.05.01期", EpisodeKey{Date: "20220501"}},
		{"【qq】 第1期上：出发团变落汤鸡，沈腾被迫洗头 2026-10-10期", EpisodeKey{Num: 1, Part: "上", Date: "20261010"}},
		// ---- 2026-11-07 v2: 栏目名连续片段(第+8位日期+期 残渣不再污染; 季号跨度删除; 描述拒识) ----
		// 「先导片」是变体: 变体身份优先, 栏目名为空
		{"第20260731期先导片上", EpisodeKey{Date: "20260731", Part: "上", Variant: "先导"}},
		{"王牌对王牌第七季2022.05.01期-会员版", EpisodeKey{Date: "20220501", Series: "王牌对王牌"}},
		{"檀健次带北江小分队回好六了！ 2025-01-04期", EpisodeKey{Date: "20250104"}},
		{"我是大侦探.20180324期", EpisodeKey{Date: "20180324", Series: "我是大侦探"}},
		{"20261027心动的周末第3期下", EpisodeKey{Date: "20261027", Num: 3, Part: "下", Series: "心动的周末"}},
		{"20260803第1期中纯享", EpisodeKey{Date: "20260803", Num: 1, Part: "中", Variant: "纯享"}},
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
		// 2026-11-06 修订：主/次部分必须一致（此前被日期/期号相同掩盖）
		{"库「第3期中（上）」vs「第3期中（下）」→ 次部分不同, 判不同", ParseEpisodeKey("第3期中（上）"), ParseEpisodeKey("第3期中（下）"), false},
		{"App「第3期中」vs 库「第3期中（上）」→ 一方无次部分, 视为通配, 判同", ParseEpisodeKey("第3期中"), ParseEpisodeKey("第3期中（上）"), true},
		{"同日期不同支：「20260803上」vs「20260803中」→ 判不同(日期不再掩盖主部分)", ParseEpisodeKey("20260803上"), ParseEpisodeKey("20260803中"), false},
		{"同日期不同次支：「20260818中（下）」vs「20260818中（上）」→ 判不同", ParseEpisodeKey("20260818中（下）"), ParseEpisodeKey("20260818中（上）"), false},
		{"库「心动的周末第1期上」vs 正片「第1期上」→ 栏目名不同, 判不同(消除串集隐患)", ParseEpisodeKey("心动的周末第1期上：高甜游戏"), ParseEpisodeKey("第1期上：又争又抢"), false},
		{"App「20261005心动的旅行上」vs 库「心动的旅行上」→ 栏目名一致但两侧都无期号/日期交集, 仍判不同(需 A2 期号才能配上)", ParseEpisodeKey("20261005心动的旅行上"), ParseEpisodeKey("心动的旅行上：草原团建"), false},
		// ---- 2026-11-07 v2: 日期三格式归一 + 日期硬否决 ----
		{"App「20261010上」vs 库「第1期上：… 2026-10-10期」→ 日期归一后直等", ParseEpisodeKey("20261010上"), ParseEpisodeKey("【qq】 第1期上：出发团变落汤鸡 2026-10-10期"), true},
		{"「第20260731期先导片上」vs「20260731先导片上」→ 栏目残渣清理后同键", ParseEpisodeKey("第20260731期先导片上"), ParseEpisodeKey("20260731先导片上"), true},
		{"日期双有不等 → 否决(即使期号相等, 现实是重播/错标)", ParseEpisodeKey("20260817第3期上"), ParseEpisodeKey("20260810第3期上"), false},
		{"日期双有不等 → 否决(一方只有期号不触发否决, 走期号规则)", ParseEpisodeKey("20260817第3期上"), ParseEpisodeKey("第3期上"), true},
		{"点分日期 vs 8位日期同集", ParseEpisodeKey("2022.05.01期"), ParseEpisodeKey("20220501"), true},
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
	if ep := PickEpisode(lib, 120, 0, "20260810(第2期上)"); ep == nil || ep.EpisodeID != 2 {
		if ep == nil {
			t.Fatal("期望命中「第2期」, 实际 nil")
		}
		t.Fatalf("期望命中 EpisodeID=2, 实际=%d", ep.EpisodeID)
	}
	// 变体不同 → 不应错配到「第1期」；同变体则应命中「第1期加更」
	if ep := PickEpisode(lib, 120, 0, "20260806(第1期加更上)"); ep == nil || ep.EpisodeID != 3 {
		if ep == nil {
			t.Fatal("期望命中「第1期加更」, 实际 nil")
		}
		t.Fatalf("期望命中 EpisodeID=3, 实际=%d", ep.EpisodeID)
	}
	// 未提供集名 → 走旧策略（集号精确相等）
	if ep := PickEpisode(lib, 2, 0); ep == nil || ep.EpisodeID != 2 {
		t.Fatal("未提供集名时应走旧策略命中 EpisodeNumber=2")
	}
	// 键明确但库里找不到同键 → 不落到"位置回退"（宁缺勿错）
	if ep := PickEpisode(lib, 3, 0, "20260801(第9期上)"); ep != nil {
		t.Fatalf("键明确但无同键时应返回 nil, 实际命中 %d", ep.EpisodeID)
	}
}

// 2026-11-07 v2: A2 期号参数(前端他源桥接现算)注入 Num + 门禁补「期号参数非空」。
// 心动9 真实场景: App「20260803上」纯日期无期号, 桥接得期1 → 命中库「第1期上」;
// 不传期号时键无交集 → nil(宁缺勿错, 不做位置回退)。
func TestPickEpisode_AppNum(t *testing.T) {
	lib := []DandanEpisode{
		{EpisodeID: 1, EpisodeTitle: "【qq】 第1期上：又争又抢"},
		{EpisodeID: 2, EpisodeTitle: "【qq】 第2期中：你忙着抢行李"},
		{EpisodeID: 3, EpisodeTitle: "【qq】 第2期上：偶像剧现场"},
	}
	if ep := PickEpisode(lib, 999, 1, "20260803上"); ep == nil || ep.EpisodeID != 1 {
		t.Fatalf("桥接期号1 + 日期形集名应命中 EpisodeID=1, 实际 %+v", ep)
	}
	if ep := PickEpisode(lib, 999, 2, "20260810中"); ep == nil || ep.EpisodeID != 2 {
		t.Fatalf("桥接期号2 + 中支应命中 EpisodeID=2, 实际 %+v", ep)
	}
	if ep := PickEpisode(lib, 999, 0, "20260803上"); ep != nil {
		t.Fatalf("不传期号时日期与期号无交集, 应返回 nil(宁缺勿错), 实际 %+v", ep)
	}
	// 纯期号形态(第3期上)此前门禁不开走旧三策略; 前端传期号后应进键匹配
	libN := []DandanEpisode{{EpisodeID: 5, EpisodeTitle: "第3期上：小屋的夜"}}
	if ep := PickEpisode(libN, 999, 3, "第3期上"); ep == nil || ep.EpisodeID != 5 {
		t.Fatalf("期号参数应打开键匹配命中 EpisodeID=5, 实际 %+v", ep)
	}
}

// v2 追加（2026-11-09，三端同规防呆）：日期样式期号「第20261005期…」不得残留「期」
func TestSeriesOf_DateStylePeriod(t *testing.T) {
	k := ParseEpisodeKey("第20261005期心动的旅行上")
	if k.Series != "心动的旅行" {
		t.Fatalf("Series = %q, want 心动的旅行（先删第N期再删日期）", k.Series)
	}
	if k.Date != "20261005" || k.Part != "上" || k.Num != 0 {
		t.Fatalf("date/part/num = %q/%q/%d, want 20261005/上/0", k.Date, k.Part, k.Num)
	}
	if ParseEpisodeKey("第3期中（上）").Part != "中" {
		t.Fatal("常规「第3期中（上）」解析回归")
	}
}

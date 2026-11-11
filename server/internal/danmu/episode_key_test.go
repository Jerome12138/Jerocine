package danmu

import (
	"strings"
	"testing"
)

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
		{"20260805(超前彩蛋)", EpisodeKey{Date: "20260805", Variant: "彩蛋,超前"}},
		// 无意义后缀（会员/未公开/未播/直拍/完整版/未删减）解析前剔除 → 等价于「20260128特辑」
		{"20260128未公开特辑", EpisodeKey{Date: "20260128", Variant: "特辑"}},
		{"20260803完整版", EpisodeKey{Date: "20260803"}},
		{"20260805超前彩蛋未播", EpisodeKey{Date: "20260805", Variant: "彩蛋,超前"}},
		{"会员版", EpisodeKey{}},
		// 主/次部分细分（2026-11-06 引入；P0-1c 2026-11-10 合并为**单个复合 Part 字段**）
		{"20260818中（下）", EpisodeKey{Date: "20260818", Part: "中下"}},
		{"20260831中上", EpisodeKey{Date: "20260831", Part: "中上"}},
		{"第3期中（上）", EpisodeKey{Num: 3, Part: "中上"}},
		{"第3期中（下）", EpisodeKey{Num: 3, Part: "中下"}},
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
		// ---- 2026-11-10 P0-1a: 变体序号（变体词后紧跟的数字必须复合为身份） ----
		{"特辑2", EpisodeKey{Variant: "特辑", VariantNum: 2}},
		{"20260128特辑2", EpisodeKey{Date: "20260128", Variant: "特辑", VariantNum: 2}},
		{"第1期特辑5上", EpisodeKey{Num: 1, Part: "上", Variant: "特辑", VariantNum: 5}},
		{"特辑（2）", EpisodeKey{Variant: "特辑", VariantNum: 2}},
		{"特辑12", EpisodeKey{Variant: "特辑", VariantNum: 12}},
		// 无序号变体 / 序号不紧跟 / 长数字: 一律不取序号（宁缺勿错）
		{"夏日清凉特辑", EpisodeKey{Variant: "特辑", Series: "夏日清凉"}},
		{"第1期加更版1", EpisodeKey{Num: 1, Variant: "加更"}},
		{"特辑2026", EpisodeKey{Variant: "特辑"}},
		{"20260128特辑", EpisodeKey{Date: "20260128", Variant: "特辑"}},
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
		// ---- 2026-11-10 P0-1a: 变体序号纳入身份 ----
		{"「特辑2」vs「特辑3」→ 序号不同, 判不同", ParseEpisodeKey("特辑2"), ParseEpisodeKey("特辑3"), false},
		{"「特辑2」vs「特辑」→ 一方无序号, 判不同(无特征键不判同, 交 L3a)", ParseEpisodeKey("特辑2"), ParseEpisodeKey("特辑"), false},
		{"「特辑2」vs「特辑2」(两侧都无期号/日期) → 仍判不同, 由 L3a 组内唯一兜底", ParseEpisodeKey("特辑2"), ParseEpisodeKey("特辑2"), false},
		{"「20260128特辑2」vs「特辑2」→ 日期单边, 走序号+变体(无共同期号/日期) → 不同", ParseEpisodeKey("20260128特辑2"), ParseEpisodeKey("特辑2"), false},
		{"「第1期特辑2」vs「第1期特辑2」→ 期号+序号一致 → 同集", ParseEpisodeKey("第1期特辑2"), ParseEpisodeKey("第1期特辑2"), true},
		{"「第1期特辑2」vs「第1期特辑3」→ 序号不同 → 否决", ParseEpisodeKey("第1期特辑2"), ParseEpisodeKey("第1期特辑3"), false},
		{"「夏日清凉特辑」vs「20260809夏日清凉特辑」→ 仅单边日期、无共同期号/日期 ⇒ 判不同(交 L3a 组内唯一兜底)", ParseEpisodeKey("夏日清凉特辑"), ParseEpisodeKey("20260809夏日清凉特辑"), false},
		{"「夏日清凉特辑」vs「夏日清凉特辑」（同身份无期号）→ 由 L3a 兜底, SameEpisode 不判同", ParseEpisodeKey("夏日清凉特辑"), ParseEpisodeKey("夏日清凉特辑"), false},
		// ---- 2026-11-10 P0-1c: 主部分合并为**复合单段** + 前缀相容（逐例等价于旧的 part+sub 两段式） ----
		{"「第3期中」vs「第3期中（上）」→ 前缀相容（中 ⊂ 中上）⇒ 同集", ParseEpisodeKey("第3期中"), ParseEpisodeKey("第3期中（上）"), true},
		{"「第3期中」vs「第3期中（下）」→ 前缀相容 ⇒ 同集", ParseEpisodeKey("第3期中"), ParseEpisodeKey("第3期中（下）"), true},
		{"「第3期中（上）」vs「第3期中（下）」→ 互不为前缀 ⇒ 不同集（区分仍在）", ParseEpisodeKey("第3期中（上）"), ParseEpisodeKey("第3期中（下）"), false},
		{"「第3期中（上）」vs「第3期中（上）」→ 全等 ⇒ 同集", ParseEpisodeKey("第3期中（上）"), ParseEpisodeKey("第3期中（上）"), true},
		{"「上」vs「中」→ 互不为前缀 ⇒ 不同集（分支不同）", ParseEpisodeKey("第1期上"), ParseEpisodeKey("第1期中"), false},
		{"「上」vs「上中」→ 前缀相容 ⇒ 同集", ParseEpisodeKey("EP8上"), ParseEpisodeKey("EP8上中"), true},
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
	if ParseEpisodeKey("第3期中（上）").Part != "中上" {
		t.Fatal("常规「第3期中（上）」解析回归（P0-1c 后主部分复合为 中上）")
	}
}

// L3a 防呆（2026-11-09）：同栏目组内唯一定位
func TestPickEpisode_SeriesUniqueL3a(t *testing.T) {
	lib := []DandanEpisode{
		{EpisodeID: 1, EpisodeNumber: "1", EpisodeTitle: "【qq】 第1期上：坞民绝技大乱斗燃爆全场"},
		{EpisodeID: 2, EpisodeNumber: "2", EpisodeTitle: "【qq】 线上集结篇：新老坞民烧脑身份推理战"},
		{EpisodeID: 3, EpisodeNumber: "3", EpisodeTitle: "【qq】 第2期上：彭冠英方媛惨遭老坞民整蛊"},
		{EpisodeID: 4, EpisodeNumber: "4", EpisodeTitle: "【qq】 心动的旅行上：草原团建！"},
		{EpisodeID: 5, EpisodeNumber: "5", EpisodeTitle: "【qq】 心动的旅行中：…"},
		{EpisodeID: 6, EpisodeNumber: "6", EpisodeTitle: "【qq】 心动的旅行下：…"},
	}
	// ① 纯栏目名（库侧无期号无日期）+ 组内唯一 ⇒ 命中
	if ep := PickEpisode(lib, 2, 0, "20260507线上集结篇"); ep == nil || ep.EpisodeID != 2 {
		t.Fatalf("L3a 应命中「线上集结篇」(id=2), got %v", ep)
	}
	// ② 带主次部分 + 组内按 (series,part) 唯一 ⇒ 命中（App 的 上/中/下 ↔ 库的 上/中/下）
	if ep := PickEpisode(lib, 1, 0, "20261005心动的旅行上"); ep == nil || ep.EpisodeID != 4 {
		t.Fatalf("L3a 应命中「心动的旅行上」(id=4), got %v", ep)
	}
	// ③ 无主次部分且组内 3 条 ⇒ 不唯一 ⇒ 不配（宁缺勿错）
	if ep := PickEpisode(lib, 5, 0, "20261005心动的旅行"); ep != nil {
		t.Fatalf("组内不唯一时应不配, got id=%d", ep.EpisodeID)
	}
	// ④ 正片不适用（series/variant 皆空）⇒ 不因 L3a 命中
	if ep := PickEpisode(lib, 1, 0, "20260803上"); ep != nil {
		t.Fatalf("正片不应走 L3a, got id=%d", ep.EpisodeID)
	}
}

// P0-1a 防呆（2026-11-10）：**变体序号复合身份** —— 桃花坞6 特辑组真实形态。
//
// 改前：特辑1..5 解析出的 Variant 全是「特辑」⇒ L3a 组内 6 条（含夏日清凉特辑）⇒ 永不唯一 ⇒ 整组丢弹幕。
// 改后：VariantNum 复合进身份 ⇒ 组内唯一 ⇒ 各自命中 ✓
func TestPickEpisode_VariantOrdinalL3a(t *testing.T) {
	lib := []DandanEpisode{
		{EpisodeID: 1, EpisodeNumber: "1", EpisodeTitle: "【qq】 第1期上：坞民绝技大乱斗"},
		{EpisodeID: 21, EpisodeNumber: "21", EpisodeTitle: "特辑1"},
		{EpisodeID: 22, EpisodeNumber: "22", EpisodeTitle: "特辑2"},
		{EpisodeID: 23, EpisodeNumber: "23", EpisodeTitle: "特辑3"},
		{EpisodeID: 24, EpisodeNumber: "24", EpisodeTitle: "特辑4"},
		{EpisodeID: 25, EpisodeNumber: "25", EpisodeTitle: "特辑5"},
		{EpisodeID: 26, EpisodeNumber: "26", EpisodeTitle: "夏日清凉特辑"},
	}
	// 带序号特辑：序号各自唯一 ⇒ 命中对应集
	for _, c := range []struct {
		app  string
		want int64
	}{
		{"20260809特辑2", 22},
		{"20260816特辑5", 25},
	} {
		ep := PickEpisode(lib, 999, 0, c.app)
		if ep == nil || ep.EpisodeID != c.want {
			t.Fatalf("App「%s」应命中 id=%d, got %v", c.app, c.want, ep)
		}
	}
	// 无序号「特辑」（App 侧首条）身份 ≠ 任何「特辑N」⇒ 不配（宁缺勿错）
	if ep := PickEpisode(lib, 999, 0, "20260809特辑"); ep != nil {
		t.Fatalf("无序号特辑与「特辑N」身份不同, 应不配, got id=%d", ep.EpisodeID)
	}
	// 夏日清凉特辑：无序号但 series 不同 ⇒ 自成一组、组内唯一 ⇒ 命中
	if ep := PickEpisode(lib, 999, 0, "20260809夏日清凉特辑"); ep == nil || ep.EpisodeID != 26 {
		t.Fatalf("夏日清凉特辑 应命中 id=26, got %v", ep)
	}
}

// ===== P0-1b（2026-11-10）：结构化身份字段契约 =====

func TestEpisodeKeyString_RoundTrip(t *testing.T) {
	k := EpisodeKey{Date: "20260803", Num: 1, ENum: 2, Part: "中上",
		Variant: "特辑", VariantNum: 2, Series: "心动的周末"}
	s := k.Serialize(5)
	want := "v1|d=20260803|n=1|e=2|p=中上|v=特辑|vn=2|s=心动的周末|o=5|from=none"
	if s != want {
		t.Fatalf("Serialize = %q, want %q", s, want)
	}
	if got, ok := ParseEpisodeKeyString(s); !ok || got != k {
		t.Fatalf("ParseEpisodeKeyString = %+v ok=%v, want %+v", got, ok, k)
	}
	// 空键往返（客户端未触发的集会传空串，此处校验"零键"本身也能往返）
	if got, ok := ParseEpisodeKeyString(EpisodeKey{}.Serialize(0)); !ok || !got.IsZero() {
		t.Fatalf("零键往返失败: %+v ok=%v", got, ok)
	}
}

func TestParseEpisodeKeyString_Rejects(t *testing.T) {
	bad := []string{
		"",                     // 空
		"v2|d=20260803",        // 版本不认识
		"d=20260803",           // 缺版本前缀
		"v1|d",                 // 段内无 '='
		"v1|=20260803",         // 键名为空
		"v1|n=abc",             // 数字非法
		"v1|n=-1",              // 数字为负
		"v1|p=左",               // 主部分非法
		"v1|p=中上上",            // 主部分超长（最多 2 字：主 + 次）
		"v1|vn=99999",          // 变体序号越界
		"v1|s=" + strings.Repeat("x", 70), // 值超长
	}
	for _, s := range bad {
		if _, ok := ParseEpisodeKeyString(s); ok {
			t.Errorf("ParseEpisodeKeyString(%q) 应拒绝(ok=false)", s)
		}
	}
	// 未知字段名：向前兼容，忽略（不判非法）
	if k, ok := ParseEpisodeKeyString("v1|d=20260803|zz=1"); !ok || k.Date != "20260803" {
		t.Errorf("未知字段应被忽略: %+v ok=%v", k, ok)
	}
}

func TestMergeEpidKey_CrossCheck(t *testing.T) {
	base := ParseEpisodeKey("20260803(第1期上)")
	// ① 完全一致 ⇒ 无冲突，键不变
	if k, cf := MergeEpidKey(base, ParseEpisodeKey("20260803(第1期上)"), false); len(cf) != 0 || k != base {
		t.Fatalf("一致时不应有冲突: cf=%v", cf)
	}
	// ② date 冲突 ⇒ 以集名解析值为准（硬证据）
	if k, cf := MergeEpidKey(ParseEpisodeKey("20260810(第1期上)"), base, false); k.Date != "20260803" ||
		len(cf) != 1 || cf[0] != "date" {
		t.Fatalf("date 冲突应以集名为准: k.Date=%q cf=%v", k.Date, cf)
	}
	// ③ num 冲突 ⇒ 以集名解析值为准
	if k, cf := MergeEpidKey(ParseEpisodeKey("20260803(第9期上)"), base, false); k.Num != 1 ||
		len(cf) != 1 || cf[0] != "num" {
		t.Fatalf("num 冲突应以集名为准: k.Num=%d cf=%v", k.Num, cf)
	}
	// ④ numBridged=true（期号来自 A2 桥接）⇒ 跳过 num 比较，桥接结果不得被打回
	if _, cf := MergeEpidKey(ParseEpisodeKey("20260803(第9期上)"), base, true); len(cf) != 0 {
		t.Fatalf("numBridged 时应跳过 num 比较: cf=%v", cf)
	}
	// ⑤ 其余字段（无独立证据）⇒ 只记冲突，**不覆盖**客户端值
	k, cf := MergeEpidKey(ParseEpisodeKey("20260803(第1期加更上)"), base, false)
	if k.Variant != "加更" {
		t.Fatalf("非硬证据字段不得被覆盖: variant=%q", k.Variant)
	}
	if len(cf) != 1 || cf[0] != "variant" {
		t.Fatalf("应记录 variant 冲突: %v", cf)
	}
}

// PickEpisodeKeyed（结构化键路径）与 PickEpisode（集名路径）必须**结果一致** —— 否则接口换路后会回归。
func TestMultiVariant_SeparatorNotCollidingWithEpid(t *testing.T) {
	// M1（2026-11-11 评审）：多变体集合**不得用 `|` 连接** —— `|` 是 epid/epk 的段分隔符，
	// 「特辑|花絮」会让串变成 7 段 ⇒ 服务端拒收 + web parseEpid 恒 null + 落库坏串 ✗
	k := ParseEpisodeKey("20260803(第1期上)特辑花絮")
	if strings.Contains(k.Variant, "|") {
		t.Fatalf("变体集合不得含 `|`（段分隔符冲突）, got %q", k.Variant)
	}
	if !strings.Contains(k.Variant, ",") ||
		!strings.Contains(k.Variant, "特辑") || !strings.Contains(k.Variant, "花絮") {
		t.Fatalf("多变体应以 `,` 连接且词齐全, got %q", k.Variant)
	}
	// ① epid 恒 5 段（4 个分隔符）——多变体也不例外
	ep := EpidOf(k)
	if n := strings.Count(ep, "|"); n != 4 {
		t.Fatalf("epid 必须恒 5 段, got %d 个分隔符: %q", n, ep)
	}
	if !strings.Contains(ep, "v="+k.Variant) {
		t.Fatalf("v 段应含逗号连接的多变体: %q", ep)
	}
	// ② epk 往返：多变体不得被服务端拒收
	s := k.Serialize(0)
	got, ok := ParseEpisodeKeyString(s)
	if !ok {
		t.Fatalf("多变体 epk 必须可解析（否则服务端拒收）: %q", s)
	}
	if got.Variant != k.Variant {
		t.Fatalf("往返后变体应保持: %q vs %q", got.Variant, k.Variant)
	}
	// ③ 回归：单变体仍然正常
	one := ParseEpisodeKey("20260803(第1期上)特辑")
	if one.Variant != "特辑" || strings.Count(EpidOf(one), "|") != 4 {
		t.Fatalf("单变体不应受影响: %q / %q", one.Variant, EpidOf(one))
	}
}

func TestParseEpKeySource(t *testing.T) {
	cases := []struct{ in, want string }{
		{"v1|d=20260803|n=1|from=anchor", EpKeySourceAnchor},
		{"v1|d=|n=0|from=self", EpKeySourceSelf},
		{"v1|d=|n=0|from=bridge", EpKeySourceBridge},
		{"v1|d=|n=0|from=none", EpKeySourceNone},
		{"v1|d=|n=0", ""},           // 缺省 ⇒ 未提供
		{"", ""},                    // 空串
		{"v1|d=|n=0|from=weird", ""}, // 未知值 ⇒ 不认（避免脏统计）
		{"v1|d=|n=0|from=", ""},      // 空值 ⇒ 不认
	}
	for _, c := range cases {
		if got := ParseEpKeySource(c.in); got != c.want {
			t.Errorf("ParseEpKeySource(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPickEpisodeKeyed_与集名路径等价(t *testing.T) {
	lib := []DandanEpisode{
		{EpisodeID: 1, EpisodeTitle: "【qq】 第1期上：又争又抢"},
		{EpisodeID: 2, EpisodeTitle: "特辑2"},
		{EpisodeID: 3, EpisodeTitle: "第2期下"},
	}
	cases := []struct {
		title  string
		appNum int
		want   int64
	}{
		{"20260803上", 1, 1},        // A2 桥接期号 = 1 ⇒ 命中「第1期上」
		{"20260809特辑2", 0, 2},     // 变体序号复合 ⇒ L3a 组内唯一
		{"20260810第2期下", 0, 3},   // 期号直配
	}
	for _, c := range cases {
		k := ParseEpisodeKey(c.title)
		keyed := PickEpisodeKeyed(lib, 999, c.appNum, k, true)
		titled := PickEpisode(lib, 999, c.appNum, c.title)
		if keyed == nil || titled == nil {
			t.Fatalf("%s: keyed=%v titled=%v（应都命中）", c.title, keyed, titled)
		}
		if keyed.EpisodeID != titled.EpisodeID || keyed.EpisodeID != c.want {
			t.Fatalf("%s: keyed=%d titled=%d want=%d", c.title, keyed.EpisodeID, titled.EpisodeID, c.want)
		}
	}
	// 既有键又无集名 ⇒ hasKey=false ⇒ 走旧策略（与改动前一致）
	if got := PickEpisodeKeyed(lib, 2, 0, EpisodeKey{}, false); got == nil || got.EpisodeID != 2 {
		t.Fatalf("无键时应走旧策略命中第 2 条, got %v", got)
	}
	// 越界/空列表 ⇒ nil（不 panic）
	if PickEpisodeKeyed(nil, 1, 0, EpisodeKey{}, true) != nil || PickEpisodeKeyed(lib, 0, 0, EpisodeKey{}, true) != nil {
		t.Fatal("空列表/越界集号应返回 nil")
	}
}

// P2（2026-11-12 评审）：日期尾随「当日集序」进 p 段 —— 202401011/202401012 = 同一期的两集,
// 序号位此前被静默丢弃 ⇒ 两集 epid 逐字相同（①强等跨源错配、弹幕池合并）。
func TestDateSeqOrdinal_P2(t *testing.T) {
	k1 := ParseEpisodeKey("202401011")
	if k1.Date != "20240101" || k1.Part != "1" {
		t.Fatalf("k1 = %+v, want Date=20240101 Part=1", k1)
	}
	k2 := ParseEpisodeKey("202401012")
	if k2.Date != "20240101" || k2.Part != "2" {
		t.Fatalf("k2 = %+v, want Date=20240101 Part=2", k2)
	}
	if EpidOf(k1) == EpidOf(k2) {
		t.Fatal("两集 epid 不得相同: " + EpidOf(k1))
	}
	// 同集判定(服务端匹配口径 SameEpisode): 同名重解析判同; p=1 与 p=2 是不同集
	if !SameEpisode(k1, ParseEpisodeKey("202401011")) {
		t.Fatal("同集名重解析应判同集")
	}
	if SameEpisode(k1, k2) {
		t.Fatal("p=1 与 p=2 是不同集, 不得判同(数字序不做前缀相容)")
	}
	// 回归: 纯 8 位日期不带序号, 行为不变
	if k0 := ParseEpisodeKey("20240101"); k0.Part != "" || k0.Date != "20240101" {
		t.Fatalf("k0 = %+v", k0)
	}
	// 8+2 位形态(传奇故事2023 的 2023060101/0102 同型)⇒ 集序 13
	if k4 := ParseEpisodeKey("2024010113"); k4.Part != "13" || k4.Date != "20240101" {
		t.Fatalf("k4 = %+v, want Part=13 Date=20240101", k4)
	}
	// 不透明长编号(≥3 位尾随数字)不拆序号
	if k3 := ParseEpisodeKey("20240101113"); k3.Part != "" || k3.Date != "20240101" {
		t.Fatalf("k3 = %+v, want Part 空且 Date=20240101", k3)
	}
	// seriesOf: 尾随集序一并占位, 残渣不得拼进栏目段
	if s := seriesOf("202401011旅行"); s != "旅行" {
		t.Fatalf("seriesOf = %q, want 旅行", s)
	}
}

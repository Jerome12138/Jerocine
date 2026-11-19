package danmu

import "testing"

// ===== R18 / R19 / R20（2026-11-16）逐条防呆 =====
//
// 背景：这三条是**逐集分派前置段**（`preRules`），修的是"附属站行把打包词/状态词/跨集区间
// 误算成 `s` 段 ⇒ 与主站正常 `第N集` 永不相等 ⇒ 一行坏集名毁一整片"。
// 诊断与规模见 Harness `docs/epid-集身份/分析-对齐率分品类归因与优化方案-2026-11-16.md`。
//
// 这些用例**故意偏离** `legacyRowRef`（重构前 if 链）⇒ 不在 `TestDecideRule_MatchesLegacyChain`
// 的等价语料里，改由本文件逐条钉住（该测试对这三条做豁免，见其"intentional"集合）。

// R18：`第N集` + 状态尾缀（全/完结/已完结）⇒ 清掉被误当栏目名的尾缀，保留 n。
func TestRuleR18_StatusTail(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string // 期望 EpidFor 的 epid
		id    RuleID // 期望命中的规则
	}{
		{"完结尾缀", "第178集完结", "s=-|v=-|p=-|n=178|d=-", RulePreStatus},
		{"已完结尾缀", "第360集已完结", "s=-|v=-|p=-|n=360|d=-", RulePreStatus},
		{"期单位同样适用", "第12期完结", "s=-|v=-|p=-|n=12|d=-", RulePreStatus},
		// 「全」单字尾缀既有链已正确：seriesOf 只取 2~8 字片段 ⇒ `全` 不入 s（本用例兼作回归钉子）
		{"全尾缀(既有链已正确)", "第3集全", "s=-|v=-|p=-|n=3|d=-", RuleSelfInName},
		// 真有栏目名：`第N集` 被占位符切开 ⇒ s 只取到栏目名（既有链已正确，R18 不该动它）
		{"真栏目名不受影响", "风味人间第3集完结", "s=风味人间|v=-|p=-|n=3|d=-", RuleSelfInName},
		// 无 `第N集` 前缀 ⇒ 不是状态尾缀形态（交给 R19 处理）
		{"无第N集前缀不走本条", "全集合集完结", "s=-|v=-|p=-|n=1|d=-", RulePrePack},
		// 负例：普通集名
		{"普通第N集", "第01集", "s=-|v=-|p=-|n=1|d=-", RuleSelfInName},
	}
	for _, tc := range cases {
		got, _, id := EpidForRule(tc.title, []string{tc.title})
		if got != tc.want || id != tc.id {
			t.Errorf("%s（%q）: got (%q, %s), want (%q, %s)", tc.name, tc.title, got, id, tc.want, tc.id)
		}
	}
}

// R19：打包条目（全集/合集）+ **该源行只有 1 集** ⇒ n=1（同 D7 单集语义）；
// 源行多集 ⇒ 不命中，仍按 D3 保留系列身份（行为不变）。
func TestRuleR19_PackAsSingleEpisode(t *testing.T) {
	t.Run("单集源行⇒n=1", func(t *testing.T) {
		for _, title := range []string{"全集", "合集", "全集完结", "合全集", "电影解说合集"} {
			got := EpidsForSourceRowWithPool([]string{title}, []string{title})
			if len(got) != 1 || got[0] != [2]string{"s=-|v=-|p=-|n=1|d=-", EpKeySourceSingle} {
				t.Errorf("%q: got %v, want n=1 / single", title, got)
			}
			if !RuleSelectsRow(RulePrePack, []string{title}) {
				t.Errorf("%q: RuleSelectsRow 应命中 R19", title)
			}
		}
	})

	t.Run("多集源行⇒不命中(保留系列身份)", func(t *testing.T) {
		titles := []string{"全集", "第01集", "第02集"}
		got := EpidsForSourceRowWithPool(titles, titles)
		if len(got) != 3 || got[0] != [2]string{"s=全集|v=-|p=-|n=-|d=-", EpKeySourceNone} {
			t.Errorf("首行（打包行）got %v, want 系列身份 / none（行为不变）", got)
		}
		if RuleSelectsRow(RulePrePack, titles) {
			t.Error("源行多集时 RuleSelectsRow 不该命中 R19（否则 scope 会把不该重算的行圈进来）")
		}
	})

	t.Run("含数字/带日期的行不受本条影响", func(t *testing.T) {
		// 含 ASCII 数字 ⇒ 不吃（与 D7 守卫同族）
		if got, _, id := EpidForRule("合集03", []string{"合集03"}); id == RulePrePack {
			t.Errorf("含数字的打包名不该走 R19，得 epid=%q", got)
		}
		// 有日期 ⇒ 交给 D4/D5/D6 的既有链路
		if _, _, id := EpidForRule("20260803全集", []string{"20260803全集"}); id == RulePrePack {
			t.Error("带日期的行不该走 R19")
		}
	})
}

// R20：跨集区间 ⇒ 打包条目，暂不产身份；且**必须抢在 R19 之前**（`第01-84合集` 同时含打包词）。
func TestRuleR20_RangePackNoIdentity(t *testing.T) {
	// 2026-11-19 起：严格"第X-Y集/期"归一为 n=first（区间→n 批）；"合集"/异常仍 skip。
	for _, title := range []string{"第61-80集完结", "第1~20集", "第 3 - 10 期"} {
		got, from, id := EpidForRule(title, []string{title})
		if got == "" || from != EpKeySourceSkip || id != RulePreRange {
			t.Errorf("%q: got (%q, %s, %s), want 非空 epid / skip / %s", title, got, from, id, RulePreRange)
		}
	}
	// 打包语义（合集）仍不归一
	if got, from, id := EpidForRule("第01-84合集", []string{"第01-84合集"}); got != "" || from != EpKeySourceSkip || id != RulePreRange {
		t.Errorf("合集: got (%q, %s, %s), want 空 epid / skip / %s", got, from, id, RulePreRange)
	}
	// 负例：单个 `第N集` 不是区间
	if _, _, id := EpidForRule("第80集", []string{"第80集"}); id != RuleSelfInName {
		t.Errorf("单个第N集不该走 R20，得 %s", id)
	}
}

// 前置性：R20 在 R19 之前 —— 区间型（含打包词）不得被 R19 当成"单集整包"给 n=1。
func TestRuleR18R20_Precedence(t *testing.T) {
	got, _, id := EpidForRule("第01-84合集", []string{"第01-84合集"})
	if id != RulePreRange || got != "" {
		t.Errorf("区间+打包词：得 (%q, %s)，应先被 R20 挡下（空 + %s）", got, id, RulePreRange)
	}
}

// RuleSelectsRow 是 `mode=scope&rule=<ID>` 的**选行谓词**（与规则本体同源）：
// R18~R20 的 `FansOutMovie=false` ⇒ `ScopeRequest.Resolve` 自动启用它 ⇒ 范围精确到"命中该形态的源行"。
func TestRuleSelectsRow_R18R20(t *testing.T) {
	cases := []struct {
		name   string
		id     RuleID
		titles []string
		want   bool
	}{
		{"R18 命中", RulePreStatus, []string{"第178集完结"}, true},
		{"R18 负例", RulePreStatus, []string{"第178集"}, false},
		{"R19 命中(单集源行)", RulePrePack, []string{"全集"}, true},
		{"R19 门控：源行多集 ⇒ 不命中", RulePrePack, []string{"全集", "第01集"}, false},
		{"R20 命中", RulePreRange, []string{"第61-80集完结"}, true},
		{"R20 负例", RulePreRange, []string{"第61集"}, false},
	}
	for _, tc := range cases {
		if got := RuleSelectsRow(tc.id, tc.titles); got != tc.want {
			t.Errorf("%s：RuleSelectsRow(%s, %v) = %v, want %v", tc.name, tc.id, tc.titles, got, tc.want)
		}
	}
}

// 登记项自洽：R18/R19 是确定层 ⇒ 产出 self/single；R20 是待定层（观察项）⇒ 产出 skip。
// 三条都必须可收紧（Scope=from-src + InFromSrc + 不扇出整片）。
func TestRuleMeta_R18R20(t *testing.T) {
	for _, id := range []RuleID{RulePreStatus, RulePrePack, RulePreRange} {
		m := RuleMetaOf(id)
		if m.Scope != ScopeFromSrc || len(m.InFromSrc) == 0 {
			t.Errorf("%s: 必须可收紧（Scope=from-src + InFromSrc），得 scope=%s in=%v", id, m.Scope, m.InFromSrc)
		}
		if m.FansOutMovie {
			t.Errorf("%s: 不读池 ⇒ FansOutMovie 必须为 false（否则 scope 不能启用选行谓词）", id)
		}
		if m.Desc == "" {
			t.Errorf("%s: 缺 Desc", id)
		}
	}
	if m := RuleMetaOf(RulePreRange); m.Layer != LayerPending {
		t.Errorf("R20 是观察项 ⇒ 层应为待定，得 %s", m.Layer)
	}
	if m := RuleMetaOf(RulePreStatus); m.OutFromSrc != EpKeySourceSelf {
		t.Errorf("R18 产出档应为 self，得 %s", m.OutFromSrc)
	}
	if m := RuleMetaOf(RulePrePack); m.OutFromSrc != EpKeySourceSingle {
		t.Errorf("R19 产出档应为 single，得 %s", m.OutFromSrc)
	}
}

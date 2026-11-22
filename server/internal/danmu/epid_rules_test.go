package danmu

import (
	"strings"
	"sync/atomic"
	"testing"
)

// ===== 决策规则登记表（`epid_rules.go`）的防呆（2026-11-14）=====

// TestDecisionRuleMetas_Integrity 登记表自检：
//   - 编号唯一、顺序 = 求值顺序（D1~D6 → D7/D8）；
//   - 产出档 `OutFromSrc` 必须在**既有值域**内（**不许新增 `from_src` 取值** ——
//     它会随 `epk` 上传且 `ParseEpKeySource` 有白名单 ⇒ 扩值域 = 跨端合约变更）；
//   - `Scope` 与 `InFromSrc` 自洽（`all` 不许带档；`from-src` 必须带且档名合法）；
//   - **确定层 ⇔ 产出档 ∈ {self, single}**（§3.1.1 的结构性判据：身份直接写在集名里）。
func TestDecisionRuleMetas_Integrity(t *testing.T) {
	metas := DecisionRuleMetas()
	wantOrder := []RuleID{
		// 前置段（2026-11-16，R18~R20）：求值顺序在 D2/D3 之前，编号沿用 R 系列号
		RulePreRange, RulePreRangeSkip, RulePrePack,
		// D10a（2026-11-22）求值位置在 D1 之后、D2 之前（n-only 行先试反查借 d，不成回落 D2-self）
		RuleSkipNoTrigger, RuleDualAnchorRev, RuleSelfInName, RuleNoneNoDate, RuleBridgeNum, RuleAnchorNum,
		RuleNonePoolMiss, RuleSingleEp, RuleBareNum,
	}
	if len(metas) != len(wantOrder) {
		t.Fatalf("登记项数 = %d, want %d", len(metas), len(wantOrder))
	}
	known := map[string]bool{
		EpKeySourceSelf: true, EpKeySourceBridge: true, EpKeySourceAnchor: true,
		EpKeySourceNone: true, EpKeySourceSkip: true, EpKeySourceSingle: true,
	}
	seen := map[RuleID]bool{}
	for i, m := range metas {
		if m.ID != wantOrder[i] {
			t.Errorf("第 %d 条 = %q, want %q（顺序即优先级，不得改）", i, m.ID, wantOrder[i])
		}
		if seen[m.ID] {
			t.Errorf("编号重复：%q", m.ID)
		}
		seen[m.ID] = true
		if m.Desc == "" {
			t.Errorf("%s 缺 Desc（登记表是文档的单一事实源）", m.ID)
		}
		if !known[m.OutFromSrc] {
			t.Errorf("%s 的产出档 %q 不在既有值域（不得新增 from_src 取值）", m.ID, m.OutFromSrc)
		}
		switch m.Layer {
		case LayerStable, LayerPending:
		default:
			t.Errorf("%s 的层 %q 非法", m.ID, m.Layer)
		}
		switch m.Scope {
		case ScopeAll:
			if len(m.InFromSrc) != 0 {
				t.Errorf("%s: Scope=all 不应给 InFromSrc（影响所有行）", m.ID)
			}
		case ScopeFromSrc:
			if len(m.InFromSrc) == 0 {
				t.Errorf("%s: Scope=from-src 必须给 InFromSrc", m.ID)
			}
			for _, s := range m.InFromSrc {
				if !known[s] {
					t.Errorf("%s 的 InFromSrc 含未知档 %q", m.ID, s)
				}
			}
		default:
			t.Errorf("%s 的 Scope %q 非法", m.ID, m.Scope)
		}
		// 确定层 ⇔ 产出 self/single（"期号聚集类" + "单集电影类"）
		stable := m.Layer == LayerStable
		identInName := m.OutFromSrc == EpKeySourceSelf || m.OutFromSrc == EpKeySourceSingle
		if stable != identInName {
			t.Errorf("%s: 层(%s) 与产出档(%s) 不自洽（确定层 ⇔ self/single）", m.ID, m.Layer, m.OutFromSrc)
		}
	}
}

// TestDecisionRuleMetas_RecomputeScope 钉住"改某条规则 ⇒ 重算范围怎么圈"：
// 本批三条（R11/R12/R13）的**输入档都是 `skip`** ⇒ 范围 = `from_src='skip'` 的源行（+ R11 按片）。
// R12/R13 是行级补充，定义域**恰好**是 `skip`；池一族（D4/D5/D6）定义域 = 到达池的那些档。
func TestDecisionRuleMetas_RecomputeScope(t *testing.T) {
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	for _, id := range []RuleID{RuleSingleEp, RuleBareNum} {
		m := RuleMetaOf(id)
		if m.Scope != ScopeFromSrc || !eq(m.InFromSrc, []string{EpKeySourceSkip}) {
			t.Errorf("%s 的定义域应为 [skip]（只填空规则的输入档），得 scope=%s in=%v", id, m.Scope, m.InFromSrc)
		}
		if m.FansOutMovie {
			t.Errorf("%s 不应扇出到整片（行级补充，不动池）", id)
		}
	}
	for _, id := range []RuleID{RuleBridgeNum, RuleAnchorNum, RuleNonePoolMiss} {
		m := RuleMetaOf(id)
		if !m.FansOutMovie {
			t.Errorf("%s 会经借用池影响同片其它集 ⇒ FansOutMovie 必须为 true（重算须按 match_key 整片）", id)
		}
		found := false
		for _, s := range m.InFromSrc {
			if s == EpKeySourceAnchor {
				found = true
			}
		}
		if !found {
			t.Errorf("%s 的定义域应含 anchor（到达池的档）", id)
		}
	}
	// D10a 双锚反查（2026-11-22）：borrower = n-only 行 —— 存量 n-only 行记 self、借成记 bridge
	// ⇒ 定义域 = [self, bridge]；借/不借都读片级池 ⇒ FansOutMovie=true。
	{
		m := RuleMetaOf(RuleDualAnchorRev)
		if !m.FansOutMovie {
			t.Errorf("D10a 会经借用池影响同片其它集 ⇒ FansOutMovie 必须为 true")
		}
		if m.Scope != ScopeFromSrc || !eq(m.InFromSrc, []string{EpKeySourceSelf, EpKeySourceBridge}) {
			t.Errorf("D10a 的定义域应为 [self, bridge]（n-only 行的存量/借成档），得 scope=%s in=%v", m.Scope, m.InFromSrc)
		}
	}
	// 门 / 提取型（D1~D3）只能全量
	for _, id := range []RuleID{RuleSkipNoTrigger, RuleSelfInName, RuleNoneNoDate} {
		if m := RuleMetaOf(id); m.Scope != ScopeAll {
			t.Errorf("%s 是门/提取型 ⇒ 定义域必须是 all（改它影响所有行）", id)
		}
	}
}

// TestRuleSelectsRow `mode=scope` 的选行谓词（2026-11-14）：
// 必须与规则本体**同源** —— 这里用 `EpidsForSourceRowWithPool` 独立交叉验证 D7（`single` 是它的**唯一**产出档，
// 可无歧义比对），再逐个钉住 D8 的正/负例与守卫。
func TestRuleSelectsRow(t *testing.T) {
	cases := []struct {
		name   string
		id     RuleID
		titles []string
		want   bool
	}{
		{"D7 命中：单集 + PDF 名（无 ASCII 数字）", RuleSingleEp, []string{"HD国语"}, true},
		{"D7 命中：单集 + 汉字片名", RuleSingleEp, []string{"正片"}, true},
		{"D7 负例：整行 2 集（守卫①）", RuleSingleEp, []string{"正片", "HD"}, false},
		{"D7 负例：单集但含 ASCII 数字（守卫②）", RuleSingleEp, []string{"第1集"}, false},
		{"D7 负例：空行", RuleSingleEp, nil, false},
		{"D8 命中：行内 distinct 裸数字 ≥2", RuleBareNum, []string{"1", "2", "3"}, true},
		{"D8 命中：补零也认", RuleBareNum, []string{"001", "002"}, true},
		{"D8 负例：只有 1 个裸数字（守卫：单条可能是线路号）", RuleBareNum, []string{"1"}, false},
		{"D8 负例：6 位数字不是裸数字（归 R11）", RuleBareNum, []string{"20260803", "20260810"}, false},
		{"D8 负例：带期号的集名", RuleBareNum, []string{"第1集", "第2集"}, false},
	}
	for _, tc := range cases {
		if got := RuleSelectsRow(tc.id, tc.titles); got != tc.want {
			t.Errorf("%s: RuleSelectsRow(%s, %v) = %v, want %v", tc.name, tc.id, tc.titles, got, tc.want)
		}
	}
	// 独立交叉验证：D7 的命中面 ⟺ 该行真的会产出 `single`（`single` 只有 D7 一个产出者 ⇒ 无歧义）。
	for _, titles := range [][]string{
		{"HD国语"}, {"正片"}, {"正片", "HD"}, {"第1集"}, {"1", "2"}, {"HD中字"},
	} {
		hasSingle := false
		for _, p := range EpidsForSourceRowWithPool(titles, titles) {
			if p[1] == EpKeySourceSingle {
				hasSingle = true
			}
		}
		if got := RuleSelectsRow(RuleSingleEp, titles); got != hasSingle {
			t.Errorf("D7 选行与真实派生不一致：titles=%v RuleSelectsRow=%v 实际产出single=%v", titles, got, hasSingle)
		}
	}
	// D8 的命中面必须 ⊆ "该行有集被判为 self"（`self` 另有 D2 产出者，故只做单向包含）。
	for _, titles := range [][]string{{"1", "2", "3"}, {"001", "002"}, {"1"}, {"第1集", "第2集"}} {
		hasSelf := false
		for _, p := range EpidsForSourceRowWithPool(titles, titles) {
			if p[1] == EpKeySourceSelf {
				hasSelf = true
			}
		}
		if RuleSelectsRow(RuleBareNum, titles) && !hasSelf {
			t.Errorf("D8 选行说命中，但该行没有 self 产出：titles=%v", titles)
		}
	}
}

// ---- 以下为"参照实现"的**if 链形态**（2026-11-22 D10 起与规则表同规的降级链），
// 仅作**行为等价**对照物：自带 n（n-only 先试双锚反查借 d）→ 无日期 none → 桥接(=双锚正查) → none。

func legacyEpidForRef(title string, allTitles []string) (epid string, fromSrc string) {
	k := ParseEpisodeKey(title)
	if !EpisodeTriggered(k) {
		return "", EpKeySourceSkip
	}
	if k.Num > 0 {
		// D10a：n-only 行先试双锚互鉴·反查借 d（借成记 bridge；不成保留 self，n 不变）
		if k.Date == "" {
			keys := make([]EpisodeKey, 0, len(allTitles))
			for _, t := range allTitles {
				keys = append(keys, ParseEpisodeKey(t))
			}
			if d := DualAnchorDate(k, keys); d != "" {
				k.Date = d
				return EpidOf(k), EpKeySourceBridge
			}
		}
		return EpidOf(k), EpKeySourceSelf
	}
	if k.Date == "" {
		return EpidOf(k), EpKeySourceNone
	}
	keys := make([]EpisodeKey, 0, len(allTitles))
	for _, t := range allTitles {
		keys = append(keys, ParseEpisodeKey(t))
	}
	if n := BridgeNum(k, keys); n > 0 {
		k.Num = n
		return EpidOf(k), EpKeySourceBridge
	}
	// D10（2026-11-22）：锚点兜底已退役 ⇒ 有日期但正查不成 ⇒ none（n 留空，身份按日期）
	return EpidOf(k), EpKeySourceNone
}

func legacyRowRef(titles, pool []string) [][2]string {
	if len(pool) == 0 {
		pool = titles
	}
	out := make([][2]string, 0, len(titles))
	onlyOne := 0
	for _, t := range titles {
		if strings.TrimSpace(t) != "" {
			onlyOne++
		}
	}
	bare := make(map[int]struct{}, len(titles))
	for _, t := range titles {
		if v, ok := bareNum(t); ok {
			bare[v] = struct{}{}
		}
	}
	bareOK := len(bare) >= 2
	for _, t := range titles {
		epid, from := legacyEpidForRef(t, pool)
		if epid == "" && from == EpKeySourceSkip {
			switch {
			case onlyOne == 1 && !hasASCIIDigit(t):
				epid, from = EpidOf(EpisodeKey{Num: 1}), EpKeySourceSingle
			case bareOK:
				if v, ok := bareNum(t); ok {
					epid, from = EpidOf(EpisodeKey{Num: v}), EpKeySourceSelf
				}
			}
		}
		out = append(out, [2]string{epid, from})
	}
	return out
}

// TestDecideRule_MatchesLegacyChain 有序规则表必须与**参照 if 链**（legacyEpidForRef，
// D10 起与规则表同规的降级链）**逐格等价**（`epid` 与 `from_src` 都相同），
// 并顺带要求语料**覆盖全部在产规则**（否则等价性是空跑）。
func TestDecideRule_MatchesLegacyChain(t *testing.T) {
	cases := []struct {
		name         string
		titles, pool []string
	}{
		{"D1 未触发（纯片名/HD）", []string{"正片", "HD"}, []string{"正片", "HD"}},
		{"D2 自带期号", []string{"第1集", "第2集", "第3集"}, []string{"第1集", "第2集", "第3集"}},
		// D3：触发但无日期（真栏目、无期号无日期）。R17 后「HD中字」剥离修饰词走 D7-single，
		// 不再是 D3 语料 —— 换真栏目形态保持 D3 覆盖。
		{"D3 触发但无日期（真栏目，无日期无期号）", []string{"风味人间"}, []string{"风味人间"}},
		{"D4 他源桥接（同日期 + 他源带期号）", []string{"20260803"}, []string{"20260803", "20260803第7期"}},
		{"D10a 双锚反查借 d（n-only 行 + 双锚行；v2 双侧空算相等）", []string{"第5期"}, []string{"第5期", "第5期20260601"}},
		{"D10a 反查 p 不等 ⇒ 回落 D2-self（v2 dual-tight 收紧，p 空≠p 上）", []string{"第5期"}, []string{"第5期", "第5期20260601上"}},
		{"D10a 反查不成 ⇒ 回落 D2-self（无双锚行）", []string{"第5期"}, []string{"第5期"}},
		// D10（2026-11-22）撤销锚点后的 D6 语料：池里只有 d+p 行（无双锚行）⇒ 一律不推 n
		{"D6 有日期但正查皆败（原 D5 语料，锚点已退役）",
			[]string{"20260803上", "20260810上", "20260817上", "20260803"},
			[]string{"20260803上", "20260810上", "20260817上", "20260803"}},
		{"D6 有日期但借推皆败", []string{"20260803"}, []string{"20260803"}},
		{"D7 单集（R12）", []string{"正片"}, []string{"正片"}},
		{"D8 裸数字（R13）", []string{"1", "2", "3"}, []string{"1", "2", "3"}},
		{"E 圆号不触发", []string{"E01.220705"}, []string{"E01.220705"}},
		{"空池退化", []string{"1", "02"}, nil},
	}
	covered := map[RuleID]bool{}
	for _, tc := range cases {
		got := EpidsForSourceRowWithPool(tc.titles, tc.pool)
		want := legacyRowRef(tc.titles, tc.pool)
		if len(got) != len(want) {
			t.Fatalf("%s: 长度 %d != %d", tc.name, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s[%d]: got %v, want %v", tc.name, i, got[i], want[i])
			}
		}
		pool := tc.pool
		if len(pool) == 0 {
			pool = tc.titles
		}
		ctx := newRuleCtx(tc.titles, pool)
		for _, title := range tc.titles {
			_, _, id := decideRow(title, ctx)
			covered[id] = true
		}
	}
	// R18~R20 是**有意偏离旧链**的新规则（旧链把「打包词/状态词/跨集区间」算进 `s` 段，
	// 与主站正常 `第N集` 永不相等 ⇒ 一行坏集名毁一整片）⇒ 不进"等价性"语料，
	// 由 `epid_r18_r20_test.go` 逐条钉住。
	// D5-anchor（D10, 2026-11-22）已退役：apply 恒不命中 ⇒ 语料**不可能**覆盖（编号不复用，
	// 登记项保留用于圈存量 anchor 档的退役重算）。
	intentional := map[RuleID]bool{
		RulePreStatus: true, RulePrePack: true, RulePreRange: true, RulePreRangeSkip: true,
		RuleAnchorNum: true,
	}
	for _, m := range DecisionRuleMetas() {
		if intentional[m.ID] {
			continue
		}
		if !covered[m.ID] {
			t.Errorf("语料未覆盖 %s（%s）—— 等价性测试会空跑", m.ID, m.Desc)
		}
	}
}

// TestDeriveRow_ParsesPoolOnce 性能防呆（2026-11-15 深夜排障）：
// D4（桥接）/D5（锚点）曾对**同一个池**在行内每个集名上重复 ParseEpisodeKey ⇒
// E×P 二次方（线上实锤：食尚玩家 585 集 × 池 6253 = 367 万次解析/行，单行磨 ~1 分钟、
// 一批 8 分钟）。修复 = ruleCtx 惰性缓存池键（poolKeysOf）+ D5 改键版 AnchorNumForKey
// ⇒ 一次源行派生的解析次数 = E + P。本测试用计数缝钉住上限，回归到二次方即失败。
func TestDeriveRow_ParsesPoolOnce(t *testing.T) {
	orig := parseEpisodeKey
	var n atomic.Int64
	parseEpisodeKey = func(s string) EpisodeKey {
		n.Add(1)
		return orig(s)
	}
	t.Cleanup(func() { parseEpisodeKey = orig })
	titles := []string{"20260803", "20260810", "20260817", "20260824", "20260831",
		"20260907", "20260914", "20260921"} // E=8，全日期形态（必走 D4→D5）
	pool := []string{ // P=40 个日期型池标题
		"20250101", "20250102", "20250103", "20250104", "20250105", "20250106", "20250107", "20250108",
		"20250109", "20250110", "20250111", "20250112", "20250113", "20250114", "20250115", "20250116",
		"20250117", "20250118", "20250119", "20250120", "20250121", "20250122", "20250123", "20250124",
		"20250125", "20250126", "20250127", "20250128", "20250201", "20250202", "20250203", "20250204",
		"20250205", "20250206", "20250207", "20250208", "20250209", "20250210", "20250211", "20250212",
	}
	_ = EpidsForSourceRowWithPool(titles, pool)
	// 修复前 = E×P + E(D5 自解析) + E(自身解析) ≈ 3×E×P 量级；修复后 = E + P = 48。
	if got := n.Load(); got > int64(len(titles)+len(pool)) {
		t.Fatalf("池被重复解析: ParseEpisodeKey 调用 %d 次, 上限 E+P=%d", got, len(titles)+len(pool))
	}
}

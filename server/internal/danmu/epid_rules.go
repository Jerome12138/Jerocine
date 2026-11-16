package danmu

import (
	"regexp"
	"strings"
)

// ===== 决策规则登记表（2026-11-14，"有序规则"落地）=====
//
// 解析链分三族（口径来源：Harness `docs/epid-集身份/审查-epid服务端规则与批量补采-2026-11-14.md` §3.1.1）：
//
//	① **预处理族**（R1 引号 / R2 变体跨度 / R3 括号 / R4 reENum / R6 日期合法性 / R7 未播 / R8 占位）——
//	   都在 `ParseEpisodeKey` 内做**文本归一化**，**没有单一"命中"语义**（一条集名常同时被多条命中，
//	   如 `"20261001"聚"乐部第5期` 同时吃 R1+R2）⇒ **不进本表**。
//	② **使能族**（R5 纯数字集名 ⇒ `Num` / R11 6 位日期 ⇒ `Date`）—— 不"决定"身份，只让该行
//	   **变成**自带期号/日期；效果**已经体现在产出档 `from_src` 上** ⇒ 也不单独登记。
//	③ **决策族 = 本表**（`decideTitle` 的有序分派 + `decideRow` 的行级补充）——
//	   唯一可**编号 / 函数化 / 顺序化**的部分。
//
// 本表同时是 §3.1.1 那张登记表的**单一事实源**（文档与代码同源，防漂移；测试 `epid_rules_test.go` 钉住）。
// 每条规则登记四件事：
//
//	· Layer        —— 确定 / 待定（**认知与审计**用：改这条规则时，它的影响半径是"本行"还是"整片"）
//	· Scope/InFromSrc —— **执行**用：改这条规则时，**重算范围**怎么圈（按 `from_src` 圈，还是只能全量）
//	· OutFromSrc   —— 产出档（写进 `movie_episode_epid.from_src`；**与既有值域一致，不新增取值**）
//	· FansOutMovie —— 会经借用池影响**同片其它集** ⇒ 重算须按 `match_key` 整片，不能只重算本行
//
// ⚠️ **RuleID 一旦发布不得重编、不得复用**（形同错误码）。将来若把"命中规则"落库到
// `movie_episode_epid`，存的就是这一列；`EpidForRule` 已是它的产出点。

// RuleID 决策规则的稳定编号（编号 = 求值顺序：`D1`~`D6` 是逐集分派，`D7`/`D8` 是行级补充）。
type RuleID string

const (
	RuleSkipNoTrigger RuleID = "D1-skip"      // 未触发 ⇒ 不产身份
	RuleSelfInName    RuleID = "D2-self"      // 集名自带期号 ⇒ 直接采用
	RuleNoneNoDate    RuleID = "D3-none"      // 触发但无日期（身份来自 栏目/变体/主次）
	RuleBridgeNum     RuleID = "D4-bridge"    // 缺期号但有日期 ⇒ 他源桥接
	RuleAnchorNum     RuleID = "D5-anchor"    // 桥接不成 ⇒ 锚点推算（同栏目组内「上」的日期序）
	RuleNonePoolMiss  RuleID = "D6-none-miss" // 有日期但桥接、锚点皆败 ⇒ 仍不产期号
	RuleSingleEp      RuleID = "D7-single"    // R12（行级补充）
	RuleBareNum       RuleID = "D8-bare-num"  // R13（行级补充）

	// ---- 逐集分派**前置**段（2026-11-16）----
	// **编号即求值序**：`P1 → P2 → P3 → D1 → … → D8`（P = pre-stage，全部在 D1 之前）。
	// 为什么必须前置：这三类形态在旧链里会被 `D2`/`D3` 先接走，产出被"打包词/状态词"污染的身份
	// （诊断见 Harness `分析-对齐率分品类归因与优化方案-2026-11-16.md`）。
	// ⚠️ 2026-11-16 改名（未发布前）：原 `R20-range`→`P1-range`、`R18-status`→`P2-status`、`R19-pack`→`P3-pack`
	// —— 老文档里的 R18/R19/R20 与此一一对应。
	RulePreRange  RuleID = "P1-range"  // 跨集区间（第X-Y集）⇒ 打包条目，暂不产身份（观察项）
	RulePreStatus RuleID = "P2-status" // `第N集` + 状态尾缀（全/完结/已完结）⇒ 尾缀不是栏目名
	RulePrePack   RuleID = "P3-pack"   // 打包条目（全集/合集）+ 该源行只有 1 集 ⇒ n=1
)

// RuleLayer 规则成熟度（§3.1.1 的两层）。**不是**重算范围的依据（那是 Scope/InFromSrc）。
type RuleLayer string

const (
	// LayerStable 确定层：身份**直接写在集名里**、不靠池/推断 ⇒ 判据已定型、不再会改。
	LayerStable RuleLayer = "确定"
	// LayerPending 待定层：靠**池 / 推断 / 口径未定稿** ⇒ 仍会改。
	LayerPending RuleLayer = "待定"
)

// RuleScope 该规则的**定义域**（= 改它时"重算范围"怎么圈）。
type RuleScope string

const (
	// ScopeAll 门 / 提取 / 归一化型：影响**所有行**（与当前档位无关）⇒ 只能用**全量 rebuild**。
	ScopeAll RuleScope = "all"
	// ScopeFromSrc 在某档上做补充 / 在池里推算型：可用 `from_src` 圈 ⇒ 见 InFromSrc。
	// 圈法**宁宽勿漏**（多算的行重算后结果相同，只多写一遍）⇒ 安全。
	ScopeFromSrc RuleScope = "from-src"
)

// RuleMeta 一条决策规则的登记项。字段含义见本文件头部。
type RuleMeta struct {
	ID           RuleID
	Layer        RuleLayer
	Scope        RuleScope
	InFromSrc    []string // Scope=from-src 时：要重算的档（按 `movie_episode_epid.from_src` 圈）
	OutFromSrc   string   // 产出档（写进 `from_src`）
	FansOutMovie bool     // true ⇒ 会改同片其它集 ⇒ 重算须按 `match_key` 整片
	Desc         string
}

// ruleCtx 一次源行派生的公共上下文（逐集不变的部分，只算一次）。
type ruleCtx struct {
	pool     []string         // 借用/锚点池（= 本片所有源的所有集名）
	nonBlank int              // 本行非空集名数（R12 守卫 ①：整行只有 1 集）
	bareN    map[int]struct{} // 本行 **distinct** 裸数字集名（R13 守卫；见 RuleBareNum）

	// 池解析缓存（性能修复 2026-11-15）：ParseEpisodeKey 是纯函数，同一池解析一次即可
	// 复用给行内所有集名 ⇒ 消灭 D4/D5 的 E×P 二次方重解析（线上慢批实锤：食尚玩家单行
	// 585 集 × 池 6253 ⇒ 367 万次解析/行）。惰性构建：只在真有集名走到 D4/D5 时才付
	// 一次 P 次解析的钱（D1/D2/D3 提前返回的集名照旧不付）。
	poolKeys []EpisodeKey
	poolDone bool // 区分「未构建」与「构建后为空池」
}

// poolKeysOf 返回池的已解析键（同一 ctx 只解析一次；单 goroutine 使用，无需加锁）。
func (c *ruleCtx) poolKeysOf() []EpisodeKey {
	if !c.poolDone {
		keys := make([]EpisodeKey, 0, len(c.pool))
		for _, t := range c.pool {
			keys = append(keys, parseEpisodeKey(t))
		}
		c.poolKeys = keys
		c.poolDone = true
	}
	return c.poolKeys
}

func newRuleCtx(titles, pool []string) *ruleCtx {
	ctx := &ruleCtx{pool: pool, bareN: make(map[int]struct{}, len(titles))}
	for _, t := range titles {
		if strings.TrimSpace(t) != "" {
			ctx.nonBlank++
		}
		if v, ok := bareNum(t); ok {
			ctx.bareN[v] = struct{}{}
		}
	}
	return ctx
}

// decisionRule 一条决策规则：**顺序即优先级**（先命中先返回）。
// 返回值 `epid` 为空串 **当且仅当** 命中 D1（未触发）或 R20（跨集区间）——
// `decideRow` 据此决定是否再跑行级补充（两者都不该被 D7/D8 救）。
type decisionRule struct {
	meta  RuleMeta
	apply func(title string, k EpisodeKey, ctx *ruleCtx) (epid string, hit bool)
}

// ===== R18/R19/R20：逐集分派**前置**段（2026-11-16）=====
//
// 背景（实测）：附属站行的集名里带「打包词 / 状态词 / 跨集区间」，被 `seriesOf` 当成**栏目名**写进
// `s` 段 ⇒ 与主站正常的「第N集」（`s=-`）**永不相等** ⇒ 制造"单槽分歧"；又因片级是"全槽 AND"，
// **一行坏集名毁一整片**（连续剧片级 46.85% 的 97.7% 由此而来）。
// 剔除这类行后的 what-if：全库片级 84.21% → 98.48%、连续剧 46.85% → 98.07%。
//
// 三条都**不读借用/锚点池**（只读集名 + 行级 `nonBlank`）⇒ `FansOutMovie=false`
// ⇒ `mode=scope&rule=<ID>` 时自动启用 `RuleSelectsRow` 选行谓词，**范围精确到"集名命中该形态的源行"**，
// 不必全表重算（这是本批「不想频繁全表重算」的落地方式）。
var (
	// reRangeEp（R20）：跨集区间 —— `第X-Y集`（连接符 `-`/`~`/全角破折号）。
	// ⚠️ **不要求**后面跟单位字：`第01-84合集` 的收尾是"合集"而不是"集"（实测漏网形态），
	// 只认 `第<数>-<数>` 这个形态本身即可（`第2024-2025季` 这类已被既有链判为"不触发"，改成 skip 无害）。
	reRangeEp = regexp.MustCompile(`第\s*\d{1,4}\s*(?:-|~|—|－)\s*\d{1,4}`)
	// reStatusTail（R18）：`……第N集` + 状态尾缀（全/完结/已完结）并**收尾**。
	reStatusTail = regexp.MustCompile(`.*第\s*\d{1,4}\s*[期集话回季部]\s*(?:全|完结|已完结)\s*$`)
	// rePackWord（R19）：打包词（`全集`/`合集`，含 `全集完结`/`合全集` 等变体）。
	rePackWord = regexp.MustCompile(`全集|合集`)
)

var preRules = []decisionRule{
	// --- R20 跨集区间：整包/区间条目 ⇒ 不产身份（观察项，暂不归一为首集）
	// 必须**先于** R18/R19 —— `第01-84合集` 同时含 R19 的打包词，不能被 R19 误当"单集整包"给 n=1。
	{meta: RuleMeta{
		ID: RulePreRange, Layer: LayerPending, Scope: ScopeFromSrc,
		InFromSrc: []string{EpKeySourceNone, EpKeySourceSkip}, OutFromSrc: EpKeySourceSkip,
		Desc: "R20：跨集区间（第X-Y集[完结/合集]）⇒ 打包条目，**暂不产身份**（观察项；同片其余集不受影响）",
	}, apply: func(title string, _ EpisodeKey, _ *ruleCtx) (string, bool) {
		if reRangeEp.MatchString(title) {
			return "", true
		}
		return "", false
	}},

	// --- R18 状态尾缀：`第N集完结/已完结/全` ⇒ 清掉被误当栏目名的尾缀（`n` 保留）
	// 只在**解析出的 `s` 恰好就是状态词**时才动手 ⇒ 真有栏目名的行（如「桃花坞第3集完结」）不受影响。
	{meta: RuleMeta{
		ID: RulePreStatus, Layer: LayerStable, Scope: ScopeFromSrc,
		InFromSrc: []string{EpKeySourceSelf}, OutFromSrc: EpKeySourceSelf,
		Desc: "R18：`第N集` + 状态尾缀（全/完结/已完结）⇒ 尾缀被误算成 s 段，清 s、保留 n（与其它源的 `第N集` 对齐）",
	}, apply: func(title string, k EpisodeKey, _ *ruleCtx) (string, bool) {
		if k.Series != "全" && k.Series != "完结" && k.Series != "已完结" {
			return "", false
		}
		if !reStatusTail.MatchString(title) {
			return "", false
		}
		k.Series = ""
		return EpidOf(k), true
	}},

	// --- R19 打包条目：`全集/合集` 且**该源行只有 1 集** ⇒ n=1（整包 = 从第 1 集起）
	// 门控用 `ctx.nonBlank == 1`（与 D7 的"整行只有 1 集"同族）：源行**同时**有打包行与正常集时
	// 本规则**不命中** ⇒ 该行仍走 D3 保留系列身份（**行为不变**，避免把"整包"当成第 1 集）。
	{meta: RuleMeta{
		ID: RulePrePack, Layer: LayerStable, Scope: ScopeFromSrc,
		InFromSrc: []string{EpKeySourceNone, EpKeySourceSkip}, OutFromSrc: EpKeySourceSingle,
		Desc: "R19：打包条目（全集/合集）且**该源行只有 1 集** ⇒ n=1（同 D7 单集语义）；源行多集时不命中 ⇒ 仍按 D3 保留系列身份",
	}, apply: func(title string, k EpisodeKey, ctx *ruleCtx) (string, bool) {
		if k.Num > 0 || k.Date != "" || ctx.nonBlank != 1 || hasASCIIDigit(title) {
			return "", false
		}
		if !rePackWord.MatchString(title) {
			return "", false
		}
		return EpidOf(EpisodeKey{Num: 1}), true
	}},
}

// titleRules **逐集**分派（D1~D6）：顺序 = 原 `EpidFor` 的 if 链（行为等价，测试钉住）。
var titleRules = []decisionRule{
	{meta: RuleMeta{
		ID: RuleSkipNoTrigger, Layer: LayerPending, Scope: ScopeAll, OutFromSrc: EpKeySourceSkip,
		Desc: "未触发（裸「第N集」不属此类；见 EpisodeTriggered）⇒ 不产身份（L6 设计）",
	}, apply: func(_ string, k EpisodeKey, _ *ruleCtx) (string, bool) {
		if !EpisodeTriggered(k) {
			return "", true
		}
		return "", false
	}},
	{meta: RuleMeta{
		ID: RuleSelfInName, Layer: LayerStable, Scope: ScopeAll, OutFromSrc: EpKeySourceSelf,
		Desc: "集名自带期号（第N集/期）⇒ 直接采用；改它 = 改已有非空值 ⇒ 必须全量",
	}, apply: func(_ string, k EpisodeKey, _ *ruleCtx) (string, bool) {
		if k.Num > 0 {
			return EpidOf(k), true
		}
		return "", false
	}},
	{meta: RuleMeta{
		ID: RuleNoneNoDate, Layer: LayerPending, Scope: ScopeAll, OutFromSrc: EpKeySourceNone,
		Desc: "触发但无日期（身份来自 栏目/变体/主次）⇒ 不产期号",
	}, apply: func(_ string, k EpisodeKey, _ *ruleCtx) (string, bool) {
		if k.Date == "" {
			return EpidOf(k), true
		}
		return "", false
	}},
	{meta: RuleMeta{
		ID: RuleBridgeNum, Layer: LayerPending, Scope: ScopeFromSrc, FansOutMovie: true,
		InFromSrc:  []string{EpKeySourceBridge, EpKeySourceAnchor, EpKeySourceNone},
		OutFromSrc: EpKeySourceBridge,
		Desc:       "缺期号但有日期 ⇒ 他源桥接（日期相等 + 栏目/变体一致 + 主次相容）",
	}, apply: func(_ string, k EpisodeKey, ctx *ruleCtx) (string, bool) {
		keys := ctx.poolKeysOf() // 缓存：同一行只解析一次池（原为每个集名重解析整池 ⇒ E×P 二次方）
		if n := BridgeNum(k, keys); n > 0 {
			k.Num = n
			return EpidOf(k), true
		}
		return "", false
	}},
	{meta: RuleMeta{
		ID: RuleAnchorNum, Layer: LayerPending, Scope: ScopeFromSrc, FansOutMovie: true,
		InFromSrc:  []string{EpKeySourceBridge, EpKeySourceAnchor, EpKeySourceNone},
		OutFromSrc: EpKeySourceAnchor,
		Desc:       "桥接不成 ⇒ 锚点推算（同栏目组内「上」的日期序）",
	}, apply: func(title string, k EpisodeKey, ctx *ruleCtx) (string, bool) {
		// 键版锚点：复用 decideTitleInCtx 已解析的 k 与池缓存（原 AnchorNumFor 会重解析 title + 整池）
		if n := AnchorNumForKey(title, k, ctx.poolKeysOf()); n > 0 {
			k.Num = n
			return EpidOf(k), true
		}
		return "", false
	}},
	{meta: RuleMeta{
		ID: RuleNonePoolMiss, Layer: LayerPending, Scope: ScopeFromSrc, FansOutMovie: true,
		InFromSrc:  []string{EpKeySourceBridge, EpKeySourceAnchor, EpKeySourceNone},
		OutFromSrc: EpKeySourceNone,
		Desc:       "有日期但桥接、锚点皆败 ⇒ 仍不产期号（与 D3「无日期」产出同为 none，但定义域不同）",
	}, apply: func(_ string, k EpisodeKey, _ *ruleCtx) (string, bool) {
		return EpidOf(k), true
	}},
}

// rowRules **行级**补充（D7/D8）：只在逐集分派给出 D1（skip）时才跑 ——
// 即"常规派生没推出身份"的行，才轮到这两条救（与 `EpidsForSourceRowWithPool` 原 switch 等价）。
var rowRules = []decisionRule{
	{meta: RuleMeta{
		ID: RuleSingleEp, Layer: LayerStable, Scope: ScopeFromSrc,
		InFromSrc: []string{EpKeySourceSkip}, OutFromSrc: EpKeySourceSingle,
		Desc: "R12：整行只有 1 集 ∧ 集名不含 ASCII 数字 ⇒ n=1（只跑 skip 档）",
	}, apply: func(title string, _ EpisodeKey, ctx *ruleCtx) (string, bool) {
		if ctx.nonBlank == 1 && !hasASCIIDigit(title) {
			return EpidOf(EpisodeKey{Num: 1}), true
		}
		return "", false
	}},
	{meta: RuleMeta{
		ID: RuleBareNum, Layer: LayerStable, Scope: ScopeFromSrc,
		InFromSrc: []string{EpKeySourceSkip}, OutFromSrc: EpKeySourceSelf,
		Desc: "R13：行内 **distinct** 裸数字名 ≥2 ⇒ n=该数字（只跑 skip 档；守护须用 distinct，双写让「≥2 条」恒成立）",
	}, apply: func(title string, _ EpisodeKey, ctx *ruleCtx) (string, bool) {
		if len(ctx.bareN) < 2 {
			return "", false
		}
		if v, ok := bareNum(title); ok {
			return EpidOf(EpisodeKey{Num: v}), true
		}
		return "", false
	}},
}

// DecisionRuleMetas 按**求值顺序**返回全部决策规则的登记项（`preRules`(R18~R20) → D1~D6 → D7/D8）。
// 直接由 `preRules`/`titleRules`/`rowRules` 派生 ⇒ 表与实现**不可能漂移**。
func DecisionRuleMetas() []RuleMeta {
	out := make([]RuleMeta, 0, len(preRules)+len(titleRules)+len(rowRules))
	for _, r := range preRules {
		out = append(out, r.meta)
	}
	for _, r := range titleRules {
		out = append(out, r.meta)
	}
	for _, r := range rowRules {
		out = append(out, r.meta)
	}
	return out
}

// RuleMetaOf 取一条规则的登记项；未知编号返回零值。
func RuleMetaOf(id RuleID) RuleMeta {
	for _, r := range preRules {
		if r.meta.ID == id {
			return r.meta
		}
	}
	for _, r := range titleRules {
		if r.meta.ID == id {
			return r.meta
		}
	}
	for _, r := range rowRules {
		if r.meta.ID == id {
			return r.meta
		}
	}
	return RuleMeta{}
}

// decideTitle 逐集分派（独立单集入口）：跑 preRules(R18~R20) + D1~D6，返回 epid + 命中编号。
// ⚠️ 上下文按"**该源行只有这一个集名**"构造（`newRuleCtx([title], pool)`）—— 这是本入口的
// 唯一合理语义：它只拿到单个 title，拿不到整行集名。需要行级门控（R19/D7/D8）时请走
// `EpidsForSourceRowWithPool`（那里用整行 titles 建 ctx）。
func decideTitle(title string, pool []string) (epid string, id RuleID) {
	return decideTitleInCtx(title, newRuleCtx([]string{title}, pool))
}

// decideTitleInCtx 同 decideTitle，但复用调用方的 ruleCtx（池键缓存生效的关键路由）。
// 求值顺序：**前置段 `preRules`（R18~R20）→ `titleRules`（D1~D6）**；前置段对不含那三种形态的
// 集名**一律不命中** ⇒ 对既有行为零影响（`TestDecideRule_MatchesLegacyChain` 仍以原语料钉住）。
func decideTitleInCtx(title string, ctx *ruleCtx) (epid string, id RuleID) {
	k := parseEpisodeKey(title)
	for _, r := range preRules {
		if e, hit := r.apply(title, k, ctx); hit {
			return e, r.meta.ID
		}
	}
	for _, r := range titleRules {
		if e, hit := r.apply(title, k, ctx); hit {
			return e, r.meta.ID
		}
	}
	// 不可达（D6 无条件命中）；保留以显式表达"分派必有结果"。
	return EpidOf(k), RuleNonePoolMiss
}

// decideRow 行级入口：先逐集分派，仅当命中 D1（skip）时再跑 D7/D8 补充。
func decideRow(title string, ctx *ruleCtx) (epid string, fromSrc string, id RuleID) {
	epid, id = decideTitleInCtx(title, ctx)
	if id != RuleSkipNoTrigger {
		return epid, RuleMetaOf(id).OutFromSrc, id
	}
	for _, r := range rowRules {
		if e, hit := r.apply(title, EpisodeKey{}, ctx); hit {
			return e, r.meta.OutFromSrc, r.meta.ID
		}
	}
	// 补充规则都没救到 ⇒ 维持 D1 的结果（空 epid + skip）。
	return epid, RuleMetaOf(id).OutFromSrc, id
}

// ---- `mode=scope` 的选行支撑（2026-11-14）----

// RuleSelectsRow 判断"某源行的整段集名"是否落在某条规则的**命中面**内 ——
// 实现方式 = **跑一遍真实分派**（`decideRow`），看有没有集命中该规则
// ⇒ 与规则本体**同源**（不存在"用 SQL 复刻守卫"的第二份真理 ✓）。
//
// 适用范围：**只对 `FansOutMovie == false` 的规则精确**（`D7`/`D8`，以及 2026-11-16 新增的
// `P2-status`/`P3-pack`/`P1-range`）——它们的判据只看本行集名（`R19` 另看行级 `ctx.nonBlank`，
// 那是"本行"信息，不是池）：`D7`/`D8` 只在 `D1`（未触发）之后才可能命中，而 `D1` 的判定
// （`EpisodeTriggered`）**不读池** ⇒ 传 `pool = titles` 与真实派生（片级池）**结果一致** ✓。
// 池依赖规则（`D4`/`D5`/`D6`）的判定需要"本片所有源"的池，本函数不适用 ——
// 那些规则的作用域本来就是"到达池的档"，按 `from_src` 圈即可（见 §3.1.1 步 3）。
func RuleSelectsRow(id RuleID, titles []string) bool {
	if len(titles) == 0 {
		return false
	}
	ctx := newRuleCtx(titles, titles)
	for _, t := range titles {
		if _, _, hit := decideRow(t, ctx); hit == id {
			return true
		}
	}
	return false
}

// PoolRelevantFromSrc 会**读借用/锚点池**的产出档 —— 池扇出（`FansOutMovie`）时必须一并重算的档。
// 依据：`self`/`single` 在读池**之前**就 return ⇒ 同片里它们不受池变化影响 ✓。
func PoolRelevantFromSrc() []string {
	return []string{EpKeySourceBridge, EpKeySourceAnchor, EpKeySourceNone}
}

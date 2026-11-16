package danmu

import "strings"

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
// 返回值 `epid` 为空串 **且仅当** 命中 D1（未触发）——`decideRow` 据此决定是否再跑行级补充。
type decisionRule struct {
	meta  RuleMeta
	apply func(title string, k EpisodeKey, ctx *ruleCtx) (epid string, hit bool)
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

// DecisionRuleMetas 按**求值顺序**返回全部决策规则的登记项（D1~D6，再 D7/D8）。
// 直接由 `titleRules`/`rowRules` 派生 ⇒ 表与实现**不可能漂移**。
func DecisionRuleMetas() []RuleMeta {
	out := make([]RuleMeta, 0, len(titleRules)+len(rowRules))
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

// decideTitle 逐集分派（独立单集入口）：跑 D1~D6，返回 epid + 命中编号。
func decideTitle(title string, pool []string) (epid string, id RuleID) {
	return decideTitleInCtx(title, &ruleCtx{pool: pool})
}

// decideTitleInCtx 同 decideTitle，但复用调用方的 ruleCtx（池键缓存生效的关键路由）。
// 与原实现的唯一差异是 ctx 来源：规则表拿到的 (title, k, ctx.pool) 逐字相同 ⇒ 行为等价。
func decideTitleInCtx(title string, ctx *ruleCtx) (epid string, id RuleID) {
	k := parseEpisodeKey(title)
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
// 适用范围：**只对 `FansOutMovie == false` 的规则精确**（`D7`/`D8`）——
// 它们的判据只看本行集名：`D7`/`D8` 只在 `D1`（未触发）之后才可能命中，而 `D1` 的判定
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

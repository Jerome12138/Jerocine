package danmu

import (
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

// EpisodeKey 从「集名/集标题」解析出的**同集判定键**（见同名文档 docs/排查-弹幕集名映射与综艺错位）。
type EpisodeKey struct {
	Date string // YYYYMMDD（19xx/20xx，无则空）
	Num  int    // 第N期 / 第N集 / 第N话 / 第N回 / 第N季 / 第N部（无则 0）
	ENum int    // E01 / EP01（无则 0）
	Part string // **主部分（复合单段）**：上 / 中 / 下；若紧跟（最多隔一个左括号）还有 上/中/下 ⇒ 复合为
	//                「中上」「中下」「上中」等（P0-1c 2026-11-10：由 part+sub 两字段合并为一个复合字段）。
	//                比较规则见 partCompatible —— **前缀相容**（短的是长的前缀 ⇒ 相容，`中` ⊂ `中上`/`中下`）。
	Variant string // 有意义的变体词集合，按字典序用 "," 连接（无则空）
	//                ⚠️ M1（2026-11-11 评审）：**不是 `|`** —— `|` 是 epid/epk 的段分隔符，
	//                多变体会把串写成 7 段 ⇒ 服务端拒收 + web parseEpid 恒 null + 落库坏串 ✗。
	// VariantNum **变体序号**（无则 0）：变体词后**紧跟**的数字 —— 「特辑2」⇒ (Variant=特辑, VariantNum=2)。
	// 必须与变体复合为身份：否则「特辑1..5」坍缩成同一个 Variant，L3a 组内不唯一 ⇒ 整组配不上
	// （桃花坞6 特辑 4~5 集丢弹幕的根因；P0-1a，2026-11-10 三端同规）。纯日期/5 位以上数字不取（见 reVariantNum）。
	VariantNum int
	Series     string // **栏目/系列名**（正片为空）：如「心动的周末」「心动的旅行」；
	//                需 ≥2 个中文字才算数（过滤「版」「片」这类残留）
}

func (k EpisodeKey) IsZero() bool {
	return k.Date == "" && k.Num == 0 && k.ENum == 0 && k.Part == "" &&
		k.Variant == "" && k.VariantNum == 0 && k.Series == ""
}

// ===== P0-1b（2026-11-10）：结构化身份字段契约 =====
//
// 客户端把「集名解析结果」序列化成传输串（HTTP 参数 `epk`）传下来：
//
//	v1|d=<date>|n=<num>|e=<eNum>|p=<part>|v=<variant>|vn=<variantNum>|s=<series>|o=<ordinal>|from=<src>
//
// 目的：① 服务端不再"重新猜"客户端的解析结果，**三端同规的漂移可被检出**（逐项交叉校验 + 打点）；
//      ② 为 P1-2（epid 升格为集唯一标识）铺路。
// 数值缺省写 0、字符串缺省写空；`o` 为可选序位兜底（本批仅透传，不参与判定）；
// `from` 为期号来源（P1-3：self|bridge|anchor|none）—— 不参与身份判定，但服务端用它判定
// "`n` 是否来自桥接"（见 `ParseEpKeySource` 与 `DanmuService.resolveAggregateEpisode` 的 numBridged）。
// L1（2026-11-11 评审）：P0-1c 起 `p` 是**复合单段**（如 `中上`），**没有独立的 `u=` 段**（旧注释残留已删）。
// 服务端仍收 `epTitle`：既作交叉校验基准，也作旧客户端回退与排查依据。

// EpKeyVersion 传输串版本前缀。格式变更时递增；服务端遇到不认识的版本一律按"未提供"处理（行为不变）。
const EpKeyVersion = "v1"

// Serialize 序列化为传输串（与 ParseEpisodeKeyString 互逆）；ordinal 为可选序位兜底，<=0 写 0。
//
// L1（2026-11-11 评审）：**与 TS `serializeEpisodeKey` 逐字对齐**（末尾补 `from=none`）——
// 原来 Go 侧少一段，同一份键两端序列化结果不同（Go 侧目前只用于测试/排查，但对齐可免后患）。
func (k EpisodeKey) Serialize(ordinal int) string {
	return strings.Join([]string{
		EpKeyVersion,
		"d=" + k.Date,
		"n=" + strconv.Itoa(k.Num),
		"e=" + strconv.Itoa(k.ENum),
		"p=" + k.Part,
		"v=" + k.Variant,
		"vn=" + strconv.Itoa(k.VariantNum),
		"s=" + k.Series,
		"o=" + strconv.Itoa(max(ordinal, 0)),
		"from=" + EpKeySourceNone,
	}, "|")
}

// ParseEpisodeKeyString 解析传输串。**版本不认识 / 结构不合法 ⇒ ok=false**（调用方按"未提供"处理，
// 行为与改动前完全一致）。未知字段名一律忽略（向前兼容）。
func ParseEpisodeKeyString(s string) (EpisodeKey, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return EpisodeKey{}, false
	}
	parts := strings.Split(s, "|")
	if len(parts) < 2 || parts[0] != EpKeyVersion {
		return EpisodeKey{}, false
	}
	var k EpisodeKey
	seen := false
	atoi := func(val string) (int, bool) {
		n, err := strconv.Atoi(val)
		if err != nil || n < 0 || n > 9999 {
			return 0, false
		}
		return n, true
	}
	for _, kv := range parts[1:] {
		i := strings.IndexByte(kv, '=')
		if i <= 0 || len(kv)-i-1 > 64 { // 限长防滥用
			return EpisodeKey{}, false
		}
		key, val := kv[:i], kv[i+1:]
		switch key {
		case "d":
			k.Date = val
		case "n":
			n, ok := atoi(val)
			if !ok {
				return EpisodeKey{}, false
			}
			k.Num = n
		case "e":
			n, ok := atoi(val)
			if !ok {
				return EpisodeKey{}, false
			}
			k.ENum = n
		case "p":
			if !isPartMark(val) {
				return EpisodeKey{}, false
			}
			k.Part = val
		case "v":
			k.Variant = val
		case "vn":
			n, ok := atoi(val)
			if !ok {
				return EpisodeKey{}, false
			}
			k.VariantNum = n
		case "s":
			k.Series = val
		case "o":
			// 序位兜底：本批仅透传（解析校验后丢弃），不参与判定
			if _, ok := atoi(val); !ok {
				return EpisodeKey{}, false
			}
		default:
			// 未知字段：向前兼容，忽略
		}
		seen = true
	}
	if !seen {
		return EpisodeKey{}, false
	}
	return k, true
}

// isPartMark 合法的 part 值：空 / 上 / 中 / 下 / **复合**（如 中上、中下、上中 —— 主部分 + 紧跟的次部分）。
func isPartMark(s string) bool {
	rs := []rune(s)
	if len(rs) == 0 {
		return true
	}
	if len(rs) > 2 {
		return false
	}
	isP := func(r rune) bool { return r == '上' || r == '中' || r == '下' }
	// P2（2026-11-12）：日期尾随「当日集序」落 p 段 ⇒ 纯数字（1~2 位）也是合法 p 值。
	allP, allD := true, true
	for _, r := range rs {
		if !isP(r) {
			allP = false
		}
		if r < '0' || r > '9' {
			allD = false
		}
	}
	return allP || allD
}

// MergeEpidKey 交叉校验合并（P0-1b）：客户端结构化键为**实测值** ⇒ 默认优先采用，再与集名解析值逐项核对。
//   - date：双方都有且不等 ⇒ **以集名解析值为准**（硬证据），记冲突；
//   - num：同上；但 numBridged=true（该期号来自 A2 桥接、非集名自带）时**跳过比较** —— 否则会把桥接
//     结果打回原值，心动9/桃花坞6 的纯日期形态会再次全丢；
//   - 其余字段（eNum/part/variant/variantNum/series）服务端**无独立证据** ⇒ 只记不一致，**不覆盖**。
//     L1（2026-11-11 评审）：P0-1c 起主部分已合并（**没有独立的 `sub`**）⇒ 不再有 `sub` 项。
//
// 只记录不填补：若客户端某字段为空而集名解析非空，属解析漂移，记冲突以便发现（不做"猜着补"）。
func MergeEpidKey(client, fromTitle EpisodeKey, numBridged bool) (EpisodeKey, []string) {
	k := client
	var conflicts []string
	if k.Date != "" && fromTitle.Date != "" && k.Date != fromTitle.Date {
		conflicts = append(conflicts, "date")
		k.Date = fromTitle.Date
	}
	if !numBridged && k.Num > 0 && fromTitle.Num > 0 && k.Num != fromTitle.Num {
		conflicts = append(conflicts, "num")
		k.Num = fromTitle.Num
	}
	if k.ENum != fromTitle.ENum {
		conflicts = append(conflicts, "eNum")
	}
	if k.Part != fromTitle.Part {
		conflicts = append(conflicts, "part")
	}
	if k.Variant != fromTitle.Variant {
		conflicts = append(conflicts, "variant")
	}
	if k.VariantNum != fromTitle.VariantNum {
		conflicts = append(conflicts, "variantNum")
	}
	if k.Series != fromTitle.Series {
		conflicts = append(conflicts, "series")
	}
	return k, conflicts
}

// epKeyConflictTotal 客户端解析 ≠ 服务端解析的累计次数（P0-1b 打点）——
// 它是"三端同规"是否漂移的哨兵：正常应恒为 0；一旦上涨说明三端解析已不一致。
var epKeyConflictTotal int64

// EpKeySources 期号来源（P1-3, 2026-11-10）：`epk` 的 `from=` 段取值。**不参与身份判定**，
// 仅供服务端在 P1-1 落库/统计时记录「这个 `n` 是怎么来的」。
const (
	EpKeySourceSelf   = "self"   // 集名自带期号
	EpKeySourceBridge = "bridge" // 他源桥接（日期相等 + 栏目/变体一致 + 主次相容）
	EpKeySourceAnchor = "anchor" // 锚点推算（同栏目组内「上」的日期序）
	EpKeySourceNone   = "none"   // 无（未触发 / 期号缺失且推算不出）
)

// ParseEpKeySource 取 `epk` 里的 `from=` 段（P1-3）。缺省 / 超长 / 空串 ⇒ ""（调用方当"未提供"）。
// 只认四个已知值，其余一律返回 ""（避免把客户端异常值当来源统计）。
func ParseEpKeySource(epk string) string {
	s := strings.TrimSpace(epk)
	if s == "" {
		return ""
	}
	for _, kv := range strings.Split(s, "|") {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		if kv[:i] != "from" {
			continue
		}
		v := kv[i+1:]
		switch v {
		// ⚠️ 老规划 §A.1 改动点 2：`EpKeySourceSingle` 必须登记在此，否则端侧若带上 `from=single`
		// 会被静默当"未提供"（退回 `epNum > 0` 旧规则）。
		case EpKeySourceSelf, EpKeySourceBridge, EpKeySourceAnchor, EpKeySourceNone, EpKeySourceSingle:
			return v
		}
		return ""
	}
	return ""
}

// NoteEpKeyConflicts 记一次字段级冲突（落日志 + 计数）。
//
// L5（2026-11-11 评审）：**明细日志限频**（每 60s 最多一条）—— 三端解析漂移时日志量会 = 请求量 ✗；
// 计数（`EpKeyConflictTotal`）不受限频影响，仍逐次累加 ✓（用于监控"是否在漂移"）。
func NoteEpKeyConflicts(fields []string) {
	if len(fields) == 0 {
		return
	}
	atomic.AddInt64(&epKeyConflictTotal, 1)
	now := time.Now().UnixMilli()
	if last := lastEpKeyConflictLogMs.Load(); now-last >= 60_000 &&
		lastEpKeyConflictLogMs.CompareAndSwap(last, now) {
		log.Printf("[danmu] epkey conflict fields=%s（明细每 60s 限一条，计数仍逐次累加）",
			strings.Join(fields, ","))
	}
}

// lastEpKeyConflictLogMs 上次打明细日志的时间（L5 限频用）。
var lastEpKeyConflictLogMs atomic.Int64

// EpKeyConflictTotal 累计冲突次数（测试/运维观测用）。
func EpKeyConflictTotal() int64 { return atomic.LoadInt64(&epKeyConflictTotal) }

var (
	reDate8 = regexp.MustCompile(`(?:19|20)\d{6}`)
	// reDate6 6 位 YYMMDD（R11，2026-11-14 **服务端口径**）：线上实测"空 epid 里的 6 位纯数字"
	// 共 2,743 条，其中**月日合法 2,742（99.96%）**、年份前缀 ∈19–26 ⇒ 这是真实存在的日期写法
	// （周更综艺，如 240103 → 20240103，同片单线路 272 集、每周三）。补齐成 8 位后可与其它源的
	// 8 位日期**直接比对**（`d` 是原始事实 ⇒ 还能参与桥接）。
	// ⚠️ 这里只用 `\d{6}` 打底，**边界与合法性在 `date6` 里手工校验**（RE2 无 lookaround，
	// 沿用"消费后手工判边界"的既有手法），避免把 7/9 位数字串截出半截当日期 ✗。
	reDate6 = regexp.MustCompile(`\d{6}`)
	reDiN   = regexp.MustCompile(`第\s*(\d{1,4})\s*[期集话回季部]`)
	reENum  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z])(?:E|EP)\s*0*(\d{1,3})(?:\D|$)`)
	// reVariant = **有意义的变体**：标识"同一集的另一份内容"，参与同集判定（两侧必须完全相等）。
	// 「先导片」排在「先导」前，避免剔出残留的「片」。
	reVariant = regexp.MustCompile(`加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外`)
	// reVariantNum 变体序号（P0-1a）：变体词后**紧跟**的数字（最多隔一个左括号/空白）。
	//   特辑2          → 2      （与「特辑」不同身份）
	//   特辑（2）      → 2
	//   加更版1        → 0      （隔着「版」⇒ 不算紧跟）
	//   特辑2026 / 特辑12345 → 0（纯日期/长数字不取）
	// ⚠️ RE2 **不支持 lookahead** ⇒ 用 `(?:[^0-9]|$)` **消费一个后继字符**做等价断言（三端同此写法，保证字面一致）。
	reVariantNum = regexp.MustCompile(`(?:加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外)[\s（(【\[《「『]*(\d{1,3})(?:[^0-9]|$)`)
	// reVariantNoise = **无意义后缀**：只是同一集的版本标签，解析前一律剔除（与括号/标点同等）。
	reVariantNoise = regexp.MustCompile(`会员|未公开|未播|直拍|完整版|未删减`)
	// reSeriesStop = 语言/类型标签，不是栏目名（避免把「正片」「国语」当系列）。
	reSeriesStop = regexp.MustCompile(`^正片$|^国语$|^粤语$|^原声$|^普通话$|^双语$`)
	// reBracket = 各类括号及其内容（【】（）()《》「」『』[]）。
	reBracket = regexp.MustCompile(`【[^】]*】|（[^）]*）|\([^)]*\)|《[^》]*》|「[^」]*」|『[^』]*』|\[[^\]]*\]`)
)

// dateSeqTail 返回 8 位日期后**紧跟**的「当日集序」（1~2 位数字）；数字串 ≥3 位（不透明长
// 编号，如源站内部 ID）或不存在 ⇒ ""。
// P2（2026-11-12 评审）：202401011/202401012 = 同一期的两集，序号位此前被静默丢弃 ⇒ 两集
// epid 逐字相同（①强等跨源错配、弹幕池合并）。RE2 不支持 lookahead ⇒ 用索引扫描 + 终点
// 字符判断（与 reVariantNum 的消费式断言同一约束）。
func dateSeqTail(s string, dateEnd int) string {
	j := dateEnd
	for j < len(s) && j-dateEnd < 2 && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == dateEnd {
		return ""
	}
	if j < len(s) && s[j] >= '0' && s[j] <= '9' {
		return "" // 数字串 ≥3 位: 不透明长编号, 不拆序号
	}
	return s[dateEnd:j]
}

// reDashDate 破折/点分日期(2026-10-10期 / 2022.05.01期)—— 弹幕库侧综艺标题的主流日期形态。
// 解析前统一归一为 8 位, 使三格式(8位/破折/点分)落到同一个键(2026-11-07 v2, 实测最大单项收益)。
var reDashDate = regexp.MustCompile(`((?:19|20)\d{2})[-.](\d{2})[-.](\d{2})`)

// reDiNWide 期号跨度: 第 + 0~8位数字 + 单位字 —— 只用于栏目名清理。
// 比 reDiN(数字限 1~4 位)宽: 「第20260731期」这类日期形态 reDiN 吃不掉,
// 残留的「第/期」单字会污染栏目名(第期先导片 ≠ 先导片, 实测 lz↔uk/md/360/ikun 全 0% 的根因)。
var reDiNWide = regexp.MustCompile(`第\s*\d{0,8}\s*[期集话回季部]`)

// reSeasonSpan 季号跨度: 第 + 阿拉伯/中文数字 + 季(王牌对王牌第七季 → 第七季 整段删除,
// 「季」是分季区分位不是栏目名成分; 栏目名在「季」边界截断, 保持连续)。
var reSeasonSpan = regexp.MustCompile(`第\s*[\d一二三四五六七八九十零两]+\s*季`)

// ParseEpisodeKey 解析集名（形态见包注释与文档）。
func ParseEpisodeKey(title string) EpisodeKey {
	var k EpisodeKey
	s := strings.TrimSpace(title)
	if s == "" {
		return k
	}
	// 日期格式归一(先于一切提取): 破折/点分 → 8 位
	s = reDashDate.ReplaceAllString(s, "$1$2$3")
	s = reVariantNoise.ReplaceAllString(s, "")
	var dateTail string
	if loc := reDate8.FindStringIndex(s); loc != nil {
		k.Date = s[loc[0]:loc[1]]
		dateTail = dateSeqTail(s, loc[1])
	}
	if m := reDiN.FindStringSubmatch(s); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			k.Num = v
		}
	}
	if m := reENum.FindStringSubmatch(s); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			k.ENum = v
		}
	}
	// R11（2026-11-14 **服务端口径**）：6 位 YYMMDD 兼容 —— 补齐为 8 位（便于与其它源的 8 位日期直接比对）。
	//   - **8 位优先**（上面已处理）⇒ 这里只在常规日期没取到时尝试；
	//   - ⚠️ **E 圆号类（`E01.220705`/`EP09`）不适用**：口径上 E 圆号整条**不产身份**
	//     （不同源 E 编号常漂 ⇒ 黄金用例 `E01.220705` 明确"不应触发"）⇒ 6 位日期不得把它"救活" ✗。
	//     8 位日期 + E 圆号的既有行为**不动**（那是另一件事）。
	if k.Date == "" && k.ENum == 0 {
		if d, _, end, ok := date6(s); ok {
			k.Date = d
			dateTail = dateSeqTail(s, end)
		}
	}
	k.Part = parsePart(s)
	// P2（2026-11-12 评审）：日期尾随「当日集序」进 p 段 —— 202401011/202401012 = 同一期按
	// 日期再分的两集，序号位此前被静默丢弃 ⇒ 两集 epid 逐字相同（①强等跨源错配、池键合并）。
	// 落 p 的行为：①强等区分两集；②档 p 双有须「相等」（数字序不做前缀相容，见 partCompatible）
	// ⇒ 正确不匹配；单侧没写序号走通配（宁弱勿错）。parsePart 已命中 上/中/下 时**不叠加** ——
	// 复合值（如「上1」）过不了 epk 的 isPartMark 校验。
	if dateTail != "" && k.Part == "" {
		k.Part = dateTail
	}
	if ms := reVariant.FindAllString(s, -1); len(ms) > 0 {
		sort.Strings(ms)
		// ⚠️ M1（2026-11-11 评审）：连接符用 `,` 而**不是 `|`** —— `|` 是 epid/epk 的**段分隔符**，
		// 多变体（如「特辑|花絮」）会把它写成 `v=特辑|花絮` ⇒ 串变 7 段 ⇒ 服务端 `ParseEpisodeKeyString`
		// 拒收 + web `parseEpid` 恒 null（续播②档失效）+ 落库坏串 ✗。段值域（中文词表/上中下/数字）无逗号 ⇒ 安全 ✓
		k.Variant = strings.Join(ms, ",")
	}
	if m := reVariantNum.FindStringSubmatch(s); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			k.VariantNum = v
		}
	}
	k.Series = seriesOf(s)
	return k
}

// parsePart 解析**主部分（复合）**：第3期中（上）→ `中上`；20260831中上 → `中上`；20260803上 → `上`；正片 → `""`。
// 次部分只在**紧邻主部分**（最多隔一个左括号）时才算，避免把摘要正文当成分支标记。
// P0-1c（2026-11-10）：由「part + sub 两字段」合并为**单个复合值** —— 少一个字段、少一处三端同步面，
// 且让 epid 串自足（`p=中上`），派生表/SQL 可直接按串匹配（见 partCompatible 的前缀相容规则）。
func parsePart(s string) string {
	rs := []rune(s)
	n := len(rs)
	isP := func(r rune) bool { return r == '上' || r == '中' || r == '下' }
	pi := -1
	for i := 0; i < n; i++ {
		if isP(rs[i]) {
			pi = i
			break
		}
	}
	if pi < 0 {
		return ""
	}
	out := rs[pi : pi+1]
	j := pi + 1
	if j < n && isLeftBracket(rs[j]) {
		j++
	}
	if j < n && isP(rs[j]) {
		out = append(out, rs[j])
	}
	return string(out)
}

func isLeftBracket(r rune) bool {
	switch r {
	case '（', '(', '【', '[', '《', '「', '『':
		return true
	}
	return false
}

// seriesOf 提取**栏目/系列名**（正片返回 ""）—— 2026-11-07 v2: **连续片段提取，连续性由构造保证**。
//
//	【qq】 第1期上：又争又抢…            → ""（去括号/期号/上下后无可用片段）
//	心动的周末第1期上：高甜游戏…          → 心动的周末
//	20261012心动的周末上                → 心动的周末
//	第20260731期先导片上                → 先导片（reDiNWide 吃掉整个「第20260731期」，无残渣）
//	檀健次带北江小分队回好六了！ 2025-01-04期 → ""（13 字描述 >8，拒识）
//	王牌对王牌第七季2022.05.01期-会员版   → 王牌对王牌（「季」边界截断）
//	20260803上 / 第3期中（上）           → ""
//
// 做法（用户拍板：栏目名必须连续，绝不跨段拼字）：截断到第一个「：」→ 逐跨度删除
// （括号/日期/第N期宽口径/变体/噪声，删除处以 \x00 占位为界）→ 取**最长纯汉字连续段** →
// 只做两类**边界截断**：尾部 上/中/下（part 标记总在尾部）、首个「季」处（季是分季区分位）
// → 要求 2~8 个中文字（≤8: 真栏目标签实测全部 ≤8；≥9 全是库侧自由描述/片名+残留）。
// 旧实现"只留中文+逐字剔除"会跨段拼出假栏目名（残渣巧合匹配），已废。
// isDigitByte 单字节数字判断（`date6` / `stripDate6` 判边界用；只处理 ASCII 数字）。
func isDigitByte(b byte) bool { return b >= '0' && b <= '9' }

// daysInMonth 当月天数（含闰年）。
func daysInMonth(year, month int) int {
	if month < 1 || month > 12 {
		return 0
	}
	if month == 2 && (year%4 == 0 && year%100 != 0 || year%400 == 0) {
		return 29
	}
	days := [...]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	return days[month]
}

// date6 在 s 里找**符合 YYMMDD 语义**的 6 位日期，返回 8 位规范形与匹配区间（R11，2026-11-14）。
//
// 边界（缺一不可）：① 前后都不接数字（否则是更长数字串的一部分 ✗，如 7/9 位）；
// ② 月 01–12；③ 日不超当月天数（含闰年）；④ 世纪：yy >= 70 ⇒ 19xx，否则 20xx。
func date6(s string) (date string, start, end int, ok bool) {
	for _, loc := range reDate6.FindAllStringIndex(s, -1) {
		if loc[0] > 0 && isDigitByte(s[loc[0]-1]) {
			continue
		}
		if loc[1] < len(s) && isDigitByte(s[loc[1]]) {
			continue
		}
		v := s[loc[0]:loc[1]]
		yy, _ := strconv.Atoi(v[0:2])
		mm, _ := strconv.Atoi(v[2:4])
		dd, _ := strconv.Atoi(v[4:6])
		year := 2000 + yy
		century := "20"
		if yy >= 70 {
			year, century = 1900+yy, "19"
		}
		if dd < 1 || dd > daysInMonth(year, mm) {
			continue
		}
		return century + v, loc[0], loc[1], true
	}
	return "", 0, 0, false
}

// stripDate6 把 s 里**符合语义**的 6 位日期替换为占位符（`seriesOf` 提栏目名用；占位符非数字 ⇒ 不会自我循环）。
func stripDate6(s, sep string) string {
	for {
		_, start, end, ok := date6(s)
		if !ok {
			return s
		}
		s = s[:start] + sep + s[end:]
	}
}

func seriesOf(s string) string {
	if i := strings.IndexAny(s, "：:"); i >= 0 {
		s = s[:i]
	}
	// 跨度删除以 \x00 占位: 残片之间永不拼接, 片段即原文中的连续子串
	const sep = "\x00"
	s = reBracket.ReplaceAllString(s, sep)
	// ⚠️ 顺序要紧：先删「第N期/集/话…」再删日期。反过来的话日期会先被替换成占位符，
	// reDiNWide 的 `\s*\d{0,8}\s*` 跨不过占位符 ⇒ 「第20261005期心动的旅行上」这类日期样式期号
	// 会残留一个「期」，栏目名变「期心动的旅行」，与库侧对不上（与 TS 侧同一 bug，2026-11-09 三端同修）
	s = reDiNWide.ReplaceAllString(s, sep)
	s = reDate8.ReplaceAllString(s, sep)
	s = stripDate6(s, sep) // R11：6 位 YYMMDD 同样剥离（否则 240103 会漏进栏目名 ✗）
	s = reSeasonSpan.ReplaceAllString(s, sep)
	s = reENum.ReplaceAllString(s, sep)
	s = reVariant.ReplaceAllString(s, sep)
	s = reVariantNoise.ReplaceAllString(s, sep)
	best := ""
	for _, frag := range strings.FieldsFunc(s, func(r rune) bool { return r == 0 || !unicode.Is(unicode.Han, r) }) {
		rs := []rune(frag)
		// 尾部 part 标记截断（上/中/下总在栏目名之后）
		for len(rs) > 0 && (rs[len(rs)-1] == '上' || rs[len(rs)-1] == '中' || rs[len(rs)-1] == '下') {
			rs = rs[:len(rs)-1]
		}
		// 「季」边界截断
		for i, r := range rs {
			if r == '季' {
				rs = rs[:i]
				break
			}
		}
		if len(rs) < 2 || len(rs) > 8 {
			continue
		}
		f := string(rs)
		if reSeriesStop.MatchString(f) {
			continue
		}
		if len(rs) > len([]rune(best)) {
			best = f
		}
	}
	return best
}

// partCompatible 主部分是否指向同一支 —— **前缀相容**（P0-1c 2026-11-10，替代旧「part 严格 + sub 通配」两段式）：
//   - 一方为空 ⇒ 通配（App 常只有「中」而库里是「中（上）/（下）」）；
//   - 短的是长的前缀 ⇒ 相容：`中` ⊂ `中上`/`中下`（保留上面的宽容）；
//   - 互不为前缀 ⇒ 不相容：`中上` ≠ `中下`（**区分仍在**，不会把上下半集混为一集）。
//
// 与旧实现（part 相等 + sub 双方非空才比较）**逐例等价**；收益是「一条规则同时表达宽容与区分」，
// 且不再需要"两处必须一起用"的隐式契约（旧实现里 epid 串丢 sub，①档必须额外叠加 SameEpisode）。
func partCompatible(a, b EpisodeKey) bool {
	if a.Part == "" || b.Part == "" {
		return true
	}
	// P2（2026-11-12）：数字序位（日期尾随「当日集序」落 p 段）**不做前缀相容** —— 前缀规则是
	// 为「中」⊂「中上」设计的，"1" 与 "12" 是不同集。任一侧为纯数字 ⇒ 要求严格相等。
	if isAllDigitPart(a.Part) || isAllDigitPart(b.Part) {
		return a.Part == b.Part
	}
	return strings.HasPrefix(a.Part, b.Part) || strings.HasPrefix(b.Part, a.Part)
}

// isAllDigitPart p 段是否为纯数字（P2 的「当日集序」形态，见 dateSeqTail）。
func isAllDigitPart(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// SameEpisode 两个键是否指向同一集。顺序即优先级：
//  1. **变体（含变体序号）与栏目名必须完全相等**（含"都为空"；一方有一方没有 → 不同集）；
//  2. 双方都有日期 → 相等 → 同一集（再校验主/次部分）；**不等 → 不同集（硬证据，即使期号相等）**；
//  3. 双方都有期号/集号且相等 → 同一集（同样校验主/次部分）；
//  4. 双方都有 E 圆号且相等 → 同一集；
//  5. 其余 → 不同集。
//
// 2026-11-06 修订：① 规则 2/3 补主/次部分校验；② 新增栏目名（Series）防衍生栏目串集。
// 2026-11-07 修订（SameEpisode′ 落地）：**日期双有不等 → 否决**——旧逻辑允许"期号相等而日期不同"
// 判同（现实里那是重播/错标），宁缺勿错。
// 2026-11-10 修订（P0-1a）：**变体序号纳入身份** —— 「特辑2」≠「特辑」；两者皆无日期/期号时
// 仍判不同（属"无特征键"，交由 L3a 组内唯一兜底）。
func SameEpisode(a, b EpisodeKey) bool {
	if a.Variant != b.Variant || a.VariantNum != b.VariantNum || a.Series != b.Series {
		return false
	}
	if a.Date != "" && b.Date != "" {
		if a.Date != b.Date {
			return false
		}
		return partCompatible(a, b)
	}
	if a.Num > 0 && b.Num > 0 && a.Num == b.Num {
		return partCompatible(a, b)
	}
	if a.ENum > 0 && b.ENum > 0 && a.ENum == b.ENum {
		return true
	}
	return false
}

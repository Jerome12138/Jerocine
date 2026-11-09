package danmu

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// EpisodeKey 从「集名/集标题」解析出的**同集判定键**（见同名文档 docs/排查-弹幕集名映射与综艺错位）。
type EpisodeKey struct {
	Date    string // YYYYMMDD（19xx/20xx，无则空）
	Num     int    // 第N期 / 第N集 / 第N话 / 第N回 / 第N季 / 第N部（无则 0）
	ENum    int    // E01 / EP01（无则 0）
	Part    string // 主部分：上 / 中 / 下（取最先出现的那个；无则空）
	Sub     string // 次部分：紧邻主部分的第二个 上/中/下（可跨一个左括号）
	Variant string // 有意义的变体词集合，按字典序用 "|" 连接（无则空）
	Series  string // **栏目/系列名**（正片为空）：如「心动的周末」「心动的旅行」；
	//                需 ≥2 个中文字才算数（过滤「版」「片」这类残留）
}

func (k EpisodeKey) IsZero() bool {
	return k.Date == "" && k.Num == 0 && k.ENum == 0 && k.Part == "" && k.Sub == "" && k.Variant == "" && k.Series == ""
}

var (
	reDate8 = regexp.MustCompile(`(?:19|20)\d{6}`)
	reDiN   = regexp.MustCompile(`第\s*(\d{1,4})\s*[期集话回季部]`)
	reENum  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z])(?:E|EP)\s*0*(\d{1,3})(?:\D|$)`)
	// reVariant = **有意义的变体**：标识"同一集的另一份内容"，参与同集判定（两侧必须完全相等）。
	// 「先导片」排在「先导」前，避免剔出残留的「片」。
	reVariant = regexp.MustCompile(`加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外`)
	// reVariantNoise = **无意义后缀**：只是同一集的版本标签，解析前一律剔除（与括号/标点同等）。
	reVariantNoise = regexp.MustCompile(`会员|未公开|未播|直拍|完整版|未删减`)
	// reSeriesStop = 语言/类型标签，不是栏目名（避免把「正片」「国语」当系列）。
	reSeriesStop = regexp.MustCompile(`^正片$|^国语$|^粤语$|^原声$|^普通话$|^双语$`)
	// reBracket = 各类括号及其内容（【】（）()《》「」『』[]）。
	reBracket = regexp.MustCompile(`【[^】]*】|（[^）]*）|\([^)]*\)|《[^》]*》|「[^」]*」|『[^』]*』|\[[^\]]*\]`)
)

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
	if m := reDate8.FindString(s); m != "" {
		k.Date = m
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
	k.Part, k.Sub = partAndSub(s)
	if ms := reVariant.FindAllString(s, -1); len(ms) > 0 {
		sort.Strings(ms)
		k.Variant = strings.Join(ms, "|")
	}
	k.Series = seriesOf(s)
	return k
}

// partAndSub 解析「主部分 + 次部分」：第3期中（上）→ 中/上；20260831中上 → 中/上；20260803上 → 上/""。
// 次部分只在**紧邻主部分**（最多隔一个左括号）时才算，避免把摘要正文当成分支标记。
func partAndSub(s string) (part, sub string) {
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
		return "", ""
	}
	part = string(rs[pi])
	j := pi + 1
	if j < n && isLeftBracket(rs[j]) {
		j++
	}
	if j < n && isP(rs[j]) {
		sub = string(rs[j])
	}
	return part, sub
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

// partCompatible 主/次部分是否指向同一支；一方缺失视为通配（App 常只有「中」而库里是「中（上）/（下）」）。
func partCompatible(a, b EpisodeKey) bool {
	if a.Part != "" && b.Part != "" && a.Part != b.Part {
		return false
	}
	if a.Sub != "" && b.Sub != "" && a.Sub != b.Sub {
		return false
	}
	return true
}

// SameEpisode 两个键是否指向同一集。顺序即优先级：
//  1. **变体与栏目名必须完全相等**（含"都为空"；一方有一方没有 → 不同集）；
//  2. 双方都有日期 → 相等 → 同一集（再校验主/次部分）；**不等 → 不同集（硬证据，即使期号相等）**；
//  3. 双方都有期号/集号且相等 → 同一集（同样校验主/次部分）；
//  4. 双方都有 E 圆号且相等 → 同一集；
//  5. 其余 → 不同集。
//
// 2026-11-06 修订：① 规则 2/3 补主/次部分校验；② 新增栏目名（Series）防衍生栏目串集。
// 2026-11-07 修订（SameEpisode′ 落地）：**日期双有不等 → 否决**——旧逻辑允许"期号相等而日期不同"
// 判同（现实里那是重播/错标），宁缺勿错。
func SameEpisode(a, b EpisodeKey) bool {
	if a.Variant != b.Variant || a.Series != b.Series {
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
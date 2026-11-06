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

// ParseEpisodeKey 解析集名（形态见包注释与文档）。
func ParseEpisodeKey(title string) EpisodeKey {
	var k EpisodeKey
	s := strings.TrimSpace(title)
	if s == "" {
		return k
	}
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

// seriesOf 提取**栏目/系列名**（正片返回 ""）：
//
//	【qq】 第1期上：又争又抢…        → ""（去括号内容+期号+上下）
//	心动的周末第1期上：高甜游戏…      → 心动的周末
//	20261012心动的周末上            → 心动的周末
//	20260803上 / 第3期中（上）       → ""
//
// 做法：截断到第一个「：」→ 去括号及其内容 → 去日期/期号/E圆号/变体 → 只留中文与字母数字 →
// 去掉 上/中/下 单字。**要求 ≥2 个中文字**，否则视为无栏目名（过滤「版」「片」这类残留）。
func seriesOf(s string) string {
	if i := strings.IndexAny(s, "：:"); i >= 0 {
		s = s[:i]
	}
	s = reBracket.ReplaceAllString(s, "")
	s = reDate8.ReplaceAllString(s, "")
	s = reDiN.ReplaceAllString(s, "")
	s = reENum.ReplaceAllString(s, " ")
	s = reVariant.ReplaceAllString(s, "")
	s = reVariantNoise.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		if r == '上' || r == '中' || r == '下' || unicode.IsDigit(r) || unicode.IsLetter(r) {
			if unicode.Is(unicode.Han, r) && (r == '上' || r == '中' || r == '下') {
				continue
			}
			if unicode.Is(unicode.Han, r) {
				b.WriteRune(r)
			}
		}
	}
	out := b.String()
	if len([]rune(out)) < 2 || reSeriesStop.MatchString(out) {
		return ""
	}
	return out
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
//  2. 双方都有日期且相等 → 同一集（再校验主/次部分）；
//  3. 双方都有期号/集号且相等 → 同一集（同样校验主/次部分）；
//  4. 双方都有 E 圆号且相等 → 同一集；
//  5. 其余 → 不同集。
//
// 2026-11-06 修订：① 规则 2/3 补主/次部分校验（此前「20260803上」与「20260803中」会判同集）；
// ② 新增栏目名（Series）—— 此前库侧「心动的周末第1期上」与正片「第1期上」同为 {Num:1,Part:上}，
// 靠"正片排序在前"侥幸不串集（用户要求：期名前后的栏目字段两边一致才算同一集）。
func SameEpisode(a, b EpisodeKey) bool {
	if a.Variant != b.Variant || a.Series != b.Series {
		return false
	}
	if a.Date != "" && b.Date != "" && a.Date == b.Date {
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
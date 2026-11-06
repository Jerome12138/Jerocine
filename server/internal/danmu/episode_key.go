package danmu

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// EpisodeKey 从「集名/集标题」解析出的**同集判定键**。
//
// 为什么需要它（2026-11-06，基于现网真实集名分布）：
//   - 电影/剧/动漫的集名 96.8% 是「第N集/话/回」，与弹幕库的集号天然对得上（旧的
//     EpisodeNumber / 第N 标题匹配已够用）；
//   - **综艺**是错位重灾区：集名标识物是**日期**（33%）、**期号**（14%）、**E圆号.日期**（16%），
//     而第三方源的「第几个视频」与弹幕库的期号毫无对应关系 → 旧三策略全落空 → 该集被判无弹幕。
//
// 于是引入本键：把 App 侧集名与库侧集标题**各自**解析成键，再按「同期 + 同变体」判定同一集。
// 关键约束（用户 2026-11-06 明确要求「要区分」）：**变体必须相等** —— 加更/陪看/纯享/先导/彩蛋/
// 特辑等，库里有同变体才匹配，否则宁缺勿错（绝不能把正片的弹幕塞给「加更」片段）。
type EpisodeKey struct {
	Date    string // YYYYMMDD（19xx/20xx，无则空）
	Num     int    // 第N期 / 第N集 / 第N话 / 第N回 / 第N季 / 第N部（无则 0）
	ENum    int    // E01 / EP01（无则 0）
	Part    string // 主部分：上 / 中 / 下（取最先出现的那个；无则空）
	Sub     string // 次部分：紧邻主部分的第二个 上/中/下（可跨一个左括号）——「第3期中（上）」→ Sub=上
	Variant string // 有意义的变体词集合，按字典序用 "|" 连接（无则空）
}

// IsZero 该标题没有任何可用特征 → 调用方应回落到旧策略。
func (k EpisodeKey) IsZero() bool {
	return k.Date == "" && k.Num == 0 && k.ENum == 0 && k.Part == "" && k.Sub == "" && k.Variant == ""
}

var (
	reDate8 = regexp.MustCompile(`(?:19|20)\d{6}`)
	reDiN   = regexp.MustCompile(`第\s*(\d{1,4})\s*[期集话回季部]`)
	reENum  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z])(?:E|EP)\s*0*(\d{1,3})(?:\D|$)`)
	// reVariant = **有意义的变体**：标识"同一集的另一份内容"，参与同集判定（两侧必须完全相等）。
	reVariant = regexp.MustCompile(`加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外`)
	// reVariantNoise = **无意义后缀**：只是同一集的另一个版本标签（会员版/未公开/未播/直拍/
	// 完整版/未删减），识别集身份没有价值 → 解析前一律剔除，与括号、标点同等对待
	// （2026-11-06 用户要求：「跟括号这些符号一样」）。
	reVariantNoise = regexp.MustCompile(`会员|未公开|未播|直拍|完整版|未删减`)
)

// ParseEpisodeKey 解析集名。对两种真实形态都有效：
//
//	20260803(第1期上) / 20260803(第1期加更上) / 20260731(先导片上) / 20260805(超前彩蛋)
//	第9期上 / E01.220705 / 20260128未公开特辑 / 20250803 / 第1集 / 第3期中（上）
//
// 注意：无意义后缀（会员/未公开/未播/直拍/完整版/未删减）在解析前已剔除，故
// 「20260128未公开特辑」等价于「20260128特辑」（Variant = 特辑）。
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
	// 主/次部分：主取最先出现的 上/中/下；次取紧邻其后的那个（可跨一个左括号）
	// —— 用来区分「第3期中（上）」与「第3期中（下）」（此前两者会塌成同一键）。
	k.Part, k.Sub = partAndSub(s)
	// 变体：全部命中词按字典序拼（「先导陪看X」→ 先导|陪看，比只取一个更不易误判）
	if ms := reVariant.FindAllString(s, -1); len(ms) > 0 {
		sort.Strings(ms)
		k.Variant = strings.Join(ms, "|")
	}
	return k
}

// partAndSub 解析「主部分 + 次部分」：
//
//	第3期中（上） → 中 / 上      第3期中（下） → 中 / 下      第3期中 → 中 / ""
//	20260803上    → 上 / ""      第3期上：又争又抢… → 上 / ""（摘要正文里的 上/下 不参与）
//
// 次部分只在**紧邻主部分**（中间最多隔一个左括号）时才算，避免把摘要正文当成分支标记。
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

// partCompatible 主/次部分是否指向同一支。
// 口径与变体一致但**放宽一档**：一方缺失视为通配（保持旧行为——App 侧常只有「中」而库里是「中（上）/（下）」），
// 双方都有且不同 → 不同集（上 ≠ 下；中（上） ≠ 中（下））。
func partCompatible(a, b EpisodeKey) bool {
	if a.Part != "" && b.Part != "" && a.Part != b.Part {
		return false
	}
	if a.Sub != "" && b.Sub != "" && a.Sub != b.Sub {
		return false
	}
	return true
}

// SameEpisode 两个键是否指向同一集。
//
// 规则（顺序即优先级）：
//  1. **变体必须完全相等**（含"都为空"）；一方有变体另一方没有 → 判为不同集；
//  2. 双方都有日期且相等 → 同一集（再校验主/次部分，见 partCompatible）；
//  3. 双方都有期号/集号且相等 → 同一集（同样校验主/次部分）；
//  4. 双方都有 E 圆号且相等 → 同一集；
//  5. 其余 → 不同集。
//
// 2026-11-06 修订：规则 2/3 补上主/次部分校验 —— 此前「20260803上」与「20260803下」、
// 「第3期中（上）」与「第3期中（下）」会因日期/期号相同而被判成同一集。
func SameEpisode(a, b EpisodeKey) bool {
	if a.Variant != b.Variant {
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

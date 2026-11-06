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
	Part    string // 上 / 中 / 下（无则空）
	Variant string // 变体词集合，按字典序用 "|" 连接（无则空）
}

// IsZero 该标题没有任何可用特征 → 调用方应回落到旧策略。
func (k EpisodeKey) IsZero() bool {
	return k.Date == "" && k.Num == 0 && k.ENum == 0 && k.Part == "" && k.Variant == ""
}

var (
	reDate8   = regexp.MustCompile(`(?:19|20)\d{6}`)
	reDiN     = regexp.MustCompile(`第\s*(\d{1,4})\s*[期集话回季部]`)
	reENum    = regexp.MustCompile(`(?i)(?:^|[^A-Za-z])(?:E|EP)\s*0*(\d{1,3})(?:\D|$)`)
	reVariant = regexp.MustCompile(`加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|会员|未公开|未播|预告|直拍|番外|完整版|未删减`)
)

// ParseEpisodeKey 解析集名。对两种真实形态都有效：
//
//	20260803(第1期上) / 20260803(第1期加更上) / 20260731(先导片上) / 20260805(超前彩蛋)
//	第9期上 / E01.220705 / 20260128未公开特辑 / 20250803 / 第1集
func ParseEpisodeKey(title string) EpisodeKey {
	s := strings.TrimSpace(title)
	var k EpisodeKey
	if s == "" {
		return k
	}
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
	// 上/中/下：取**最先出现**的那个（「第1期中纯享」→ 中）
	k.Part = firstRuneOf(s, "上中下")
	// 变体：全部命中词按字典序拼（「先导陪看X」→ 先导|陪看，比只取一个更不易误判）
	if ms := reVariant.FindAllString(s, -1); len(ms) > 0 {
		sort.Strings(ms)
		k.Variant = strings.Join(ms, "|")
	}
	return k
}

func firstRuneOf(s string, set string) string {
	best := -1
	found := ""
	for _, r := range set {
		if i := strings.IndexRune(s, r); i >= 0 && (best < 0 || i < best) {
			best, found = i, string(r)
		}
	}
	return found
}

// SameEpisode 两个键是否指向同一集。
//
// 规则（顺序即优先级）：
//  1. **变体必须完全相等**（含"都为空"）；一方有变体另一方没有 → 判为不同集；
//  2. 双方都有日期且相等 → 同一集；
//  3. 双方都有期号/集号且相等 → 同一集（但若双方都带 上/中/下 且不同 → 不同集）；
//  4. 双方都有 E 圆号且相等 → 同一集；
//  5. 其余 → 不同集。
func SameEpisode(a, b EpisodeKey) bool {
	if a.Variant != b.Variant {
		return false
	}
	if a.Date != "" && b.Date != "" && a.Date == b.Date {
		return true
	}
	if a.Num > 0 && b.Num > 0 && a.Num == b.Num {
		if a.Part != "" && b.Part != "" && a.Part != b.Part {
			return false
		}
		return true
	}
	if a.ENum > 0 && b.ENum > 0 && a.ENum == b.ENum {
		return true
	}
	return false
}

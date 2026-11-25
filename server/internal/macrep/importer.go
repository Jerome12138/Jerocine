package macrep

// 源站「替换助手」公告解析（2026-11-25）
//
// 设计原则（用户明确要求）：
//  **解析可以宽松（页面结构/域名都会变），但输出口径必须严格：只出 host → host。**
//  带协议、带路径、带端口、含空格、或两侧不都是合法 host 的，一律进 rejected（不猜）。
//
// 解析策略：先剥 HTML 标签得到纯文本，再按行扫描——
//  - 遇到 vod_pic / vod_play_url / vod_down_url 记下"当前字段"
//  - 遇到形如 `a.com → b.com` / `[https://]a.com/ -> [https://]b.com/` 的行，配对成规则
// 这样即便页面改版（标题/说明文字变了），只要还保留"字段名 + 箭头"这两个结构特征就能解析。

import (
	"regexp"
	"strings"
)

// 目标字段（只认这三个；其它字段的公告一律忽略）
const (
	FieldPic  = "vod_pic"
	FieldPlay = "vod_play_url"
	FieldDown = "vod_down_url"
)

// Rule 解析出的规则（host 已规范化：小写、去协议、去路径、去端口）
type Rule struct {
	Field       string
	FromHost    string
	ToHost      string
	OpsTag      string
	AnnouncedAt int64
	Note        string
}

// Reject 被拒绝的候选（保留原文，便于人工核对/补规则）
type Reject struct {
	Raw    string
	Reason string
}

var (
	// 裸 host：至少 a.b，只允许字母数字与连字符，每段不为空且不以连字符开头/结尾
	hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	// 箭头：全角 → / ASCII -> / => （公告里最常用 →）
	arrowRe = regexp.MustCompile(`\s*(?:→|->|=>)\s*`)
	// HTML 标签
	tagRe = regexp.MustCompile(`(?s)<[^>]*>`)
	// 字段名出现在行内（含反引号包裹）
	fieldRes = []*regexp.Regexp{
		regexp.MustCompile(FieldPic),
		regexp.MustCompile(FieldPlay),
		regexp.MustCompile(FieldDown),
	}
	opsRe = regexp.MustCompile(`ops:[a-z0-9-]+`)
)

// ParseRules 解析公告页 HTML。返回 (采信规则, 被拒候选)。
// 采信规则内部已按 (field, from) 去重，保留**先出现**的那条（页面按时间倒序 ⇒ 即最新公告）。
func ParseRules(page string) ([]Rule, []Reject) {
	// 标签替换成**空格**而不是换行：公告里 host 常被 <code> 包裹，
	// 换成换行会把 `a.com→b.com` 拆成两行，导致箭头配不上对。
	text := tagRe.ReplaceAllString(page, " ")
	lines := strings.Split(text, "\n")

	var rules []Rule
	var rejects []Reject
	var curField string
	seen := map[string]bool{}

	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// 1) 更新当前字段（一行可能同时带字段名与箭头，先更新再用）
		for _, re := range fieldRes {
			if m := re.FindString(line); m != "" {
				curField = m
			}
		}
		// 2) 找箭头并配对
		loc := arrowRe.FindStringIndex(line)
		if loc == nil {
			continue
		}
		left := strings.TrimSpace(line[:loc[0]])
		right := strings.TrimSpace(line[loc[1]:])
		from, okFrom := extractHost(left)
		to, okTo := extractHost(right)
		if !okFrom || !okTo {
			if strings.Contains(left, ".") || strings.Contains(right, ".") {
				rejects = append(rejects, Reject{Raw: line, Reason: "两侧不都是合法 host（可能带路径/端口/多值）"})
			}
			continue
		}
		if from == to {
			rejects = append(rejects, Reject{Raw: line, Reason: "from 与 to 相同"})
			continue
		}
		if curField == "" {
			rejects = append(rejects, Reject{Raw: line, Reason: "找不到所属字段（vod_pic/vod_play_url/vod_down_url）"})
			continue
		}
		key := curField + "|" + from
		if seen[key] {
			continue // 页面里同一规则重复出现，只保留首条（时间倒序 ⇒ 更新的在前）
		}
		seen[key] = true
		r := Rule{Field: curField, FromHost: from, ToHost: to}
		if t := findOpsTag(lines, i); t != "" {
			r.OpsTag = t
		}
		rules = append(rules, r)
	}
	return rules, rejects
}

// extractHost 从一段文本里抽出**唯一的裸 host**。
//
// 严格口径（用户要求：一定是"域名换域名"，不能匹配错）：
//
//	只剥离纯修饰部分 —— 前后引号/反引号/括号、`scheme://`、**单个尾部斜杠**。
//	剥离后若还含空白、`/`、`?`、`#`、`:`（路径/查询/端口/多值），**直接拒绝**，绝不截断猜测。
func extractHost(s string) (string, bool) {
	s = strings.TrimSpace(s)
	// 去掉包裹的标点/引号/反引号（成对或单边都可能出现）
	s = strings.Trim(s, "`'\u2018\u2019\u201c\u201d()\uff08\uff09[]\u3010\u3011<>\u300a\u300b,,\uff0c;\uff1b\u3002 ")
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	s = strings.TrimSuffix(s, "/") // 只允许一个尾部斜杠这种纯修饰
	s = strings.ToLower(strings.TrimSpace(s))
	// 剥离后必须已是裸 host：任何分隔符都视为"不是域名→域名"，拒绝
	if strings.ContainsAny(s, " \t/?#:") {
		return "", false
	}
	if !validHost(s) {
		return "", false
	}
	return s, true
}

// validHost 严格裸 host 校验：长度、至少一个点、字符集、每段不以连字符开头/结尾。
func validHost(h string) bool {
	if len(h) < 4 || len(h) > 253 {
		return false
	}
	if strings.Count(h, ".") < 1 {
		return false
	}
	return hostRe.MatchString(h)
}

// findOpsTag 在箭头行的邻近行里找 ops 标记。
// 页面把 ops 标记放在规则块的**下一行**（如 `[ops:liangzi-play-cdnlz28]`），
// 故先向后看 2 行、再向前看 2 行，避免相邻公告互相串味。
func findOpsTag(lines []string, at int) string {
	for d := 0; d <= 2; d++ {
		if j := at + d; j < len(lines) {
			if t := opsRe.FindString(lines[j]); t != "" {
				return t
			}
		}
	}
	for d := 1; d <= 2; d++ {
		if j := at - d; j >= 0 {
			if t := opsRe.FindString(lines[j]); t != "" {
				return t
			}
		}
	}
	return ""
}

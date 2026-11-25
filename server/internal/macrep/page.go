package macrep

// 源站「替换助手」页面的**结构化数据**解析（2026-11-25）。
//
// 关站页在 <div data-rep-data="[...]"> 里直接内嵌了全部公告 JSON，字段远比页面文本完整：
//
//	[{ id, time(秒), original, applied,
//	   rules:[{ id, type, original, replacement, published_target, note, warning, executable,
//	            fields:[{ table, field, label }] }] }]
//
// 早期实现是对剥完标签的纯文本做"箭头 + 邻近两行"猜日期，结果：
//  - 公告日期全为 0（日期行离箭头行超过 2 行；页面里还有 2026/11/18 这种斜杠格式）；
//  - 公告分类 / 备注 / warning / executable 全丢，管理端看不出"仅迁移展示，不在本库执行"。
// 故改成：**优先解析内嵌 JSON**，解析不到才回退到旧的文本扫描（页面改版兜底）。

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
)

var repDataRe = regexp.MustCompile(`(?s)data-rep-data="([^"]*)"`)

// 只声明**我们真正要用**的字段：源站的 executable / applied / warning / published_target / id
// 一个都不读（json 解码会忽略未声明字段），避免有人以为我们在采信它们。
type srcPageGroup struct {
	Time     int64         `json:"time"`
	Original string        `json:"original"`
	Rules    []srcPageRule `json:"rules"`
}

type srcPageRule struct {
	Type        string         `json:"type"`
	Original    string         `json:"original"`
	Replacement string         `json:"replacement"`
	Note        string         `json:"note"`
	Fields      []srcPageField `json:"fields"`
}

type srcPageField struct {
	Table string `json:"table"`
	Field string `json:"field"`
	Label string `json:"label"`
}

// ParsePage 解析公告页：优先内嵌 JSON（data-rep-data），失败则回退文本扫描。
func ParsePage(page string) ([]Rule, []Reject) {
	if m := repDataRe.FindStringSubmatch(page); m != nil {
		if rules, rejects, ok := parseRepData(m[1]); ok {
			return rules, rejects
		}
	}
	return ParseRules(page)
}

// parseRepData 解析 data-rep-data 属性值（HTML 实体已转义，需先反转义）。
// 返回 ok=false 表示属性存在但无法解析（交给文本扫描兜底）。
func parseRepData(attr string) ([]Rule, []Reject, bool) {
	raw := html.UnescapeString(attr)
	var groups []srcPageGroup
	if err := json.Unmarshal([]byte(raw), &groups); err != nil || len(groups) == 0 {
		return nil, nil, false
	}

	var rules []Rule
	var rejects []Reject
	seen := map[string]bool{}

	for _, g := range groups {
		for _, r := range g.Rules {
			from, okFrom := extractHost(r.Original)
			if !okFrom {
				rejects = append(rejects, Reject{Raw: r.Original, Reason: "旧域名不是合法 host（带路径/端口/多值）"})
				continue
			}
			to, okTo := extractHost(r.Replacement)
			if !okTo {
				rejects = append(rejects, Reject{Raw: r.Original + " → " + r.Replacement, Reason: "新域名不是合法 host（带路径/端口/多值）"})
				continue
			}
			if from == to {
				rejects = append(rejects, Reject{Raw: r.Original + " → " + r.Replacement, Reason: "from 与 to 相同"})
				continue
			}
			if len(r.Fields) == 0 {
				rejects = append(rejects, Reject{Raw: r.Original, Reason: "公告未声明目标字段"})
				continue
			}
			for _, f := range r.Fields {
				if !knownField(f.Field) {
					rejects = append(rejects, Reject{Raw: r.Original + " @ " + f.Field, Reason: "未知目标字段"})
					continue
				}
				key := f.Field + "|" + from
				if seen[key] {
					continue // 同一规则重复出现：保留首条（页面按时间倒序 ⇒ 最新的在前）
				}
				seen[key] = true
				// 只取有用的：源站的 executable / applied / warning / published_target / id 不采信
				rules = append(rules, Rule{
					Field:       f.Field,
					FieldLabel:  f.Label,
					Type:        r.Type,
					FromHost:    from,
					ToHost:      to,
					AnnouncedAt: announcedMilli(g.Time),
					Note:        stripOpsTag(r.Note),
				})
			}
		}
	}
	return rules, rejects, true
}

// knownField 只认三个目标列（其它字段的公告一律不采信）。
func knownField(f string) bool {
	switch f {
	case FieldPic, FieldPlay, FieldDown:
		return true
	}
	return false
}

// announcedMilli 源站 time 是**秒**；个别改版可能直接给毫秒，按量级判断。
func announcedMilli(t int64) int64 {
	if t <= 0 {
		return 0
	}
	if t > 1e12 { // 已是毫秒
		return t
	}
	return t * 1000
}

// opsRe 源站备注里的内部发布标记，形如 [ops:liangzi-play-cdnlz28]。
// 那是给站长自己看的发布凭据，对下游没有价值 ⇒ 写库前直接剥掉（不单独存列，也不留在备注里）。
var opsRe = regexp.MustCompile(`\[?\s*ops:[a-z0-9-]+\s*\]?`)

// stripOpsTag 去掉备注里的 [ops:xxx] 片段，并把因此产生的多余空白/分隔符收干净。
func stripOpsTag(note string) string {
	out := strings.TrimSpace(note)
	if out == "" {
		return ""
	}
	out = opsRe.ReplaceAllString(out, "")
	return strings.Trim(strings.Join(strings.Fields(out), " "), " -—,，;；")
}

// Package live 直播源解析：拉取 m3u 播放列表并抽取频道（名称/分组/台标/地址）。
// 解析是纯函数（playlist.go），取数是薄 HTTP 客户端（client.go）。
package live

import (
	"bufio"
	"strings"
)

// Channel 一个直播频道。
type Channel struct {
	Name  string `json:"name"`  // 频道名（EXTINF 尾段或 tvg-name）
	Group string `json:"group"` // 分组（group-title 或 #EXTGRP，缺省"其他"）
	Logo  string `json:"logo"`  // 台标（tvg-logo，可空）
	URL   string `json:"url"`   // 播放地址
}

// DefaultGroup 未标注分组的频道归入该组。
const DefaultGroup = "其他"

// ParsePlaylist 解析 m3u 文本为频道列表（#EXTINF 属性行 + 下一非注释行为地址）。
// 兼容形态：
//   - #EXTINF:-1 tvg-id="" tvg-name="CCTV1" tvg-logo="http://x/1.png" group-title="央视",CCTV-1 综合
//   - #EXTINF:-1,CCTV-1（无属性）
//   - #EXTGRP:央视（分组独立行，优先级低于 group-title）
//   - 非标准源可能把地址写在 #EXTINF 同行/缺 #EXTINF 直接给 URL —— 不猜，忽略无地址的条目。
//   - URL 行允许 \r（CRLF）；重复地址去重（同名同址只留一条）。
func ParsePlaylist(text string) []Channel {
	var out []Channel
	seen := make(map[string]struct{}, 64)
	pending := -1 // 待配地址的 EXTINF 下标；-1 = 无
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimRight(sc.Text(), "\r"))
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "#EXTINF"):
			// 上一条 EXTINF 悬挂（未等到地址行又来一条 EXTINF）→ 先丢弃
			if pending >= 0 && pending < len(out) && out[pending].URL == "" {
				out = out[:pending]
			}
			name, group, logo := parseExtInf(line)
			out = append(out, Channel{Name: name, Group: group, Logo: logo})
			pending = len(out) - 1
		case strings.HasPrefix(line, "#EXTGRP:"):
			if pending >= 0 && pending < len(out) && out[pending].Group == DefaultGroup {
				g := strings.TrimSpace(strings.TrimPrefix(line, "#EXTGRP:"))
				if g != "" {
					out[pending].Group = g
				}
			}
		case strings.HasPrefix(line, "#"):
			// 其它注释指令（#EXTM3U/#EXTVLCOPT/...）忽略
		default:
			// 频道地址行
			if pending >= 0 && pending < len(out) && isPlayableURL(line) {
				ch := out[pending]
				ch.URL = line
				if _, dup := seen[ch.URL]; !dup {
					seen[ch.URL] = struct{}{}
					out[pending] = ch
					pending = -1
					continue
				}
			}
			// 该行不是可播地址（或重复）: 丢弃悬挂条目，避免占位
			if pending >= 0 && pending < len(out) {
				out = out[:pending]
			}
			pending = -1
		}
	}
	// 末尾悬挂（有 EXTINF 无地址）丢弃
	if pending >= 0 && pending < len(out) {
		out = out[:pending]
	}
	if out == nil {
		out = []Channel{}
	}
	return out
}

// parseExtInf 解析 #EXTINF 行：属性取 tvg-name/tvg-logo/group-title，名称取最后一个逗号后的段。
func parseExtInf(line string) (name, group, logo string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "#EXTINF"))
	// 名称 = 最后一个逗号之后的段（属性值里可能含逗号, 从属性行右侧找最后一个不在引号内的逗号:
	// 简化处理 —— 标准 m3u 属性组在前、名称在最后一个逗号后; 属性值含逗号的源极少,
	// 若按"最后一个逗号"切错, 优先用 tvg-name 兜底）
	var attrsPart, titlePart string
	if idx := strings.LastIndex(rest, ","); idx >= 0 {
		attrsPart, titlePart = rest[:idx], strings.TrimSpace(rest[idx+1:])
	} else {
		attrsPart, titlePart = rest, ""
	}
	group = DefaultGroup
	for _, attr := range splitAttrs(attrsPart) {
		k, v, ok := strings.Cut(attr, "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"`)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "tvg-name":
			if v != "" && titlePart == "" {
				titlePart = v
			}
		case "tvg-logo":
			logo = v
		case "group-title":
			if v != "" {
				group = v
			}
		}
	}
	if titlePart == "" {
		titlePart = "未命名频道"
	}
	return titlePart, group, logo
}

// splitAttrs 按空格切属性段（引号内空格不切）。
func splitAttrs(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// isPlayableURL 宽松判定可播地址：http(s)/rtmp/rtsp/mms 协议或以 .m3u8/.flv 结尾的裸路径。
func isPlayableURL(s string) bool {
	ls := strings.ToLower(s)
	for _, p := range []string{"http://", "https://", "rtmp://", "rtsp://", "mms://", "udp://", "rtp://"} {
		if strings.HasPrefix(ls, p) {
			return true
		}
	}
	return strings.HasSuffix(ls, ".m3u8") || strings.HasSuffix(ls, ".flv")
}

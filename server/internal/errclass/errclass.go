// Package errclass 把底层异常归一成"异常原因字典"里的一类。
//
// 为什么需要它：台账（task_run.error / collect_failure.cause）里塞的是原始异常全文
// （含完整 URL、Go 原生报错），既长又难比对；线上统计"失败原因"时只能看前缀。
//
// 归一则：
//   - 台账只落 `[code] 中文短文案`（如 `[net_timeout] 源站请求超时`），原始异常进日志；
//   - 前端按 code 分类展示；
//   - 是否值得重试、重试几次由**字典**决定，避免各处各写一套判断。
//
// 2026-11-25 线上实测（289 条失败台账）里最高频的是 net_timeout(207)、net_tls(48)、
// upstream_5xx(15)、bad_payload(4)、blocked_ssrf(2)、db_deadlock(47，来自 collect_failure)。
package errclass

import (
	"errors"
	"regexp"
	"strings"
)

// Blame 责任归属：用于区分"源站问题 / 本端问题 / 安全策略"。
const (
	BlameUpstream = "upstream"
	BlameSelf     = "self"
	BlameSecurity = "security"
)

// 归类码
const (
	CodeNetTimeout  = "net_timeout"  // 请求超时（响应头/整体）
	CodeNetRead     = "net_read"     // 读 body 超时
	CodeNetTLS      = "net_tls"      // TLS 握手超时 / 证书问题
	CodeNetReset    = "net_reset"    // 连接被重置
	CodeNetRefused  = "net_refused"  // 连接被拒绝 / 不可达
	CodeNetDNS      = "net_dns"      // 域名解析失败
	CodeUpstream5xx = "upstream_5xx" // 源站 5xx
	CodeUpstream4xx = "upstream_4xx" // 源站 4xx
	CodeBadPayload  = "bad_payload"  // 源站返回非预期内容（非 JSON 等）
	CodeSSRF        = "blocked_ssrf" // 安全策略：解析到非公网地址
	CodeDeadlock    = "db_deadlock"  // MySQL 死锁 1213
	CodeCanceled    = "canceled"     // 本端取消（停机/用户取消）
	CodeUnknown     = "unknown"
)

// Class 一类异常。
type Class struct {
	Code      string
	Short     string // 中文短文案（写入台账、UI 展示）
	Retryable bool
	RetryMax  int
	Blame     string
}

var upstreamStatusRe = regexp.MustCompile(`upstream status (\d{3})`)

// 规则表：**自上而下第一条命中即返回**（顺序即优先级）。
var table = []struct {
	keys []string
	cls  Class
}{
	{[]string{"error 1213", "deadlock found", "try restarting transaction"},
		Class{CodeDeadlock, "数据库死锁（写并发过高）", true, 3, BlameSelf}},
	{[]string{"blocked non-public address"},
		Class{CodeSSRF, "源站域名解析到非公网地址（已按安全策略拒绝）", false, 0, BlameSecurity}},
	{[]string{"context canceled"},
		Class{CodeCanceled, "任务已取消（停机或手动停止）", false, 0, BlameSelf}},
	{[]string{"upstream status 5"},
		Class{CodeUpstream5xx, "源站返回 5xx", true, 2, BlameUpstream}},
	{[]string{"upstream status 4"},
		Class{CodeUpstream4xx, "源站返回 4xx", false, 0, BlameUpstream}},
	{[]string{"invalid character", "unexpected end of json", "cannot unmarshal", "invalid json"},
		Class{CodeBadPayload, "源站返回非预期内容（非 JSON）", false, 0, BlameUpstream}},
	{[]string{"while reading body", "reading body"},
		Class{CodeNetRead, "读取响应超时", true, 1, BlameUpstream}},
	{[]string{"tls handshake timeout", "tls:", "x509:"},
		Class{CodeNetTLS, "TLS 握手失败/超时", true, 1, BlameUpstream}},
	{[]string{"timeout awaiting response headers", "i/o timeout", "client.timeout", "context deadline exceeded"},
		Class{CodeNetTimeout, "源站请求超时", true, 1, BlameUpstream}},
	{[]string{"connection reset by peer", "broken pipe"},
		Class{CodeNetReset, "连接被对端重置", true, 1, BlameUpstream}},
	{[]string{"connection refused", "no route to host", "network is unreachable"},
		Class{CodeNetRefused, "连接被拒绝/网络不可达", true, 1, BlameUpstream}},
	{[]string{"no such host", "server misbehaving", "lookup "},
		Class{CodeNetDNS, "域名解析失败", false, 0, BlameUpstream}},
}

// Classify 归一一个 error（nil 返回 unknown，调用方自行判空）。
func Classify(err error) Class {
	if err == nil {
		return Class{CodeUnknown, "未知错误", false, 0, BlameSelf}
	}
	return ClassifyMsg(err.Error())
}

// ClassifyMsg 归一一段错误文案。
func ClassifyMsg(msg string) Class {
	low := strings.ToLower(msg)
	for _, r := range table {
		for _, k := range r.keys {
			if strings.Contains(low, k) {
				c := r.cls
				// 5xx/4xx 带上真实状态码，便于一眼看出是 502 还是 503
				if c.Code == CodeUpstream5xx || c.Code == CodeUpstream4xx {
					if m := upstreamStatusRe.FindStringSubmatch(msg); len(m) == 2 {
						c.Short = "源站返回 " + m[1]
					}
				}
				return c
			}
		}
	}
	return Class{CodeUnknown, "未知错误", false, 0, BlameSelf}
}

// Format 归一成台账文案：`[code] 短文案`。
func Format(err error) string {
	if err == nil {
		return ""
	}
	return FormatMsg(err.Error())
}

// FormatMsg 归一一段错误文案。已经是归一格式（以 '[' 开头）时原样返回，便于重复调用幂等。
func FormatMsg(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	if strings.HasPrefix(msg, "[") {
		return msg
	}
	c := ClassifyMsg(msg)
	return "[" + c.Code + "] " + c.Short
}

// Is 判定错误是否属于某归类（供调用方做分支）。
func Is(err error, code string) bool {
	return err != nil && Classify(err).Code == code
}

// Retryable 该错误是否值得自动重试。
func Retryable(err error) bool { return Classify(err).Retryable }

// RetryMax 该类错误的重试次数上限（不可重试返回 0）。
func RetryMax(err error) int { return Classify(err).RetryMax }

// Unwrap 便于 errors.Is/As 链式判断（保留标准库习惯）。
func Unwrap(err error) error { return errors.Unwrap(err) }

package errclass

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// 线上真实出现过的文案（2026-11-25 台账抽样）必须都能归类，且不落 unknown。
func TestClassifyRealCases(t *testing.T) {
	cases := []struct {
		msg  string
		code string
	}{
		{`Get "https://www.huyaapi.com/api.php/provide/vod/at/json/?ac=detail&h=3&pg=1": net/http: timeout awaiting response headers`, CodeNetTimeout},
		{`Get "https://bfzyapi.com/api.php/...": net/http: TLS handshake timeout`, CodeNetTLS},
		{`Get "https://api.mdzyapi.com/...": context deadline exceeded (Client.Timeout or context cancellation while reading body)`, CodeNetRead},
		{`Get "https://api.okzyw.net/...": net/http: i/o timeout`, CodeNetTimeout},
		{`spider: upstream status 502`, CodeUpstream5xx},
		{`spider: upstream status 500`, CodeUpstream5xx},
		{`Get "https://x/": connection reset by peer`, CodeNetReset},
		{`invalid character '2' after top-level value`, CodeBadPayload},
		{`Get "https://360zy.com/...": dial tcp 1.2.3.4:443: connect: connection refused`, CodeNetRefused},
		{`spider: blocked non-public address`, CodeSSRF},
		{`Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction`, CodeDeadlock},
		{`Get "https://x/": dial tcp: lookup x.com: no such host`, CodeNetDNS},
		{`context canceled`, CodeCanceled},
	}
	for _, c := range cases {
		got := ClassifyMsg(c.msg)
		if got.Code != c.code {
			t.Fatalf("ClassifyMsg(%q) = %s, want %s", c.msg, got.Code, c.code)
		}
		if got.Short == "" {
			t.Fatalf("%s 缺中文短文案", c.code)
		}
	}
}

// 5xx 要带上真实状态码，便于一眼分辨 502/503。
func TestUpstreamKeepsStatus(t *testing.T) {
	if got := ClassifyMsg("spider: upstream status 503").Short; got != "源站返回 503" {
		t.Fatalf("短文案 = %q", got)
	}
}

// 重试策略来自字典：5xx 可重试 2 次；bad_payload / ssrf 不重试。
func TestRetryPolicy(t *testing.T) {
	yes := map[string]int{
		CodeUpstream5xx: 2,
		CodeNetReset:    1,
		CodeNetTimeout:  1,
		CodeNetTLS:      1,
		CodeDeadlock:    3,
	}
	for code, max := range yes {
		c := ClassifyMsg(sampleFor(code))
		if !c.Retryable || c.RetryMax != max {
			t.Fatalf("%s 重试策略不对: %+v", code, c)
		}
	}
	for _, code := range []string{CodeBadPayload, CodeSSRF, CodeUpstream4xx, CodeNetDNS, CodeCanceled, CodeUnknown} {
		c := ClassifyMsg(sampleFor(code))
		if c.Retryable {
			t.Fatalf("%s 不应重试: %+v", code, c)
		}
	}
}

// 台账文案：`[code] 短文案`，且对已归一的串幂等。
func TestFormat(t *testing.T) {
	got := Format(fmt.Errorf(`Get "https://x/": net/http: timeout awaiting response headers`))
	if got != "[net_timeout] 源站请求超时" {
		t.Fatalf("Format = %q", got)
	}
	if again := FormatMsg(got); again != got {
		t.Fatalf("幂等失败: %q", again)
	}
	if Format(nil) != "" {
		t.Fatal("nil 应为空串")
	}
	if FormatMsg("   ") != "" {
		t.Fatal("空白应为空串")
	}
}

// 责任归属：区分源站 / 本端 / 安全策略，供"哪些能避免"的统计使用。
func TestBlame(t *testing.T) {
	if ClassifyMsg("spider: blocked non-public address").Blame != BlameSecurity {
		t.Fatal("ssrf 应归安全策略")
	}
	if ClassifyMsg("Error 1213: Deadlock found").Blame != BlameSelf {
		t.Fatal("死锁应归本端")
	}
	if ClassifyMsg("spider: upstream status 502").Blame != BlameUpstream {
		t.Fatal("5xx 应归源站")
	}
}

// Is/Retryable 便捷函数与 errors.Is 链兼容。
func TestHelpers(t *testing.T) {
	err := fmt.Errorf("wrap: %w", errors.New("connection reset by peer"))
	if !Is(err, CodeNetReset) {
		t.Fatal("Is 判定失败")
	}
	if !Retryable(err) || RetryMax(err) != 1 {
		t.Fatal("Retryable/RetryMax 判定失败")
	}
	if Unwrap(err) == nil {
		t.Fatal("Unwrap 应能取到内层")
	}
	if Is(context.Canceled, CodeNetReset) {
		t.Fatal("无关键不应命中")
	}
}

func sampleFor(code string) string {
	switch code {
	case CodeUpstream5xx:
		return "spider: upstream status 502"
	case CodeUpstream4xx:
		return "spider: upstream status 404"
	case CodeNetReset:
		return "connection reset by peer"
	case CodeNetTimeout:
		return "net/http: timeout awaiting response headers"
	case CodeNetTLS:
		return "net/http: TLS handshake timeout"
	case CodeDeadlock:
		return "Error 1213: Deadlock found"
	case CodeBadPayload:
		return "invalid character '2' after top-level value"
	case CodeSSRF:
		return "blocked non-public address"
	case CodeNetDNS:
		return "lookup x.com: no such host"
	case CodeCanceled:
		return "context canceled"
	}
	return "something else entirely"
}

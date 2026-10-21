package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"server/internal/botdetect"
)

const uaChrome = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"

// TestHeartbeatBotReason 心跳的机器人判定要能读到真实请求头 —— 线上那批僵尸会话 UA 是
// 正常 Chrome, 唯一的破绽是"缺浏览器特征头"与"机房 IP", 这两条必须真的接上。
func TestHeartbeatBotReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newCtx := func(hdr map[string]string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/online/heartbeat", nil)
		c.Request.Header.Set("User-Agent", uaChrome)
		for k, v := range hdr {
			c.Request.Header.Set(k, v)
		}
		return c
	}

	real := map[string]string{"Accept-Language": "zh-CN,zh;q=0.9", "Sec-Fetch-Mode": "cors"}
	if got := heartbeatBotReason(newCtx(real), uaChrome, false, "58.48.225.70"); got != botdetect.ReasonHuman {
		t.Fatalf("家宽 + 完整浏览器头应判真人, got %q", got)
	}
	// 缺两个特征头 ⇒ header
	if got := heartbeatBotReason(newCtx(nil), uaChrome, false, "58.48.225.70"); got != botdetect.ReasonHeader {
		t.Fatalf("缺浏览器头应判 header, got %q", got)
	}
	// 头齐全但落在腾讯云 43.128.0.0/10 ⇒ datacenter(线上僵尸会话的主要特征)
	if got := heartbeatBotReason(newCtx(real), uaChrome, false, "43.134.163.79"); got != botdetect.ReasonDatacenter {
		t.Fatalf("机房 IP 应判 datacenter, got %q", got)
	}
	// 客户端自报 webdriver ⇒ webdriver
	if got := heartbeatBotReason(newCtx(real), uaChrome, true, "58.48.225.70"); got != botdetect.ReasonWebDriver {
		t.Fatalf("webdriver 应判 webdriver, got %q", got)
	}
	// UA 自报爬虫 ⇒ ua
	if got := heartbeatBotReason(newCtx(real), "curl/8.5.0", false, "58.48.225.70"); got != botdetect.ReasonUA {
		t.Fatalf("爬虫 UA 应判 ua, got %q", got)
	}
}

package entity

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSiteConfigJSON_HidesSMTPColumnGroup 锁住公开面信息泄漏:
// SiteConfig 经 GET /config/site(公开、无鉴权、Cache-Control public)直接序列化出网。
// smtp_host(邮件服务商) 与 smtp_user(发件邮箱, 常是站长私人邮箱)属于运维侦察信息,
// 必须整组 json:"-"; 管理端读走独立掩码 DTO(service.MailConfigView), 不依赖这里的 tag。
func TestSiteConfigJSON_HidesSMTPColumnGroup(t *testing.T) {
	b, err := json.Marshal(SiteConfig{
		Id: 1, SiteName: "站点", Domain: "https://x.example",
		SmtpEnabled: 1, SmtpHost: "smtp.qq.com", SmtpPort: 465, SmtpSecure: "ssl",
		SmtpUser: "owner@qq.com", SmtpPass: "authcode", SmtpFromName: "Jerocine",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	for _, leak := range []string{
		"smtpEnabled", "smtpHost", "smtpPort", "smtpSecure", "smtpUser", "smtpPass", "smtpFromName",
		"smtp.qq.com", "owner@qq.com", "authcode", "Jerocine",
	} {
		if strings.Contains(got, leak) {
			t.Fatalf("公开序列化泄漏 %q: %s", leak, got)
		}
	}
	// 基础字段必须保留(前端 /config/site 契约)
	for _, keep := range []string{"siteName", "domain", "registerMode", "updatedAt"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("基础字段 %q 丢失: %s", keep, got)
		}
	}
}

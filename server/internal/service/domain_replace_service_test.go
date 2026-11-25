package service

import "testing"

// 钉住图床兜底链的新规则（2026-11-25 用户要求）：
// **只保留最新 2 个** —— [新域名(主), 旧域名(备)]，不累积历史。
func TestBuildImgHosts(t *testing.T) {
	cases := []struct{ primary, backup, want string }{
		{"p.lzp261118.com", "p.lztutu.com", "p.lzp261118.com,p.lztutu.com"},
		{"P.LZP261118.COM", " p.lztutu.com ", "p.lzp261118.com,p.lztutu.com"},
		{"img.a.com", "img.a.com", "img.a.com"}, // 新==旧 ⇒ 只留一个
		{"", "img.b.com", "img.b.com"},          // 主为空 ⇒ 只留备用
		{"img.a.com", "", "img.a.com"},
		{"", "", ""}, // 全空 ⇒ 空串（调用方不会写库）
	}
	for _, c := range cases {
		if got := buildImgHosts(c.primary, c.backup); got != c.want {
			t.Errorf("buildImgHosts(%q,%q) = %q, want %q", c.primary, c.backup, got, c.want)
		}
	}
}

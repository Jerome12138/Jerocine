package mailer

import (
	"bufio"
	"encoding/base64"
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestConfigReady(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"完整", Config{Host: "smtp.qq.com", Port: 465, User: "a@qq.com", Pass: "code"}, true},
		{"缺 host", Config{Port: 465, User: "a@qq.com", Pass: "code"}, false},
		{"缺 user", Config{Host: "smtp.qq.com", Port: 465, Pass: "code"}, false},
		{"缺 pass", Config{Host: "smtp.qq.com", Port: 465, User: "a@qq.com"}, false},
		{"端口 0", Config{Host: "h", Port: 0, User: "a@qq.com", Pass: "code"}, false},
		{"端口越界", Config{Host: "h", Port: 70000, User: "a@qq.com", Pass: "code"}, false},
		{"端口 65535 边界", Config{Host: "h", Port: 65535, User: "a@qq.com", Pass: "code"}, true},
	}
	for _, c := range cases {
		if got := c.cfg.Ready(); got != c.want {
			t.Errorf("%s: Ready()=%v want %v", c.name, got, c.want)
		}
	}
}

func TestNewSenderUnconfigured(t *testing.T) {
	if _, err := NewSender(Config{}); err != ErrMailerUnconfigured {
		t.Fatalf("空配置应拒: %v", err)
	}
	s, err := NewSender(Config{Host: "smtp.qq.com", Port: 465, User: "a@qq.com", Pass: "code"})
	if err != nil || s == nil {
		t.Fatalf("完整配置应可构造: %v", err)
	}
}

// TestStartTLSRequiresAdvertised 锁住降级风险: starttls 模式下服务器未声明 STARTTLS 必须拒发。
// 此前用的是 smtp.SendMail("能升就升"), 能力项被剥离时会把 AUTH 凭据明文发出去 ——
// 管理员选 starttls 的意图恰恰是"必须加密"。
func TestStartTLSRequiresAdvertised(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		if _, err := conn.Write([]byte("220 test ESMTP\r\n")); err != nil {
			return
		}
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "EHLO"):
				// 刻意只声明 AUTH, 不声明 STARTTLS(模拟能力项被剥离)
				_, _ = conn.Write([]byte("250-test\r\n250 AUTH PLAIN LOGIN\r\n"))
			case strings.HasPrefix(upper, "QUIT"):
				_, _ = conn.Write([]byte("221 bye\r\n"))
				return
			default:
				_, _ = conn.Write([]byte("502 not implemented\r\n"))
			}
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	s, err := NewSender(Config{Host: host, Port: port, Secure: SecureStartTLS, User: "a@b.com", Pass: "p"})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	err = s.Send("to@b.com", "s", "b")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("未声明 STARTTLS 应拒发, got %v", err)
	}
	ln.Close() // 收掉服务端 goroutine
	<-done
}

func TestPresets(t *testing.T) {
	if len(Presets) < 6 {
		t.Fatalf("预设数量异常: %d", len(Presets))
	}
	qq := PresetByID("qq")
	if qq == nil || qq.Host != "smtp.qq.com" || qq.Port != 465 || qq.Secure != SecureSSL || qq.Note == "" {
		t.Fatalf("QQ 预设不对: %+v", qq)
	}
	gm := PresetByID("gmail")
	if gm == nil || gm.Secure != SecureStartTLS || gm.Port != 587 {
		t.Fatalf("Gmail 预设不对: %+v", gm)
	}
	if PresetByID("nope") != nil {
		t.Fatal("未知 id 应回 nil")
	}
	// 每个预设的 secure 合法
	for _, p := range Presets {
		switch p.Secure {
		case SecureSSL, SecureStartTLS, SecureNone:
		default:
			t.Errorf("预设 %s secure 非法: %q", p.Id, p.Secure)
		}
	}
}

func TestFromAddr(t *testing.T) {
	if got := (Config{User: "a@qq.com"}).from(); got != "a@qq.com" {
		t.Fatalf("无显示名应裸地址: %q", got)
	}
	if got := (Config{User: "a@qq.com", FromName: "Jerocine"}).from(); got != "Jerocine <a@qq.com>" {
		t.Fatalf("有显示名: %q", got)
	}
}

func TestBuildMessage(t *testing.T) {
	msg := string(buildMessage("Jerocine <a@qq.com>", "b@x.com", "中文主题", "正文"))
	if !strings.Contains(msg, "From: Jerocine <a@qq.com>\r\n") ||
		!strings.Contains(msg, "To: b@x.com\r\n") ||
		!strings.Contains(msg, "=?UTF-8?B?") || // 主题 Base64 编码
		!strings.Contains(msg, "Content-Type: text/plain; charset=UTF-8") ||
		!strings.Contains(msg, base64.StdEncoding.EncodeToString([]byte("正文"))) {
		t.Fatalf("报文拼接不对:\n%s", msg)
	}
}

func TestTemplates(t *testing.T) {
	// 验证码: 不同 purpose 区分文案, 6 位码与有效期都在正文里
	reg := CodeBody("register", "123456", 5)
	if !strings.Contains(reg, "123456") || !strings.Contains(reg, "5 分钟") || !strings.Contains(reg, "注册") {
		t.Fatalf("注册码正文缺要素: %s", reg)
	}
	if got := CodeSubject(""); got != CodeSubject("register") {
		t.Fatalf("缺省 purpose 应按注册: %q", got)
	}
	if !strings.Contains(CodeSubject("reset"), "找回密码") {
		t.Fatal("reset 主题应对应找回密码")
	}
	// 激活: 链接 + 7 天 + 兜底指引(进度查询重发)
	act := ActivateBody("https://x/activate?token=abc")
	if !strings.Contains(act, "https://x/activate?token=abc") || !strings.Contains(act, "7 天") ||
		!strings.Contains(act, "查询注册申请进度") {
		t.Fatalf("激活正文缺要素: %s", act)
	}
	// 驳回: 理由 + 重申
	rej := RejectBody("信息不完整")
	if !strings.Contains(rej, "信息不完整") || !strings.Contains(rej, "重新") {
		t.Fatalf("驳回正文缺要素: %s", rej)
	}
	// P3 重置: 链接 + 30 分钟 + 安全提示
	rst := ResetBody("https://x/reset?token=abc")
	if !strings.Contains(rst, "30 分钟") || !strings.Contains(rst, "忽略本邮件") {
		t.Fatalf("重置正文缺要素: %s", rst)
	}
	if !strings.Contains(TestBody(), "SMTP") {
		t.Fatal("测试邮件应点明 SMTP 通道")
	}
}

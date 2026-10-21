// Package mailer 出站邮件通道(SMTP) + 服务商预设。零第三方依赖: net/smtp + crypto/tls。
//
// 设计(docs/design/账号注册功能设计.md §3): 兼容性来自参数可配而非名单硬编码 ——
// 预设只是给管理后台一键填充默认值, host/port/加密/账号全部可手改。465 隐式 TLS 走
// tls.Dial + smtp.NewClient; 587 STARTTLS; 25 明文。
//
// 凭据(SMTP 授权码/应用专用密码)存 site_config, 由 RegisterService 每次发送前按当前
// 配置构造 sender —— 配置改完即热生效, 无需重启。测试经 Sender 接口注入替身, 不真发信。
package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SecureMode 加密方式。
const (
	SecureSSL      = "ssl"      // 465 隐式 TLS
	SecureStartTLS = "starttls" // 587 先明文后 STARTTLS 升级
	SecureNone     = "none"     // 25 明文(仅内网/自建)
)

// Config SMTP 连接配置(来自 site_config 邮件列组或后台测试表单)。
type Config struct {
	Host     string // smtp.qq.com
	Port     int    // 465
	Secure   string // ssl / starttls / none
	User     string // 账号(邮箱地址)
	Pass     string // 授权码 / 应用专用密码 / 密码
	FromName string // 发件人显示名(空则用 User)
}

// Sender 发送接口(测试替身注入, 不真发信)。
type Sender interface {
	Send(to, subject, body string) error
}

// ErrMailerUnconfigured 配置不完整(host/port/user/pass 缺失)。
var ErrMailerUnconfigured = errors.New("mailer: smtp not configured")

// Ready 配置是否可用(host/user/pass 非空且端口合法)。
func (c Config) Ready() bool {
	return strings.TrimSpace(c.Host) != "" && c.Port > 0 && c.Port < 65536 &&
		strings.TrimSpace(c.User) != "" && c.Pass != ""
}

// addr host:port。
func (c Config) addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }

// from 发件人: 有显示名时 "名 <邮箱>", 否则裸邮箱。
func (c Config) from() string {
	if c.FromName != "" {
		return fmt.Sprintf("%s <%s>", c.FromName, c.User)
	}
	return c.User
}

// Preset 服务商预设(仅默认值, host/port/secure 均可在后台手改)。
type Preset struct {
	Id     string `json:"id"`
	Name   string `json:"name"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Secure string `json:"secure"`
	Note   string `json:"note"` // 后台展示的注意事项(如"须用授权码")
}

// Presets 内置预设表, 经 GET /manage/mail/config 下发, 前端下拉一键填充。
var Presets = []Preset{
	{Id: "qq", Name: "QQ 邮箱", Host: "smtp.qq.com", Port: 465, Secure: SecureSSL, Note: "须使用「授权码」(设置→账号→POP3/SMTP 服务), 不是登录密码"},
	{Id: "163", Name: "163 邮箱", Host: "smtp.163.com", Port: 465, Secure: SecureSSL, Note: "须使用客户端授权码"},
	{Id: "gmail", Name: "Gmail", Host: "smtp.gmail.com", Port: 587, Secure: SecureStartTLS, Note: "须使用应用专用密码(需两步验证)"},
	{Id: "outlook", Name: "Outlook / Office365", Host: "smtp.office365.com", Port: 587, Secure: SecureStartTLS, Note: "账号 + 密码(或企业 SMTP AUTH)"},
	{Id: "exmail", Name: "腾讯企业邮", Host: "smtp.exmail.qq.com", Port: 465, Secure: SecureSSL, Note: "需管理端开启 SMTP"},
	{Id: "aliyun", Name: "阿里企业邮", Host: "smtp.mxhichina.com", Port: 465, Secure: SecureSSL, Note: "需管理端开启 SMTP"},
	{Id: "custom", Name: "自定义", Host: "", Port: 465, Secure: SecureSSL, Note: "任何标准 SMTP 服务器"},
}

// PresetByID 取预设; 不存在返回 nil。
func PresetByID(id string) *Preset {
	for i := range Presets {
		if Presets[i].Id == id {
			return &Presets[i]
		}
	}
	return nil
}

// smtpSender net/smtp 实现的 Sender。每次 Send 新建连接(发送低频, 无池化必要)。
type smtpSender struct{ cfg Config }

// NewSender 构造 sender; 配置不完整返回 ErrMailerUnconfigured。
func NewSender(cfg Config) (Sender, error) {
	if !cfg.Ready() {
		return nil, ErrMailerUnconfigured
	}
	return &smtpSender{cfg: cfg}, nil
}

func (s *smtpSender) Send(to, subject, body string) error {
	if to == "" {
		return errors.New("mailer: empty recipient")
	}
	msg := buildMessage(s.cfg.from(), to, subject, body)
	auth := smtp.PlainAuth("", s.cfg.User, s.cfg.Pass, s.cfg.Host)
	switch s.cfg.Secure {
	case SecureSSL, "":
		return sendImplicitTLS(s.cfg, auth, to, msg)
	case SecureStartTLS:
		// 显式 STARTTLS: 服务器没声明就拒发。**不能用 smtp.SendMail** ——
		// 它是"能升就升"的机会式加密, 服务器(或被中间人剥掉能力项)不声明 STARTTLS 时
		// 会把 AUTH 凭据明文发出去, 而管理员选 starttls 的意图就是必须加密。
		return sendPlainOrStartTLS(s.cfg, auth, to, msg, true)
	case SecureNone:
		// 明文模式(内网/自建): 服务器支持才升级, 不强制。
		return sendPlainOrStartTLS(s.cfg, auth, to, msg, false)
	default:
		return fmt.Errorf("mailer: unknown secure mode %q", s.cfg.Secure)
	}
}

// sendPlainOrStartTLS 先明文建连(带拨号/整链路超时), 再按需升级 STARTTLS, 最后走协议。
// requireTLS=true 时服务器未声明 STARTTLS 直接报错(不降级)。
func sendPlainOrStartTLS(cfg Config, auth smtp.Auth, to string, msg []byte, requireTLS bool) error {
	dialer := &net.Dialer{Timeout: SendTimeout}
	conn, err := dialer.Dial("tcp", cfg.addr())
	if err != nil {
		return fmt.Errorf("dial %s: %w", cfg.addr(), err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(SendTimeout))
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()
	hasTLS, _ := client.Extension("STARTTLS")
	if requireTLS && !hasTLS {
		return errors.New("smtp: 服务器未声明 STARTTLS, 已拒绝明文发送凭据(请检查端口与加密方式)")
	}
	if hasTLS {
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	return sendWithClient(client, cfg, auth, to, msg)
}

// sendImplicitTLS 465 隐式 TLS: 先 tls.Dial 建加密通道, 再走 SMTP 协议。
func sendImplicitTLS(cfg Config, auth smtp.Auth, to string, msg []byte) error {
	dialer := &net.Dialer{Timeout: SendTimeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", cfg.addr(), &tls.Config{ServerName: cfg.Host})
	if err != nil {
		return fmt.Errorf("tls dial %s: %w", cfg.addr(), err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(SendTimeout)) // 认证/数据传输同样受总时限约束
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()
	return sendWithClient(client, cfg, auth, to, msg)
}

// sendWithClient 已建连(且已按需加密)的会话上跑完 AUTH → MAIL → RCPT → DATA。
func sendWithClient(client *smtp.Client, cfg Config, auth smtp.Auth, to string, msg []byte) error {
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(cfg.User); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return client.Quit()
}

// buildMessage 组装 RFC822 报文(UTF-8 + Base64 主题, 防中文标题乱码)。
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + encodeHeader(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("\r\n")
	b.WriteString(base64Encode(body))
	return []byte(b.String())
}

// SendTimeout 单次发送总时限(含拨号/认证/数据), 防 SMTP 服务器挂起拖死请求。
const SendTimeout = 10 * time.Second

// encodeHeader Base64 编码邮件头(Subject), 防中文乱码。
func encodeHeader(s string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

func base64Encode(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// 编译期断言: 默认实现满足接口。
var _ Sender = (*smtpSender)(nil)

package service

import (
	"context"
	"crypto/rand"
	"errors"
	"regexp"
	"strings"
	"time"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/platform/auth"
	"server/internal/platform/mailer"
)

// RegisterService 账号自注册 + 出站邮件通道(设计 docs/design/账号注册功能设计.md)。
//   - 全局三态 register_mode: off(仅后台建号) / open(自由注册) / apply(申请审核, P2);
//   - 邮箱验证码只进 Redis(coord DB1), 频率: 同邮箱 1 分钟 1 封 + 日 5 封 + 全局日配额(默认 50);
//   - open 注册成功直接复用 UserService.IssueToken 签发登录态, 不另造轮子;
//   - SMTP 凭据存 site_config(json:"-" 硬隔离), 每次发送按当前配置热构造 sender。
type RegisterService struct {
	siteCfg    repository.SiteConfigRepository
	users      repository.UserRepository
	userSvc    *UserService
	dailyQuota int // 全局出站日配额(默认 50, env MAIL_DAILY_QUOTA 可配)
	now        func() time.Time
	// newSender sender 工厂(测试替身注入; 生产走 mailer.NewSender)。
	newSender func(cfg mailer.Config) (mailer.Sender, error)
}

// NewRegisterService 构造注册服务。dailyQuota<=0 时按 50。
func NewRegisterService(siteCfg repository.SiteConfigRepository, users repository.UserRepository, userSvc *UserService, dailyQuota int) *RegisterService {
	if dailyQuota <= 0 {
		dailyQuota = 50
	}
	return &RegisterService{
		siteCfg: siteCfg, users: users, userSvc: userSvc, dailyQuota: dailyQuota,
		now: time.Now, newSender: mailer.NewSender,
	}
}

// ---- 常量(设计 §5) ----

const (
	regCodeTTL     = 5 * time.Minute // 验证码有效期
	regTryLock     = 5               // 连续错误 5 次
	emailPerDay    = 5               // 同邮箱日发码 5 封
	regPasswordMin = 6               // 密码长度规则与登录侧一致(不双标)
	regPasswordMax = 64
	regNameMax     = 32 // 与 ManageUpdateUser 同名规则
)

// 注册模式取值(与 site_config.register_mode 对齐)。
const (
	RegisterModeOff   = "off"
	RegisterModeOpen  = "open"
	RegisterModeApply = "apply"
)

// purpose: register(注册/申请) / reset(P3 找回密码)。
const (
	PurposeRegister = "register"
	PurposeReset    = "reset"
)

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// ValidEmail 邮箱格式(导出供 handler/前端同源校验提示用)。
func ValidEmail(email string) bool { return emailRe.MatchString(email) }

// ---- 视图 ----

// RegisterConfigView GET /auth/register-config: 登录页据此显隐注册入口。
type RegisterConfigView struct {
	Mode      string `json:"mode"`      // off / open / apply
	SmtpReady bool   `json:"smtpReady"` // 邮件通道是否可用(未配置时 open/apply 实际不可用)
}

// MailConfigView GET /manage/mail/config: 非凭据字段 + 凭据掩码 + 预设表。
type MailConfigView struct {
	RegisterMode   string          `json:"registerMode"`
	SmtpEnabled    int             `json:"smtpEnabled"`
	SmtpHost       string          `json:"smtpHost"`
	SmtpPort       int             `json:"smtpPort"`
	SmtpSecure     string          `json:"smtpSecure"`
	SmtpUser       string          `json:"smtpUser"`
	SmtpFromName   string          `json:"smtpFromName"`
	SmtpPassMasked string          `json:"smtpPassMasked"` // 已配置时的掩码(如 a****@qq.com); 未配置空串
	SmtpPassSet    bool            `json:"smtpPassSet"`
	Presets        []mailer.Preset `json:"presets"`
}

// MailConfigInput PUT /manage/mail/config: SmtpPass 空串 = 不修改已存值(掩码回显防误清空)。
type MailConfigInput struct {
	RegisterMode string `json:"registerMode"`
	SmtpEnabled  int    `json:"smtpEnabled"`
	SmtpHost     string `json:"smtpHost"`
	SmtpPort     int    `json:"smtpPort"`
	SmtpSecure   string `json:"smtpSecure"`
	SmtpUser     string `json:"smtpUser"`
	SmtpPass     string `json:"smtpPass"`
	SmtpFromName string `json:"smtpFromName"`
}

// ---- 公开: 配置查询 / 发码 / open 注册 ----

// RegisterConfig 站点注册配置(公开)。mode 如实返回, 由前端文案说明不可用原因。
func (s *RegisterService) RegisterConfig(ctx context.Context) (RegisterConfigView, error) {
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil {
		return RegisterConfigView{}, err
	}
	return RegisterConfigView{Mode: normalizeMode(cfg.RegisterMode), SmtpReady: smtpReady(cfg)}, nil
}

// SendCode 发送邮箱验证码(注册/申请/找回共用入口)。
// 顺序: 模式门 → 邮件通道门 → 邮箱格式 → 频率(分钟窗→邮箱日配额→全局日配额) → 发信。
// reset 用途(P3)不受 register_mode 约束。
func (s *RegisterService) SendCode(ctx context.Context, purpose, email string) error {
	if purpose != PurposeRegister && purpose != PurposeReset {
		return domain.ErrInvalidArgument
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if !ValidEmail(email) {
		return domain.ErrInvalidArgument
	}
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil {
		return err
	}
	if purpose == PurposeRegister && normalizeMode(cfg.RegisterMode) == RegisterModeOff {
		return domain.ErrRegisterDisabled
	}
	mc := smtpConfigFromSite(cfg)
	if cfg.SmtpEnabled == 0 || !mc.Ready() {
		return domain.ErrMailNotConfigured
	}
	// 频率(分钟窗优先, 给最具体的错误)
	if !cache.AllowSendMinute(ctx, email) {
		return domain.ErrCodeSendTooFast
	}
	// 计数含本次: 第 emailPerDay+1 封才拒(前 emailPerDay 封放行)
	if cache.CountSendDay(ctx, email) > emailPerDay {
		return domain.ErrQuotaExceeded
	}
	if cache.CountMailQuota(ctx, cache.TodayDay(s.now())) > s.dailyQuota {
		return domain.ErrQuotaExceeded
	}
	sender, err := s.newSender(mc)
	if err != nil {
		return domain.ErrMailNotConfigured
	}
	code, err := randDigits(6)
	if err != nil {
		return err
	}
	cache.SaveRegCode(ctx, purpose, email, code, regCodeTTL)
	if err := sender.Send(email, mailer.CodeSubject(purpose), mailer.CodeBody(purpose, code, int(regCodeTTL.Minutes()))); err != nil {
		return errors.New("验证码发送失败: " + err.Error())
	}
	return nil
}

// Register open 模式自注册: 校验 → 查重 → 建号(email_verified=1) → 直接签发登录态。
// apply 模式不走本端点(走 register/apply, P2); off 模式拒绝。
func (s *RegisterService) Register(ctx context.Context, userName, password, email, code string) (LoginResult, error) {
	userName = strings.TrimSpace(userName)
	email = strings.TrimSpace(strings.ToLower(email))
	code = strings.TrimSpace(code)
	if userName == "" || len(userName) > regNameMax || !ValidEmail(email) || code == "" ||
		len(password) < regPasswordMin || len(password) > regPasswordMax {
		return LoginResult{}, domain.ErrInvalidArgument
	}
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	if normalizeMode(cfg.RegisterMode) != RegisterModeOpen {
		return LoginResult{}, domain.ErrRegisterDisabled
	}
	// 查重先于验证码: 名字/邮箱被占不该消耗用户的验证码尝试次数
	if _, err := s.users.GetByName(ctx, userName); err == nil {
		return LoginResult{}, domain.ErrConflict
	} else if err != domain.ErrUserNotFound {
		return LoginResult{}, err
	}
	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return LoginResult{}, domain.ErrEmailTaken
	} else if err != domain.ErrUserNotFound {
		return LoginResult{}, err
	}
	if !cache.VerifyRegCode(ctx, PurposeRegister, email, code) {
		if cache.AddRegTry(ctx, email) >= regTryLock {
			return LoginResult{}, domain.ErrCodeTooManyTries
		}
		return LoginResult{}, domain.ErrCodeInvalid
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return LoginResult{}, domain.ErrInvalidArgument
	}
	u := &entity.User{UserName: userName, Password: hashed, Role: entity.RoleUser, Email: email, EmailVerified: 1}
	if err := s.users.Create(ctx, u); err != nil {
		return LoginResult{}, err
	}
	cache.DelRegCode(ctx, PurposeRegister, email)
	cache.DelRegTry(ctx, email)
	return s.userSvc.IssueToken(ctx, u)
}

// ---- 管理后台: 邮件配置(掩码读/独立写/表单值测试) ----

// GetMailConfig 读邮件与注册配置(凭据只回掩码, 同 TmdbAPIKey 范式)。
func (s *RegisterService) GetMailConfig(ctx context.Context) (MailConfigView, error) {
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil {
		return MailConfigView{}, err
	}
	return MailConfigView{
		RegisterMode:   normalizeMode(cfg.RegisterMode),
		SmtpEnabled:    cfg.SmtpEnabled,
		SmtpHost:       cfg.SmtpHost,
		SmtpPort:       cfg.SmtpPort,
		SmtpSecure:     normalizeSecure(cfg.SmtpSecure),
		SmtpUser:       cfg.SmtpUser,
		SmtpFromName:   cfg.SmtpFromName,
		SmtpPassMasked: maskSecret(cfg.SmtpPass),
		SmtpPassSet:    cfg.SmtpPass != "",
		Presets:        mailer.Presets,
	}, nil
}

// SaveMailConfig 写邮件与注册配置。smtp_pass 空串 = 保留已存值; 启用前必须配置完整。
func (s *RegisterService) SaveMailConfig(ctx context.Context, in MailConfigInput) error {
	in.SmtpHost = strings.TrimSpace(in.SmtpHost)
	in.SmtpUser = strings.TrimSpace(in.SmtpUser)
	in.SmtpFromName = strings.TrimSpace(in.SmtpFromName)
	mode := normalizeMode(in.RegisterMode)
	if in.SmtpEnabled != 0 && in.SmtpEnabled != 1 {
		return domain.ErrInvalidArgument
	}
	if in.SmtpPort < 1 || in.SmtpPort > 65535 {
		return domain.ErrInvalidArgument
	}
	if sec := normalizeSecure(in.SmtpSecure); sec == "" {
		return domain.ErrInvalidArgument
	} else {
		in.SmtpSecure = sec
	}
	cur, err := s.siteCfg.Get(ctx)
	if err != nil {
		return err
	}
	pass := cur.SmtpPass
	if in.SmtpPass != "" {
		pass = in.SmtpPass
	}
	mc := mailer.Config{
		Host: in.SmtpHost, Port: in.SmtpPort, Secure: in.SmtpSecure,
		User: in.SmtpUser, Pass: pass,
	}
	if in.SmtpEnabled == 1 && !mc.Ready() {
		return domain.ErrInvalidArgument // 启用时配置必须完整
	}
	// 读-改-写(整行 UpdateAll upsert, 不动其它列)
	cur.RegisterMode = mode
	cur.SmtpEnabled = in.SmtpEnabled
	cur.SmtpHost = in.SmtpHost
	cur.SmtpPort = in.SmtpPort
	cur.SmtpSecure = in.SmtpSecure
	cur.SmtpUser = in.SmtpUser
	cur.SmtpPass = pass
	cur.SmtpFromName = in.SmtpFromName
	if err := s.siteCfg.Save(ctx, cur); err != nil {
		return err
	}
	cache.InvalidateConfig(ctx)
	return nil
}

// TestMail 用表单当前值(未保存)直发测试邮件; pass 空则用已存值(掩码回显后无法重输)。
// 错误原文上抛给 handler 展示给管理员(535 鉴权失败/连接超时定位全靠它)。
func (s *RegisterService) TestMail(ctx context.Context, to string, in MailConfigInput) error {
	to = strings.TrimSpace(strings.ToLower(to))
	if !ValidEmail(to) {
		return domain.ErrInvalidArgument
	}
	cur, err := s.siteCfg.Get(ctx)
	if err != nil {
		return err
	}
	pass := cur.SmtpPass
	if in.SmtpPass != "" {
		pass = in.SmtpPass
	}
	mc := mailer.Config{
		Host: strings.TrimSpace(in.SmtpHost), Port: in.SmtpPort,
		Secure: normalizeSecure(in.SmtpSecure), User: strings.TrimSpace(in.SmtpUser),
		Pass: pass, FromName: strings.TrimSpace(in.SmtpFromName),
	}
	sender, err := s.newSender(mc)
	if err != nil {
		return domain.ErrMailNotConfigured
	}
	if err := sender.Send(to, mailer.TestSubject, mailer.TestBody()); err != nil {
		return errors.New("测试邮件发送失败: " + err.Error())
	}
	return nil
}

// ---- 内部工具 ----

func smtpConfigFromSite(c *entity.SiteConfig) mailer.Config {
	return mailer.Config{
		Host: c.SmtpHost, Port: c.SmtpPort, Secure: normalizeSecure(c.SmtpSecure),
		User: c.SmtpUser, Pass: c.SmtpPass, FromName: c.SmtpFromName,
	}
}

// smtpReady 通道是否真的能发(开关 + 配置完整)。
func smtpReady(c *entity.SiteConfig) bool {
	return c.SmtpEnabled == 1 && smtpConfigFromSite(c).Ready()
}

// normalizeMode 非法值一律按 off(保守关)。
func normalizeMode(m string) string {
	switch m {
	case RegisterModeOpen, RegisterModeApply:
		return m
	default:
		return RegisterModeOff
	}
}

// normalizeSecure 空按 ssl(默认 465); 非法返回空串由调用方拒绝。
func normalizeSecure(sec string) string {
	switch sec {
	case "", mailer.SecureSSL:
		return mailer.SecureSSL
	case mailer.SecureStartTLS, mailer.SecureNone:
		return sec
	default:
		return ""
	}
}

// maskSecret 凭据掩码: 前 4 + … + 后 4; 过短整串打码(与 handler.maskKey 同规则)。
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) < 12 {
		return "••••"
	}
	return s[:4] + "…" + s[len(s)-4:]
}

// randDigits n 位数字验证码(crypto/rand)。
func randDigits(n int) (string, error) {
	const digits = "0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = digits[int(b[i])%len(digits)]
	}
	return string(b), nil
}

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
//   - apply 申请不收密码; 审批通过发一次性激活链接(7 天), 用户到站设密码才落 users;
//   - SMTP 凭据存 site_config(json:"-" 硬隔离), 每次发送按当前配置热构造 sender。
type RegisterService struct {
	siteCfg repository.SiteConfigRepository
	users   repository.UserRepository
	applies repository.RegisterApplyRepository
	tx      repository.TxManager
	userSvc *UserService
	// baseURL 激活链接前缀兜底(站点域名 site_config.domain 为空时用; env MAIL_BASE_URL)。
	baseURL    string
	dailyQuota int // 全局出站日配额(默认 50, env MAIL_DAILY_QUOTA 可配)
	now        func() time.Time
	newSender  func(cfg mailer.Config) (mailer.Sender, error)
}

// NewRegisterService 构造注册服务。dailyQuota<=0 时按 50; tx/applies 为 nil 时 activate 路径不可用(测试按需传)。
func NewRegisterService(
	siteCfg repository.SiteConfigRepository,
	users repository.UserRepository,
	applies repository.RegisterApplyRepository,
	tx repository.TxManager,
	userSvc *UserService,
	dailyQuota int,
	baseURL string,
) *RegisterService {
	if dailyQuota <= 0 {
		dailyQuota = 50
	}
	return &RegisterService{
		siteCfg: siteCfg, users: users, applies: applies, tx: tx, userSvc: userSvc,
		baseURL: baseURL, dailyQuota: dailyQuota,
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
	regReasonMax   = 500
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

// 出站邮件类别(频率门/日配额分类别计数, 见 cache.AllowSendMinute)。
// code=验证码; activate/reject=申请审核链路邮件; reset=找回密码链接。全局日配额仍跨类别统一计数。
const (
	mailKindCode     = "code"
	mailKindActivate = "activate"
	mailKindReject   = "reject"
	mailKindReset    = "reset"
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
	sender, err := s.readyMailSender(cfg)
	if err != nil {
		return err
	}
	// 频率(分钟窗优先, 给最具体的错误)
	if err := s.mailGate(ctx, mailKindCode, email); err != nil {
		return err
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
		return LoginResult{}, domain.ErrUserNameTaken
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

// ---- P2: 申请制(apply 模式): 申请 → 人工审核 → 一次性激活链接 → 到站设密码 ----

// Apply 提交注册申请(刻意不收密码, 设计 §9.2): 模式门 → 参数/查重 → 验证码 → 落 pending。
// 同邮箱已有在途申请(pending/approved) → ErrApplyPending(引导去进度查询/收邮件, 不重复排队)。
func (s *RegisterService) Apply(ctx context.Context, userName, email, reason, code, sourceIP string) error {
	userName = strings.TrimSpace(userName)
	email = strings.TrimSpace(strings.ToLower(email))
	reason = strings.TrimSpace(reason)
	code = strings.TrimSpace(code)
	if userName == "" || len(userName) > regNameMax || !ValidEmail(email) || code == "" || len(reason) > regReasonMax {
		return domain.ErrInvalidArgument
	}
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil {
		return err
	}
	if normalizeMode(cfg.RegisterMode) != RegisterModeApply {
		return domain.ErrRegisterDisabled
	}
	// 查重先于验证码(不消耗尝试次数): users 名/邮箱 + 未闭环申请
	if _, err := s.users.GetByName(ctx, userName); err == nil {
		return domain.ErrUserNameTaken
	} else if err != domain.ErrUserNotFound {
		return err
	}
	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return domain.ErrEmailTaken
	} else if err != domain.ErrUserNotFound {
		return err
	}
	if a, err := s.applies.GetByEmail(ctx, email); err == nil &&
		(a.Status == entity.ApplyStatusPending || a.Status == entity.ApplyStatusApproved) {
		return domain.ErrApplyPending
	} else if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if !cache.VerifyRegCode(ctx, PurposeRegister, email, code) {
		if cache.AddRegTry(ctx, email) >= regTryLock {
			return domain.ErrCodeTooManyTries
		}
		return domain.ErrCodeInvalid
	}
	if err := s.applies.Create(ctx, &entity.RegisterApply{
		UserName: userName, Email: email, Reason: reason, SourceIp: sourceIP,
		Status: entity.ApplyStatusPending,
	}); err != nil {
		return err
	}
	cache.DelRegCode(ctx, PurposeRegister, email)
	cache.DelRegTry(ctx, email)
	return nil
}

// ApplyStatusView GET /auth/apply/status: 进度查询三通道之一(邮件丢失/过期兜底)。
type ApplyStatusView struct {
	Status    string `json:"status"`    // pending / approved / rejected / none
	Reason    string `json:"reason"`    // 驳回理由(rejected 时)
	CanResend bool   `json:"canResend"` // approved: 激活邮件可重发
}

// ApplyStatus 按邮箱查最新申请状态; 无记录回 none(不泄漏是否存在他人申请——同邮箱才能查到自己那条)。
func (s *RegisterService) ApplyStatus(ctx context.Context, email string) (ApplyStatusView, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if !ValidEmail(email) {
		return ApplyStatusView{}, domain.ErrInvalidArgument
	}
	a, err := s.applies.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ApplyStatusView{Status: "none"}, nil
		}
		return ApplyStatusView{}, err
	}
	return ApplyStatusView{
		Status:    applyStatusName(a.Status),
		Reason:    a.RejectReason,
		CanResend: a.Status == entity.ApplyStatusApproved && a.ActivatedAt.IsZero(),
	}, nil
}

// ResendActivation 用户侧重发激活邮件(已通过但链接丢了/过期): 作废旧 token 发新链接。
func (s *RegisterService) ResendActivation(ctx context.Context, email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if !ValidEmail(email) {
		return domain.ErrInvalidArgument
	}
	a, err := s.applies.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrInvalidArgument // 没有在途申请
		}
		return err
	}
	if a.Status != entity.ApplyStatusApproved || !a.ActivatedAt.IsZero() {
		return domain.ErrInvalidArgument // 仅 approved 且未激活可重发
	}
	return s.sendActivateMail(ctx, a)
}

// Approve 管理员通过申请: pending → approved 先落库(审核决策不因邮件抖动丢失),
// 再发激活邮件; 邮件失败只回 warning(状态已 approved, 后台可补发)。
// 重复审批/已驳回(非 pending) → ErrConflict(幂等拒绝, 设计 §7)。
func (s *RegisterService) Approve(ctx context.Context, id int64, reviewedBy int64) (warning string, _ error) {
	if err := s.applies.Approve(ctx, id, reviewedBy); err != nil {
		return "", err // ErrNotFound / ErrConflict
	}
	a, err := s.applies.GetById(ctx, id)
	if err != nil {
		return "", err
	}
	if err := s.sendActivateMail(ctx, a); err != nil {
		return err.Error(), nil // 已 approved, 原因透出供补发定位
	}
	return "", nil
}

// Reject 管理员驳回申请(理由必填): pending → rejected + 理由落库, 再发驳回邮件(失败只 warning)。
func (s *RegisterService) Reject(ctx context.Context, id int64, reviewedBy int64, reason string) (warning string, _ error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > regReasonMax {
		return "", domain.ErrInvalidArgument
	}
	if err := s.applies.Reject(ctx, id, reviewedBy, reason); err != nil {
		return "", err
	}
	a, err := s.applies.GetById(ctx, id)
	if err != nil {
		return "", err
	}
	if err := s.sendRejectMail(ctx, a, reason); err != nil {
		return err.Error(), nil // 理由已落库, 用户登录/进度查询可见
	}
	return "", nil
}

// ResendActivationByID 后台补发激活邮件(approved; 配额内)。
func (s *RegisterService) ResendActivationByID(ctx context.Context, id int64) (warning string, _ error) {
	a, err := s.applies.GetById(ctx, id)
	if err != nil {
		return "", err
	}
	if a.Status != entity.ApplyStatusApproved || !a.ActivatedAt.IsZero() {
		return "", domain.ErrInvalidArgument // 仅 approved 且未激活可补发
	}
	if err := s.sendActivateMail(ctx, a); err != nil {
		return err.Error(), nil
	}
	return "", nil
}

// ApplyList 后台申请列表(status 为 entity.ApplyStatusAny 时不过滤), 按时间倒序分页。
func (s *RegisterService) ApplyList(ctx context.Context, status int8, page repository.Page) ([]entity.RegisterApply, int64, error) {
	return s.applies.List(ctx, status, page)
}

// Activate 激活: peek token(不消费) → 校验 → 事务建号(email_verified=1)+激活时间戳 → 消费 token → 自动登录。
// token 有效性只读校验、最终消费放在事务成功后: 用户名窗口期被占(ErrUserNameTaken)时用户改个名重试,
// 链接仍然有效(设计 §9.3)。
func (s *RegisterService) Activate(ctx context.Context, token, userName, password string) (LoginResult, error) {
	token = strings.TrimSpace(token)
	userName = strings.TrimSpace(userName)
	if token == "" || userName == "" || len(userName) > regNameMax ||
		len(password) < regPasswordMin || len(password) > regPasswordMax {
		return LoginResult{}, domain.ErrInvalidArgument
	}
	applyID, ok := cache.PeekActivateToken(ctx, token)
	if !ok {
		return LoginResult{}, domain.ErrCodeExpired
	}
	a, err := s.applies.GetById(ctx, applyID)
	if err != nil {
		return LoginResult{}, domain.ErrCodeExpired
	}
	if a.Status != entity.ApplyStatusApproved || !a.ActivatedAt.IsZero() {
		return LoginResult{}, domain.ErrCodeExpired // 已激活/重放
	}
	// 审批→激活窗口期: 邮箱可能被 open 注册者占用(唯一索引), 用户名同理
	if _, err := s.users.GetByEmail(ctx, a.Email); err == nil {
		return LoginResult{}, domain.ErrEmailTaken
	} else if err != domain.ErrUserNotFound {
		return LoginResult{}, err
	}
	if _, err := s.users.GetByName(ctx, userName); err == nil {
		return LoginResult{}, domain.ErrUserNameTaken // 前端回表单改名重试(token 仍在)
	} else if err != domain.ErrUserNotFound {
		return LoginResult{}, err
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return LoginResult{}, domain.ErrInvalidArgument
	}
	u := &entity.User{UserName: userName, Password: hashed, Role: entity.RoleUser, Email: a.Email, EmailVerified: 1}
	if err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.users.Create(ctx, u); err != nil {
			return err
		}
		return s.applies.MarkActivated(ctx, a.Id, s.now())
	}); err != nil {
		return LoginResult{}, err
	}
	cache.DelActivateToken(ctx, token) // 事务成功才一次性消费
	return s.userSvc.IssueToken(ctx, u)
}

// ApplyFeedback 登录申请态查询(经 UserService.SetApplyLookup 注入, 登录即时反馈三通道之一):
// 仅 apply 模式启用; 按用户名/邮箱取最新申请。返回 (状态码, 驳回理由, 是否命中)。
func (s *RegisterService) ApplyFeedback(ctx context.Context, account string) (int8, string, bool) {
	return s.applyFeedbackFor(ctx, account)
}

// applyFeedbackFor Login 钩子实现: 仅 apply 模式启用; off/open 维持笼统 401。
// 按用户名或邮箱取最新申请。
func (s *RegisterService) applyFeedbackFor(ctx context.Context, account string) (int8, string, bool) {
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil || normalizeMode(cfg.RegisterMode) != RegisterModeApply {
		return 0, "", false
	}
	a, err := s.applies.LatestByAccount(ctx, account)
	if err != nil {
		return 0, "", false
	}
	return a.Status, a.RejectReason, true
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

// mailGate 出站邮件频率门(所有非测试出站统一走): 分钟窗 → 邮箱日配额 → 全局日配额。
//
//	ErrCodeSendTooFast / ErrQuotaExceeded 含义对用户可读, 直接返回。
func (s *RegisterService) mailGate(ctx context.Context, kind, email string) error {
	if !cache.AllowSendMinute(ctx, kind, email) {
		return domain.ErrCodeSendTooFast
	}
	if cache.CountSendDay(ctx, kind, email) > emailPerDay {
		return domain.ErrQuotaExceeded
	}
	if cache.CountMailQuota(ctx, cache.TodayDay(s.now())) > s.dailyQuota {
		return domain.ErrQuotaExceeded
	}
	return nil
}

// readyMailSender 按当前站点配置热构造 sender; 通道未就绪 → ErrMailNotConfigured。
func (s *RegisterService) readyMailSender(cfg *entity.SiteConfig) (mailer.Sender, error) {
	mc := smtpConfigFromSite(cfg)
	if cfg.SmtpEnabled == 0 || !mc.Ready() {
		return nil, domain.ErrMailNotConfigured
	}
	return s.newSender(mc)
}

// sendActivateMail 作废旧 token → 生成并存储新 token(7 天) → 发激活邮件。
// 错误原文上抛: SMTP 失败由调用方(Approve/Resend)转 warning, 配额/频率类错误直接给用户。
func (s *RegisterService) sendActivateMail(ctx context.Context, a *entity.RegisterApply) error {
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil {
		return err
	}
	sender, err := s.readyMailSender(cfg)
	if err != nil {
		return err
	}
	if err := s.mailGate(ctx, mailKindActivate, a.Email); err != nil {
		return err
	}
	token, err := randHex(32)
	if err != nil {
		return err
	}
	cache.RotateActivateToken(ctx, a.Id) // 重发: 旧链接立即失效
	cache.SaveActivateToken(ctx, token, a.Id, cache.ActivateTTL())
	if err := sender.Send(a.Email, mailer.ActivateSubject, mailer.ActivateBody(s.activateURL(cfg, token))); err != nil {
		return errors.New("激活邮件发送失败: " + err.Error())
	}
	return nil
}

// sendRejectMail 发驳回通知(理由已落库; 失败仅 warning)。
func (s *RegisterService) sendRejectMail(ctx context.Context, a *entity.RegisterApply, reason string) error {
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil {
		return err
	}
	sender, err := s.readyMailSender(cfg)
	if err != nil {
		return err
	}
	if err := s.mailGate(ctx, mailKindReject, a.Email); err != nil {
		return err
	}
	if err := sender.Send(a.Email, mailer.RejectSubject, mailer.RejectBody(reason)); err != nil {
		return errors.New("驳回邮件发送失败: " + err.Error())
	}
	return nil
}

// activateURL 激活链接: 优先站点域名(site_config.domain), 兜底 env MAIL_BASE_URL, 再兜底相对路径。
func (s *RegisterService) activateURL(cfg *entity.SiteConfig, token string) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.Domain), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(s.baseURL), "/")
	}
	return base + "/activate?token=" + token
}

// applyStatusName 状态码 → 前端视图字符串。
func applyStatusName(st int8) string {
	switch st {
	case entity.ApplyStatusPending:
		return "pending"
	case entity.ApplyStatusApproved:
		return "approved"
	case entity.ApplyStatusRejected:
		return "rejected"
	default:
		return "none"
	}
}

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

package domain

import "errors"

// 领域错误: service/repository 返回这些哨兵错误, handler 层映射为对应 HTTP 状态码 (RFC7807)。
var (
	ErrNotFound        = errors.New("resource not found")
	ErrMovieNotFound   = errors.New("movie not found")
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrConflict        = errors.New("resource conflict")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrForbidden       = errors.New("forbidden")
	// ErrInvalidSourceID 采集源 id 不符合命名规则。
	// id 是 collect_source 主键, 被 movie_play_source.site_id / collect_failure.source_id /
	// source_health.source_id / cron_task.source_ids 以字符串引用(无外键), 一旦落库不可再改,
	// 因此入库前必须守住格式(见 manage_service.collectSourceIDRe)。
	ErrInvalidSourceID = errors.New("invalid source id: 小写字母开头, 仅小写字母/数字/下划线, 2~32 字符")
	// ErrInvalidMove 轮播位排序请求无法执行: 越界 / 已在边界 / 自动位不可下移(恒在尾部)。
	ErrInvalidMove = errors.New("无法移动：已在边界或目标位无效")
	// ---- 账号注册 / 邮箱验证码 (docs/design/账号注册功能设计.md) ----
	// ErrRegisterDisabled 注册模式为 off(仅后台建号), 公开注册入口关闭。
	ErrRegisterDisabled = errors.New("注册未开启")
	// ErrMailNotConfigured 邮件通道未配置/停用: open/apply 模式依赖邮箱验证码, 实际不可用。
	ErrMailNotConfigured = errors.New("邮件服务未配置，暂时无法注册")
	// ErrEmailTaken 邮箱已被注册(唯一索引 uk_users_email; 未验证所有者也统一此文案, 不泄漏)。
	ErrEmailTaken = errors.New("该邮箱已被注册")
	// ErrUserNameTaken 用户名被占用(open 注册 / apply 申请 / activate 窗口期查重; mapStatus 409)。
	// 取代裸 ErrConflict("resource conflict"): 该文案会直达终端用户, 必须可读;
	// 激活窗口期冲突时前端据此提示改名重试(token 仍有效, 设计 §9.3)。
	ErrUserNameTaken = errors.New("用户名已被占用")
	// ErrCodeInvalid 验证码错误(含输错/已消费)。
	ErrCodeInvalid = errors.New("验证码错误")
	// ErrCodeExpired 验证码已过期(TTL 5 分钟, Redis 键不存在即过期)。
	ErrCodeExpired = errors.New("验证码已过期，请重新获取")
	// ErrCodeTooManyTries 同邮箱验证码连续错误超限(锁 10 分钟)。
	ErrCodeTooManyTries = errors.New("验证码错误次数过多，请 10 分钟后再试")
	// ErrCodeSendTooFast 同邮箱发码太频繁(1 分钟 1 封)。
	ErrCodeSendTooFast = errors.New("验证码发送太频繁，请稍后再试")
	// ErrQuotaExceeded 邮箱日配额(5 封/天)或全局日配额(默认 50 封/天)已用尽。
	ErrQuotaExceeded = errors.New("今日验证码发送次数已达上限，请明天再试")
	// ErrApplyPending 登录时发现该账号有审核中的注册申请(仅 apply 模式返回, 见设计 §5)。
	ErrApplyPending = errors.New("注册申请审核中，请等待邮件通知")
	// ErrApplyRejected 登录时发现该账号的注册申请被驳回; 文案含驳回理由, 可重新申请。
	ErrApplyRejected = errors.New("注册申请被驳回")
)

// ApplyRejectedError 携带驳回理由的包装错误: errors.Is(err, ErrApplyRejected) 为真,
// Error() 文案即用户可见全文(含理由), 前端据此提示"可重新申请"。
type ApplyRejectedError struct{ Reason string }

func (e *ApplyRejectedError) Error() string {
	if e.Reason == "" {
		return ErrApplyRejected.Error()
	}
	return ErrApplyRejected.Error() + "：" + e.Reason
}

func (e *ApplyRejectedError) Unwrap() error { return ErrApplyRejected }

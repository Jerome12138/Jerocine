package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/dto"
	"server/internal/service"
)

// ---- 公开: 注册配置 / 发码 / open 注册 ----

// RegisterConfig GET /auth/register-config — 登录页据此显隐注册入口。
func (h *Handlers) RegisterConfig(c *gin.Context) {
	v, err := h.Register.RegisterConfig(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, v)
}

// SendRegisterCode POST /auth/register/code {email, purpose?} — 发邮箱验证码。
// purpose 缺省 register(注册/申请); reset 为 P3 找回密码预留。
func (h *Handlers) SendRegisterCode(c *gin.Context) {
	var req struct {
		Email   string `json:"email"`
		Purpose string `json:"purpose"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	purpose := strings.TrimSpace(req.Purpose)
	if purpose == "" {
		purpose = service.PurposeRegister
	}
	if err := h.Register.SendCode(c.Request.Context(), purpose, req.Email); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"sent": true})
}

// RegisterAccount POST /auth/register {userName, password, email, code} — open 模式自注册, 成功即登录。
func (h *Handlers) RegisterAccount(c *gin.Context) {
	var req struct {
		UserName string `json:"userName"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Code     string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserName == "" || req.Password == "" || req.Code == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "account/password/email/code required")
		return
	}
	res, err := h.Register.Register(c.Request.Context(), req.UserName, req.Password, req.Email, req.Code)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, res)
}

// RegisterApplyReq POST /auth/register/apply 请求体。
type RegisterApplyReq struct {
	UserName string `json:"userName"`
	Email    string `json:"email"`
	Reason   string `json:"reason"`
	Code     string `json:"code"`
}

// RegisterApply POST /auth/register/apply {userName, email, reason?, code} — apply 模式申请(不收密码)。
func (h *Handlers) RegisterApply(c *gin.Context) {
	var req RegisterApplyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Register.Apply(c.Request.Context(), req.UserName, req.Email, req.Reason, req.Code, c.ClientIP()); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"applied": true})
}

// ApplyStatus GET /auth/apply/status?email= — 进度查询(邮件丢失/过期兜底通道)。
func (h *Handlers) ApplyStatus(c *gin.Context) {
	v, err := h.Register.ApplyStatus(c.Request.Context(), c.Query("email"))
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, v)
}

// ApplyResendReq POST /auth/apply/resend 请求体。
type ApplyResendReq struct {
	Email string `json:"email"`
}

// ApplyResend POST /auth/apply/resend {email} — 已通过但激活链接丢了/过期 → 作废旧 token 重发。
func (h *Handlers) ApplyResend(c *gin.Context) {
	var req ApplyResendReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "email required")
		return
	}
	if err := h.Register.ResendActivation(c.Request.Context(), req.Email); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"resent": true})
}

// ActivateReq POST /auth/activate 请求体。
type ActivateReq struct {
	Token    string `json:"token"`
	UserName string `json:"userName"` // 窗口期用户名可能被占 → 允许改名(与申请时可不同)
	Password string `json:"password"`
}

// Activate POST /auth/activate {token, userName, password} — 一次性激活: 建号(email_verified=1) → 自动登录。
func (h *Handlers) Activate(c *gin.Context) {
	var req ActivateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" || req.Password == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "token/userName/password required")
		return
	}
	res, err := h.Register.Activate(c.Request.Context(), req.Token, req.UserName, req.Password)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, res)
}

// ---- 公开: 找回密码(P3) ----

// ResetLinkReq POST /auth/password/reset-link 请求体。
type ResetLinkReq struct {
	Email string `json:"email"`
}

// RequestPwdReset POST /auth/password/reset-link {email} — 忘记密码: 发一次性重置链接(30 分钟)。
// 防账号枚举: 邮箱是否存在响应一致(200 {sent}); 措辞由前端中性化。
func (h *Handlers) RequestPwdReset(c *gin.Context) {
	var req ResetLinkReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Email) == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "email required")
		return
	}
	if err := h.Register.RequestPasswordReset(c.Request.Context(), req.Email); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"sent": true})
}

// ResetPwdReq POST /auth/password/reset 请求体。
type ResetPwdReq struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// ResetPwd POST /auth/password/reset {token, password} — 一次性链接设新密码; 成功后该用户全端登出。
func (h *Handlers) ResetPwd(c *gin.Context) {
	var req ResetPwdReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" || req.Password == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "token/password required")
		return
	}
	if err := h.Register.ResetPassword(c.Request.Context(), req.Token, req.Password); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"reset": true})
}

// ---- 管理后台: 邮件与注册配置(凭据掩码读 / 独立写 / 表单值测试) ----

// GetMailConfig GET /manage/mail/config — 读配置(凭据只回掩码) + 服务商预设表。
func (h *Handlers) GetMailConfig(c *gin.Context) {
	v, err := h.Register.GetMailConfig(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, v)
}

// SaveMailConfig PUT /manage/mail/config {registerMode, smtp…, smtpPass?}
// smtpPass 空串 = 不修改已存值(前端拿到的是掩码, 无法回传明文)。
func (h *Handlers) SaveMailConfig(c *gin.Context) {
	var req service.MailConfigInput
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Register.SaveMailConfig(c.Request.Context(), req); err != nil {
		dto.Fail(c, err)
		return
	}
	// 回掩码后的最新配置, 前端即时刷新(含 smtpPassSet)
	v, err := h.Register.GetMailConfig(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, v)
}

// TestMail POST /manage/mail/test {to, …表单当前值} — 未保存值直发, 回显 SMTP 原文错误。
func (h *Handlers) TestMail(c *gin.Context) {
	var req struct {
		service.MailConfigInput
		To string `json:"to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Register.TestMail(c.Request.Context(), req.To, req.MailConfigInput); err != nil {
		// 错误原文直接透出(仅管理员端点): 535 鉴权失败/连接超时定位全靠它
		if errors.Is(err, domain.ErrInvalidArgument) {
			dto.Fail(c, err)
			return
		}
		dto.Error(c, http.StatusBadGateway, err.Error())
		return
	}
	dto.OK(c, gin.H{"sent": true})
}

// ---- 管理后台: 注册审核(apply 模式) ----

// ManageApplies GET /manage/register/applies?status=&page=&size= — 申请列表(分页, 时间倒序)。
func (h *Handlers) ManageApplies(c *gin.Context) {
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	status := int8(queryInt(c, "status", int(entity.ApplyStatusAny)))
	list, total, err := h.Register.ApplyList(c.Request.Context(), status, page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	n := page.Normalize(20)
	dto.Page(c, list, n.Current, n.Size, total)
}

// ManageApprove POST /manage/register/applies/:id/approve — 通过(pending→approved + 发激活邮件)。
// 邮件发送失败不阻断: 响应体 warning 透出原因, 后台可补发。
func (h *Handlers) ManageApprove(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok || id <= 0 {
		dto.Error(c, http.StatusBadRequest, "bad id")
		return
	}
	warning, err := h.Register.Approve(c.Request.Context(), id, currentUserID(c))
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"id": id, "approved": true, "warning": warning})
}

// manageRejectReq POST /manage/register/applies/:id/reject 请求体。
type manageRejectReq struct {
	Reason string `json:"reason"`
}

// ManageReject POST /manage/register/applies/:id/reject {reason} — 驳回(理由必填; 驳回邮件失败仅 warning)。
func (h *Handlers) ManageReject(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok || id <= 0 {
		dto.Error(c, http.StatusBadRequest, "bad id")
		return
	}
	var req manageRejectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusBadRequest, "bad body")
		return
	}
	warning, err := h.Register.Reject(c.Request.Context(), id, currentUserID(c), req.Reason)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"id": id, "rejected": true, "warning": warning})
}

// ManageResendActivation POST /manage/register/applies/:id/resend-activation — 激活邮件补发。
func (h *Handlers) ManageResendActivation(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok || id <= 0 {
		dto.Error(c, http.StatusBadRequest, "bad id")
		return
	}
	warning, err := h.Register.ResendActivationByID(c.Request.Context(), id)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"id": id, "resent": true, "warning": warning})
}

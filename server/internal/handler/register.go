package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/domain"
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

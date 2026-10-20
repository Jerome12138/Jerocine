package service

import (
	"context"
	"errors"
	"strings"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/platform/auth"
	"server/internal/platform/mailer"
)

// 找回密码(P3, 设计 §5/§8/§10.2): 忘记密码 → 邮箱收一次性重置链接(30 分钟)
// → 到站设新密码 → 成功即 ClearUserTokens 全端登出(旧 JWT 全失效)。
// 与激活链接同范式: peek 只读校验、事务/写成功后才消费 token; 重新申请作废旧链接。

// RequestPasswordReset 发重置链接邮件。防账号枚举: 邮箱不存在时同样静默成功
// (前端文案中性化"若该邮箱已注册将会收到邮件"); 频率/配额/通道错误照实返回。
// 顺序: 邮箱格式 → 频率门(分钟窗→邮箱日配额→全局日配额) → 通道门 → 查用户
// → 仅存在才作废旧 token 发新链接。
func (s *RegisterService) RequestPasswordReset(ctx context.Context, email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if !ValidEmail(email) {
		return domain.ErrInvalidArgument
	}
	if err := s.mailGate(ctx, mailKindReset, email); err != nil {
		return err
	}
	cfg, err := s.siteCfg.Get(ctx)
	if err != nil {
		return err
	}
	sender, err := s.readyMailSender(cfg)
	if err != nil {
		return err
	}
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil // 防枚举: 不发信也不报错
		}
		return err
	}
	token, err := randHex(32)
	if err != nil {
		return err
	}
	cache.RotatePwdResetToken(ctx, int64(u.ID)) // 重新申请: 旧链接立即失效
	cache.SavePwdResetToken(ctx, token, int64(u.ID), cache.PwdResetTTL())
	if err := sender.Send(email, mailer.ResetSubject, mailer.ResetBody(s.resetURL(cfg, token))); err != nil {
		return errors.New("重置邮件发送失败: " + err.Error())
	}
	return nil
}

// ResetPassword 用重置链接设新密码: peek(不消费) → 用户存在 → 改密 →
// ClearUserTokens 全端登出 → 消费 token(单次)。失败不烧链接(可重试/重新申请)。
func (s *RegisterService) ResetPassword(ctx context.Context, token, password string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(password) < regPasswordMin || len(password) > regPasswordMax {
		return domain.ErrInvalidArgument
	}
	userID, ok := cache.PeekPwdResetToken(ctx, token)
	if !ok {
		return domain.ErrResetTokenInvalid
	}
	u, err := s.users.GetById(ctx, uint(userID))
	if err != nil {
		return domain.ErrResetTokenInvalid // 用户被删/查不到: 链接作废
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return domain.ErrInvalidArgument
	}
	if err := s.users.UpdatePassword(ctx, u.ID, hashed); err != nil {
		return err
	}
	cache.ClearUserTokens(ctx, int64(u.ID)) // 全端登出: 旧 JWT 全部失效
	cache.DelPwdResetToken(ctx, token)      // 写成功才一次性消费
	return nil
}

// resetURL 重置链接: 优先站点域名(site_config.domain), 兜底 env MAIL_BASE_URL(与激活链接同规)。
func (s *RegisterService) resetURL(cfg *entity.SiteConfig, token string) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.Domain), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(s.baseURL), "/")
	}
	return base + "/reset-password?token=" + token
}

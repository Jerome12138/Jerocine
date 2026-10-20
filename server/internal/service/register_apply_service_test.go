package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// ---- apply 申请(不收密码) ----

func applyModeSite() *entity.SiteConfig {
	site := mailReadySite()
	site.RegisterMode = RegisterModeApply
	return site
}

// applyByCode 发码 → 提交申请, 返回申请行(helper)。
func applyByCode(t *testing.T, svc *RegisterService, fs *fakeSender, userName, email, reason string) *entity.RegisterApply {
	t.Helper()
	ctx := context.Background()
	if err := svc.SendCode(ctx, PurposeRegister, email); err != nil {
		t.Fatalf("发码: %v", err)
	}
	if err := svc.Apply(ctx, userName, email, reason, fs.lastCode(t), "1.2.3.4"); err != nil {
		t.Fatalf("申请: %v", err)
	}
	a, err := svc.applies.GetByEmail(ctx, strings.ToLower(email))
	if err != nil {
		t.Fatalf("查申请行: %v", err)
	}
	return a
}

// clearActivateWindow 清激活邮件 1 分钟窗(测试里连续触发 审批→失败→补发 用)。
func clearActivateWindow(email string) {
	cache.Del(context.Background(), cache.KeyRegSendMin("activate", email))
}

func TestApplyGates(t *testing.T) {
	ctx := context.Background()

	t.Run("off/open 模式拒绝", func(t *testing.T) {
		for _, mode := range []string{RegisterModeOff, RegisterModeOpen} {
			site := mailReadySite()
			site.RegisterMode = mode
			svc, _, _ := newRegisterSvc(t, site, 50)
			if err := svc.Apply(ctx, "u1", "a@qq.com", "", "123456", ""); !errors.Is(err, domain.ErrRegisterDisabled) {
				t.Fatalf("mode=%s 应拒: %v", mode, err)
			}
		}
	})
	t.Run("参数校验", func(t *testing.T) {
		svc, _, _ := newRegisterSvc(t, applyModeSite(), 50)
		bad := []struct{ name, email, reason, code string }{
			{"", "a@qq.com", "", "123456"},                         // 空用户名
			{strings.Repeat("x", 33), "a@qq.com", "", "123456"},    // 用户名过长
			{"u1", "bad-email", "", "123456"},                      // 邮箱格式
			{"u1", "a@qq.com", strings.Repeat("r", 501), "123456"}, // 理由过长
			{"u1", "a@qq.com", "", ""},                             // 缺验证码
		}
		for i, c := range bad {
			if err := svc.Apply(ctx, c.name, c.email, c.reason, c.code, ""); !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("case %d: got %v", i, err)
			}
		}
	})
	t.Run("查重先于验证码(不消耗尝试次数)", func(t *testing.T) {
		svc, users, _ := newRegisterSvc(t, applyModeSite(), 50)
		users.byName["taken"] = &entity.User{UserName: "taken"}
		if err := svc.Apply(ctx, "taken", "fresh@qq.com", "", "000000", ""); !errors.Is(err, domain.ErrUserNameTaken) {
			t.Fatalf("用户名占用: %v", err)
		}
		users.byEmail["taken@qq.com"] = &entity.User{Email: "taken@qq.com"}
		if err := svc.Apply(ctx, "fresh", "taken@qq.com", "", "000000", ""); !errors.Is(err, domain.ErrEmailTaken) {
			t.Fatalf("邮箱占用: %v", err)
		}
		for i := 0; i < 4; i++ {
			if err := svc.Apply(ctx, "fresh", "fresh@qq.com", "", "000000", ""); !errors.Is(err, domain.ErrCodeInvalid) {
				t.Fatalf("第 %d 次错码: %v", i+1, err)
			}
		}
		if err := svc.Apply(ctx, "fresh", "fresh@qq.com", "", "000000", ""); !errors.Is(err, domain.ErrCodeTooManyTries) {
			t.Fatalf("第 5 次应锁: %v", err)
		}
	})
	t.Run("同邮箱在途申请(pending)重复提交 → ErrApplyPending", func(t *testing.T) {
		svc, _, fs := newRegisterSvc(t, applyModeSite(), 50)
		applyByCode(t, svc, fs, "ann", "dup@qq.com", "")
		if err := svc.Apply(ctx, "ann2", "dup@qq.com", "", "000000", ""); !errors.Is(err, domain.ErrApplyPending) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestApplyHappyPath(t *testing.T) {
	ctx := context.Background()
	svc, users, applies, fs := newRegisterSvcWith(t, applyModeSite(), 50)
	a := applyByCode(t, svc, fs, "bob", "Bob@QQ.com", "朋友推荐")

	if a.Status != entity.ApplyStatusPending || a.Email != "bob@qq.com" || a.SourceIp != "1.2.3.4" || a.Reason != "朋友推荐" {
		t.Fatalf("pending 行字段不对: %+v", a)
	}
	if len(applies.rows) != 1 {
		t.Fatalf("应只有 1 条申请, got %d", len(applies.rows))
	}
	// 申请不落 users
	if len(users.byName) != 0 {
		t.Fatalf("申请不应建号: %+v", users.byName)
	}
	// 进度查询: pending, 不可重发
	v, err := svc.ApplyStatus(ctx, "bob@qq.com")
	if err != nil || v.Status != "pending" || v.CanResend {
		t.Fatalf("status 视图不对: %+v err=%v", v, err)
	}
	// 未知邮箱 → none
	v2, err := svc.ApplyStatus(ctx, "nobody@qq.com")
	if err != nil || v2.Status != "none" {
		t.Fatalf("none 视图不对: %+v err=%v", v2, err)
	}
}

// ---- 审批 / 驳回 / 激活 ----

var activateURLRe = regexp.MustCompile(`/activate\?token=([0-9a-f]{64})`)

// lastActivateToken 取最后一封激活邮件里的 token。
func (fs *fakeSender) lastActivateToken(t *testing.T) string {
	t.Helper()
	for i := len(fs.sent) - 1; i >= 0; i-- {
		if m := activateURLRe.FindStringSubmatch(fs.sent[i].Body); m != nil {
			return m[1]
		}
	}
	t.Fatalf("没有激活邮件: %+v", fs.sent)
	return ""
}

func TestApproveAndActivateFlow(t *testing.T) {
	ctx := context.Background()
	svc, users, applies, fs := newRegisterSvcWith(t, applyModeSite(), 50)
	a := applyByCode(t, svc, fs, "carl", "carl@qq.com", "")

	// 管理员通过 → 激活邮件(token 在 URL 里, 站点域名兜底 base)
	warning, err := svc.Approve(ctx, a.Id, 9001)
	if err != nil || warning != "" {
		t.Fatalf("审批: warning=%q err=%v", warning, err)
	}
	if applies.rows[0].Status != entity.ApplyStatusApproved || applies.rows[0].ReviewedBy != 9001 {
		t.Fatalf("审批行状态不对: %+v", applies.rows[0])
	}
	token := fs.lastActivateToken(t)
	if !strings.Contains(fs.sent[len(fs.sent)-1].Body, "https://site.example/activate?token="+token) {
		t.Fatalf("激活链接 base 兜底不对: %s", fs.sent[len(fs.sent)-1].Body)
	}

	// 重复审批 → 幂等拒绝
	if _, err := svc.Approve(ctx, a.Id, 9001); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("重复审批应 conflict: %v", err)
	}

	// 激活: 设密码建号 + 自动登录
	res, err := svc.Activate(ctx, token, "carl", "secret1")
	if err != nil {
		t.Fatalf("激活: %v", err)
	}
	if res.Token == "" || res.UserName != "carl" || res.Role != entity.RoleUser {
		t.Fatalf("激活应自动登录: %+v", res)
	}
	u := users.byName["carl"]
	if u == nil || u.Email != "carl@qq.com" || u.EmailVerified != 1 {
		t.Fatalf("激活落库用户不对: %+v", u)
	}
	if applies.rows[0].ActivatedAt.IsZero() {
		t.Fatal("activated_at 应已写")
	}

	// token 一次性: 二次激活(即便改名) → ErrCodeExpired
	if _, err := svc.Activate(ctx, token, "carl2", "secret1"); !errors.Is(err, domain.ErrCodeExpired) {
		t.Fatalf("旧 token 应失效: %v", err)
	}
}

func TestActivateRenameOnConflict(t *testing.T) {
	ctx := context.Background()
	svc, _, _, fs := newRegisterSvcWith(t, applyModeSite(), 50)
	a := applyByCode(t, svc, fs, "dave", "dave@qq.com", "")
	if _, err := svc.Approve(ctx, a.Id, 9001); err != nil {
		t.Fatal(err)
	}
	token := fs.lastActivateToken(t)

	// 窗口期用户名被 open 注册者占用 → ErrUserNameTaken, 但 token 仍有效
	svc.users.(*fakeRegUsers).byName["dave"] = &entity.User{UserName: "dave"}
	if _, err := svc.Activate(ctx, token, "dave", "secret1"); !errors.Is(err, domain.ErrUserNameTaken) {
		t.Fatalf("用户名占用应 conflict: %v", err)
	}
	// 改名重试成功
	res, err := svc.Activate(ctx, token, "dave2024", "secret1")
	if err != nil {
		t.Fatalf("改名重试: %v", err)
	}
	if res.UserName != "dave2024" {
		t.Fatalf("应按新名建号: %+v", res)
	}
}

func TestActivateRejectsAfterReject(t *testing.T) {
	ctx := context.Background()
	svc, _, _, fs := newRegisterSvcWith(t, applyModeSite(), 50)
	a := applyByCode(t, svc, fs, "erin", "erin@qq.com", "")

	// 驳回: 空理由拒绝; 合法理由落库 + 驳回邮件
	if _, err := svc.Reject(ctx, a.Id, 9001, "  "); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("空理由应拒: %v", err)
	}
	warning, err := svc.Reject(ctx, a.Id, 9001, "邀请码缺失")
	if err != nil || warning != "" {
		t.Fatalf("驳回: warning=%q err=%v", warning, err)
	}
	row, _ := svc.applies.GetById(ctx, a.Id)
	if row.Status != entity.ApplyStatusRejected || row.RejectReason != "邀请码缺失" {
		t.Fatalf("驳回行不对: %+v", row)
	}
	if len(fs.sent) == 0 || !strings.Contains(fs.sent[len(fs.sent)-1].Subject, "未通过") ||
		!strings.Contains(fs.sent[len(fs.sent)-1].Body, "邀请码缺失") {
		t.Fatalf("驳回邮件要素不对: %+v", fs.sent)
	}
	// 已驳回后重复驳回 → conflict
	if _, err := svc.Reject(ctx, a.Id, 9001, "x"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("重复驳回应 conflict: %v", err)
	}
	// 进度查询: rejected + 理由, 不可重发
	v, _ := svc.ApplyStatus(ctx, "erin@qq.com")
	if v.Status != "rejected" || v.Reason != "邀请码缺失" || v.CanResend {
		t.Fatalf("驳回状态视图不对: %+v", v)
	}
	// 重申(同邮箱再申请)允许: 新开一行 pending
	clearMinuteWindow("erin@qq.com")
	if err := svc.SendCode(ctx, PurposeRegister, "erin@qq.com"); err != nil {
		t.Fatalf("重申发码: %v", err)
	}
	if err := svc.Apply(ctx, "erin2", "erin@qq.com", "补邀请码", fs.lastCode(t), ""); err != nil {
		t.Fatalf("重申: %v", err)
	}
	v2, _ := svc.ApplyStatus(ctx, "erin@qq.com")
	if v2.Status != "pending" {
		t.Fatalf("重申后应 pending: %+v", v2)
	}
}

func TestResendActivationRotatesToken(t *testing.T) {
	ctx := context.Background()
	svc, _, applies, fs := newRegisterSvcWith(t, applyModeSite(), 50)
	a := applyByCode(t, svc, fs, "frank", "frank@qq.com", "")
	if _, err := svc.Approve(ctx, a.Id, 9001); err != nil {
		t.Fatal(err)
	}
	oldToken := fs.lastActivateToken(t)

	// 用户侧重发: 同邮箱, approved(先过分钟窗, 模拟上一封激活邮件已发了一分钟)
	clearActivateWindow("frank@qq.com")
	if err := svc.ResendActivation(ctx, "frank@qq.com"); err != nil {
		t.Fatalf("重发: %v", err)
	}
	newToken := fs.lastActivateToken(t)
	if newToken == oldToken {
		t.Fatal("重发应换新 token")
	}
	// 旧 token 立即失效, 新 token 可激活
	if _, ok := cache.PeekActivateToken(context.Background(), oldToken); ok {
		t.Fatal("旧 token 应已作废")
	}
	if _, err := svc.Activate(ctx, newToken, "frank", "secret1"); err != nil {
		t.Fatalf("新 token 激活: %v", err)
	}
	// 已激活后再重发 → ErrInvalidArgument
	if err := svc.ResendActivation(ctx, "frank@qq.com"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("已激活后重发: %v", err)
	}

	t.Run("pending 申请重发被拒", func(t *testing.T) {
		svc2, _, fs2 := newRegisterSvc(t, applyModeSite(), 50)
		applyByCode(t, svc2, fs2, "gina", "gina@qq.com", "")
		if err := svc2.ResendActivation(ctx, "gina@qq.com"); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("后台补发(approved)", func(t *testing.T) {
		svc3, _, _, fs3 := newRegisterSvcWith(t, applyModeSite(), 50)
		a3 := applyByCode(t, svc3, fs3, "hank", "hank@qq.com", "")
		if _, err := svc3.Approve(ctx, a3.Id, 9001); err != nil {
			t.Fatal(err)
		}
		clearActivateWindow("hank@qq.com") // 审批那封已占分钟窗
		w, err := svc3.ResendActivationByID(ctx, a3.Id)
		if err != nil || w != "" {
			t.Fatalf("补发: warning=%q err=%v", w, err)
		}
	})
	if applies.rows[0].Status != entity.ApplyStatusApproved {
		t.Fatal("sanity")
	}
}

func TestApproveMailFailureKeepsApproved(t *testing.T) {
	ctx := context.Background()
	svc, _, applies, fs := newRegisterSvcWith(t, applyModeSite(), 50)
	a := applyByCode(t, svc, fs, "ivan", "ivan@qq.com", "")

	// 激活邮件发不出去(配额耗尽是真实场景; 这里用 SMTP 原文错误)
	fs.err = errors.New("421 too many messages")
	warning, err := svc.Approve(ctx, a.Id, 9001)
	if err != nil {
		t.Fatalf("审批不应被邮件失败阻断: %v", err)
	}
	if !strings.Contains(warning, "421 too many messages") {
		t.Fatalf("warning 应含 SMTP 原文: %q", warning)
	}
	// 状态已 approved → 补发通道可用
	if applies.rows[0].Status != entity.ApplyStatusApproved {
		t.Fatalf("应按 approved 落库: %+v", applies.rows[0])
	}
	fs.err = nil
	clearActivateWindow("ivan@qq.com") // 上次失败也占了分钟窗, 模拟过了窗口期
	w, err := svc.ResendActivationByID(ctx, a.Id)
	if err != nil || w != "" {
		t.Fatalf("恢复后补发: warning=%q err=%v", w, err)
	}
}

func TestApplyListStatusFilter(t *testing.T) {
	ctx := context.Background()
	svc, _, fs := newRegisterSvc(t, applyModeSite(), 50)
	a1 := applyByCode(t, svc, fs, "u1", "u1@qq.com", "")
	applyByCode(t, svc, fs, "u2", "u2@qq.com", "")
	if _, err := svc.Reject(ctx, a1.Id, 9001, "no"); err != nil {
		t.Fatal(err)
	}

	all, total, err := svc.ApplyList(ctx, entity.ApplyStatusAny, repository.Page{Current: 1, Size: 20})
	if err != nil || total != 2 || len(all) != 2 {
		t.Fatalf("全量列表不对: total=%d len=%d err=%v", total, len(all), err)
	}
	pend, _, _ := svc.ApplyList(ctx, entity.ApplyStatusPending, repository.Page{Current: 1, Size: 20})
	if len(pend) != 1 || pend[0].UserName != "u2" {
		t.Fatalf("pending 过滤不对: %+v", pend)
	}
}

// ---- 登录申请态反馈(仅 apply 模式) ----

func TestLoginApplyFeedback(t *testing.T) {
	ctx := context.Background()

	t.Run("pending → ErrApplyPending", func(t *testing.T) {
		svc, _, fs := newRegisterSvc(t, applyModeSite(), 50)
		applyByCode(t, svc, fs, "jack", "jack@qq.com", "")
		userSvc := svc.userSvc
		userSvc.SetApplyLookup(svc.ApplyFeedback)
		if _, err := userSvc.Login(ctx, "jack", "whatever"); !errors.Is(err, domain.ErrApplyPending) {
			t.Fatalf("got %v", err)
		}
		// 邮箱账号同样命中
		if _, err := userSvc.Login(ctx, "jack@qq.com", "whatever"); !errors.Is(err, domain.ErrApplyPending) {
			t.Fatalf("email account: %v", err)
		}
	})
	t.Run("rejected → 包装错误带理由", func(t *testing.T) {
		svc, _, fs := newRegisterSvc(t, applyModeSite(), 50)
		a := applyByCode(t, svc, fs, "kate", "kate@qq.com", "")
		if _, err := svc.Reject(ctx, a.Id, 9001, "资料不全"); err != nil {
			t.Fatal(err)
		}
		userSvc := svc.userSvc
		userSvc.SetApplyLookup(svc.ApplyFeedback)
		_, err := userSvc.Login(ctx, "kate", "whatever")
		if !errors.Is(err, domain.ErrApplyRejected) {
			t.Fatalf("got %v", err)
		}
		var re *domain.ApplyRejectedError
		if !errors.As(err, &re) || !strings.Contains(err.Error(), "资料不全") {
			t.Fatalf("理由应随错误透出: %v", err)
		}
	})
	t.Run("off/open 模式不开启(笼统 401)", func(t *testing.T) {
		for _, mode := range []string{RegisterModeOff, RegisterModeOpen} {
			site := mailReadySite()
			site.RegisterMode = mode
			svc, _, _ := newRegisterSvc(t, site, 50)
			// 直接在 applies 里造一条 pending(绕过模式门), 验证反馈仍不开启
			_ = svc.applies.Create(ctx, &entity.RegisterApply{UserName: "leo", Email: "leo@qq.com", Status: entity.ApplyStatusPending})
			svc.userSvc.SetApplyLookup(svc.ApplyFeedback)
			if _, err := svc.userSvc.Login(ctx, "leo", "whatever"); !errors.Is(err, domain.ErrUnauthorized) {
				t.Fatalf("mode=%s 应笼统 401: %v", mode, err)
			}
		}
	})
	t.Run("无申请记录的账号维持 ErrUnauthorized", func(t *testing.T) {
		svc, _, _ := newRegisterSvc(t, applyModeSite(), 50)
		svc.userSvc.SetApplyLookup(svc.ApplyFeedback)
		if _, err := svc.userSvc.Login(ctx, "ghost", "whatever"); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("got %v", err)
		}
	})
}

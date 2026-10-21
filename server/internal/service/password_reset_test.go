package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/platform/mailer"
)

// 找回密码 service 测试(见 password_reset.go; 设计 §5/§8/§10.2)。

// ---- fakeRegUsers 的 P3 扩展(同包方法, 文件分离) ----

func (f *fakeRegUsers) GetById(_ context.Context, id uint) (*entity.User, error) {
	for _, m := range []map[string]*entity.User{f.byName, f.byEmail} {
		for _, u := range m {
			if u.ID == id {
				return u, nil
			}
		}
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeRegUsers) UpdatePassword(_ context.Context, id uint, hashed string) error {
	u, err := f.GetById(context.Background(), id)
	if err != nil {
		return err
	}
	u.Password = hashed
	return nil
}

// failUpdateUsers UpdatePassword 恒失败(验证"失败不烧链接")。
type failUpdateUsers struct{ *fakeRegUsers }

func (f failUpdateUsers) UpdatePassword(_ context.Context, _ uint, _ string) error {
	return errors.New("db down")
}

func resetSite() *entity.SiteConfig {
	s := mailReadySite()
	s.Domain = "https://jerocine.art" // 链接前缀只看站点域名(env 兜底已移除)
	return s
}

func newPwdResetSvc(t *testing.T) (*RegisterService, *fakeRegUsers, *fakeSender) {
	t.Helper()
	setupCache(t)
	users := newFakeRegUsers()
	userSvc := NewUserService(users, nil, nil, nil, nil, testTokenManager(t), 5, time.Hour)
	svc := NewRegisterService(&fakeSiteCfg{cfg: resetSite()}, users, &fakeApplies{}, fakeTx{}, userSvc)
	fs := &fakeSender{}
	svc.newSender = func(mailer.Config) (mailer.Sender, error) { return fs, nil }
	return svc, users, fs
}

func seedResetUser(users *fakeRegUsers) *entity.User {
	u := &entity.User{ID: 7, UserName: "jerry", Email: "jerry@x.com", Password: "old-hashed"}
	users.byName[u.UserName] = u
	users.byEmail[u.Email] = u
	return u
}

func clearResetWindow(email string) {
	cache.Del(context.Background(), cache.KeyRegSendMin("reset", email))
}

// resetTokenFromMail 从重置邮件正文里抠出 token(链接独占一行, 行尾即 token 终点;
// randHex(32) 只产十六进制字符, 不会含换行/空白)。
func resetTokenFromMail(t *testing.T, fs *fakeSender) string {
	t.Helper()
	if len(fs.sent) == 0 {
		t.Fatal("没有已发送邮件")
	}
	body := fs.sent[len(fs.sent)-1].Body
	i := strings.Index(body, "/reset-password?token=")
	if i < 0 {
		t.Fatalf("邮件正文找不到重置链接: %s", body)
	}
	rest := body[i+len("/reset-password?token="):]
	if j := strings.IndexAny(rest, "\r\n"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

// ---- 申请重置链接 ----

func TestRequestPwdResetNoLeakUnknownEmail(t *testing.T) {
	svc, users, fs := newPwdResetSvc(t)
	ctx := context.Background()
	if err := svc.RequestPasswordReset(ctx, "ghost@x.com"); err != nil {
		t.Fatalf("邮箱不存在应静默成功(防枚举): %v", err)
	}
	if len(fs.sent) != 0 {
		t.Fatal("不存在的邮箱不应发信")
	}
	if _, ok := cache.PeekPwdResetToken(ctx, "whatever"); ok {
		t.Fatal("不应产生任何 token")
	}
	_ = users
}

func TestRequestPwdResetSendsLink(t *testing.T) {
	svc, users, fs := newPwdResetSvc(t)
	seedResetUser(users)
	ctx := context.Background()

	if err := svc.RequestPasswordReset(ctx, "jerry@x.com"); err != nil {
		t.Fatalf("已注册邮箱应发链接: %v", err)
	}
	if len(fs.sent) != 1 {
		t.Fatalf("应发 1 封, got %d", len(fs.sent))
	}
	m := fs.sent[0]
	if m.To != "jerry@x.com" || m.Subject != mailer.ResetSubject {
		t.Fatalf("邮件主题/收件人不符: %+v", m)
	}
	if !strings.Contains(m.Body, "https://jerocine.art/reset-password?token=") {
		t.Fatalf("链接应优先站点域名: %s", m.Body)
	}
	tok := resetTokenFromMail(t, fs)
	if id, ok := cache.PeekPwdResetToken(ctx, tok); !ok || id != 7 {
		t.Fatalf("token 应映射 userID=7, got (%d,%v)", id, ok)
	}
}

func TestRequestPwdResetRotatesOldToken(t *testing.T) {
	svc, users, fs := newPwdResetSvc(t)
	seedResetUser(users)
	ctx := context.Background()

	if err := svc.RequestPasswordReset(ctx, "jerry@x.com"); err != nil {
		t.Fatal(err)
	}
	oldTok := resetTokenFromMail(t, fs)
	clearResetWindow("jerry@x.com")
	if err := svc.RequestPasswordReset(ctx, "jerry@x.com"); err != nil {
		t.Fatal(err)
	}
	newTok := resetTokenFromMail(t, fs)
	if oldTok == newTok {
		t.Fatal("重新申请应换新 token")
	}
	if _, ok := cache.PeekPwdResetToken(ctx, oldTok); ok {
		t.Fatal("重新申请后旧链接应立即失效")
	}
	if id, ok := cache.PeekPwdResetToken(ctx, newTok); !ok || id != 7 {
		t.Fatalf("新链接应可用, got (%d,%v)", id, ok)
	}
}

func TestRequestPwdResetMinuteGate(t *testing.T) {
	svc, users, _ := newPwdResetSvc(t)
	seedResetUser(users)
	ctx := context.Background()
	if err := svc.RequestPasswordReset(ctx, "jerry@x.com"); err != nil {
		t.Fatal(err)
	}
	// 1 分钟窗内第二次(未过窗) → 频率错误照实返回
	err := svc.RequestPasswordReset(ctx, "jerry@x.com")
	if !errors.Is(err, domain.ErrCodeSendTooFast) {
		t.Fatalf("分钟窗应挡第二次, got %v", err)
	}
}

// ---- 用链接重置密码 ----

func TestResetPasswordHappyPath(t *testing.T) {
	svc, users, fs := newPwdResetSvc(t)
	u := seedResetUser(users)
	ctx := context.Background()
	if err := svc.RequestPasswordReset(ctx, "jerry@x.com"); err != nil {
		t.Fatal(err)
	}
	tok := resetTokenFromMail(t, fs)
	// 预置多设备 token 集合(证明全端登出)
	cache.CoordClient().ZAdd(ctx, cache.KeyToken(7), redis.Z{Score: 1, Member: "jwt-a"}, redis.Z{Score: 2, Member: "jwt-b"})

	if err := svc.ResetPassword(ctx, tok, "newsecret1"); err != nil {
		t.Fatalf("重置应成功: %v", err)
	}
	if u.Password == "old-hashed" {
		t.Fatal("密码应已更新")
	}
	if _, ok := cache.PeekPwdResetToken(ctx, tok); ok {
		t.Fatal("成功后 token 应被一次性消费")
	}
	if n, _ := cache.CoordClient().Exists(ctx, cache.KeyToken(7)).Result(); n != 0 {
		t.Fatal("改密后应清空全部设备 token(全端登出)")
	}
}

func TestResetPasswordSingleUse(t *testing.T) {
	svc, users, fs := newPwdResetSvc(t)
	seedResetUser(users)
	ctx := context.Background()
	if err := svc.RequestPasswordReset(ctx, "jerry@x.com"); err != nil {
		t.Fatal(err)
	}
	tok := resetTokenFromMail(t, fs)
	if err := svc.ResetPassword(ctx, tok, "newsecret1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(ctx, tok, "newsecret2"); !errors.Is(err, domain.ErrResetTokenInvalid) {
		t.Fatalf("同 token 二次使用应被拒, got %v", err)
	}
}

func TestResetPasswordInvalidToken(t *testing.T) {
	svc, _, _ := newPwdResetSvc(t)
	ctx := context.Background()
	if err := svc.ResetPassword(ctx, "not-exist", "newsecret1"); !errors.Is(err, domain.ErrResetTokenInvalid) {
		t.Fatalf("无效 token → ErrResetTokenInvalid, got %v", err)
	}
	if err := svc.ResetPassword(ctx, "", "newsecret1"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("空 token → ErrInvalidArgument, got %v", err)
	}
	if err := svc.ResetPassword(ctx, "tok", "123"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("短密码 → ErrInvalidArgument, got %v", err)
	}
}

// 写失败不烧链接: 改密报错时 token 仍有效, 用户可重试。
func TestResetPasswordKeepsTokenOnWriteFailure(t *testing.T) {
	setupCache(t)
	users := newFakeRegUsers()
	seedResetUser(users)
	fs := &fakeSender{}
	svc := NewRegisterService(&fakeSiteCfg{cfg: resetSite()}, failUpdateUsers{users}, &fakeApplies{}, fakeTx{}, nil)
	svc.newSender = func(mailer.Config) (mailer.Sender, error) { return fs, nil }
	ctx := context.Background()
	if err := svc.RequestPasswordReset(ctx, "jerry@x.com"); err != nil {
		t.Fatal(err)
	}
	tok := resetTokenFromMail(t, fs)
	if err := svc.ResetPassword(ctx, tok, "newsecret1"); err == nil {
		t.Fatal("UpdatePassword 失败应上抛")
	}
	if _, ok := cache.PeekPwdResetToken(ctx, tok); !ok {
		t.Fatal("写失败不应消费 token")
	}
}

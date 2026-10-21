package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/platform/mailer"
)

// ---- 测试替身 ----

type fakeSiteCfg struct {
	repository.SiteConfigRepository
	cfg *entity.SiteConfig
}

func (f *fakeSiteCfg) Get(context.Context) (*entity.SiteConfig, error) { return f.cfg, nil }
func (f *fakeSiteCfg) Save(_ context.Context, c *entity.SiteConfig) error {
	*f.cfg = *c
	return nil
}

type fakeRegUsers struct {
	repository.UserRepository
	byName  map[string]*entity.User
	byEmail map[string]*entity.User
	nextID  uint
}

func newFakeRegUsers() *fakeRegUsers {
	return &fakeRegUsers{byName: map[string]*entity.User{}, byEmail: map[string]*entity.User{}}
}

func (f *fakeRegUsers) GetByName(_ context.Context, name string) (*entity.User, error) {
	if u, ok := f.byName[name]; ok {
		return u, nil
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeRegUsers) GetByEmail(_ context.Context, email string) (*entity.User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeRegUsers) Create(_ context.Context, u *entity.User) error {
	f.nextID++
	u.ID = f.nextID + 10000
	f.byName[u.UserName] = u
	if u.Email != "" {
		f.byEmail[u.Email] = u
	}
	return nil
}

type sentMail struct{ To, Subject, Body string }

type fakeSender struct {
	sent []sentMail
	err  error
}

func (f *fakeSender) Send(to, subject, body string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, sentMail{to, subject, body})
	return nil
}

// ---- P2 替身: 申请审核 repo + 事务管理器 ----

type fakeApplies struct {
	repository.RegisterApplyRepository
	rows   []*entity.RegisterApply
	nextID int64
}

func (f *fakeApplies) Create(_ context.Context, a *entity.RegisterApply) error {
	f.nextID++
	a.Id = f.nextID
	f.rows = append(f.rows, a)
	return nil
}

func (f *fakeApplies) GetById(_ context.Context, id int64) (*entity.RegisterApply, error) {
	for _, a := range f.rows {
		if a.Id == id {
			return a, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeApplies) GetByEmail(_ context.Context, email string) (*entity.RegisterApply, error) {
	for i := len(f.rows) - 1; i >= 0; i-- {
		if f.rows[i].Email == email {
			return f.rows[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeApplies) LatestByAccount(_ context.Context, account string) (*entity.RegisterApply, error) {
	for i := len(f.rows) - 1; i >= 0; i-- {
		if f.rows[i].UserName == account || f.rows[i].Email == account {
			return f.rows[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeApplies) List(_ context.Context, status int8, _ repository.Page) ([]entity.RegisterApply, int64, error) {
	var out []entity.RegisterApply
	for _, a := range f.rows {
		if status == entity.ApplyStatusAny || a.Status == status {
			out = append(out, *a)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeApplies) Approve(_ context.Context, id int64, reviewedBy int64) error {
	a, err := f.GetById(context.Background(), id)
	if err != nil {
		return err
	}
	if a.Status != entity.ApplyStatusPending {
		return domain.ErrConflict // 幂等闸: 重复审批/已驳回
	}
	a.Status = entity.ApplyStatusApproved
	a.RejectReason = ""
	a.ReviewedBy = reviewedBy
	a.ReviewedAt = time.Now()
	return nil
}

func (f *fakeApplies) Reject(_ context.Context, id int64, reviewedBy int64, reason string) error {
	a, err := f.GetById(context.Background(), id)
	if err != nil {
		return err
	}
	if a.Status != entity.ApplyStatusPending {
		return domain.ErrConflict
	}
	a.Status = entity.ApplyStatusRejected
	a.RejectReason = reason
	a.ReviewedBy = reviewedBy
	a.ReviewedAt = time.Now()
	return nil
}

func (f *fakeApplies) MarkActivated(_ context.Context, id int64, at time.Time) error {
	a, err := f.GetById(context.Background(), id)
	if err != nil {
		return err
	}
	a.ActivatedAt = at
	return nil
}

// fakeTx 无真实事务(替身 repo 本就同步内存), 仅验证 service 把两笔写放进同一 WithinTx。
type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

var codeRe = regexp.MustCompile(`验证码: (\d{6})`)

// lastCode 取最后一封验证码邮件里的 6 位码(端到端串联 SendCode → 校验)。
func (f *fakeSender) lastCode(t *testing.T) string {
	t.Helper()
	if len(f.sent) == 0 {
		t.Fatal("没有已发送邮件")
	}
	m := codeRe.FindStringSubmatch(f.sent[len(f.sent)-1].Body)
	if m == nil {
		t.Fatalf("邮件正文找不到验证码: %s", f.sent[len(f.sent)-1].Body)
	}
	return m[1]
}

func mailReadySite() *entity.SiteConfig {
	return &entity.SiteConfig{
		Id: 1, RegisterMode: RegisterModeOpen, SmtpEnabled: 1,
		SmtpHost: "smtp.qq.com", SmtpPort: 465, SmtpSecure: "ssl",
		SmtpUser: "bot@qq.com", SmtpPass: "authcode", SmtpFromName: "Jerocine",
	}
}

// newRegisterSvc 装配: 真 UserService(IssueToken 路径) + 假 repo + 假 sender。
func newRegisterSvc(t *testing.T, site *entity.SiteConfig, quota int) (*RegisterService, *fakeRegUsers, *fakeSender) {
	t.Helper()
	setupCache(t)
	users := newFakeRegUsers()
	userSvc := NewUserService(users, nil, nil, nil, nil, testTokenManager(t), 5, time.Hour)
	svc := NewRegisterService(&fakeSiteCfg{cfg: site}, users, &fakeApplies{}, fakeTx{}, userSvc, quota, "https://site.example")
	fs := &fakeSender{}
	svc.newSender = func(mailer.Config) (mailer.Sender, error) { return fs, nil }
	return svc, users, fs
}

// newRegisterSvcWith 同 newRegisterSvc, 但把 applies/sender 交回调用方(P2 断言审核行用)。
func newRegisterSvcWith(t *testing.T, site *entity.SiteConfig, quota int) (*RegisterService, *fakeRegUsers, *fakeApplies, *fakeSender) {
	t.Helper()
	setupCache(t)
	users := newFakeRegUsers()
	userSvc := NewUserService(users, nil, nil, nil, nil, testTokenManager(t), 5, time.Hour)
	applies := &fakeApplies{}
	svc := NewRegisterService(&fakeSiteCfg{cfg: site}, users, applies, fakeTx{}, userSvc, quota, "https://site.example")
	fs := &fakeSender{}
	svc.newSender = func(mailer.Config) (mailer.Sender, error) { return fs, nil }
	return svc, users, applies, fs
}

func clearMinuteWindow(email string) {
	cache.Del(context.Background(), cache.KeyRegSendMin("code", email))
}

// ---- RegisterConfig ----

func TestRegisterConfigModes(t *testing.T) {
	cases := []struct {
		mode      string
		enabled   int
		wantMode  string
		wantReady bool
	}{
		{"off", 1, RegisterModeOff, true},
		{"open", 1, RegisterModeOpen, true},
		{"apply", 1, RegisterModeApply, true},
		{"open", 0, RegisterModeOpen, false},  // 开关关 → smtpReady=false
		{"open", 1, RegisterModeOpen, false},  // 占位, 下面覆盖 site
		{"garbage", 1, RegisterModeOff, true}, // 非法值保守关
		{"", 1, RegisterModeOff, true},        // 空保守关
		{"OPEN", 1, RegisterModeOff, true},    // 大小写不宽容(存储侧约定小写)
	}
	for i, c := range cases {
		site := mailReadySite()
		site.RegisterMode = c.mode
		site.SmtpEnabled = c.enabled
		if i == 4 { // 第二组 open+enabled 但配置不完整 → 不可用
			site.SmtpPass = ""
		}
		svc, _, _ := newRegisterSvc(t, site, 50)
		v, err := svc.RegisterConfig(context.Background())
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if v.Mode != c.wantMode || v.SmtpReady != c.wantReady {
			t.Errorf("case %d (%+v): got mode=%s ready=%v", i, c, v.Mode, v.SmtpReady)
		}
	}
}

// ---- SendCode ----

func TestSendCodeGates(t *testing.T) {
	ctx := context.Background()
	t.Run("purpose 非法", func(t *testing.T) {
		svc, _, _ := newRegisterSvc(t, mailReadySite(), 50)
		if err := svc.SendCode(ctx, "nope", "a@qq.com"); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("邮箱格式错", func(t *testing.T) {
		svc, _, _ := newRegisterSvc(t, mailReadySite(), 50)
		if err := svc.SendCode(ctx, PurposeRegister, "not-an-email"); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("off 模式拒绝注册码但不拒 reset(P3 找回)", func(t *testing.T) {
		site := mailReadySite()
		site.RegisterMode = RegisterModeOff
		svc, _, _ := newRegisterSvc(t, site, 50)
		if err := svc.SendCode(ctx, PurposeRegister, "a@qq.com"); !errors.Is(err, domain.ErrRegisterDisabled) {
			t.Fatalf("register purpose 应被 off 拒: %v", err)
		}
		if err := svc.SendCode(ctx, PurposeReset, "a@qq.com"); err != nil {
			t.Fatalf("reset purpose 不受 off 约束: %v", err)
		}
	})
	t.Run("邮件通道未配置", func(t *testing.T) {
		for _, mut := range []func(*entity.SiteConfig){
			func(s *entity.SiteConfig) { s.SmtpEnabled = 0 },
			func(s *entity.SiteConfig) { s.SmtpPass = "" },
			func(s *entity.SiteConfig) { s.SmtpHost = "" },
			func(s *entity.SiteConfig) { s.SmtpUser = "" },
		} {
			site := mailReadySite()
			mut(site)
			svc, _, _ := newRegisterSvc(t, site, 50)
			if err := svc.SendCode(ctx, PurposeRegister, "a@qq.com"); !errors.Is(err, domain.ErrMailNotConfigured) {
				t.Fatalf("应报未配置: %v", err)
			}
		}
	})
}

func TestSendCodeSuccessAndConsumable(t *testing.T) {
	ctx := context.Background()
	svc, _, fs := newRegisterSvc(t, mailReadySite(), 50)
	const email = "a@qq.com"
	if err := svc.SendCode(ctx, PurposeRegister, email); err != nil {
		t.Fatalf("发码: %v", err)
	}
	if len(fs.sent) != 1 {
		t.Fatalf("应发 1 封, got %d", len(fs.sent))
	}
	if fs.sent[0].To != email || !strings.Contains(fs.sent[0].Subject, "注册") {
		t.Fatalf("邮件要素不对: %+v", fs.sent[0])
	}
	code := fs.lastCode(t)
	if !cache.VerifyRegCode(ctx, PurposeRegister, email, code) {
		t.Fatal("发出的码应可在 Redis 校验(不明文)")
	}
	// 邮箱大小写归一
	if err := svc.SendCode(ctx, PurposeRegister, strings.ToUpper(email)); err == nil {
		t.Fatal("1 分钟窗内应拒")
	}
}

func TestSendCodeRateLimits(t *testing.T) {
	ctx := context.Background()
	const email = "rl@qq.com"

	t.Run("1 分钟窗", func(t *testing.T) {
		svc, _, _ := newRegisterSvc(t, mailReadySite(), 50)
		if err := svc.SendCode(ctx, PurposeRegister, email); err != nil {
			t.Fatal(err)
		}
		if err := svc.SendCode(ctx, PurposeRegister, email); !errors.Is(err, domain.ErrCodeSendTooFast) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("邮箱日配额 5 封", func(t *testing.T) {
		svc, _, _ := newRegisterSvc(t, mailReadySite(), 50)
		for i := 0; i < 5; i++ {
			clearMinuteWindow(email)
			if err := svc.SendCode(ctx, PurposeRegister, email); err != nil {
				t.Fatalf("第 %d 封: %v", i+1, err)
			}
		}
		clearMinuteWindow(email)
		if err := svc.SendCode(ctx, PurposeRegister, email); !errors.Is(err, domain.ErrQuotaExceeded) {
			t.Fatalf("第 6 封应超邮箱日配额: %v", err)
		}
	})
	t.Run("全局日配额", func(t *testing.T) {
		svc, _, _ := newRegisterSvc(t, mailReadySite(), 2) // 全局 2 封(跨邮箱共享)
		for i := 0; i < 2; i++ {
			addr := fmt.Sprintf("g%d@qq.com", i)
			if err := svc.SendCode(ctx, PurposeRegister, addr); err != nil {
				t.Fatalf("第 %d 封: %v", i+1, err)
			}
		}
		if err := svc.SendCode(ctx, PurposeRegister, "g2@qq.com"); !errors.Is(err, domain.ErrQuotaExceeded) {
			t.Fatalf("第 3 个邮箱应撞全局配额: %v", err)
		}
	})
	t.Run("发送失败原文上抛", func(t *testing.T) {
		svc, _, fs := newRegisterSvc(t, mailReadySite(), 50)
		fs.err = errors.New("535 auth failed")
		err := svc.SendCode(ctx, PurposeRegister, "err@qq.com")
		if err == nil || !strings.Contains(err.Error(), "535 auth failed") {
			t.Fatalf("错误原文应上抛: %v", err)
		}
	})
}

// ---- Register(open 模式) ----

func TestRegisterGates(t *testing.T) {
	ctx := context.Background()
	base := func() *entity.SiteConfig { return mailReadySite() }

	t.Run("off 模式", func(t *testing.T) {
		site := base()
		site.RegisterMode = RegisterModeOff
		svc, _, _ := newRegisterSvc(t, site, 50)
		if _, err := svc.Register(ctx, "u1", "secret1", "u1@qq.com", "123456"); !errors.Is(err, domain.ErrRegisterDisabled) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("apply 模式不走本端点", func(t *testing.T) {
		site := base()
		site.RegisterMode = RegisterModeApply
		svc, _, _ := newRegisterSvc(t, site, 50)
		if _, err := svc.Register(ctx, "u1", "secret1", "u1@qq.com", "123456"); !errors.Is(err, domain.ErrRegisterDisabled) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("参数校验", func(t *testing.T) {
		svc, _, _ := newRegisterSvc(t, base(), 50)
		bad := [][4]string{
			{"", "secret1", "u@qq.com", "123456"},                      // 空用户名
			{"u1", "secret1", "bad-email", "123456"},                   // 邮箱格式
			{"u1", "12345", "u@qq.com", "123456"},                      // 密码过短
			{"u1", "secret1", "u@qq.com", ""},                          // 缺验证码
			{strings.Repeat("x", 33), "secret1", "u@qq.com", "123456"}, // 用户名过长
		}
		for i, c := range bad {
			if _, err := svc.Register(ctx, c[0], c[1], c[2], c[3]); !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("case %d: got %v", i, err)
			}
		}
	})
	t.Run("查重先于验证码(不消耗尝试次数)", func(t *testing.T) {
		site := base()
		svc, users, _ := newRegisterSvc(t, site, 50)
		users.byName["taken"] = &entity.User{UserName: "taken"}
		if _, err := svc.Register(ctx, "taken", "secret1", "new@qq.com", "000000"); !errors.Is(err, domain.ErrUserNameTaken) {
			t.Fatalf("用户名占用: %v", err)
		}
		users.byEmail["taken@qq.com"] = &entity.User{Email: "taken@qq.com"}
		if _, err := svc.Register(ctx, "fresh", "secret1", "taken@qq.com", "000000"); !errors.Is(err, domain.ErrEmailTaken) {
			t.Fatalf("邮箱占用: %v", err)
		}
		// 错码 5 次才锁; 上面两次未消耗计数
		for i := 0; i < 4; i++ {
			if _, err := svc.Register(ctx, "fresh", "secret1", "fresh@qq.com", "000000"); !errors.Is(err, domain.ErrCodeInvalid) {
				t.Fatalf("第 %d 次错码应 ErrCodeInvalid: %v", i+1, err)
			}
		}
		if _, err := svc.Register(ctx, "fresh", "secret1", "fresh@qq.com", "000000"); !errors.Is(err, domain.ErrCodeTooManyTries) {
			t.Fatalf("第 5 次错码应锁: %v", err)
		}
	})
}

func TestRegisterHappyPath(t *testing.T) {
	ctx := context.Background()
	svc, users, fs := newRegisterSvc(t, mailReadySite(), 50)
	const email = "Happy@QQ.com"
	if err := svc.SendCode(ctx, PurposeRegister, email); err != nil {
		t.Fatalf("发码: %v", err)
	}
	code := fs.lastCode(t)
	res, err := svc.Register(ctx, "homer", "secret1", email, code)
	if err != nil {
		t.Fatalf("注册: %v", err)
	}
	if res.Token == "" || res.UserName != "homer" || res.Role != entity.RoleUser {
		t.Fatalf("注册结果异常(应直接登录): %+v", res)
	}
	u := users.byName["homer"]
	if u == nil || u.Email != "happy@qq.com" || u.EmailVerified != 1 {
		t.Fatalf("落库用户异常: %+v", u)
	}
	// 验证码一次性: 旧码换新邮箱再注册应失败(码已消费)
	clearMinuteWindow("new2@qq.com")
	if err := svc.SendCode(ctx, PurposeRegister, "new2@qq.com"); err != nil {
		t.Fatalf("二段发码: %v", err)
	}
	code2 := fs.lastCode(t)
	if _, err := svc.Register(ctx, "other", "secret1", "new2@qq.com", code2); err != nil {
		t.Fatalf("第二人注册: %v", err)
	}
	if _, err := svc.Register(ctx, "third", "secret1", "third@qq.com", code); !errors.Is(err, domain.ErrCodeInvalid) {
		t.Fatalf("旧码应已消费: %v", err)
	}
	if len(users.byName) != 2 {
		t.Fatalf("应只有 2 个用户, got %d", len(users.byName))
	}
}

// ---- 管理后台: 邮件配置 ----

func TestMailConfigMaskAndSave(t *testing.T) {
	ctx := context.Background()
	site := mailReadySite()
	site.SmtpPass = "supersecretcode"
	svc, _, _ := newRegisterSvc(t, site, 50)

	v, err := svc.GetMailConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !v.SmtpPassSet || v.SmtpPassMasked != "supe…code" {
		t.Fatalf("掩码/标记不对: %+v", v)
	}
	if v.RegisterMode != RegisterModeOpen || len(v.Presets) < 6 || v.SmtpPort != 465 {
		t.Fatalf("视图字段不对: %+v", v)
	}

	// 未配置时掩码为空
	site2 := &entity.SiteConfig{Id: 1}
	svc2, _, _ := newRegisterSvc(t, site2, 50)
	v2, _ := svc2.GetMailConfig(ctx)
	if v2.SmtpPassSet || v2.SmtpPassMasked != "" || v2.RegisterMode != RegisterModeOff {
		t.Fatalf("未配置视图不对: %+v", v2)
	}

	t.Run("保存: 启用但配置不完整应拒", func(t *testing.T) {
		fresh := &entity.SiteConfig{Id: 1} // 库里没配过 pass
		svcFresh, _, _ := newRegisterSvc(t, fresh, 50)
		err := svcFresh.SaveMailConfig(context.Background(), MailConfigInput{
			RegisterMode: "open", SmtpEnabled: 1, SmtpHost: "smtp.qq.com",
			SmtpPort: 465, SmtpSecure: "ssl", SmtpUser: "a@qq.com", // 缺 pass
		})
		if !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("保存: pass 空保留原值", func(t *testing.T) {
		err := svc.SaveMailConfig(ctx, MailConfigInput{
			RegisterMode: "apply", SmtpEnabled: 1, SmtpHost: "smtp.163.com",
			SmtpPort: 465, SmtpSecure: "ssl", SmtpUser: "a@163.com", SmtpFromName: "J",
		})
		if err != nil {
			t.Fatal(err)
		}
		if site.SmtpPass != "supersecretcode" {
			t.Fatalf("pass 空串不应清掉: %q", site.SmtpPass)
		}
		if site.RegisterMode != RegisterModeApply || site.SmtpHost != "smtp.163.com" {
			t.Fatalf("其余字段应更新: %+v", site)
		}
	})
	t.Run("保存: pass 非空则替换", func(t *testing.T) {
		err := svc.SaveMailConfig(ctx, MailConfigInput{
			RegisterMode: "open", SmtpEnabled: 1, SmtpHost: "smtp.qq.com",
			SmtpPort: 465, SmtpSecure: "ssl", SmtpUser: "a@qq.com", SmtpPass: "newcode",
		})
		if err != nil || site.SmtpPass != "newcode" {
			t.Fatalf("pass 应替换: err=%v site=%+v", err, site)
		}
	})
	t.Run("保存: 非法 secure/端口/mode 归一或拒绝", func(t *testing.T) {
		if err := svc.SaveMailConfig(ctx, MailConfigInput{SmtpEnabled: 0, SmtpPort: 465, SmtpSecure: "weird"}); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("非法 secure 应拒: %v", err)
		}
		if err := svc.SaveMailConfig(ctx, MailConfigInput{SmtpEnabled: 0, SmtpPort: 99999, SmtpSecure: "ssl"}); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("端口越界应拒: %v", err)
		}
		if err := svc.SaveMailConfig(ctx, MailConfigInput{SmtpEnabled: 2, SmtpPort: 465, SmtpSecure: "ssl"}); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("enabled 非 0/1 应拒: %v", err)
		}
		// mode 非法 → 保守 off; secure 空 → 默认 ssl
		if err := svc.SaveMailConfig(ctx, MailConfigInput{SmtpEnabled: 0, SmtpPort: 465, SmtpSecure: "", RegisterMode: "garbage"}); err != nil {
			t.Fatal(err)
		}
		if site.RegisterMode != RegisterModeOff || site.SmtpSecure != "ssl" {
			t.Fatalf("归一化不对: %+v", site)
		}
	})
}

func TestTestMailUsesFormValues(t *testing.T) {
	ctx := context.Background()
	site := mailReadySite()
	site.SmtpPass = "storedcode"
	svc, _, fs := newRegisterSvc(t, site, 50)

	t.Run("收件人非法", func(t *testing.T) {
		if err := svc.TestMail(ctx, "nope", MailConfigInput{}); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("pass 空用已存值, 其余用表单值", func(t *testing.T) {
		var gotCfg mailer.Config
		svc.newSender = func(c mailer.Config) (mailer.Sender, error) {
			gotCfg = c
			return fs, nil
		}
		err := svc.TestMail(ctx, "me@x.com", MailConfigInput{
			SmtpHost: "smtp.gmail.com", SmtpPort: 587, SmtpSecure: "starttls",
			SmtpUser: "form@gmail.com", SmtpFromName: "表单名", // pass 空 → storedcode
		})
		if err != nil {
			t.Fatal(err)
		}
		if gotCfg.Host != "smtp.gmail.com" || gotCfg.Port != 587 || gotCfg.Secure != "starttls" ||
			gotCfg.User != "form@gmail.com" || gotCfg.Pass != "storedcode" || gotCfg.FromName != "表单名" {
			t.Fatalf("应用表单值+已存 pass: %+v", gotCfg)
		}
		if len(fs.sent) != 1 || fs.sent[0].To != "me@x.com" {
			t.Fatalf("测试邮件未发: %+v", fs.sent)
		}
	})
	t.Run("pass 非空用表单值", func(t *testing.T) {
		var gotCfg mailer.Config
		svc.newSender = func(c mailer.Config) (mailer.Sender, error) {
			gotCfg = c
			return fs, nil
		}
		if err := svc.TestMail(ctx, "me@x.com", MailConfigInput{
			SmtpHost: "smtp.qq.com", SmtpPort: 465, SmtpSecure: "ssl",
			SmtpUser: "form@qq.com", SmtpPass: "formcode",
		}); err != nil {
			t.Fatal(err)
		}
		if gotCfg.Pass != "formcode" {
			t.Fatalf("表单 pass 应优先: %+v", gotCfg)
		}
	})
	t.Run("发送失败原文上抛", func(t *testing.T) {
		fs.err = errors.New("connection refused")
		svc.newSender = func(mailer.Config) (mailer.Sender, error) { return fs, nil }
		err := svc.TestMail(ctx, "me@x.com", MailConfigInput{SmtpHost: "h", SmtpPort: 465, SmtpSecure: "ssl", SmtpUser: "u", SmtpPass: "p"})
		if err == nil || !strings.Contains(err.Error(), "connection refused") {
			t.Fatalf("got %v", err)
		}
		fs.err = nil
	})
}

// TestTestMailCountsGlobalQuota 设计 §5: 全局日配额对**所有出站邮件**(码/激活/驳回/重置/测试)
// 统一计数 —— 测试邮件不能成为绕过配额的旁路, 且只有真发出去才计数(失败不计)。
func TestTestMailCountsGlobalQuota(t *testing.T) {
	ctx := context.Background()
	svc, _, fs := newRegisterSvc(t, mailReadySite(), 1) // 全局 1 封
	in := MailConfigInput{SmtpHost: "smtp.qq.com", SmtpPort: 465, SmtpSecure: "ssl", SmtpUser: "bot@qq.com", SmtpPass: "code"}
	day := cache.TodayDay(svc.now())

	t.Run("发送失败不占配额", func(t *testing.T) {
		fs.err = errors.New("535 auth failed")
		if err := svc.TestMail(ctx, "me@x.com", in); err == nil {
			t.Fatal("发送失败应上抛")
		}
		if n := cache.ReadMailQuota(ctx, day); n != 0 {
			t.Fatalf("没真出站不应计数: %d", n)
		}
		fs.err = nil
	})
	t.Run("发送成功计入并受配额约束", func(t *testing.T) {
		if err := svc.TestMail(ctx, "me@x.com", in); err != nil {
			t.Fatalf("首封应成功: %v", err)
		}
		if n := cache.ReadMailQuota(ctx, day); n != 1 {
			t.Fatalf("测试邮件应计入全局配额: %d", n)
		}
		if err := svc.TestMail(ctx, "me@x.com", in); !errors.Is(err, domain.ErrQuotaExceeded) {
			t.Fatalf("配额已满应拒: %v", err)
		}
	})
}

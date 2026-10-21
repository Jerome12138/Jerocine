package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/platform/reqctx"
)

// TestLoginSessionListAndRevoke 登录会话: 签发时记录 IP/UA → 后台可见 → 踢掉后该 token 立即失效。
func TestLoginSessionListAndRevoke(t *testing.T) {
	setupCache(t)
	svc := NewUserService(nil, nil, nil, nil, nil, testTokenManager(t), 5, time.Hour)
	ctx := reqctx.With(context.Background(), "9.9.9.9", "UA-X")
	u := &entity.User{UserName: "bob"}
	u.ID = 42

	res, err := svc.IssueToken(ctx, u)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	rows := svc.ListLoginSessions(ctx, "", 10)
	if len(rows) != 1 || rows[0].UserID != 42 || rows[0].UserName != "bob" ||
		rows[0].IP != "9.9.9.9" || rows[0].UA != "UA-X" {
		t.Fatalf("会话行不对: %+v", rows)
	}
	if _, err := svc.Authenticate(ctx, res.Token); err != nil {
		t.Fatalf("token 应有效: %v", err)
	}
	// keyword 过滤: 命中用户名/IP, 不命中别的
	if got := svc.ListLoginSessions(ctx, "bob", 10); len(got) != 1 {
		t.Fatalf("按用户名过滤失败: %+v", got)
	}
	if got := svc.ListLoginSessions(ctx, "9.9.9.9", 10); len(got) != 1 {
		t.Fatalf("按 IP 过滤失败: %+v", got)
	}
	if got := svc.ListLoginSessions(ctx, "nobody", 10); len(got) != 0 {
		t.Fatalf("不匹配应为空: %+v", got)
	}

	if err := svc.RevokeLoginSession(ctx, rows[0].ID); err != nil {
		t.Fatalf("踢人: %v", err)
	}
	if _, err := svc.Authenticate(ctx, res.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("被踢后 token 应失效, got %v", err)
	}
	if got := svc.ListLoginSessions(ctx, "", 10); len(got) != 0 {
		t.Fatalf("踢掉后列表应为空: %+v", got)
	}
	if err := svc.RevokeLoginSession(ctx, rows[0].ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("重复踢应 ErrNotFound, got %v", err)
	}
}

// TestLoginPolicyFromProvider 登录策略(有效期/设备数)每次签发时读提供者: TTL 生效 + 设备数超限踢最旧。
func TestLoginPolicyFromProvider(t *testing.T) {
	setupCache(t)
	svc := NewUserService(nil, nil, nil, nil, nil, testTokenManager(t), 5, time.Hour)
	svc.SetPolicyProvider(func(context.Context) (time.Duration, int) {
		return 2 * time.Hour, 1 // 2 小时 + 只允许 1 台设备
	})
	ctx := context.Background()
	u := &entity.User{UserName: "carol"}
	u.ID = 7

	first, err := svc.IssueToken(ctx, u)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	claims, err := svc.tokens.Parse(first.Token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d := time.Until(claims.ExpiresAt.Time); d < 110*time.Minute || d > 130*time.Minute {
		t.Fatalf("TTL 应约 2 小时, got %v", d)
	}

	// JWT 的 iat/exp 是秒级、RS256 PKCS1v15 又是确定性签名 ⇒ 同一秒内对同一用户连签两次
	// 会得到**完全相同**的 token(等价于复用，无害)。跨秒才能造出两个不同会话。
	time.Sleep(1100 * time.Millisecond)
	second, err := svc.IssueToken(ctx, u)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if _, err := svc.Authenticate(ctx, second.Token); err != nil {
		t.Fatalf("新 token 应有效: %v", err)
	}
	if _, err := svc.Authenticate(ctx, first.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("设备数为 1 时旧 token 应被顶掉, got %v", err)
	}
}

// TestRuntimeSettingsDefaultsAndSave 运行参数: 未配置回退默认, 保存校验范围, 落库后可读回。
func TestRuntimeSettingsDefaultsAndSave(t *testing.T) {
	setupCache(t)
	site := &entity.SiteConfig{Id: 1}
	m := &ManageService{siteCfg: &fakeSiteCfg{cfg: site}}
	ctx := context.Background()

	got, err := m.RuntimeSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.AuthTokenTTLHours != DefaultAuthTokenTTLHours ||
		got.MaxDeviceLogin != DefaultMaxDeviceLogin ||
		got.SpiderMaxGoroutine != DefaultSpiderMaxGoroutine {
		t.Fatalf("未配置应回退默认: %+v", got)
	}

	bad := []RuntimeSettings{
		{AuthTokenTTLHours: 0, MaxDeviceLogin: 5, SpiderMaxGoroutine: 32},
		{AuthTokenTTLHours: 9000, MaxDeviceLogin: 5, SpiderMaxGoroutine: 32},
		{AuthTokenTTLHours: 168, MaxDeviceLogin: 0, SpiderMaxGoroutine: 32},
		{AuthTokenTTLHours: 168, MaxDeviceLogin: 51, SpiderMaxGoroutine: 32},
		{AuthTokenTTLHours: 168, MaxDeviceLogin: 5, SpiderMaxGoroutine: 0},
		{AuthTokenTTLHours: 168, MaxDeviceLogin: 5, SpiderMaxGoroutine: 257},
	}
	for i, in := range bad {
		if err := m.SaveRuntimeSettings(ctx, in); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("case %d 应拒: %v", i, err)
		}
	}

	okIn := RuntimeSettings{AuthTokenTTLHours: 720, MaxDeviceLogin: 10, SpiderMaxGoroutine: 8}
	if err := m.SaveRuntimeSettings(ctx, okIn); err != nil {
		t.Fatalf("save: %v", err)
	}
	if site.AuthTokenTTLHours != 720 || site.MaxDeviceLogin != 10 || site.SpiderMaxGoroutine != 8 {
		t.Fatalf("未落库: %+v", site)
	}
	if got, _ := m.RuntimeSettings(ctx); got != okIn {
		t.Fatalf("读回不一致: %+v", got)
	}
}

// TestSaveSitePreservesIndependentColumns 保存「站点信息」只写表单字段, 其余 json:"-" 列
// (TMDB 凭据 / SMTP / 邮件日配额 / 登录策略 / 采集并发)必须从库里回填 —— 落库是 UpdateAll,
// 漏一列就会被清零(邮件配额的坑: 清零后静默回落到默认 50)。挂掉说明回填清单漏了。
func TestSaveSitePreservesIndependentColumns(t *testing.T) {
	setupCache(t)
	site := &entity.SiteConfig{
		Id: 1, SiteName: "old", Domain: "https://x.example",
		TmdbAPIKey: "tmdb-key", RegisterMode: RegisterModeApply,
		SmtpEnabled: 1, SmtpHost: "smtp.qq.com", SmtpPort: 465, SmtpSecure: "ssl",
		SmtpUser: "bot@qq.com", SmtpPass: "code", SmtpFromName: "J", MailDailyQuota: 7,
		AuthTokenTTLHours: 720, MaxDeviceLogin: 10, SpiderMaxGoroutine: 8,
	}
	m := &ManageService{siteCfg: &fakeSiteCfg{cfg: site}}

	in := &entity.SiteConfig{SiteName: "new", Domain: "https://x.example", Logo: "l"}
	if err := m.SaveSite(context.Background(), in); err != nil {
		t.Fatalf("save: %v", err)
	}
	if site.SiteName != "new" || site.Logo != "l" {
		t.Fatalf("表单字段未落库: %+v", site)
	}
	if site.TmdbAPIKey != "tmdb-key" || site.SmtpPass != "code" || site.SmtpHost != "smtp.qq.com" ||
		site.RegisterMode != RegisterModeApply || site.MailDailyQuota != 7 {
		t.Fatalf("邮件/TMDB 列被清零: %+v", site)
	}
	if site.AuthTokenTTLHours != 720 || site.MaxDeviceLogin != 10 || site.SpiderMaxGoroutine != 8 {
		t.Fatalf("运行参数被清零: %+v", site)
	}
}

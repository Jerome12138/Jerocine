package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"server/internal/cache"
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
		got.SpiderMaxGoroutine != DefaultSpiderMaxGoroutine ||
		got.SpiderMasterMaxWorkers != DefaultSpiderMasterWorkers ||
		got.SpiderSlaveMaxWorkers != DefaultSpiderSlaveWorkers {
		t.Fatalf("未配置应回退默认: %+v", got)
	}

	good := RuntimeSettings{AuthTokenTTLHours: 168, MaxDeviceLogin: 5,
		SpiderMaxGoroutine: 32, SpiderMasterMaxWorkers: 12, SpiderSlaveMaxWorkers: 8}
	with := func(f func(*RuntimeSettings)) RuntimeSettings {
		c := good
		f(&c)
		return c
	}
	bad := []RuntimeSettings{
		with(func(c *RuntimeSettings) { c.AuthTokenTTLHours = 0 }),
		with(func(c *RuntimeSettings) { c.AuthTokenTTLHours = 9000 }),
		with(func(c *RuntimeSettings) { c.MaxDeviceLogin = 0 }),
		with(func(c *RuntimeSettings) { c.MaxDeviceLogin = 51 }),
		with(func(c *RuntimeSettings) { c.SpiderMaxGoroutine = 0 }),
		with(func(c *RuntimeSettings) { c.SpiderMaxGoroutine = 257 }),
		with(func(c *RuntimeSettings) { c.SpiderMasterMaxWorkers = 0 }),
		with(func(c *RuntimeSettings) { c.SpiderMasterMaxWorkers = 257 }),
		with(func(c *RuntimeSettings) { c.SpiderSlaveMaxWorkers = 0 }),
		with(func(c *RuntimeSettings) { c.SpiderSlaveMaxWorkers = 257 }),
	}
	for i, in := range bad {
		if err := m.SaveRuntimeSettings(ctx, in); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("case %d 应拒: %v", i, err)
		}
	}

	okIn := RuntimeSettings{AuthTokenTTLHours: 720, MaxDeviceLogin: 10,
		SpiderMaxGoroutine: 30, SpiderMasterMaxWorkers: 10, SpiderSlaveMaxWorkers: 6}
	if err := m.SaveRuntimeSettings(ctx, okIn); err != nil {
		t.Fatalf("save: %v", err)
	}
	if site.AuthTokenTTLHours != 720 || site.MaxDeviceLogin != 10 ||
		site.SpiderMaxGoroutine != 30 || site.SpiderMasterMaxWorkers != 10 || site.SpiderSlaveMaxWorkers != 6 {
		t.Fatalf("未落库: %+v", site)
	}
	if got, _ := m.RuntimeSettings(ctx); got != okIn {
		t.Fatalf("读回不一致: %+v", got)
	}
}

// TestSaveSitePreservesIndependentColumns 保存「站点信息」只写表单字段, 其余 json:"-" 列
// (TMDB 凭据 / SMTP / 邮件日配额 / 登录策略 / 采集并发 / 弹幕开关与白名单)必须从库里回填 ——
// 落库是 UpdateAll, 漏一列就会被清零(邮件配额的坑: 清零后静默回落到默认 50)。
// 挂掉说明回填清单漏了。
func TestSaveSitePreservesIndependentColumns(t *testing.T) {
	setupCache(t)
	site := &entity.SiteConfig{
		Id: 1, SiteName: "old", Domain: "https://x.example",
		TmdbAPIKey: "tmdb-key", RegisterMode: RegisterModeApply,
		SmtpEnabled: 1, SmtpHost: "smtp.qq.com", SmtpPort: 465, SmtpSecure: "ssl",
		SmtpUser: "bot@qq.com", SmtpPass: "code", SmtpFromName: "J", MailDailyQuota: 7,
		AuthTokenTTLHours: 720, MaxDeviceLogin: 10, SpiderMaxGoroutine: 8,
		SpiderMasterMaxWorkers: 9, SpiderSlaveMaxWorkers: 5,
		DanmuEnabled: 1, DanmuProviders: entity.StringSlice{"bilibili", "tencent", "iqiyi"},
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
	if site.AuthTokenTTLHours != 720 || site.MaxDeviceLogin != 10 || site.SpiderMaxGoroutine != 8 ||
		site.SpiderMasterMaxWorkers != 9 || site.SpiderSlaveMaxWorkers != 5 {
		t.Fatalf("运行参数被清零: %+v", site)
	}
	if site.DanmuEnabled != 1 {
		t.Fatalf("弹幕总开关被清零: %+v", site)
	}
	if strings.Join([]string(site.DanmuProviders), ",") != "bilibili,tencent,iqiyi" {
		t.Fatalf("弹幕平台白名单被清零: %+v", site)
	}
}

// TestSaveSiteRejectsEmptySiteName 站名必填: 绑定实体无 binding 规则, 服务层兜底防空站名落库
// (空格串也算空 —— 管理端手滑全空格提交不该静默写掉库内站名)。
func TestSaveSiteRejectsEmptySiteName(t *testing.T) {
	setupCache(t)
	site := &entity.SiteConfig{Id: 1, SiteName: "old", Domain: "https://x.example"}
	m := &ManageService{siteCfg: &fakeSiteCfg{cfg: site}}

	for _, name := range []string{"", "   "} {
		err := m.SaveSite(context.Background(), &entity.SiteConfig{SiteName: name})
		if !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("空站名 %q 应 ErrInvalidArgument, got %v", name, err)
		}
	}
	if site.SiteName != "old" {
		t.Fatalf("被拒的保存不该动库内值: %q", site.SiteName)
	}
	if err := m.SaveSite(context.Background(), &entity.SiteConfig{SiteName: "新站名"}); err != nil {
		t.Fatalf("非空站名应放行: %v", err)
	}
	if site.SiteName != "新站名" {
		t.Fatalf("正常保存未落库: %q", site.SiteName)
	}
}

// TestLoginSessionSearchWidensWindow 带 keyword 时取数窗口放宽到缓存上限: 搜索必须能命中
// "最近 limit 条"之外的会话(否则管理员面对 >limit 会话量时搜不到更早的行还不自知)。
// 用可控的 token(会话 id = sha256(token) 前 16 字节)保证等分下字典序确定性:
// 201 个 "ff.." filler 排在 "00.." needle 前面 ⇒ 无 keyword 取 200 条时 needle 必然不在窗口内。
func TestLoginSessionSearchWidensWindow(t *testing.T) {
	setupCache(t)
	svc := NewUserService(nil, nil, nil, nil, nil, testTokenManager(t), 5, time.Hour)
	ctx := reqctx.With(context.Background(), "9.9.9.9", "UA-X")

	// 穷举 token 使会话 id 命中目标十六进制前缀(sha256 均匀, 平均 256 次内命中)
	tokenWithIDPrefix := func(tag, want string) string {
		t.Helper()
		for i := 0; i < 1<<20; i++ {
			tok := fmt.Sprintf("%s-%d", tag, i)
			if strings.HasPrefix(cache.SessionID(tok), want) {
				return tok
			}
		}
		t.Fatalf("找不到前缀 %s 的 token", want)
		return ""
	}

	// needle 最先写入(最老/分最低), 201 个 filler 在后
	needle := tokenWithIDPrefix("needle", "00")
	cache.SaveSession(ctx, needle, 1, "needle-老会话", "8.8.8.8", "UA-OLD", time.Hour)
	for i := 0; i < 201; i++ {
		tok := tokenWithIDPrefix(fmt.Sprintf("bulk-%d", i), "ff")
		cache.SaveSession(ctx, tok, int64(10+i), fmt.Sprintf("bulk-%d", i), "1.1.1.1", "UA", time.Hour)
	}

	// 无 keyword: 行为不变 —— 仍按 limit 截断(201 filler + needle 共 202 行, 只回 200)
	if got := svc.ListLoginSessions(ctx, "", 200); len(got) != 200 {
		t.Fatalf("无 keyword 应保持 limit 截断: got %d", len(got))
	}
	// 带 keyword: 窗口放宽, 最老的 needle 也能搜到(修复前它落在最近 200 条之外, 搜不到)
	got := svc.ListLoginSessions(ctx, "needle", 200)
	if len(got) != 1 || got[0].UserName != "needle-老会话" {
		t.Fatalf("keyword 搜索应命中窗口外会话: %+v", got)
	}
	if got := svc.ListLoginSessions(ctx, "8.8.8.8", 200); len(got) != 1 {
		t.Fatalf("按 IP 搜索窗口外会话失败: %+v", got)
	}
}

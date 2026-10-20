package cache

import (
	"context"
	"testing"
	"time"
)

// 注册验证码协调态助手(见 regcode.go; 设计 docs/design/账号注册功能设计.md §2.2/§5)。

func TestRegCodeHashNotPlaintext(t *testing.T) {
	h := HashRegCode("123456")
	if h == "123456" {
		t.Fatal("不能明文存储")
	}
	if h != HashRegCode("123456") || h == HashRegCode("654321") {
		t.Fatal("哈希应稳定且可区分")
	}
}

func TestRegCodeSaveVerifyConsume(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	const email = "a@qq.com"
	if VerifyRegCode(ctx, "register", email, "123456") {
		t.Fatal("未发的码不应校验过")
	}
	SaveRegCode(ctx, "register", email, "123456", 5*time.Minute)
	if !VerifyRegCode(ctx, "register", email, "123456") {
		t.Fatal("发的码应校验过")
	}
	if VerifyRegCode(ctx, "register", email, "000000") {
		t.Fatal("错码不应过")
	}
	if VerifyRegCode(ctx, "reset", email, "123456") {
		t.Fatal("purpose 不应串用")
	}
	DelRegCode(ctx, "register", email)
	if VerifyRegCode(ctx, "register", email, "123456") {
		t.Fatal("消费后不应再校验过(一次性)")
	}
}

func TestRegCodeExpiry(t *testing.T) {
	mr := newTestCache(t)
	ctx := context.Background()
	SaveRegCode(ctx, "register", "b@qq.com", "123456", 10*time.Minute)
	if !VerifyRegCode(ctx, "register", "b@qq.com", "123456") {
		t.Fatal("刚发的码应过")
	}
	mr.FastForward(11 * time.Minute) // miniredis 快进时钟, 让键真正过期
	if VerifyRegCode(ctx, "register", "b@qq.com", "123456") {
		t.Fatal("过期码不应过")
	}
}

func TestRegTryLock(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	const email = "c@qq.com"
	for i := 1; i <= 4; i++ {
		if n := AddRegTry(ctx, email); n != i {
			t.Fatalf("第 %d 次失败计数应=%d, got %d", i, i, n)
		}
	}
	if n := AddRegTry(ctx, email); n != 5 {
		t.Fatalf("第 5 次应到锁阈值, got %d", n)
	}
	DelRegTry(ctx, email)
	if n := AddRegTry(ctx, email); n != 1 {
		t.Fatalf("清空后应重新从 1 计, got %d", n)
	}
}

func TestSendWindows(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	const email = "d@qq.com"
	if !AllowSendMinute(ctx, "code", email) {
		t.Fatal("首封应放行")
	}
	if AllowSendMinute(ctx, "code", email) {
		t.Fatal("1 分钟窗口内第二封应拒")
	}
	if !AllowSendMinute(ctx, "code", "other@qq.com") {
		t.Fatal("不同邮箱互不影响")
	}
	// 不同邮件类别互不占窗: 申请验证码之后, 激活邮件仍可立即发
	if !AllowSendMinute(ctx, "activate", email) {
		t.Fatal("激活邮件不应被验证码窗口挡住")
	}
	// 邮箱日配额: 按类别分别计数
	if n := CountSendDay(ctx, "code", email); n != 1 {
		t.Fatalf("日计数应=1, got %d", n)
	}
	if n := CountSendDay(ctx, "code", email); n != 2 {
		t.Fatalf("日计数应=2, got %d", n)
	}
	if n := CountSendDay(ctx, "activate", email); n != 1 {
		t.Fatalf("激活类日计数应独立从 1 起, got %d", n)
	}
}

func TestMailQuotaDay(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	day := TodayDay(time.Date(2026, 10, 20, 23, 59, 59, 0, time.Local))
	key := KeyMailQuota(day)
	if CoordClient().Exists(ctx, key).Val() != 0 {
		t.Fatal("初始不应存在")
	}
	for want := 1; want <= 3; want++ {
		if n := CountMailQuota(ctx, day); n != want {
			t.Fatalf("配额计数应=%d, got %d", want, n)
		}
	}
	ttl := CoordClient().TTL(ctx, key).Val()
	if ttl <= 0 || ttl > 24*time.Hour {
		t.Fatalf("首封起应有 24h TTL, got %v", ttl)
	}
	// 同日键复用, 跨日键隔离
	if CountMailQuota(ctx, "20991231") != 1 {
		t.Fatal("跨日键应独立")
	}
}

package cache

import (
	"context"
	"testing"
	"time"
)

// 找回密码重置 token 协调态助手(见 pwdreset.go; 设计 docs/design/账号注册功能设计.md §5/§10.2)。

func TestPwdResetSavePeekConsume(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	const tok = "tok-abc"
	if _, ok := PeekPwdResetToken(ctx, tok); ok {
		t.Fatal("未存的 token 不应 peek 到")
	}
	SavePwdResetToken(ctx, tok, 42, PwdResetTTL())
	id, ok := PeekPwdResetToken(ctx, tok)
	if !ok || id != 42 {
		t.Fatalf("存的 token 应 peek 到 userID=42, got (%d,%v)", id, ok)
	}
	if _, ok := PeekPwdResetToken(ctx, tok); !ok {
		t.Fatal("peek 不应消费(重试不烧链接)")
	}
	DelPwdResetToken(ctx, tok)
	if _, ok := PeekPwdResetToken(ctx, tok); ok {
		t.Fatal("消费后不应再 peek 到(单次)")
	}
}

func TestPwdResetRotate(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	SavePwdResetToken(ctx, "old", 42, PwdResetTTL())
	SavePwdResetToken(ctx, "other", 43, PwdResetTTL())
	if got := RotatePwdResetToken(ctx, 42); got != "old" {
		t.Fatalf("rotate 应返回旧 token, got %q", got)
	}
	if _, ok := PeekPwdResetToken(ctx, "old"); ok {
		t.Fatal("rotate 后旧 token 应立即失效")
	}
	if _, ok := PeekPwdResetToken(ctx, "other"); !ok {
		t.Fatal("rotate 不应波及其他用户")
	}
	// 重新申请: 同用户发新链接照常可用
	SavePwdResetToken(ctx, "new", 42, PwdResetTTL())
	if id, ok := PeekPwdResetToken(ctx, "new"); !ok || id != 42 {
		t.Fatalf("新 token 应可用, got (%d,%v)", id, ok)
	}
}

func TestPwdResetTTLIs30Min(t *testing.T) {
	if PwdResetTTL() != 30*time.Minute {
		t.Fatalf("设计 §5: 重置链接 30 分钟, got %v", PwdResetTTL())
	}
}

func TestPwdResetExpiry(t *testing.T) {
	mr := newTestCache(t)
	ctx := context.Background()
	SavePwdResetToken(ctx, "tok-exp", 42, PwdResetTTL())
	mr.FastForward(31 * time.Minute)
	if _, ok := PeekPwdResetToken(ctx, "tok-exp"); ok {
		t.Fatal("过期 token 不应 peek 到")
	}
}

package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 闸门时长归一化：<=0 → 默认；超上限 → 截断（防"永久停摆"）。
func TestGateTTL(t *testing.T) {
	cases := []struct {
		in   int
		want time.Duration
	}{
		{0, 60 * time.Minute},
		{-5, 60 * time.Minute},
		{30, 30 * time.Minute},
		{360, 360 * time.Minute},
		{9999, 360 * time.Minute}, // 截断
	}
	for _, c := range cases {
		if got := GateTTL(c.in); got != c.want {
			t.Fatalf("GateTTL(%d) = %s, want %s", c.in, got, c.want)
		}
	}
}

// 未接入协调 Redis：开闸门必须报错（不能假成功），查闸门不能炸。
func TestTaskGate_NoCoord(t *testing.T) {
	Init(nil, nil)
	ctx := context.Background()
	if err := TaskGateOpen(ctx, "deploy", time.Minute); !errors.Is(err, ErrCoordUnavailable) {
		t.Fatalf("coordRdb==nil 应返回 ErrCoordUnavailable, got %v", err)
	}
	if _, _, ok, err := TaskGateInfo(ctx); ok || !errors.Is(err, ErrCoordUnavailable) {
		t.Fatalf("未接入时应 ok=false + ErrCoordUnavailable, got ok=%v err=%v", ok, err)
	}
	if r := TaskGateBlockReason(ctx); r != "" {
		t.Fatalf("未接入时不应报「维护中」, got %q", r)
	}
}

// 开/读/关闸门：窗口开着时 BlockReason 给出可读原因，关闭后恢复空串。
func TestTaskGateOpenInfoClose(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()

	if _, _, ok, err := TaskGateInfo(ctx); ok || err != nil {
		t.Fatalf("初始应未打开, got ok=%v err=%v", ok, err)
	}
	if err := TaskGateOpen(ctx, "deploy-agent", 30*time.Minute); err != nil {
		t.Fatalf("开闸门失败: %v", err)
	}
	by, until, ok, err := TaskGateInfo(ctx)
	if err != nil || !ok || by != "deploy-agent" {
		t.Fatalf("闸门应开着且带打开者: by=%q ok=%v err=%v", by, ok, err)
	}
	if d := time.Until(until); d <= 0 || d > 30*time.Minute {
		t.Fatalf("剩余时间应在 (0, 30m], got %s", d)
	}
	if r := TaskGateBlockReason(ctx); r == "" {
		t.Fatal("窗口开着时拦新触发的原因不能为空（否则调用方无法判断）")
	}

	if err := TaskGateClose(ctx); err != nil {
		t.Fatalf("关闸门失败: %v", err)
	}
	if _, _, ok, _ := TaskGateInfo(ctx); ok {
		t.Fatal("关闭后应未打开")
	}
	if r := TaskGateBlockReason(ctx); r != "" {
		t.Fatalf("关闭后不应再拦, got %q", r)
	}
}

// 空 by 兜底 + 白名单增删（resume 只会恢复白名单里的 job）。
func TestTaskGateJobsWhitelist(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()

	if err := TaskGateOpen(ctx, "", time.Minute); err != nil {
		t.Fatalf("开闸门失败: %v", err)
	}
	if by, _, _, _ := TaskGateInfo(ctx); by != "unknown" {
		t.Fatalf("空 by 应兜底为 unknown, got %q", by)
	}

	if got := TaskGateJobs(ctx); len(got) != 0 {
		t.Fatalf("初始白名单应为空, got %v", got)
	}
	TaskGateMarkJob(ctx, "src_b")
	TaskGateMarkJob(ctx, "src_a")
	got := TaskGateJobs(ctx)
	if len(got) != 2 || got[0] != "src_a" || got[1] != "src_b" {
		t.Fatalf("白名单应按字典序返回, got %v", got)
	}
	TaskGateJobsClear(ctx)
	if len(TaskGateJobs(ctx)) != 0 {
		t.Fatal("清空后白名单应为空")
	}
}

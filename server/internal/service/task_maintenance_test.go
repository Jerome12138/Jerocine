package service

// 回归守卫：启动路径上的方法必须对 **nil 接收者**安全。
//
// 事故（2026-11-26 发版）：`App` 字面量漏填 `taskCenter` ⇒ `main` 里
// `app.taskCenter.NormalizeCronSchedules(rootCtx)` 拿到 nil 接收者 ⇒ 访问 `s.crons` 直接空指针 ⇒
// 新容器反复重启、线上约 5 分钟不可用。
//
// 为什么单测容易漏：测试总是自己构造好服务，永远不会传 nil。这条用例专门"扎一针"。
// 以后凡是 main 启动路径上会调的 TaskCenterService 方法，都应加进来。

import (
	"context"
	"testing"

	"server/internal/domain/entity"
	"server/internal/taskcenter"
)

func TestTaskCenterNilReceiverSafe(t *testing.T) {
	var svc *TaskCenterService // 故意 nil

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil 接收者不应 panic —— 启动路径会带病运行并反复重启: %v", r)
		}
	}()

	// main 启动路径
	svc.NormalizeCronSchedules(context.Background())
	svc.attachScheduler(nil)
	// 调度器三接口
	_ = svc.Definitions()
	_ = svc.Settings(context.Background())
	if crons, err := svc.Crons(context.Background()); err != nil || crons != nil {
		t.Fatalf("nil 服务应返回 (nil, nil), got (%v, %v)", crons, err)
	}
	// 策略兜底
	if svc.retention() != defaultRetentionDays {
		t.Fatal("nil 服务的保留天数应回落默认值")
	}
	if svc.schedulerTick() != defaultSchedulerTick {
		t.Fatal("nil 服务的 tick 应回落默认值")
	}
	if svc.ledger() != nil || svc.scheduler() != nil {
		t.Fatal("nil 服务的依赖访问器应返回 nil")
	}
	// 执行面：RunDefinition 走 safeRunTask（Def.Run 为 no-op），不能因为服务 nil 就炸
	if err := svc.RunDefinition(context.Background(), taskcenter.Def{
		Key: "builtin:probe", Run: func(context.Context) error { return nil },
	}); err != nil {
		t.Fatalf("nil 服务的 RunDefinition 应正常执行 Def.Run, got %v", err)
	}
	// 采集轮次要给出明确错误（而不是 panic）
	if err := svc.RunCronRound(context.Background(), entity.CronTask{Id: 1}); err == nil {
		t.Fatal("nil 服务跑采集轮次应返回「采集服务未装配」")
	}
}

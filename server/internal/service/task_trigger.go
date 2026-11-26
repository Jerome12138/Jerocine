package service

import (
	"context"

	"server/internal/cache"
)

// 触发闸门（维护窗口）—— **触发新任务的唯一判断点**。
//
// 背景：原先"要不要放过这次触发"散在 4 处各自写一遍
//   `if reason := cache.TaskGateBlockReason(ctx); reason != "" { return ... }`
// （内置手动执行 / 采集定时任务手动执行 / 单源手动采集 / 调度器 tick）。
// 每加一个触发入口就得记得再写一遍 —— 漏一个，维护窗口就"看起来开着、活儿照跑"，
// 而这种失效**没有任何报错**，只有"迁移期间库里还有采集在写"这种事后现象。
//
// 现在收敛成：所有触发入口只调 GuardTrigger；判断与文案仍单点在 cache.TaskGateBlockReason。
// 新增触发器时唯一要做的事就是开头 `if err := GuardTrigger(ctx); err != nil { return err }`，
// 并由 task_trigger_test.go 的 TestAllTriggerEntriesRespectGate 兜住（漏写即红）。

// MaintenanceWindowError 维护窗口内拒绝触发（可读文案由 cache 单点给出）。
// 用独立类型而不是裸 fmt.Errorf：调用方可以 errors.As 出来做差异化处理（比如定时链路只需要"跳过"）。
type MaintenanceWindowError struct{ Reason string }

func (e *MaintenanceWindowError) Error() string { return e.Reason }

// GuardTrigger 维护窗口开着时返回 *MaintenanceWindowError，否则 nil。
func GuardTrigger(ctx context.Context) error {
	if reason := cache.TaskGateBlockReason(ctx); reason != "" {
		return &MaintenanceWindowError{Reason: reason}
	}
	return nil
}

// gateBlockedNow 供调度器等"跳过即可、不必报错"的场景使用（语义与 GuardTrigger 同源）。
func gateBlockedNow(ctx context.Context) bool {
	return GuardTrigger(ctx) != nil
}

// describeGate 便于日志里带出拒绝原因（空串=没开窗口）。
func describeGate(ctx context.Context) string {
	if err := GuardTrigger(ctx); err != nil {
		return err.Error()
	}
	return ""
}

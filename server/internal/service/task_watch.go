package service

// 异步任务观察者（手动执行 / 调度触发共用一套规则）—— 2026-11-26 抽公共
//
// 之前两处各写一份"3 小时死循环"，问题有二：
//   1. 固定 3h 对长任务不够（豆瓣欠账补查实测 ≈31h），会被误杀成 failed；
//   2. 缺"卡死"识别：真挂住时也要白等到 3h。
// 现在按 Def 配置：WatchTimeout（上限，默认 3h）+ WatchStall（进度停滞上限，默认 30m）。

import (
	"context"
	"time"

	"server/internal/taskcenter"
)

// watchOutcome 观察结果。
type watchOutcome struct {
	State   taskcenter.State
	Message string
	Error   string
	Total   int
	Done    int
	Failed  int
	// Reason 超时/停滞时的固定原因（已含中文说明）
	Reason string
}

// waitDefTerminal 轮询 Def.Status 直到终态 / 停滞 / 超时。
//
// 返回 ok=false 表示"这个定义没有状态探针"（无需观察，调用方立即收尾即可）。
// poll 可注入（测试用毫秒级）；<=0 用 10s。
func waitDefTerminal(ctx context.Context, d taskcenter.Def, poll time.Duration) (out watchOutcome, ok bool) {
	if d.Status == nil {
		return watchOutcome{}, false
	}
	if poll <= 0 {
		poll = 10 * time.Second
	}
	timeout := d.WatchTimeoutOrDefault()
	stall := d.WatchStallOrDefault()
	deadline := time.Now().Add(timeout)
	var lastUpdate int64
	lastProgress := -1

	for {
		if ctx.Err() != nil {
			return watchOutcome{State: taskcenter.StateFailed, Reason: "观察被取消（服务停机）"}, true
		}
		if !time.Now().Before(deadline) {
			return watchOutcome{State: taskcenter.StateFailed,
				Reason: "[timeout] 超过 " + timeout.String() + " 仍未结束，已按超时收尾（后台可能仍在跑）"}, true
		}
		select {
		case <-ctx.Done():
			return watchOutcome{State: taskcenter.StateFailed, Reason: "观察被取消（服务停机）"}, true
		case <-time.After(poll):
		}

		st := d.Status(ctx)
		switch st.State {
		case taskcenter.StateSucceeded:
			return watchOutcome{State: taskcenter.StateSucceeded, Message: st.Message,
				Total: pn(st, 0), Done: pn(st, 1), Failed: pn(st, 2)}, true
		case taskcenter.StateFailed:
			return watchOutcome{State: taskcenter.StateFailed, Error: st.Error, Message: st.Message,
				Total: pn(st, 0), Done: pn(st, 1), Failed: pn(st, 2)}, true
		}

		// 停滞判定：有 UpdatedAt 就看 UpdatedAt；没有就看 done 数是否推进
		moved := false
		if st.UpdatedAt > 0 {
			if st.UpdatedAt != lastUpdate {
				lastUpdate = st.UpdatedAt
				moved = true
			}
		} else if st.Progress != nil {
			if st.Progress.Done != lastProgress {
				lastProgress = st.Progress.Done
				moved = true
			}
		} else {
			moved = true // 没有进度信息，无法判停滞
		}
		if !moved && lastUpdate > 0 && time.Since(time.UnixMilli(lastUpdate)) > stall {
			return watchOutcome{State: taskcenter.StateFailed,
				Error:  "[stalled] 进度超过 " + stall.String() + " 没有推进，已按失败收尾",
				Reason: "停滞"}, true
		}
	}
}

func pn(st taskcenter.Status, which int) int {
	if st.Progress == nil {
		return 0
	}
	switch which {
	case 0:
		return st.Progress.Total
	case 1:
		return st.Progress.Done
	default:
		return st.Progress.Failed
	}
}

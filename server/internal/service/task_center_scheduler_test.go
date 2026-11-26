package service

import (
	"testing"

	"server/internal/domain/entity"
	"server/internal/taskcenter"
)

// 生效配置 = task_setting 覆盖注册表默认值（启用位与周期各自独立覆盖）。
func TestSchedulerEffective(t *testing.T) {
	s := &TaskScheduler{}
	def := taskcenter.Def{Key: "builtin:hot_refresh", Enabled: true, DefaultSchedule: "every:10m"}

	t.Run("无配置用默认", func(t *testing.T) {
		enabled, schedule := s.effective(def, map[string]entity.TaskSetting{})
		if !enabled || schedule != "every:10m" {
			t.Fatalf("默认值不对: enabled=%v schedule=%q", enabled, schedule)
		}
	})

	t.Run("停用生效", func(t *testing.T) {
		enabled, schedule := s.effective(def, map[string]entity.TaskSetting{
			"builtin:hot_refresh": {TaskKey: "builtin:hot_refresh", Enabled: false},
		})
		if enabled {
			t.Fatal("应被配置停用")
		}
		if schedule != "every:10m" {
			t.Fatalf("空 schedule 应保留默认, got %q", schedule)
		}
	})

	t.Run("周期覆盖", func(t *testing.T) {
		_, schedule := s.effective(def, map[string]entity.TaskSetting{
			"builtin:hot_refresh": {TaskKey: "builtin:hot_refresh", Enabled: true, Schedule: "daily:05:00"},
		})
		if schedule != "daily:05:00" {
			t.Fatalf("周期未覆盖: %q", schedule)
		}
	})
}

// 默认不启用的任务（如欠账补查）在无配置时不应被调度。
func TestSchedulerEffective_DefaultDisabled(t *testing.T) {
	s := &TaskScheduler{}
	def := taskcenter.Def{Key: "builtin:douban_backfill", Enabled: false}
	enabled, _ := s.effective(def, map[string]entity.TaskSetting{})
	if enabled {
		t.Fatal("默认不启用的任务不应被调度")
	}
}

// 内置任务的状态来自台账：running/success/failed/canceled 映射 + 耗时计算。
func TestStatusFromRun(t *testing.T) {
	cases := []struct {
		status string
		want   taskcenter.State
	}{
		{entity.TaskRunRunning, taskcenter.StateRunning},
		{entity.TaskRunSuccess, taskcenter.StateSucceeded},
		{entity.TaskRunFailed, taskcenter.StateFailed},
		{entity.TaskRunCanceled, taskcenter.StateCanceled},
		{"", taskcenter.StateIdle},
	}
	for _, c := range cases {
		got := statusFromRun(entity.TaskRun{Status: c.status, StartedAt: 1000, EndedAt: 1500})
		if got.State != c.want {
			t.Fatalf("%q → %v, want %v", c.status, got.State, c.want)
		}
	}
	// 已结束：耗时 = 起止差
	if got := statusFromRun(entity.TaskRun{Status: entity.TaskRunSuccess, StartedAt: 1000, EndedAt: 1500}); got.ElapsedMs != 500 {
		t.Fatalf("耗时应为 500ms, got %d", got.ElapsedMs)
	}
	// 运行中：耗时 = now - startedAt（>0 即可）
	if got := statusFromRun(entity.TaskRun{Status: entity.TaskRunRunning, StartedAt: 1000}); got.ElapsedMs <= 0 {
		t.Fatalf("运行中应有耗时, got %d", got.ElapsedMs)
	}
	// 失败原因/摘要透传（已由 errclass 归一）
	got := statusFromRun(entity.TaskRun{Status: entity.TaskRunFailed, Error: "[net_timeout] 源站请求超时", Message: "1 个源失败"})
	if got.Error == "" || got.Message == "" {
		t.Fatalf("摘要/原因应透传: %+v", got)
	}
}

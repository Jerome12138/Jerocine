package service

import (
	"context"
	"testing"
	"time"

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

// 调度态快照要能回答"下次什么时候跑"：生效周期（用户覆盖过的）、NextAt、是否在跑、窗口开没开。
func TestSchedulerSnapshot(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()
	fs := &fakeSettings{sets: []entity.TaskSetting{
		{TaskKey: "builtin:hot_refresh", Enabled: true, Schedule: "daily:09:30"},
	}}
	reg := taskcenter.NewRegistry()
	reg.Register(taskcenter.Def{Key: "builtin:hot_refresh", Name: "热度", Kind: taskcenter.KindBuiltin,
		Enabled: true, DefaultSchedule: "daily:04:00", Run: func(context.Context) error { return nil }})
	crons := &fakeCrons{t: entity.CronTask{Id: 7, Schedule: "weekly:1 03:00", State: entity.StateEnabled}}
	svc := &TaskCenterService{reg: reg, settings: fs, crons: crons}
	s := NewTaskScheduler(svc)
	svc.attachScheduler(s)

	// 模拟"已经算过下次时间"
	s.next["builtin:hot_refresh"] = time.Now().Add(2 * time.Hour)
	s.next["cron:7"] = time.Now().Add(3 * time.Hour)

	items, gateOpen, tickMs := svc.ScheduleSnapshot(ctx)
	if gateOpen {
		t.Fatal("没开窗口不该报 gateOpen")
	}
	if tickMs != 30000 {
		t.Fatalf("tick 应为 30000ms, got %d", tickMs)
	}
	if len(items) != 2 {
		t.Fatalf("应有内置 + 采集两条, got %d: %+v", len(items), items)
	}
	// 按 key 排序：cron:7 < builtin:hot_refresh（字符串序）
	byKey := map[string]ScheduleItem{}
	for _, it := range items {
		byKey[it.Key] = it
	}
	hot := byKey["builtin:hot_refresh"]
	if hot.Schedule != "daily:09:30" {
		t.Fatalf("快照应给**生效**周期（用户覆盖后的），got %q", hot.Schedule)
	}
	if hot.ScheduleText == "" || hot.NextAt == 0 {
		t.Fatalf("应有可读周期与下次时间: %+v", hot)
	}
	cron := byKey["cron:7"]
	if !cron.Enabled || cron.Schedule != "weekly:1 03:00" || cron.NextAt == 0 {
		t.Fatalf("采集任务快照不对: %+v", cron)
	}
}

// 未启动调度器时快照要"空而不炸"（handler 会在任何时刻被调用）。
func TestScheduleSnapshot_NotStarted(t *testing.T) {
	svc := &TaskCenterService{}
	items, gateOpen, tickMs := svc.ScheduleSnapshot(context.Background())
	if len(items) != 0 || gateOpen || tickMs != 0 {
		t.Fatalf("未启动应返回空快照, got %+v %v %d", items, gateOpen, tickMs)
	}
}

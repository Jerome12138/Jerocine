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

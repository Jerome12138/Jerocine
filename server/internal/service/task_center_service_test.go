package service

import (
	"context"
	"testing"

	"server/internal/cache"
	"server/internal/domain/entity"
	"server/internal/taskcenter"
)

// 采集(cron) 任务的状态/进度映射：**total=0 时必须给 nil**，否则前端又会画出
// "已完成 0 页 / 共 ? 页 · 0%" 的假进度。
func TestViewOfCron_ProgressMapping(t *testing.T) {
	svc := &TaskCenterService{}
	names := map[string]string{"src_lz": "lz 量子资源"}
	allIDs := []string{"src_lz", "src_uk"}
	// 快照由调用方（List）取一次后传入 —— 这里给"空快照"基线，进度断言只走台账
	noSnaps := map[string]cache.JobProgress{}

	t.Run("运行中且有总数", func(t *testing.T) {
		v := svc.viewOfCron(context.Background(),
			entity.CronTask{Id: 3, Spec: "0 3 * * *", Model: entity.CronModelSomeSrc, SourceIds: []string{"src_lz"}, Time: 24, State: entity.StateEnabled},
			entity.TaskRun{Id: 11, Status: entity.TaskRunRunning, Total: 20, Done: 8, Failed: 2, StartedAt: 1000},
			names, allIDs, noSnaps)
		if v.Key != "cron:3" || v.State != string(taskcenter.StateRunning) {
			t.Fatalf("基础字段不对: %+v", v)
		}
		if v.Progress == nil || v.Progress.Total != 20 || v.Progress.Unit != "页" {
			t.Fatalf("进度不对: %+v", v.Progress)
		}
		if v.Target != "lz 量子资源" {
			t.Fatalf("目标应展示源名, got %q", v.Target)
		}
		if !v.Enabled {
			t.Fatal("启用态应为 true")
		}
	})

	t.Run("已完成但无分页数据", func(t *testing.T) {
		v := svc.viewOfCron(context.Background(),
			entity.CronTask{Id: 4, Spec: "0 4 * * *", Model: entity.CronModelAutoAll, State: entity.StateEnabled},
			entity.TaskRun{Id: 12, Status: entity.TaskRunSuccess, Total: 0, Done: 0, StartedAt: 100, EndedAt: 160},
			names, allIDs, noSnaps)
		if v.State != string(taskcenter.StateSucceeded) {
			t.Fatalf("状态应为 succeeded: %+v", v)
		}
		if v.Progress != nil {
			t.Fatalf("总数未知必须给 nil（否则出现假进度）: %+v", v.Progress)
		}
		if v.ElapsedMs != 60 {
			t.Fatalf("耗时应为 60ms, got %d", v.ElapsedMs)
		}
	})

	t.Run("没有运行记录的启用任务", func(t *testing.T) {
		v := svc.viewOfCron(context.Background(),
			entity.CronTask{Id: 5, Spec: "0 5 * * *", Model: entity.CronModelRecover, State: entity.StateDisabled},
			entity.TaskRun{}, names, allIDs, noSnaps)
		if v.State != string(taskcenter.StateIdle) {
			t.Fatalf("无记录应为 idle: %+v", v)
		}
		if v.Enabled {
			t.Fatal("停用任务应为 false")
		}
		if v.Name != "失败页补采" {
			t.Fatalf("补充任务名不对: %q", v.Name)
		}
	})
}

// 内置任务：task_setting 覆盖默认值；低于下限时按以下限执行。
func TestViewOfDef_SettingOverride(t *testing.T) {
	svc := &TaskCenterService{}
	def := taskcenter.Def{
		Key: "builtin:douban_score", Kind: taskcenter.KindBuiltin, Type: "douban_score",
		Name: "豆瓣评分同步", DefaultSchedule: "weekly:1 03:00", Enabled: true,
		DefaultIntervalMs: 4000, MinIntervalMs: 2000, IntervalLabel: "3~5s 随机",
		Editable: []string{"enabled", "schedule", "remark", "interval"},
		Caps:     []string{taskcenter.CapRun},
		Status: func(context.Context) taskcenter.Status {
			return taskcenter.Status{State: taskcenter.StateIdle, Message: "空闲"}
		},
	}

	t.Run("无配置用默认", func(t *testing.T) {
		v := svc.viewOfDef(context.Background(), def, entity.TaskSetting{})
		if !v.Enabled || v.Schedule != "weekly:1 03:00" || v.IntervalMs != 4000 {
			t.Fatalf("默认值不对: %+v", v)
		}
		if v.ScheduleText != "每周一 03:00" {
			t.Fatalf("中文展示不对: %q", v.ScheduleText)
		}
	})

	t.Run("配置覆盖", func(t *testing.T) {
		v := svc.viewOfDef(context.Background(), def, entity.TaskSetting{
			TaskKey: "builtin:douban_score", Enabled: false, Schedule: "daily:05:30", IntervalMs: 3000,
		})
		if v.Enabled || v.Schedule != "daily:05:30" || v.IntervalMs != 3000 {
			t.Fatalf("覆盖失败: %+v", v)
		}
	})

	t.Run("低于下限按下限", func(t *testing.T) {
		v := svc.viewOfDef(context.Background(), def, entity.TaskSetting{
			TaskKey: "builtin:douban_score", Enabled: true, IntervalMs: 100,
		})
		if v.IntervalMs != 2000 {
			t.Fatalf("应抬到下限 2000, got %d", v.IntervalMs)
		}
	})
}

// 运行中、台账还没页数（刚开始）→ 用 Redis 快照兜底；快照只取一次、按源聚合。
func TestViewOfCron_ProgressFromSnapshots(t *testing.T) {
	svc := &TaskCenterService{}
	names := map[string]string{"src_lz": "lz", "src_uk": "uk"}
	allIDs := []string{"src_lz", "src_uk"}
	snaps := map[string]cache.JobProgress{
		"src_lz": {SourceId: "src_lz", Total: 10, Done: 4, Failed: 1},
		"src_uk": {SourceId: "src_uk", Total: 5, Done: 5},
	}
	// AutoAll = 全部源
	v := svc.viewOfCron(context.Background(),
		entity.CronTask{Id: 6, Schedule: "daily:03:00", Model: entity.CronModelAutoAll, State: entity.StateEnabled},
		entity.TaskRun{Id: 13, Status: entity.TaskRunRunning, StartedAt: 1000}, names, allIDs, snaps)
	if v.Progress == nil || v.Progress.Total != 15 || v.Progress.Done != 9 || v.Progress.Failed != 1 {
		t.Fatalf("应按全部源聚合快照, got %+v", v.Progress)
	}
	// SomeSrc = 只算指定源
	v = svc.viewOfCron(context.Background(),
		entity.CronTask{Id: 7, Schedule: "daily:03:00", Model: entity.CronModelSomeSrc, SourceIds: []string{"src_uk"}, State: entity.StateEnabled},
		entity.TaskRun{Id: 14, Status: entity.TaskRunRunning, StartedAt: 1000}, names, allIDs, snaps)
	if v.Progress == nil || v.Progress.Total != 5 || v.Progress.Done != 5 {
		t.Fatalf("SomeSrc 只应聚合指定源, got %+v", v.Progress)
	}
}

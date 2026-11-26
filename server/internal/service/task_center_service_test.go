package service

import (
	"context"
	"testing"
	"time"

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
			names, allIDs, noSnaps, 0)
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
			names, allIDs, noSnaps, 0)
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
			entity.TaskRun{}, names, allIDs, noSnaps, 0)
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
		v := svc.viewOfDef(context.Background(), def, entity.TaskSetting{}, nil, nil)
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
		}, nil, nil)
		if v.Enabled || v.Schedule != "daily:05:30" || v.IntervalMs != 3000 {
			t.Fatalf("覆盖失败: %+v", v)
		}
	})

	t.Run("低于下限按下限", func(t *testing.T) {
		v := svc.viewOfDef(context.Background(), def, entity.TaskSetting{
			TaskKey: "builtin:douban_score", Enabled: true, IntervalMs: 100,
		}, nil, nil)
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
		entity.TaskRun{Id: 13, Status: entity.TaskRunRunning, StartedAt: 1000}, names, allIDs, snaps, 0)
	if v.Progress == nil || v.Progress.Total != 15 || v.Progress.Done != 9 || v.Progress.Failed != 1 {
		t.Fatalf("应按全部源聚合快照, got %+v", v.Progress)
	}
	// SomeSrc = 只算指定源
	v = svc.viewOfCron(context.Background(),
		entity.CronTask{Id: 7, Schedule: "daily:03:00", Model: entity.CronModelSomeSrc, SourceIds: []string{"src_uk"}, State: entity.StateEnabled},
		entity.TaskRun{Id: 14, Status: entity.TaskRunRunning, StartedAt: 1000}, names, allIDs, snaps, 0)
	if v.Progress == nil || v.Progress.Total != 5 || v.Progress.Done != 5 {
		t.Fatalf("SomeSrc 只应聚合指定源, got %+v", v.Progress)
	}
}

// idle 任务的静态周期提示不再当正文（与"计划"列重复）；备注必须能显示出来。
func TestViewOfDef_IdleMessage(t *testing.T) {
	svc := &TaskCenterService{}
	d := taskcenter.Def{
		Key: "builtin:probe", Type: "probe", Name: "探针", Kind: taskcenter.KindBuiltin,
		Enabled: true, DefaultSchedule: "every:20m",
		Status: func(context.Context) taskcenter.Status {
			return taskcenter.Status{State: taskcenter.StateIdle, Message: "每 20 分钟扫描一次"}
		},
	}
	// 无备注：行尾不该出现"每 20 分钟扫描一次"
	v := svc.viewOfDef(context.Background(), d, entity.TaskSetting{}, nil, nil)
	if v.Message != "" {
		t.Fatalf("idle 静态提示不应作为正文展示（与计划列重复），got %q", v.Message)
	}
	// 有备注：展示备注（此前被静态提示盖住）
	v = svc.viewOfDef(context.Background(), d, entity.TaskSetting{TaskKey: d.Key, Enabled: true, Remark: "人工备注"}, nil, nil)
	if v.Message != "人工备注" {
		t.Fatalf("idle 时应展示备注, got %q", v.Message)
	}
}

// 高频任务(QuietOnSuccess)成功不写台账 ⇒ 列表必须靠轻量 lastrun 记录显示"上次运行"，
// 否则永远显示"未运行"，让人以为内置任务从没自动跑过（2026-11-26 用户实测提问）。
func TestViewOfDef_LastRunFallback(t *testing.T) {
	svc := &TaskCenterService{}
	d := taskcenter.Def{
		Key: "builtin:backdrop", Type: "backdrop", Name: "横图补采", Kind: taskcenter.KindBuiltin,
		Enabled: true, DefaultSchedule: "every:20m", QuietOnSuccess: true,
	}
	now := time.Now().UnixMilli()

	// 台账没有行 + lastrun 成功 ⇒ 展示"成功 + 上次运行时间 + 下次时间"
	v := svc.viewOfDef(context.Background(), d, entity.TaskSetting{},
		map[string]cache.TaskLastRun{"backdrop": {AtMs: now, OK: true}},
		map[string]int64{d.Key: now + 600000})
	if v.State != string(taskcenter.StateSucceeded) || v.LastRunAt != 0 && v.StartedAt != now {
		t.Fatalf("应从 lastrun 兜底出上次运行: %+v", v)
	}
	if v.StartedAt != now {
		t.Fatalf("上次运行时间应取自 lastrun, got %d", v.StartedAt)
	}
	if v.NextAt != now+600000 {
		t.Fatalf("下次触发时刻应合并进视图, got %d", v.NextAt)
	}

	// lastrun 失败 ⇒ 状态失败
	v = svc.viewOfDef(context.Background(), d, entity.TaskSetting{},
		map[string]cache.TaskLastRun{"backdrop": {AtMs: now, OK: false}}, nil)
	if v.State != string(taskcenter.StateFailed) {
		t.Fatalf("lastrun 失败应映射成 failed, got %s", v.State)
	}

	// 没有 lastrun 也没有台账 ⇒ 空闲、无下次时间（停用/待配置的正常表现）
	v = svc.viewOfDef(context.Background(), d, entity.TaskSetting{}, nil, nil)
	if v.State != string(taskcenter.StateIdle) || v.NextAt != 0 {
		t.Fatalf("无记录应为 idle 且无 nextAt: %+v", v)
	}
}

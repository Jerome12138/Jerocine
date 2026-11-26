package service

import (
	"context"
	"strings"
	"testing"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/taskcenter"
)

// ---- 测试替身 ----

type fakeSettings struct {
	sets    []entity.TaskSetting
	enabled map[string]bool
}

func (f *fakeSettings) List(context.Context) (map[string]entity.TaskSetting, error) {
	m := map[string]entity.TaskSetting{}
	for _, s := range f.sets {
		m[s.TaskKey] = s
	}
	return m, nil
}
func (f *fakeSettings) Get(context.Context, string) (entity.TaskSetting, bool, error) {
	return entity.TaskSetting{}, false, nil
}
func (f *fakeSettings) Upsert(_ context.Context, s entity.TaskSetting) error {
	f.sets = append(f.sets, s)
	return nil
}
func (f *fakeSettings) SetEnabled(_ context.Context, key string, v bool, _ int64) error {
	if f.enabled == nil {
		f.enabled = map[string]bool{}
	}
	f.enabled[key] = v
	return nil
}

type fakeCrons struct {
	t      entity.CronTask
	upsert int
}

func (f *fakeCrons) List(context.Context) ([]entity.CronTask, error) {
	return []entity.CronTask{f.t}, nil
}
func (f *fakeCrons) Get(context.Context, int64) (*entity.CronTask, error) {
	c := f.t
	return &c, nil
}
func (f *fakeCrons) Upsert(_ context.Context, c *entity.CronTask) error {
	f.t = *c
	f.upsert++
	return nil
}
func (f *fakeCrons) Delete(context.Context, int64) error { return nil }

var _ repository.TaskSettingRepository = (*fakeSettings)(nil)
var _ repository.CronTaskRepository = (*fakeCrons)(nil)

func newTCS() (*TaskCenterService, *fakeSettings) {
	fs := &fakeSettings{}
	return &TaskCenterService{settings: fs, reg: newRegistryWithBuiltins()}, fs
}

// newRegistryWithBuiltins 复用生产注册逻辑（只注册元数据，不触发任何外部调用）。
func newRegistryWithBuiltins() *taskcenter.Registry {
	s := &TaskCenterService{reg: taskcenter.NewRegistry()}
	s.registerBuiltins()
	return s.reg
}

// TaskSettingInput 校验：白名单 / 周期语法 / 间隔下限。
func TestUpdateSetting_BuiltinValidation(t *testing.T) {
	svc, fs := newTCS()
	ctx := context.Background()

	t.Run("间隔低于下限被拒并给出下限", func(t *testing.T) {
		ms := 100
		err := svc.UpdateSetting(ctx, "builtin:douban_score", TaskSettingInput{IntervalMs: &ms})
		if err == nil || !strings.Contains(err.Error(), "不能小于") {
			t.Fatalf("应拒绝低于下限, got %v", err)
		}
	})

	t.Run("不支持改间隔的任务被拒", func(t *testing.T) {
		ms := 5000
		err := svc.UpdateSetting(ctx, "builtin:domain_fetch", TaskSettingInput{IntervalMs: &ms})
		if err == nil || !strings.Contains(err.Error(), "不支持修改") {
			t.Fatalf("domain_fetch 不可改间隔, got %v", err)
		}
	})

	t.Run("非法周期被拒", func(t *testing.T) {
		bad := "0 3 * * *"
		err := svc.UpdateSetting(ctx, "builtin:douban_score", TaskSettingInput{Schedule: &bad})
		if err == nil {
			t.Fatal("裸 cron 应被拒（内置任务用友好文本）")
		}
	})

	t.Run("合法更新落库且启用位单独写", func(t *testing.T) {
		sch := "daily:05:30"
		rm := "每周看一眼"
		ms := 3000
		off := false
		if err := svc.UpdateSetting(ctx, "builtin:douban_score", TaskSettingInput{
			Enabled: &off, Schedule: &sch, Remark: &rm, IntervalMs: &ms,
		}); err != nil {
			t.Fatalf("应成功: %v", err)
		}
		if v, ok := fs.enabled["builtin:douban_score"]; !ok || v != false {
			t.Fatalf("启用位应写 false: %+v", fs.enabled)
		}
		if len(fs.sets) == 0 {
			t.Fatal("应写入配置")
		}
		last := fs.sets[len(fs.sets)-1]
		if last.Schedule != "daily:05:30" || last.Remark != "每周看一眼" || last.IntervalMs != 3000 {
			t.Fatalf("落库值不对: %+v", last)
		}
	})

	t.Run("未知任务被拒", func(t *testing.T) {
		if err := svc.UpdateSetting(ctx, "builtin:nope", TaskSettingInput{}); err == nil {
			t.Fatal("未知 key 应报错")
		}
	})
}

// cron 任务：启用/计划/备注可改，间隔不可在此改。
func TestUpdateSetting_Cron(t *testing.T) {
	crons := &fakeCrons{t: entity.CronTask{Id: 7, Spec: "0 3 * * *", State: entity.StateEnabled}}
	svc := &TaskCenterService{crons: crons, settings: &fakeSettings{}, reg: newRegistryWithBuiltins()}
	ctx := context.Background()

	off := false
	spec := "30 4 * * *"
	rm := "改成凌晨"
	if err := svc.UpdateSetting(ctx, "cron:7", TaskSettingInput{Enabled: &off, Schedule: &spec, Remark: &rm}); err != nil {
		t.Fatalf("应成功: %v", err)
	}
	if crons.t.State != entity.StateDisabled || crons.t.Spec != spec || crons.t.Remark != rm {
		t.Fatalf("cron 未按预期更新: %+v", crons.t)
	}

	ms := 5000
	if err := svc.UpdateSetting(ctx, "cron:7", TaskSettingInput{IntervalMs: &ms}); err == nil {
		t.Fatal("采集任务的间隔不应在这里改")
	}
	if err := svc.UpdateSetting(ctx, "cron:7", TaskSettingInput{Schedule: strPtr("")}); err == nil {
		t.Fatal("空 cron 计划应被拒")
	}
}

func strPtr(s string) *string { return &s }

// 调度接管的回归护栏：**每个有周期的内置任务都必须有 Run**，否则调度器会空转；
// 高频任务必须 NoLedger（否则 20~30 分钟一条把台账刷爆）。
func TestBuiltinRegistry_SchedulingContract(t *testing.T) {
	reg := newRegistryWithBuiltins()
	got := map[string]taskcenter.Def{}
	for _, d := range reg.All() {
		got[d.Key] = d
	}

	want := []string{
		"builtin:douban_score", "builtin:douban_backfill", "builtin:domain_fetch",
		"builtin:hot_refresh", "builtin:backdrop", "builtin:health",
		"builtin:history_prune", "builtin:year_verify",
	}
	for _, k := range want {
		d, ok := got[k]
		if !ok {
			t.Fatalf("任务未注册（调度不会接管）: %s", k)
		}
		if d.DefaultSchedule != "" && d.Run == nil {
			t.Fatalf("%s 声明了周期却没有 Run：调度会空转", k)
		}
	}

	noLedger := []string{"builtin:backdrop", "builtin:health", "builtin:year_verify"}
	for _, k := range noLedger {
		if !got[k].NoLedger {
			t.Fatalf("%s 属高频任务，应标记 NoLedger", k)
		}
	}

	// 欠账补查：默认不启用（只手动），避免一上来就把 2.7 万条历史欠账全刷一遍
	if got["builtin:douban_backfill"].Enabled {
		t.Fatal("builtin:douban_backfill 默认应为不启用")
	}
	// 周期文案必须能被解析（否则 NextRun 永远为零值、永不触发）
	for k, d := range got {
		if err := taskcenter.ValidateSchedule(d.DefaultSchedule); err != nil {
			t.Fatalf("%s 的默认周期不合法: %v", k, err)
		}
	}
}

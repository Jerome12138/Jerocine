package service

import (
	"context"
	"strings"
	"testing"
	"time"

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
	list   []entity.CronTask // 非空时 List 返回它（多行用例）
	upsert int
}

func (f *fakeCrons) List(context.Context) ([]entity.CronTask, error) {
	if len(f.list) > 0 { // 多行场景（如周期归一用例）
		return f.list, nil
	}
	return []entity.CronTask{f.t}, nil
}
func (f *fakeCrons) Get(context.Context, int64) (*entity.CronTask, error) {
	c := f.t
	return &c, nil
}
func (f *fakeCrons) Upsert(_ context.Context, c *entity.CronTask) error {
	// 多行场景要写回 list（否则用例里读 list 看不到变化）
	for i := range f.list {
		if f.list[i].Id == c.Id {
			f.list[i] = *c
			f.upsert++
			return nil
		}
	}
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
//
// P3-2 之后定义由各域服务提供，所以这里给"零值实例"让各自的 TaskDefs() 能跑：
// TaskDefs() 只描述元数据、不访问字段（闭包内自带 nil 守卫），因此零值是安全的。
// 注意：这些闭包**不要**在单测里真的执行（零值服务会 nil 解引用）——要执行就用本地的探针 Def。
func newRegistryWithBuiltins() *taskcenter.Registry {
	s := &TaskCenterService{
		reg:        taskcenter.NewRegistry(),
		film:       &FilmService{},
		hot:        &HotService{},
		backdrop:   &BackdropService{},
		manage:     &ManageService{},
		yearVerify: &YearVerifyService{},
	}
	s.registerBuiltins()
	return s.reg
}

// TaskSettingInput 校验：白名单 / 周期语法 / 间隔下限。
func TestUpdateSetting_BuiltinValidation(t *testing.T) {
	svc, fs := newTCS()
	ctx := context.Background()

	t.Run("间隔低于下限被拒并给出下限", func(t *testing.T) {
		ms := 100
		_, err := svc.UpdateSetting(ctx, "builtin:douban_score", TaskSettingInput{IntervalMs: &ms})
		if err == nil || !strings.Contains(err.Error(), "不能小于") {
			t.Fatalf("应拒绝低于下限, got %v", err)
		}
	})

	t.Run("不支持改间隔的任务被拒", func(t *testing.T) {
		ms := 5000
		_, err := svc.UpdateSetting(ctx, "builtin:domain_fetch", TaskSettingInput{IntervalMs: &ms})
		if err == nil || !strings.Contains(err.Error(), "不支持修改") {
			t.Fatalf("domain_fetch 不可改间隔, got %v", err)
		}
	})

	t.Run("非法周期被拒", func(t *testing.T) {
		bad := "0 3 * * *"
		_, err := svc.UpdateSetting(ctx, "builtin:douban_score", TaskSettingInput{Schedule: &bad})
		if err == nil {
			t.Fatal("裸 cron 应被拒（内置任务用友好文本）")
		}
	})

	t.Run("合法更新落库且启用位单独写", func(t *testing.T) {
		sch := "daily:05:30"
		rm := "每周看一眼"
		ms := 3000
		off := false
		if _, err := svc.UpdateSetting(ctx, "builtin:douban_score", TaskSettingInput{
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
		if _, err := svc.UpdateSetting(ctx, "builtin:nope", TaskSettingInput{}); err == nil {
			t.Fatal("未知 key 应报错")
		}
	})
}

// 停用正在跑的任务：**不中断本轮**，但要让调用方知道"延迟生效"，前端才能如实提示
// （否则页面显示"已停用"、后台还在跑，是假象）。
func TestUpdateSetting_DeferredWhenRunning(t *testing.T) {
	ctx := context.Background()
	off, on := false, true

	t.Run("内置任务在跑 → deferred", func(t *testing.T) {
		runs := &fakeRuns{latest: []entity.TaskRun{{Id: 3, Type: "year_verify", Status: entity.TaskRunRunning}}}
		svc := &TaskCenterService{settings: &fakeSettings{}, reg: newRegistryWithBuiltins(), runs: runs}
		deferred, err := svc.UpdateSetting(ctx, "builtin:year_verify", TaskSettingInput{Enabled: &off})
		if err != nil {
			t.Fatalf("应成功: %v", err)
		}
		if !deferred {
			t.Fatal("正在跑的内置任务停用应返回 deferred=true")
		}
	})

	t.Run("采集任务在跑 → deferred 且照常落库", func(t *testing.T) {
		crons := &fakeCrons{t: entity.CronTask{Id: 7, State: entity.StateEnabled, Schedule: "daily:03:00"}}
		runs := &fakeRuns{latestCron: map[int64]entity.TaskRun{7: {Id: 1, CronId: 7, Status: entity.TaskRunRunning}}}
		svc := &TaskCenterService{crons: crons, settings: &fakeSettings{}, reg: newRegistryWithBuiltins(), runs: runs}
		deferred, err := svc.UpdateSetting(ctx, "cron:7", TaskSettingInput{Enabled: &off})
		if err != nil {
			t.Fatalf("应成功: %v", err)
		}
		if !deferred {
			t.Fatal("正在跑的采集任务停用应返回 deferred=true")
		}
		if crons.t.State != entity.StateDisabled {
			t.Fatal("停用必须落库（只是本轮不中断）")
		}
	})

	t.Run("空闲任务不 deferred", func(t *testing.T) {
		runs := &fakeRuns{latest: []entity.TaskRun{{Id: 5, Type: "year_verify", Status: entity.TaskRunSuccess}}}
		svc := &TaskCenterService{settings: &fakeSettings{}, reg: newRegistryWithBuiltins(), runs: runs}
		deferred, err := svc.UpdateSetting(ctx, "builtin:year_verify", TaskSettingInput{Enabled: &off})
		if err != nil || deferred {
			t.Fatalf("没在跑就不该 deferred: err=%v deferred=%v", err, deferred)
		}
	})

	t.Run("启用方向不 deferred", func(t *testing.T) {
		runs := &fakeRuns{latest: []entity.TaskRun{{Type: "year_verify", Status: entity.TaskRunRunning}}}
		svc := &TaskCenterService{settings: &fakeSettings{}, reg: newRegistryWithBuiltins(), runs: runs}
		deferred, err := svc.UpdateSetting(ctx, "builtin:year_verify", TaskSettingInput{Enabled: &on})
		if err != nil || deferred {
			t.Fatalf("启用不该 deferred: err=%v deferred=%v", err, deferred)
		}
	})
}

// 批量停用：把"正在跑"的挑出来单独回报（其余照常停用），未知任务进 errors（部分失败不回滚）。
func TestBatchSetEnabled_ReportsDeferred(t *testing.T) {
	ctx := context.Background()
	crons := &fakeCrons{t: entity.CronTask{Id: 7, State: entity.StateEnabled, Schedule: "daily:03:00"}}
	runs := &fakeRuns{
		latestCron: map[int64]entity.TaskRun{7: {Id: 1, CronId: 7, Status: entity.TaskRunRunning}},
		latest:     []entity.TaskRun{{Type: "year_verify", Status: entity.TaskRunRunning}},
	}
	svc := &TaskCenterService{crons: crons, settings: &fakeSettings{}, reg: newRegistryWithBuiltins(), runs: runs}

	ok, deferred, errs := svc.BatchSetEnabled(ctx, []string{"cron:7", "builtin:year_verify", "builtin:nope"}, false)
	if ok != 2 {
		t.Fatalf("应成功 2 条, got %d (%v)", ok, errs)
	}
	if len(deferred) != 2 {
		t.Fatalf("两条都在跑, 都应回报 deferred: %v", deferred)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "未知任务") {
		t.Fatalf("未知任务应进 errors: %v", errs)
	}
	if crons.t.State != entity.StateDisabled {
		t.Fatal("采集任务应被停用")
	}

	// 启用方向：即使有在跑实例也不报 deferred（不是"延迟停用"）
	crons.t.State = entity.StateDisabled
	ok, deferred, errs = svc.BatchSetEnabled(ctx, []string{"cron:7"}, true)
	if ok != 1 || len(deferred) != 0 || len(errs) != 0 {
		t.Fatalf("启用不该 deferred: ok=%d deferred=%v errs=%v", ok, deferred, errs)
	}
}

// cron 任务：启用/计划/备注可改，间隔不可在此改。
// 计划统一写**友好文本**（时间语义统一后不再收 cron 表达式）。
func TestUpdateSetting_Cron(t *testing.T) {
	crons := &fakeCrons{t: entity.CronTask{Id: 7, Spec: "0 3 * * *", Schedule: "daily:03:00", State: entity.StateEnabled}}
	svc := &TaskCenterService{crons: crons, settings: &fakeSettings{}, reg: newRegistryWithBuiltins()}
	ctx := context.Background()

	off := false
	sched := "daily:04:30"
	rm := "改成凌晨"
	if _, err := svc.UpdateSetting(ctx, "cron:7", TaskSettingInput{Enabled: &off, Schedule: &sched, Remark: &rm}); err != nil {
		t.Fatalf("应成功: %v", err)
	}
	if crons.t.State != entity.StateDisabled || crons.t.Schedule != sched || crons.t.Remark != rm {
		t.Fatalf("cron 未按预期更新: %+v", crons.t)
	}

	ms := 5000
	if _, err := svc.UpdateSetting(ctx, "cron:7", TaskSettingInput{IntervalMs: &ms}); err == nil {
		t.Fatal("采集任务的间隔不应在这里改")
	}
	if _, err := svc.UpdateSetting(ctx, "cron:7", TaskSettingInput{Schedule: strPtr("")}); err == nil {
		t.Fatal("空计划应被拒（停用请用 enabled=false）")
	}
	// 旧的 cron 表达式不再是合法计划（迁移已统一为友好文本）
	if _, err := svc.UpdateSetting(ctx, "cron:7", TaskSettingInput{Schedule: strPtr("30 4 * * *")}); err == nil {
		t.Fatal("裸 cron 表达式应被拒（统一时间语义）")
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

	quietOnSuccess := []string{"builtin:backdrop", "builtin:health", "builtin:year_verify"}
	for _, k := range quietOnSuccess {
		if !got[k].QuietOnSuccess {
			t.Fatalf("%s 属高频任务，应标记 QuietOnSuccess（成功不记、失败记）", k)
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

// 回归：**只改周期/备注**（不传 enabled）不得把默认启用的内置任务停用。
// 踩过的坑：Upsert 的 INSERT 分支用实体零值建行（enabled=false）⇒ 任务静默不跑、零报错。
func TestUpdateSetting_PartialUpdateKeepsEnabled(t *testing.T) {
	ctx := context.Background()
	fs := &fakeSettings{}
	svc := &TaskCenterService{settings: fs, reg: newRegistryWithBuiltins()}

	sch := "daily:06:10"
	deferred, err := svc.UpdateSetting(ctx, "builtin:hot_refresh", TaskSettingInput{Schedule: &sch})
	if err != nil {
		t.Fatalf("应成功: %v", err)
	}
	if deferred {
		t.Fatal("只改周期不该报 deferred（那是停用才会有的语义）")
	}
	v, ok := fs.enabled["builtin:hot_refresh"]
	if !ok || !v {
		t.Fatalf("启用位应保持默认开启(true), got ok=%v v=%v", ok, v)
	}
	// 默认不启用的任务：只改备注也不能被"顺手启用"
	rm := "补查用"
	if _, err := svc.UpdateSetting(ctx, "builtin:douban_backfill", TaskSettingInput{Remark: &rm}); err != nil {
		t.Fatalf("应成功: %v", err)
	}
	if v, ok := fs.enabled["builtin:douban_backfill"]; !ok || v {
		t.Fatalf("默认停用的任务不该被局部更新打开, got ok=%v v=%v", ok, v)
	}
}

// 显式停用仍要 deferred 语义（在跑 ⇒ 等本轮结束）。
func TestUpdateSetting_ExplicitDisableStillDeferred(t *testing.T) {
	ctx := context.Background()
	off := false
	runs := &fakeRuns{latest: []entity.TaskRun{{Type: "year_verify", Status: entity.TaskRunRunning}}}
	svc := &TaskCenterService{settings: &fakeSettings{}, reg: newRegistryWithBuiltins(), runs: runs}
	deferred, err := svc.UpdateSetting(ctx, "builtin:year_verify", TaskSettingInput{Enabled: &off})
	if err != nil {
		t.Fatalf("应成功: %v", err)
	}
	if !deferred {
		t.Fatal("显式停用且在跑 ⇒ 应 deferred")
	}
}

// 策略默认值兜底：直接构造 &TaskCenterService{} 时不能把保留天数算成 0（否则"删光全部台账"）。
func TestRetentionAndTickFallback(t *testing.T) {
	svc := &TaskCenterService{}
	if got := svc.retention(); got != 30 {
		t.Fatalf("默认保留天数应为 30, got %d", got)
	}
	if got := svc.schedulerTick(); got != 30*time.Second {
		t.Fatalf("默认 tick 应为 30s, got %s", got)
	}
	custom := &TaskCenterService{retentionDays: 7, tickEvery: 5 * time.Second}
	if custom.retention() != 7 || custom.schedulerTick() != 5*time.Second {
		t.Fatalf("配置值应生效: %d / %s", custom.retention(), custom.schedulerTick())
	}
}

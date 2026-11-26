package service

// 调度器的**纯替身**单测：证明它真的只依赖三个接口，不需要构造半个 TaskCenterService。
// （这是 P3-6「接口化」的验收点：能这样写测试，就说明耦合已经断开。）

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"server/internal/domain/entity"
	"server/internal/taskcenter"
)

type fakeCatalog struct {
	defs  []taskcenter.Def
	sets  map[string]entity.TaskSetting
	crons []entity.CronTask
}

func (f *fakeCatalog) Definitions() []taskcenter.Def { return f.defs }
func (f *fakeCatalog) Settings(context.Context) map[string]entity.TaskSetting {
	if f.sets == nil {
		return map[string]entity.TaskSetting{}
	}
	return f.sets
}
func (f *fakeCatalog) Crons(context.Context) ([]entity.CronTask, error) { return f.crons, nil }

type fakeRunner struct {
	builtinCalls int32
	cronCalls    int32
	lastCron     entity.CronTask
}

func (f *fakeRunner) RunDefinition(context.Context, taskcenter.Def) error {
	atomic.AddInt32(&f.builtinCalls, 1)
	return nil
}
func (f *fakeRunner) RunCronRound(_ context.Context, t entity.CronTask) error {
	atomic.AddInt32(&f.cronCalls, 1)
	f.lastCron = t
	return nil
}

type fakeLedger struct {
	starts  int32
	records int32
	finish  int32
}

func (f *fakeLedger) Start(string, string, int64, string, string, int) int64 {
	atomic.AddInt32(&f.starts, 1)
	return 1
}
func (f *fakeLedger) Finish(int64, string, int, int, int, string, string) {
	atomic.AddInt32(&f.finish, 1)
}
func (f *fakeLedger) Record(string, string, int64, string, string, string, int, int, int, string, string) int64 {
	atomic.AddInt32(&f.records, 1)
	return 1
}

// 到点触发 → 调 Runner + 记台账；未到点 → 什么都不做；已停用 → 不触发。
func TestSchedulerWithFakes(t *testing.T) {
	ctx := context.Background()
	cat := &fakeCatalog{
		defs: []taskcenter.Def{
			{Key: "builtin:a", Name: "A", Kind: taskcenter.KindBuiltin, Enabled: true, DefaultSchedule: "every:1m", Run: func(context.Context) error { return nil }},
			{Key: "builtin:off", Name: "OFF", Kind: taskcenter.KindBuiltin, Enabled: false, DefaultSchedule: "every:1m", Run: func(context.Context) error { return nil }},
		},
	}
	run := &fakeRunner{}
	led := &fakeLedger{}
	s := newTaskScheduler(cat, run, led, time.Minute, nil)

	// 首次 tick：只登记"下次时间"，不触发
	s.tick(ctx)
	if n := atomic.LoadInt32(&run.builtinCalls); n != 0 {
		t.Fatalf("首次 tick 不应触发, got %d", n)
	}
	// 把 a 的下次时间调到过去 → 到点触发
	s.next["builtin:a"] = time.Now().Add(-time.Second)
	s.tick(ctx)
	waitFor(t, func() bool { return atomic.LoadInt32(&run.builtinCalls) == 1 })
	waitFor(t, func() bool { return atomic.LoadInt32(&led.finish) == 1 })

	// 停用的任务永远不该被触发
	if n := atomic.LoadInt32(&run.builtinCalls); n != 1 {
		t.Fatalf("停用任务不该触发, 总调用 %d", n)
	}
}

// 采集分支：到点调 RunCronRound（不再直接抓 spider）。
func TestSchedulerCronWithFakes(t *testing.T) {
	ctx := context.Background()
	cat := &fakeCatalog{crons: []entity.CronTask{
		{Id: 5, Schedule: "daily:03:00", State: entity.StateEnabled},
		{Id: 6, Schedule: "", State: entity.StateEnabled},             // 周期待人工配置 → 跳过
		{Id: 7, Schedule: "daily:03:00", State: entity.StateDisabled}, // 停用 → 跳过
	}}
	run := &fakeRunner{}
	s := newTaskScheduler(cat, run, nil, time.Minute, nil)

	s.tick(ctx) // 登记下次时间
	key := taskcenter.RefCron(5)
	if _, ok := s.next[key]; !ok {
		t.Fatalf("启用的采集任务应登记下次时间, next=%v", s.next)
	}
	if _, ok := s.next[taskcenter.RefCron(6)]; ok {
		t.Fatal("空周期任务不该登记")
	}
	s.next[key] = time.Now().Add(-time.Second)
	s.tick(ctx)
	waitFor(t, func() bool { return atomic.LoadInt32(&run.cronCalls) == 1 })
	if run.lastCron.Id != 5 {
		t.Fatalf("应触发 id=5, got %d", run.lastCron.Id)
	}
}

// 台账为 nil（未装配）时跑任务不能 panic —— 这是"降级不阻断"的底线。
func TestSchedulerNilLedger(t *testing.T) {
	ctx := context.Background()
	cat := &fakeCatalog{defs: []taskcenter.Def{
		{Key: "builtin:a", Name: "A", Kind: taskcenter.KindBuiltin, Enabled: true, DefaultSchedule: "every:1m",
			QuietOnSuccess: true, Run: func(context.Context) error { return nil }},
	}}
	run := &fakeRunner{}
	s := newTaskScheduler(cat, run, nil, time.Minute, nil)
	s.next["builtin:a"] = time.Now().Add(-time.Second)
	s.tick(ctx)
	waitFor(t, func() bool { return atomic.LoadInt32(&run.builtinCalls) == 1 })
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"server/internal/cache"
	"server/internal/domain/entity"
	"server/internal/taskcenter"
)

// 闸门必须**真的拦住每一个触发入口**。
// 这条用例的价值不在"验证某个函数"，而在"新增触发入口忘了调 GuardTrigger 时变红"——
// 维护窗口失效是没有报错的（只有"迁移期间库里还有采集在写"这种事后现象），必须靠用例兜住。
func TestAllTriggerEntriesRespectGate(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()
	if err := cache.TaskGateOpen(ctx, "deploy.sh", time.Minute); err != nil {
		t.Fatalf("开闸门: %v", err)
	}

	// ① 任务中心手动执行（内置）—— 用探针 Def，避免真的去跑零值服务
	svc := &TaskCenterService{reg: probeRegistry(), settings: &fakeSettings{}}
	if err := svc.RunTask(ctx, "builtin:hot_refresh"); !isGateErr(err) {
		t.Fatalf("内置手动执行应被闸门拦, got %v", err)
	}
	// ② 任务中心手动执行（采集）
	if err := svc.RunTask(ctx, "cron:7"); !isGateErr(err) {
		t.Fatalf("采集手动执行应被闸门拦, got %v", err)
	}
	// ③ 采集页"立即执行"（TriggerCron）
	sp := &SpiderService{crons: &fakeCrons{t: entity.CronTask{Id: 7, State: entity.StateEnabled, Schedule: "daily:03:00"}}}
	if err := sp.TriggerCron(ctx, 7); !isGateErr(err) {
		t.Fatalf("TriggerCron 应被闸门拦, got %v", err)
	}
	// ④ 单源手动采集（StartCollect）—— 必须**严格**是闸门错（若有人把闸门挪到查源之后，
	//    这里会拿到"源不存在"之类的别的错，用例必须变红，而不是静默放过）
	if err := sp.StartCollect("src_x", 24); !isGateErr(err) {
		t.Fatalf("StartCollect 应被闸门拦, got %v", err)
	}
	// ⑤ 调度器整轮（tick 依赖同一判断源）
	if err := GuardTrigger(ctx); err == nil {
		t.Fatal("GuardTrigger 必须认为窗口开着（否则调度器 tick 会照常触发）")
	}
}

// 闸门关闭后同一批入口必须恢复可用（别把"永久拒绝"写进判断里）。
func TestTriggerEntriesResumeAfterGateClosed(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()
	svc := &TaskCenterService{reg: probeRegistry(), settings: &fakeSettings{}}
	if err := cache.TaskGateOpen(ctx, "deploy.sh", time.Minute); err != nil {
		t.Fatalf("开闸门: %v", err)
	}
	if err := cache.TaskGateClose(ctx); err != nil {
		t.Fatalf("关闸门: %v", err)
	}
	// 内置任务此时会真的去跑（hot 服务未装配 → 返回"未装配"错），只要不是闸门错即可
	if err := svc.RunTask(ctx, "builtin:hot_refresh"); isGateErr(err) {
		t.Fatalf("窗口关闭后不该再被闸门拦, got %v", err)
	}
}

func isGateErr(err error) bool {
	var mw *MaintenanceWindowError
	return errors.As(err, &mw)
}

// 规则层状态机与存储层 JobControl 必须等价 —— 防止两处各自演化出不同迁移表。
func TestJobStateMachineMatchesCache(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()

	states := []string{taskcenter.JobRunning, taskcenter.JobPaused, taskcenter.JobCanceled, taskcenter.JobDone, taskcenter.JobError}
	acts := []struct {
		act  taskcenter.JobAction
		to   string
		from []string
	}{
		{taskcenter.JobActionPause, cache.JobPaused, []string{cache.JobRunning}},
		{taskcenter.JobActionResume, cache.JobRunning, []string{cache.JobPaused}},
		{taskcenter.JobActionCancel, cache.JobCanceled, []string{cache.JobRunning, cache.JobPaused}},
	}
	for _, a := range acts {
		for _, cur := range states {
			// 规则层判断
			wantNext, wantOK := taskcenter.NextJobState(cur, a.act)
			// 存储层判断（先造出该状态，再按前置条件翻译成 JobControl 的 from 参数）
			src := "src_" + string(a.act) + "_" + cur
			if err := cache.JobStart(ctx, src, "t", 10, 24); err != nil {
				t.Fatalf("JobStart: %v", err)
			}
			cache.JobSetState(ctx, src, cur)
			gotErr := cache.JobControl(ctx, src, a.to, a.from...)
			gotOK := gotErr == nil
			if wantOK != gotOK {
				t.Fatalf("迁移表不一致: act=%s cur=%s 规则层=%v 存储层=%v (%v)",
					a.act, cur, wantOK, gotOK, gotErr)
			}
			if wantOK {
				if st := cache.JobReadState(ctx, src); st != wantNext {
					t.Fatalf("目标状态不一致: act=%s cur=%s 规则层=%s 存储层=%s", a.act, cur, wantNext, st)
				}
			}
		}
	}
}

// probeRegistry 只含一个"什么都不做"的内置任务：用于断言触发链路（闸门/放行），
// 不牵扯真实服务（零值服务真的执行会 nil 解引用）。
func probeRegistry() *taskcenter.Registry {
	reg := taskcenter.NewRegistry()
	reg.Register(taskcenter.Def{
		Key: "builtin:probe", Type: "probe", Name: "探针", Kind: taskcenter.KindBuiltin,
		Enabled: true, DefaultSchedule: "daily:04:00",
		Run: func(context.Context) error { return nil },
	})
	return reg
}

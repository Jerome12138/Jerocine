package service

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"server/internal/cache"
	"server/internal/taskcenter"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// initTestCache 给本包测试接一个 miniredis 当协调存储；测试结束恢复成"未接入"。
// 本包其他测试依赖 coordRdb==nil 的语义，所以必须还原（勿删）。
func initTestCache(t *testing.T) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache.Init(rdb, rdb)
	t.Cleanup(func() {
		cache.Init(nil, nil)
		mr.Close()
	})
}

// 维护窗口 = 拦新触发 + 协作暂停在跑的采集源；resume 只恢复"因窗口而暂停"的那些。
// 关键回归点：**绝不误恢复用户手工暂停的任务**（否则 resume 会擅自把别人停掉的任务开起来）。
func TestPauseAllResumeAll(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()
	svc := &TaskCenterService{reg: probeRegistry()}

	// 三个源：一个在跑、一个已被用户手工暂停、一个从未启动
	if err := cache.JobStart(ctx, "src_run", "在跑的源", 100, 24); err != nil {
		t.Fatalf("JobStart: %v", err)
	}
	if err := cache.JobStart(ctx, "src_manual", "手工暂停的源", 100, 24); err != nil {
		t.Fatalf("JobStart: %v", err)
	}
	if err := cache.JobControl(ctx, "src_manual", cache.JobPaused, cache.JobRunning); err != nil {
		t.Fatalf("手工暂停: %v", err)
	}

	rep, err := svc.PauseAll(ctx, "deploy-agent", 30)
	if err != nil {
		t.Fatalf("PauseAll 失败: %v", err)
	}
	if rep.By != "deploy-agent" || rep.Until <= time.Now().Unix() {
		t.Fatalf("报告应带打开者与关闭时刻: %+v", rep)
	}
	if len(rep.Errors) != 0 {
		t.Fatalf("不应有错误: %v", rep.Errors)
	}
	// 只有"在跑"的那个被暂停，手工暂停的不在报告里
	if len(rep.Items) != 1 || rep.Items[0].Key != "job:src_run" || rep.Items[0].Action != "paused" {
		t.Fatalf("报告应只含在跑的源且已暂停: %+v", rep.Items)
	}
	if r := cache.JobReadState(ctx, "src_run"); r != cache.JobPaused {
		t.Fatalf("在跑的源应被暂停, got %s", r)
	}

	// 窗口内：状态可查、新触发被拦
	st, err := svc.ControlState(ctx)
	if err != nil || !st.Open || len(st.PausedJobs) != 1 || st.PausedJobs[0] != "src_run" {
		t.Fatalf("control 状态不对: %+v err=%v", st, err)
	}
	if err := svc.RunTask(ctx, "builtin:hot_refresh"); err == nil || !strings.Contains(err.Error(), "维护窗口") {
		t.Fatalf("窗口内手动执行应被拦, got %v", err)
	}
	if err := svc.RunTask(ctx, "job:src_run"); err == nil || !strings.Contains(err.Error(), "维护窗口") {
		t.Fatalf("窗口内手动执行采集源也应被拦, got %v", err)
	}

	// 恢复：只恢复 src_run；手工暂停的 src_manual 保持暂停
	rep, err = svc.ResumeAll(ctx)
	if err != nil {
		t.Fatalf("ResumeAll 失败: %v", err)
	}
	if len(rep.Items) != 1 || rep.Items[0].Action != "resumed" || rep.Items[0].Key != "job:src_run" {
		t.Fatalf("应只恢复闸门暂停的那一个: %+v", rep.Items)
	}
	if r := cache.JobReadState(ctx, "src_run"); r != cache.JobRunning {
		t.Fatalf("src_run 应恢复为 running, got %s", r)
	}
	if r := cache.JobReadState(ctx, "src_manual"); r != cache.JobPaused {
		t.Fatalf("手工暂停的 src_manual 必须保持 paused（不能被误恢复）, got %s", r)
	}
	if st, _ := svc.ControlState(ctx); st.Open || len(st.PausedJobs) != 0 {
		t.Fatalf("resume 后窗口应关闭且白名单清空: %+v", st)
	}
	if err := svc.RunTask(ctx, "builtin:probe"); err != nil && strings.Contains(err.Error(), "维护窗口") {
		t.Fatalf("窗口关闭后不应再被闸门拦: %v", err)
	}
}

// 协调存储不可用：开窗口必须**报错**，绝不能假成功（否则 agent 以为安静了，实际还在跑）。
func TestPauseAll_NoCoord(t *testing.T) {
	cache.Init(nil, nil)
	svc := &TaskCenterService{reg: newRegistryWithBuiltins()}
	if _, err := svc.PauseAll(context.Background(), "deploy", 30); err == nil {
		t.Fatal("协调存储不可用时应报错")
	}
	if _, err := svc.ResumeAll(context.Background()); err == nil {
		t.Fatal("协调存储不可用时 resume 也应报错")
	}
}

// 恢复时任务已结束/已回收（job 键没了）：跳过并说明原因，不报错。
func TestResumeAll_JobGone(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()
	svc := &TaskCenterService{reg: newRegistryWithBuiltins()}

	if err := cache.TaskGateOpen(ctx, "deploy", time.Minute); err != nil {
		t.Fatalf("开闸门失败: %v", err)
	}
	cache.TaskGateMarkJob(ctx, "src_gone") // 白名单里有，但 job 键不存在（已 GC）
	rep, err := svc.ResumeAll(ctx)
	if err != nil {
		t.Fatalf("ResumeAll 不应失败: %v", err)
	}
	if len(rep.Items) != 1 || rep.Items[0].Action != "skipped" || !strings.Contains(rep.Items[0].Reason, "已结束") {
		t.Fatalf("已回收的 job 应 skipped 并说明: %+v", rep.Items)
	}
}

// 调度器：维护窗口开着时整轮不触发；关闭后恢复触发。
func TestSchedulerTick_GateBlocksTrigger(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()

	var calls int32
	reg := taskcenter.NewRegistry()
	reg.Register(taskcenter.Def{
		Key: "builtin:probe", Type: "probe", Name: "探针", Kind: taskcenter.KindBuiltin,
		Enabled: true, DefaultSchedule: "every:1m",
		Run: func(context.Context) error { atomic.AddInt32(&calls, 1); return nil },
	})
	tc := &TaskCenterService{reg: reg, settings: &fakeSettings{}}
	s := NewTaskScheduler(tc)
	s.next["builtin:probe"] = time.Now().Add(-time.Minute) // 已到点，若无闸门必然触发

	if err := cache.TaskGateOpen(ctx, "deploy-agent", time.Minute); err != nil {
		t.Fatalf("开闸门失败: %v", err)
	}
	s.tick(ctx)
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("维护窗口内不应触发, got %d 次", n)
	}

	if err := cache.TaskGateClose(ctx); err != nil {
		t.Fatalf("关闸门失败: %v", err)
	}
	s.next["builtin:probe"] = time.Now().Add(-time.Minute)
	s.tick(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&calls) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("窗口关闭后应触发一次, got %d 次", n)
	}
}

// 不可协作暂停的内置长任务：如实回报（pauseable=false + 原因），不假装暂停。
func TestNonPauseableRunning_ReportsHonestly(t *testing.T) {
	svc := &TaskCenterService{reg: taskcenter.NewRegistry()}
	svc.reg.Register(taskcenter.Def{
		Key: "builtin:fake_long", Name: "假装很长的任务", Kind: taskcenter.KindBuiltin,
		Status: func(context.Context) taskcenter.Status {
			return taskcenter.Status{State: taskcenter.StateRunning}
		},
	})
	items := svc.nonPauseableRunning(context.Background())
	if len(items) != 1 {
		t.Fatalf("应回报 1 条: %+v", items)
	}
	if items[0].Pauseable || items[0].Action != "skipped" || items[0].Reason == "" {
		t.Fatalf("不可暂停的长任务必须如实标注: %+v", items[0])
	}
}

// control 要能回答"还有哪些源没停到页边界"：部署脚本靠它轮询安静点。
func TestControlState_RunningJobs(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()
	svc := &TaskCenterService{reg: newRegistryWithBuiltins()}

	if err := cache.JobStart(ctx, "src_a", "A", 10, 24); err != nil {
		t.Fatalf("JobStart: %v", err)
	}
	if err := cache.JobStart(ctx, "src_b", "B", 10, 24); err != nil {
		t.Fatalf("JobStart: %v", err)
	}
	if err := cache.TaskGateOpen(ctx, "deploy.sh", time.Minute); err != nil {
		t.Fatalf("开闸门: %v", err)
	}
	st, err := svc.ControlState(ctx)
	if err != nil {
		t.Fatalf("ControlState: %v", err)
	}
	if !st.Open || len(st.RunningJobs) != 2 {
		t.Fatalf("窗口开着且两个源都在跑: %+v", st)
	}

	// 暂停后 runningJobs 为空 = 安静点到了（部署脚本据此开始迁移）
	if _, err := svc.PauseAll(ctx, "deploy.sh", 30); err != nil {
		t.Fatalf("PauseAll: %v", err)
	}
	st, _ = svc.ControlState(ctx)
	if len(st.RunningJobs) != 0 {
		t.Fatalf("暂停后不应还有 running 的源: %+v", st.RunningJobs)
	}
	if len(st.PausedJobs) != 2 {
		t.Fatalf("白名单应有 2 个源: %+v", st.PausedJobs)
	}
}

// 部署重启后 resume：被窗口暂停的源已被启动扫尾标成"服务重启中断"，
// resume 必须**跳过并说清楚**，不能把它翻回 running 造出幽灵。
func TestResumeAll_AfterRestartInterrupted(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()
	svc := &TaskCenterService{reg: newRegistryWithBuiltins()}

	if err := cache.JobStart(ctx, "src_a", "A", 10, 24); err != nil {
		t.Fatalf("JobStart: %v", err)
	}
	if _, err := svc.PauseAll(ctx, "deploy.sh", 30); err != nil {
		t.Fatalf("PauseAll: %v", err)
	}
	// 模拟容器重建: 新进程启动时把遗留的 paused 扫成 error
	if n, err := cache.JobMarkInterrupted(ctx); err != nil || n != 1 {
		t.Fatalf("扫尾应标记 1 个, got n=%d err=%v", n, err)
	}

	rep, err := svc.ResumeAll(ctx)
	if err != nil {
		t.Fatalf("ResumeAll 不应失败: %v", err)
	}
	if len(rep.Items) != 1 || rep.Items[0].Action != "skipped" {
		t.Fatalf("已中断的源应 skipped: %+v", rep.Items)
	}
	if !strings.Contains(rep.Items[0].Reason, "中断") {
		t.Fatalf("原因应说明是重启中断: %q", rep.Items[0].Reason)
	}
	if cur := cache.JobReadState(ctx, "src_a"); cur != cache.JobError {
		t.Fatalf("不能翻回 running（幽灵），应保持 error, got %s", cur)
	}
}

// 停用判据要覆盖"刚起轮次、台账还没落"的窗口：任务级单飞锁在 = 在跑。
// 只看台账会漏这一小段，导致停用时误报"没在跑"（deferred 提示错）。
func TestIsTaskRunning_LockHeld(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()
	svc := &TaskCenterService{reg: newRegistryWithBuiltins()}

	// 锁被持有，但台账里没有任何 running 行（模拟刚抢到锁、台账还没写）
	if _, _, err := cache.TryLock(ctx, cache.KeyLockCronTask(7), time.Minute); err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if !svc.isTaskRunning(ctx, "cron:7") {
		t.Fatal("锁在就该判为在跑（覆盖台账未落的窗口）")
	}
	// 换个任务 id：锁不匹配就不该误判
	if svc.isTaskRunning(ctx, "cron:8") {
		t.Fatal("别的任务 id 不该被误判为在跑")
	}
}

// resume 只有"真的失败"才留在白名单（关窗+清空+源仍 paused = 没人再管它，要等 24h TTL）。
// 已结束/已中断/已在跑属于"不需要恢复"，不该留。
func TestResumeKeepFailedSet(t *testing.T) {
	if keepOnResumeError(cache.ErrJobNotFound) {
		t.Fatal("已回收的 job 不该留在白名单")
	}
	if keepOnResumeError(cache.ErrJobStateMismatch) {
		t.Fatal("状态已变化（已中断/已在跑）不该留在白名单")
	}
	if !keepOnResumeError(errors.New("redis: connection refused")) {
		t.Fatal("真正的失败必须留在白名单待重试")
	}
}

// 集成：resume 成功后白名单清空（成功路径的行为基线）。
func TestResumeAll_ClearsWhitelist(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()
	svc := &TaskCenterService{reg: newRegistryWithBuiltins()}

	if err := cache.JobStart(ctx, "src_a", "A", 10, 24); err != nil {
		t.Fatalf("JobStart: %v", err)
	}
	if _, err := svc.PauseAll(ctx, "deploy.sh", 30); err != nil {
		t.Fatalf("PauseAll: %v", err)
	}
	if _, err := svc.ResumeAll(ctx); err != nil {
		t.Fatalf("ResumeAll: %v", err)
	}
	if got := cache.TaskGateJobs(ctx); len(got) != 0 {
		t.Fatalf("恢复成功后白名单应清空, got %v", got)
	}
	if r := cache.JobReadState(ctx, "src_a"); r != cache.JobRunning {
		t.Fatalf("应恢复为 running, got %s", r)
	}
}

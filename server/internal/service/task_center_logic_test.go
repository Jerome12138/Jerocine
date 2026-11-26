package service

// 任务中心核心逻辑的单元测试补齐（2026-11-26，TDD 式回归集）
//
// 覆盖"改了就容易错、错了又没报错"的四块：
//   ① 异步任务观察者 waitDefTerminal（终态/停滞/超时/取消）
//   ② 豆瓣两条任务共用进度时的状态归属（doubanStatusFor）
//   ③ cron 存量周期归一（NormalizeCronSchedules 三分支）
//   ④ 台账保留期（PruneHistory 用配置而不是写死 30 天）
// 另加几个纯函数/小入口：NextRuns、EnsureTaskKeyRegistered、WatchTimeoutOrDefault。

import (
	"context"
	"strings"
	"testing"
	"time"

	"server/internal/domain/entity"
	"server/internal/taskcenter"
)

// ---------- ① 异步任务观察者 ----------

func TestWaitDefTerminal(t *testing.T) {
	t.Run("立刻成功：带进度数字", func(t *testing.T) {
		d := taskcenter.Def{Status: func(context.Context) taskcenter.Status {
			return taskcenter.Status{State: taskcenter.StateSucceeded, Message: "完成",
				Progress: &taskcenter.Progress{Total: 10, Done: 9, Failed: 1}}
		}}
		out, ok := waitDefTerminal(context.Background(), d, time.Millisecond)
		if !ok || out.State != taskcenter.StateSucceeded {
			t.Fatalf("应成功: ok=%v out=%+v", ok, out)
		}
		if out.Total != 10 || out.Done != 9 || out.Failed != 1 {
			t.Fatalf("进度数字应带出: %+v", out)
		}
	})

	t.Run("先运行后成功", func(t *testing.T) {
		calls := 0
		d := taskcenter.Def{Status: func(context.Context) taskcenter.Status {
			calls++
			if calls < 2 {
				return taskcenter.Status{State: taskcenter.StateRunning,
					Progress: &taskcenter.Progress{Total: 5, Done: 1}, UpdatedAt: time.Now().UnixMilli()}
			}
			return taskcenter.Status{State: taskcenter.StateSucceeded, Message: "完成"}
		}}
		out, ok := waitDefTerminal(context.Background(), d, time.Millisecond)
		if !ok || out.State != taskcenter.StateSucceeded || calls < 2 {
			t.Fatalf("应轮询到成功: calls=%d out=%+v", calls, out)
		}
	})

	t.Run("进度停滞 → 失败收尾（不白等到超时）", func(t *testing.T) {
		stale := time.Now().Add(-time.Hour).UnixMilli() // 一小时没更新
		d := taskcenter.Def{
			WatchStall:   time.Minute,
			WatchTimeout: time.Hour,
			Status: func(context.Context) taskcenter.Status {
				return taskcenter.Status{State: taskcenter.StateRunning, UpdatedAt: stale}
			},
		}
		out, ok := waitDefTerminal(context.Background(), d, time.Millisecond)
		if !ok || out.State != taskcenter.StateFailed {
			t.Fatalf("停滞应判失败: %+v", out)
		}
		if !strings.Contains(out.Error, "stalled") {
			t.Fatalf("失败原因应说明停滞: %q", out.Error)
		}
	})

	t.Run("超时 → 失败收尾并说明后台可能仍在跑", func(t *testing.T) {
		d := taskcenter.Def{
			WatchTimeout: 30 * time.Millisecond,
			WatchStall:   time.Hour,
			Status: func(context.Context) taskcenter.Status {
				// 一直在推进（UpdatedAt 每轮更新）⇒ 不会命中停滞，只能靠超时
				return taskcenter.Status{State: taskcenter.StateRunning, UpdatedAt: time.Now().UnixMilli()}
			},
		}
		out, ok := waitDefTerminal(context.Background(), d, 5*time.Millisecond)
		if !ok || out.State != taskcenter.StateFailed {
			t.Fatalf("超时应判失败: %+v", out)
		}
		if !strings.Contains(out.Reason, "timeout") {
			t.Fatalf("原因应含 timeout: %q", out.Reason)
		}
	})

	t.Run("ctx 取消（停机）→ 立刻返回，不挂住", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		d := taskcenter.Def{Status: func(context.Context) taskcenter.Status {
			return taskcenter.Status{State: taskcenter.StateRunning}
		}}
		out, ok := waitDefTerminal(ctx, d, time.Millisecond)
		if !ok || out.State != taskcenter.StateFailed {
			t.Fatalf("取消应按失败收尾: %+v", out)
		}
	})

	t.Run("没有状态探针 → ok=false（调用方立即收尾）", func(t *testing.T) {
		if _, ok := waitDefTerminal(context.Background(), taskcenter.Def{}, time.Millisecond); ok {
			t.Fatal("无 Status 应返回 ok=false")
		}
	})
}

func TestWatchDefaults(t *testing.T) {
	if got := (taskcenter.Def{}).WatchTimeoutOrDefault(); got != taskcenter.DefaultWatchTimeout {
		t.Fatalf("默认超时应为 %s, got %s", taskcenter.DefaultWatchTimeout, got)
	}
	if got := (taskcenter.Def{}).WatchStallOrDefault(); got != taskcenter.DefaultWatchStall {
		t.Fatalf("默认停滞应为 %s, got %s", taskcenter.DefaultWatchStall, got)
	}
	custom := taskcenter.Def{WatchTimeout: 48 * time.Hour, WatchStall: time.Minute}
	if custom.WatchTimeoutOrDefault() != 48*time.Hour || custom.WatchStallOrDefault() != time.Minute {
		t.Fatal("自定义值应生效")
	}
}

// ---------- ② 豆瓣两任务的状态归属 ----------

func TestDoubanStatusFor_Attribution(t *testing.T) {
	defer setDoubanBatchProgress(func(p *DoubanBatchProgress) { *p = DoubanBatchProgress{} })

	svc := &FilmService{}
	score := svc.doubanStatusFor("douban_score")
	backfill := svc.doubanStatusFor("douban_backfill")

	// 评分同步在跑
	setDoubanBatchProgress(func(p *DoubanBatchProgress) {
		*p = DoubanBatchProgress{State: "running", Task: "douban_score",
			Total: 100, Done: 30, StartedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli()}
	})
	if st := score(context.Background()); st.State != taskcenter.StateRunning || st.Progress == nil {
		t.Fatalf("评分同步应运行中并带进度: %+v", st)
	}
	if st := backfill(context.Background()); st.State != taskcenter.StateIdle || st.Progress != nil {
		t.Fatalf("另一条任务必须空闲、且不能透传别人的进度: %+v", st)
	}

	// 换成欠账补查在跑 → 归属反过来
	setDoubanBatchProgress(func(p *DoubanBatchProgress) {
		*p = DoubanBatchProgress{State: "running", Task: "douban_backfill", Total: 50, Done: 1}
	})
	if st := score(context.Background()); st.State != taskcenter.StateIdle {
		t.Fatalf("评分同步此时应空闲: %+v", st)
	}
	if st := backfill(context.Background()); st.State != taskcenter.StateRunning {
		t.Fatalf("欠账补查应运行中: %+v", st)
	}
}

// ---------- ③ cron 周期归一 ----------

func TestNormalizeCronSchedules(t *testing.T) {
	crons := &fakeCrons{list: []entity.CronTask{
		{Id: 1, Spec: "0 */20 * * * ?", State: entity.StateEnabled},                          // 可转换
		{Id: 2, Spec: "0 5 */2 * * ?", State: entity.StateEnabled},                           // 步长 → 转不动
		{Id: 3, Spec: "0 0 4 * * 0", Schedule: "weekly:7 04:00", State: entity.StateEnabled}, // 已归一
	}}
	svc := &TaskCenterService{crons: crons, reg: taskcenter.NewRegistry()}

	svc.NormalizeCronSchedules(context.Background())

	byID := map[int64]entity.CronTask{}
	for _, c := range crons.list {
		byID[c.Id] = c
	}
	if byID[1].Schedule != "every:20m" {
		t.Fatalf("#1 应转成 every:20m, got %q", byID[1].Schedule)
	}
	if byID[2].Schedule != "" || byID[2].State != entity.StateDisabled {
		t.Fatalf("#2 转不动应保持空周期并停用: %+v", byID[2])
	}
	if !strings.Contains(byID[2].Remark, "待人工") {
		t.Fatalf("#2 应标注待人工配置, got remark=%q", byID[2].Remark)
	}
	if byID[3].Schedule != "weekly:7 04:00" {
		t.Fatalf("已归一的不该被改: %+v", byID[3])
	}

	// 幂等：再跑一次不应再写库（#1 已转换、#3 已跳过、#2 已标注过）
	before := crons.upsert
	svc.NormalizeCronSchedules(context.Background())
	if crons.upsert != before {
		t.Fatalf("重复执行应幂等, upsert %d → %d", before, crons.upsert)
	}
}

// ---------- ④ 台账保留期 ----------

func TestPruneHistory_UsesConfiguredRetention(t *testing.T) {
	runs := &fakeRuns{}
	svc := &TaskCenterService{runs: runs, retentionDays: 7}

	before := time.Now().AddDate(0, 0, -7).UnixMilli()
	if err := svc.PruneHistory(context.Background()); err != nil {
		t.Fatalf("PruneHistory: %v", err)
	}
	if runs.pruneCalls != 1 {
		t.Fatalf("应调用一次 PruneBefore, got %d", runs.pruneCalls)
	}
	// 允许 5 秒误差（now 在调用前后各取一次）
	if d := runs.pruneBefore - before; d > 5000 || d < -5000 {
		t.Fatalf("保留期应取配置的 7 天: diff=%dms", d)
	}

	// 未配置时回落 30 天（不能用 0，否则会删光台账）
	runs2 := &fakeRuns{}
	svc2 := &TaskCenterService{runs: runs2}
	_ = svc2.PruneHistory(context.Background())
	want30 := time.Now().AddDate(0, 0, -30).UnixMilli()
	if d := runs2.pruneBefore - want30; d > 5000 || d < -5000 {
		t.Fatalf("未配置应回落 30 天: diff=%dms", d)
	}
}

// ---------- 其他小入口 ----------

func TestNextRuns(t *testing.T) {
	svc := &TaskCenterService{reg: taskcenter.NewRegistry()}
	s := NewTaskScheduler(svc)
	svc.attachScheduler(s)
	at := time.Now().Add(time.Hour).Truncate(time.Second)
	s.next["builtin:probe"] = at

	got := svc.nextRuns()
	if got["builtin:probe"] != at.UnixMilli() {
		t.Fatalf("应返回下次触发时刻, got %v", got)
	}
	// 未启动调度器 → 空 map，不 panic
	empty := (&TaskCenterService{}).nextRuns()
	if len(empty) != 0 {
		t.Fatalf("未启动应返回空: %v", empty)
	}
}

func TestEnsureTaskKeyRegistered(t *testing.T) {
	svc := &TaskCenterService{reg: newRegistryWithBuiltins()}
	if d, err := svc.EnsureTaskKeyRegistered("builtin:backdrop"); err != nil || d.Key != "builtin:backdrop" {
		t.Fatalf("已知任务应返回定义: %+v %v", d, err)
	}
	if _, err := svc.EnsureTaskKeyRegistered("builtin:not-exist"); err == nil {
		t.Fatal("未知任务应报错")
	}
	// 与调度器/视图共用同一个"生效周期"口径
	d, _ := svc.EnsureTaskKeyRegistered("builtin:backdrop")
	if taskcenter.ValidateSchedule(d.DefaultSchedule) != nil {
		t.Fatalf("默认周期必须是合法友好文本: %q", d.DefaultSchedule)
	}
}

// 豆瓣批次节奏：配置的「接口间隔」必须真的折成抖动区间（此前只存库不生效），
// 低于下限要抬到 3s（否则调快=自寻封禁）。
func TestDoubanJitterFor(t *testing.T) {
	cases := []struct {
		ms       int
		min, max time.Duration
	}{
		{10000, 8 * time.Second, 12 * time.Second}, // 默认 10s ±20%
		{30000, 24 * time.Second, 36 * time.Second},
		{3000, 2400 * time.Millisecond, 3600 * time.Millisecond}, // 正好下限
		{1000, 2400 * time.Millisecond, 3600 * time.Millisecond}, // 低于下限 → 按 3s
		{0, 2400 * time.Millisecond, 3600 * time.Millisecond},
	}
	for _, c := range cases {
		mn, mx := DoubanJitterFor(c.ms)
		if mn != c.min || mx != c.max {
			t.Fatalf("DoubanJitterFor(%d) = (%s,%s), want (%s,%s)", c.ms, mn, mx, c.min, c.max)
		}
	}
}

// 台账状态必须用 entity 的取值（success/failed），不能把 taskcenter 的 "succeeded" 写进去：
// 否则历史页认不出、今日成功不计、PruneBefore 也永不清理（2026-11-26 审查发现）。
func TestWatchBuiltinRun_LedgerStatus(t *testing.T) {
	repo := &fakeTaskRunRepo{}
	led := NewTaskRunService(repo)
	svc := &TaskCenterService{ledgerSvc: led, reg: newRegistryWithBuiltins()}

	// 成功：状态=success、error 必须为空
	id := led.Start(entity.TaskRunKindManual, "douban_score", 0, "", "手动执行 · 豆瓣评分同步", 0)
	if id == 0 {
		t.Fatal("建行失败")
	}
	svc.watchBuiltinRun(id, taskcenter.Def{
		Key: "builtin:douban_score", Type: "douban_score", Async: true,
		Status: func(context.Context) taskcenter.Status {
			return taskcenter.Status{State: taskcenter.StateSucceeded, Message: "完成"}
		},
	})
	var got entity.TaskRun
	for _, r := range repo.rows {
		if r.Id == id {
			got = r
		}
	}
	if got.Status != entity.TaskRunSuccess {
		t.Fatalf("成功行状态必须是 success（不是 succeeded）, got %q", got.Status)
	}
	if got.Error != "" {
		t.Fatalf("成功行不该有 error 文案, got %q", got.Error)
	}

	// 失败：状态=failed 且原因落到 error
	id2 := led.Start(entity.TaskRunKindManual, "douban_backfill", 0, "", "手动执行 · 补查", 0)
	svc.watchBuiltinRun(id2, taskcenter.Def{
		Key: "builtin:douban_backfill", Type: "douban_backfill", Async: true,
		Status: func(context.Context) taskcenter.Status {
			return taskcenter.Status{State: taskcenter.StateFailed, Error: "subject_ip_rate_limit"}
		},
	})
	got = entity.TaskRun{}
	for _, r := range repo.rows {
		if r.Id == id2 {
			got = r
		}
	}
	if got.Status != entity.TaskRunFailed || got.Error == "" {
		t.Fatalf("失败行状态/原因不对: status=%q error=%q", got.Status, got.Error)
	}
}

// 用户要求：多接口任务"已完成"时，列表也要显示总数/完成数（不能只有运行中才有进度）。
func TestDoubanStatus_KeepsProgressWhenDone(t *testing.T) {
	defer setDoubanBatchProgress(func(p *DoubanBatchProgress) { *p = DoubanBatchProgress{} })
	svc := &FilmService{}
	probe := svc.doubanStatusFor("douban_backfill")

	setDoubanBatchProgress(func(p *DoubanBatchProgress) {
		*p = DoubanBatchProgress{State: "done", Task: "douban_backfill", Total: 2000, Done: 1998, Failed: 2}
	})
	st := probe(context.Background())
	if st.State != taskcenter.StateSucceeded {
		t.Fatalf("应为 succeeded: %+v", st)
	}
	if st.Progress == nil || st.Progress.Total != 2000 || st.Progress.Done != 1998 {
		t.Fatalf("完成态也要带进度数字: %+v", st.Progress)
	}
}

// 安静点（发版前）：已按长任务暂停的豆瓣批次**不能**算在 runningBuiltin 里，
// 否则脚本会白等到超时（暂停中的 goroutine 仍活着、busy 仍为 true）。
func TestControlState_ExcludesPausedLong(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()

	reg := taskcenter.NewRegistry()
	reg.Register(taskcenter.Def{Key: "builtin:douban_score", Type: "douban_score",
		Kind: taskcenter.KindBuiltin, Enabled: true, Run: func(context.Context) error { return nil }})
	reg.Register(taskcenter.Def{Key: "builtin:backdrop", Type: "backdrop",
		Kind: taskcenter.KindBuiltin, Enabled: true, Run: func(context.Context) error { return nil }})
	svc := &TaskCenterService{reg: reg, film: &FilmService{}}
	sched := NewTaskScheduler(svc)
	svc.attachScheduler(sched)

	// 模拟：豆瓣在跑（busy）+ 横图在跑（busy）+ 豆瓣已被暂停
	sched.mu.Lock()
	sched.busy["builtin:douban_score"] = true
	sched.busy["builtin:backdrop"] = true
	sched.mu.Unlock()
	doubanBatchPaused.Store(true)
	defer doubanBatchPaused.Store(false)

	st, err := svc.ControlState(ctx)
	if err != nil {
		t.Fatalf("ControlState: %v", err)
	}
	if len(st.PausedLong) == 0 {
		t.Fatal("应报告已暂停的长任务")
	}
	for _, k := range st.RunningBuiltin {
		if k == "builtin:douban_score" {
			t.Fatalf("已暂停的长任务不该出现在 runningBuiltin: %v", st.RunningBuiltin)
		}
	}
	found := false
	for _, k := range st.RunningBuiltin {
		if k == "builtin:backdrop" {
			found = true
		}
	}
	if !found {
		t.Fatalf("仍在跑的短任务必须留在 runningBuiltin: %v", st.RunningBuiltin)
	}
}

// 重启把批次打断后：resume 只关窗不恢复任何批次，但**必须**告诉人"断点还在，手动执行可续"
func TestResumeAll_ReportsOrphanCursor(t *testing.T) {
	initTestCache(t)
	ctx := context.Background()
	svc := &TaskCenterService{reg: newRegistryWithBuiltins(), film: &FilmService{}}

	// 模拟重启后的状态：不是 busy、没在跑，但游标还在
	saveDoubanCursor(ctx, doubanCursor{Task: "douban_backfill", Total: 2000, Done: 300})

	rep, err := svc.ResumeAll(ctx)
	if err != nil {
		t.Fatalf("ResumeAll: %v", err)
	}
	var hit *TaskGateItem
	for i := range rep.Items {
		if rep.Items[i].Key == "builtin:douban_backfill" {
			hit = &rep.Items[i]
		}
	}
	if hit == nil {
		t.Fatalf("有断点却什么都没报（操作者会以为一切正常）: %+v", rep.Items)
	}
	if hit.Action != "skipped" || !strings.Contains(hit.Reason, "断点") {
		t.Fatalf("应回报 skipped 且说明断点保留, got %+v", *hit)
	}
}

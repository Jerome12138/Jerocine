package service

// 统一任务中心 · 调度层（Phase 2 收敛）
//
// 背景：内置任务原先各自 `go func + ticker`，散在 5 个服务里，既看不到也改不了周期。
// 本调度器把它们收敛成**一个 ticker**：每 30s 检查注册表，谁到点就触发谁，
// 并统一写 task_run 台账（这样"历史记录"能覆盖全部任务类型）。
//
// 与旧 ticker 的关系：旧 Start 方法保留（便于回滚），但 main 不再调用；
// 各服务内部的 due/backoff/幂等语义原样保留，调度器只负责"什么时候叫它"。

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"server/internal/cache"
	"server/internal/domain/entity"
	"server/internal/errclass"
	"server/internal/taskcenter"
)

// 轮询间隔默认值（可被 TaskCenterService.tickEvery / TASK_TICK_SECONDS 覆盖）
const schedulerTickEvery = defaultSchedulerTick

// 调度器只依赖**三个能力接口**，不再直接抓 *TaskCenterService 的私有字段。
//
// 为什么这么切：调度器的本质是"读任务清单 → 到点 → 调执行 → 记台账"，
// 它不需要知道任务中心是怎么拼出清单的、台账写到哪里。切开之后：
//
//	· 新增任务来源（比如以后多个库/多租户）只需要实现 Catalog；
//	· tick 逻辑可以用纯替身单测（见 task_center_scheduler_fake_test.go），不必构造半个服务。
type (
	// SchedCatalog 任务清单 + 用户配置（只读）。
	SchedCatalog interface {
		Definitions() []taskcenter.Def
		Settings(ctx context.Context) map[string]entity.TaskSetting
		Crons(ctx context.Context) ([]entity.CronTask, error)
	}
	// SchedRunner 实际执行能力。
	SchedRunner interface {
		RunDefinition(ctx context.Context, d taskcenter.Def) error
		RunCronRound(ctx context.Context, t entity.CronTask) error
	}
	// SchedLedger 台账写入口（可为 nil：未装配时降级为不写台账）。
	SchedLedger interface {
		Start(kind, taskType string, cronID int64, sourceID, name string, hours int) int64
		Finish(id int64, status string, total, done, failed int, message, errMsg string)
		Record(kind, taskType string, cronID int64, sourceID, name, status string,
			total, done, failed int, message, errMsg string) int64
	}
)

// TaskScheduler 统一调度器（依赖三个接口，不依赖具体服务）。
type TaskScheduler struct {
	cat SchedCatalog
	run SchedRunner
	led SchedLedger

	tickEvery time.Duration
	// attach 启动时回调（把自身挂到任务中心上，供只读诊断接口用）；可为 nil。
	attach func(*TaskScheduler)

	mu   sync.Mutex
	next map[string]time.Time // 每个任务的下次触发时间（内存态；重启后重新按 schedule 计算）
	busy map[string]bool      // 单飞：同一任务不并发
	// gateLogged 维护窗口的日志节流：只在"开→关/关→开"变化时打一行，
	// 否则 30s 一 tick 会把日志刷爆（窗口可能开几十分钟）。
	gateLogged bool
}

// NewTaskScheduler 生产装配：TaskCenterService 同时实现三个接口。
func NewTaskScheduler(tc *TaskCenterService) *TaskScheduler {
	var led SchedLedger
	if l := tc.ledger(); l != nil { // 避免"nil 指针装进接口"后调用 panic
		led = l
	}
	return newTaskScheduler(tc, tc, led, tc.schedulerTick(), tc.attachScheduler)
}

// newTaskScheduler 内部构造（测试可注入纯替身）。
func newTaskScheduler(cat SchedCatalog, run SchedRunner, led SchedLedger,
	tick time.Duration, attach func(*TaskScheduler)) *TaskScheduler {
	if tick <= 0 {
		tick = schedulerTickEvery
	}
	return &TaskScheduler{
		cat: cat, run: run, led: led, tickEvery: tick, attach: attach,
		next: map[string]time.Time{}, busy: map[string]bool{},
	}
}

// Start 启动调度循环（ctx 取消即退出）。
func (s *TaskScheduler) Start(ctx context.Context) {
	// 挂到任务中心上，供只读诊断接口（GET /task-center/schedule）读"下次什么时候跑"
	if s.attach != nil {
		s.attach(s)
	}
	go func() {
		t := time.NewTicker(s.tickEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("[task-scheduler] 退出")
				return
			case <-t.C:
				s.tick(ctx)
			}
		}
	}()
}

// effective 合成"实际生效的启用态与周期"：task_setting 覆盖注册表默认值。
// effective 生效的(启用位, 周期) —— 合并逻辑单点在 taskcenter.Resolve。
func (s *TaskScheduler) effective(d taskcenter.Def, settings map[string]entity.TaskSetting) (bool, string) {
	rc := taskcenter.Resolve(d, userConfigOf(settings[d.Key]))
	return rc.Enabled, rc.Schedule
}

// userConfigOf 把 task_setting 行映射成规则层的 UserConfig。
// 语义：**没有行**（TaskKey 为空）= 用户什么都没改（不能用零值 false 当成"用户停用了"）。
func userConfigOf(set entity.TaskSetting) taskcenter.UserConfig {
	if set.TaskKey == "" {
		return taskcenter.UserConfig{Remark: set.Remark}
	}
	en := set.Enabled // 拷一份：UserConfig 用指针表达"用户给过"
	return taskcenter.UserConfig{Enabled: &en, Schedule: set.Schedule, IntervalMs: set.IntervalMs, Remark: set.Remark}
}

func (s *TaskScheduler) tick(ctx context.Context) {
	// 维护窗口（POST /manage/task-center/pause）：整轮不触发 —— 部署/迁移期间要的是安静窗口，
	// 边迁移边开新采集是最坏情况。
	// 注意语义：窗口期内 s.next **不前进**，所以窗口关闭后的第一个 tick 会把"错过的轮次"补跑**一次**
	// （每个任务最多一次，不会堆积：next 下次就按 now 重算了）。这是有意的（别把一整天的采集都跳过），
	// 但也意味着 resume 之后会有一小波集中触发 —— 部署脚本把它放在清缓存之后、作为最后一步。
	if s.gateBlocked(ctx) {
		return
	}

	settings := s.cat.Settings(ctx)
	now := time.Now()

	for _, d := range s.cat.Definitions() {
		if d.Run == nil {
			continue
		}
		enabled, schedule := s.effective(d, settings)
		if !enabled || schedule == "" {
			continue
		}

		s.mu.Lock()
		nextAt, known := s.next[d.Key]
		if !known {
			// 首次见到：按周期排到**下一个**时间点，不立刻触发（避免重启瞬间全量开跑）
			s.next[d.Key] = taskcenter.NextRun(schedule, now)
			s.mu.Unlock()
			continue
		}
		due := !now.Before(nextAt)
		if due {
			s.next[d.Key] = taskcenter.NextRun(schedule, now)
		}
		busy := s.busy[d.Key]
		s.mu.Unlock()

		if due && !busy {
			go s.runOne(ctx, d)
		}
	}

	// ---- 采集定时任务：同样由中心调度（时间语义已统一为友好文本）----
	s.tickCron(ctx, now)
}

// gateBlocked 维护窗口是否开着（开着则整轮不触发），并做日志节流。
func (s *TaskScheduler) gateBlocked(ctx context.Context) bool {
	reason := describeGate(ctx) // 与 GuardTrigger 同源（判断单点在 task_trigger.go）
	open := reason != ""
	s.mu.Lock()
	changed := open != s.gateLogged
	s.gateLogged = open
	s.mu.Unlock()
	if !changed {
		return open
	}
	if open {
		log.Printf("[task-scheduler] 进入维护窗口, 暂停全部触发: %s", reason)
	} else {
		log.Printf("[task-scheduler] 维护窗口已关闭, 恢复正常调度")
	}
	return open
}

// tickCron 采集定时任务的调度分支。每条都读 cron_task.schedule（友好文本），
// 到点调 spider.RunCronTask；台账由采集侧自己写（避免双重记账）。
func (s *TaskScheduler) tickCron(ctx context.Context, now time.Time) {
	crons, err := s.cat.Crons(ctx)
	if err != nil {
		return
	}
	for i := range crons {
		t := crons[i]
		if t.State != entity.StateEnabled || t.Schedule == "" {
			continue // 未启用 / 周期待人工配置
		}
		key := taskcenter.RefCron(t.Id)

		s.mu.Lock()
		nextAt, known := s.next[key]
		if !known {
			s.next[key] = taskcenter.NextRun(t.Schedule, now)
			s.mu.Unlock()
			continue
		}
		due := !now.Before(nextAt)
		if due {
			s.next[key] = taskcenter.NextRun(t.Schedule, now)
		}
		busy := s.busy[key]
		s.mu.Unlock()

		if due && !busy {
			go s.runCron(ctx, t)
		}
	}
}

// runCron 触发一轮采集任务（台账在 executeTask 内写）。
func (s *TaskScheduler) runCron(ctx context.Context, t entity.CronTask) {
	key := taskcenter.RefCron(t.Id)
	s.mu.Lock()
	if s.busy[key] {
		s.mu.Unlock()
		return
	}
	s.busy[key] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy[key] = false
		s.mu.Unlock()
	}()

	start := time.Now()
	task := t
	err := s.run.RunCronRound(ctx, task)
	if err != nil {
		log.Printf("[task-scheduler] %s 跳过/失败: %v", key, err)
		return
	}
	log.Printf("[task-scheduler] %s 完成 · 耗时 %s", key, time.Since(start).Round(time.Second))
}

// runOne 触发一次并写台账（失败只记日志，绝不影响调度循环）。
func (s *TaskScheduler) runOne(ctx context.Context, d taskcenter.Def) {
	s.mu.Lock()
	if s.busy[d.Key] {
		s.mu.Unlock()
		return
	}
	s.busy[d.Key] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy[d.Key] = false
		s.mu.Unlock()
	}()

	start := time.Now()
	// 台账策略（2026-11-25 用户要求）：
	//   · 普通任务：开始即落"运行中"，结束时收尾；
	//   · 高频任务(QuietOnSuccess)：**先跑，失败才补落一条失败台账**（成功不留痕，避免刷爆）
	// 写入统一走 TaskRunService（与采集任务同一条实现）—— 此前调度器直连 repo，两套口径。
	led := s.led
	runID := int64(0)
	if led != nil && !d.QuietOnSuccess {
		runID = led.Start(entity.TaskRunKindBuiltin, d.Type, 0, "", d.Name, 0)
	}

	err := s.run.RunDefinition(ctx, d)
	cost := time.Since(start).Round(time.Second)
	// 轻量留痕：QuietOnSuccess 的任务不写台账，但列表要看得到"上次运行"
	cache.TaskLastRunSet(context.WithoutCancel(ctx), d.Type, time.Now().UnixMilli(), err == nil)

	if led != nil && d.QuietOnSuccess && err != nil {
		// 高频任务失败：补落一条终态台账，让"异常"页能看见并支持重跑/忽略
		led.Record(entity.TaskRunKindBuiltin, d.Type, 0, "", d.Name,
			entity.TaskRunFailed, 0, 0, 0, "", errclass.Format(err))
	}

	if runID > 0 {
		switch {
		case err != nil:
			led.Finish(runID, entity.TaskRunFailed, 0, 0, 0, "", errclass.Format(err))
		case d.Async:
			// 异步任务（如豆瓣刷分）：Run 只是"启动后台批"，真正结果要看 Def.Status。
			// 这里不能立刻记成功，否则页面显示"一触发就完成"且看不到进度。
			go s.watchAsync(d, runID)
		default:
			led.Finish(runID, entity.TaskRunSuccess, 0, 0, 0, "完成 · 耗时 "+cost.String(), "")
		}
	}

	if err != nil {
		log.Printf("[task-scheduler] %s(%s) 失败(%s): %v", d.Key, d.Name, cost, err)
		return
	}
	log.Printf("[task-scheduler] %s(%s) 完成 · 耗时 %s", d.Key, d.Name, cost)
}

// safeRunTask 兜 panic：单个任务炸掉不能带崩调度器。
func safeRunTask(ctx context.Context, d taskcenter.Def) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("任务 panic: %v", r)
			log.Printf("[task-scheduler] %s panic: %v", d.Key, r)
		}
	}()
	return d.Run(ctx)
}

// ScheduleItem 一个任务的调度态快照（只读诊断用）。
type ScheduleItem struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Enabled      bool   `json:"enabled"`
	Schedule     string `json:"schedule"`
	ScheduleText string `json:"scheduleText"`
	// NextAt 下次触发时刻（unix 毫秒；0 = 还没算出/已停用）
	NextAt int64 `json:"nextAt"`
	// Running 同进程内是否正有一轮在跑
	Running bool `json:"running"`
}

// Snapshot 调度器内存态快照（next/busy 的真实值 + 生效配置，别处看不到）。
//
// 用途：运维/agent 排查"为什么这个任务没跑" —— 看 NextAt 有没有算出来（0=没算过/已停用）、
// 窗口是不是开着（gateOpen）、以及**生效**的周期（不是注册表默认值）。
func (s *TaskScheduler) Snapshot(ctx context.Context) (items []ScheduleItem, gateOpen bool, tickMs int64) {
	settings := s.cat.Settings(ctx)
	crons := map[string]entity.CronTask{}
	if list, err := s.cat.Crons(ctx); err == nil {
		for _, t := range list {
			crons[taskcenter.RefCron(t.Id)] = t
		}
	}

	// 拷一份再解锁：下面要查库/组装，别持锁
	s.mu.Lock()
	nextCopy := make(map[string]time.Time, len(s.next))
	for k, v := range s.next {
		nextCopy[k] = v
	}
	busyCopy := make(map[string]bool, len(s.busy))
	for k, v := range s.busy {
		busyCopy[k] = v
	}
	gateOpen, tickMs = s.gateLogged, s.tickEvery.Milliseconds()
	s.mu.Unlock()

	items = make([]ScheduleItem, 0, len(s.cat.Definitions())+len(crons))
	seen := make(map[string]bool, len(items))
	for _, d := range s.cat.Definitions() {
		rc := taskcenter.Resolve(d, userConfigOf(settings[d.Key]))
		it := ScheduleItem{
			Key: d.Key, Name: d.Name, Kind: d.Kind,
			Enabled: rc.Enabled, Schedule: rc.Schedule,
			ScheduleText: taskcenter.DescribeSchedule(rc.Schedule),
			Running:      busyCopy[d.Key],
		}
		if at, ok := nextCopy[d.Key]; ok {
			it.NextAt = at.UnixMilli()
		}
		items = append(items, it)
		seen[d.Key] = true
	}
	// 采集任务：只列"调度器已经算过下次时间"的（没算过 = 停用/周期待配置）
	for k, at := range nextCopy {
		if seen[k] {
			continue
		}
		t, ok := crons[k]
		if !ok {
			continue
		}
		items = append(items, ScheduleItem{
			Key: k, Name: cronTaskName(t, map[string]string{}), Kind: taskcenter.KindCollect,
			Enabled: t.State == entity.StateEnabled, Schedule: t.Schedule,
			ScheduleText: taskcenter.DescribeSchedule(t.Schedule),
			NextAt:       at.UnixMilli(), Running: busyCopy[k],
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return items, gateOpen, tickMs
}

// ScheduleSnapshot / ScheduleGate 是任务中心对外暴露的只读诊断入口（handler 用）。
func (s *TaskCenterService) ScheduleSnapshot(ctx context.Context) ([]ScheduleItem, bool, int64) {
	sched := s.scheduler()
	if sched == nil {
		return []ScheduleItem{}, false, 0
	}
	return sched.Snapshot(ctx)
}

// NextRuns 只读"下次触发时刻"（任务 key → unix 毫秒），给列表展示用。
// 未启动/还没算过的任务不在 map 内；调度器为 nil 时返回空。
func (s *TaskScheduler) NextRuns() map[string]int64 {
	if s == nil {
		return map[string]int64{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int64, len(s.next))
	for k, v := range s.next {
		out[k] = v.UnixMilli()
	}
	return out
}

// watchAsync 异步任务的台账收尾：轮询 Def.Status 到终态才 Finish（最长 3 小时）。
// 与手动执行的观察者（TaskCenterService.watchBuiltinRun）同语义，只是这里只有 SchedLedger。
func (s *TaskScheduler) watchAsync(d taskcenter.Def, runID int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[task-scheduler] 异步观察者 panic(%s run=%d): %v", d.Key, runID, r)
		}
	}()
	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Hour)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Second)
		if d.Status == nil {
			s.led.Finish(runID, entity.TaskRunSuccess, 0, 0, 0, "完成（无状态探针）", "")
			return
		}
		st := d.Status(ctx)
		switch st.State {
		case taskcenter.StateSucceeded:
			msg := st.Message
			if msg == "" {
				msg = "完成"
			}
			total, done, failed := progressNums(st)
			s.led.Finish(runID, entity.TaskRunSuccess, total, done, failed, msg, "")
			return
		case taskcenter.StateFailed:
			msg := st.Error
			if msg == "" {
				msg = firstNonEmptyStr(st.Message, "失败")
			}
			total, done, failed := progressNums(st)
			s.led.Finish(runID, entity.TaskRunFailed, total, done, failed, "", msg)
			return
		}
	}
	s.led.Finish(runID, entity.TaskRunFailed, 0, 0, 0, "", "[timeout] 超过 3 小时仍未结束，已按超时收尾")
}

// progressNums 把 Status.Progress 摊成台账的 total/done/failed。
func progressNums(st taskcenter.Status) (total, done, failed int) {
	if st.Progress == nil {
		return 0, 0, 0
	}
	return st.Progress.Total, st.Progress.Done, st.Progress.Failed
}

// firstNonEmptyStr 取第一个非空字符串（台账文案兜底）。
func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

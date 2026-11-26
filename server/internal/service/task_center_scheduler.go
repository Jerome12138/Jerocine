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
	"strconv"
	"sync"
	"time"

	"server/internal/cache"
	"server/internal/domain/entity"
	"server/internal/errclass"
	"server/internal/taskcenter"
)

const schedulerTickEvery = 30 * time.Second

// TaskScheduler 统一调度器。
type TaskScheduler struct {
	tc        *TaskCenterService
	tickEvery time.Duration

	mu   sync.Mutex
	next map[string]time.Time // 每个任务的下次触发时间（内存态；重启后重新按 schedule 计算）
	busy map[string]bool      // 单飞：同一任务不并发
	// gateLogged 维护窗口的日志节流：只在"开→关/关→开"变化时打一行，
	// 否则 30s 一 tick 会把日志刷爆（窗口可能开几十分钟）。
	gateLogged bool
}

func NewTaskScheduler(tc *TaskCenterService) *TaskScheduler {
	return &TaskScheduler{
		tc:        tc,
		tickEvery: schedulerTickEvery,
		next:      map[string]time.Time{},
		busy:      map[string]bool{},
	}
}

// Start 启动调度循环（ctx 取消即退出）。
func (s *TaskScheduler) Start(ctx context.Context) {
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
func (s *TaskScheduler) effective(d taskcenter.Def, settings map[string]entity.TaskSetting) (bool, string) {
	enabled, schedule := d.Enabled, d.DefaultSchedule
	if set, ok := settings[d.Key]; ok {
		enabled = set.Enabled
		if set.Schedule != "" {
			schedule = set.Schedule
		}
	}
	return enabled, schedule
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

	settings := map[string]entity.TaskSetting{}
	if s.tc.settings != nil {
		if m, err := s.tc.settings.List(ctx); err == nil {
			settings = m
		}
	}
	now := time.Now()

	for _, d := range s.tc.reg.All() {
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
	reason := cache.TaskGateBlockReason(ctx)
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
	if s.tc.spider == nil || s.tc.crons == nil {
		return
	}
	crons, err := s.tc.crons.List(ctx)
	if err != nil {
		return
	}
	for i := range crons {
		t := crons[i]
		if t.State != entity.StateEnabled || t.Schedule == "" {
			continue // 未启用 / 周期待人工配置
		}
		key := "cron:" + strconv.FormatInt(t.Id, 10)

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
	key := "cron:" + strconv.FormatInt(t.Id, 10)
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
	err := s.tc.spider.RunCronTask(ctx, &task, entity.TaskRunKindCron)
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
	runID := int64(0)
	// 台账策略（2026-11-25 用户要求）：
	//   · 普通任务：开始即落"运行中"，结束时收尾；
	//   · 高频任务(QuietOnSuccess)：**先跑，失败才补落一条失败台账**（成功不留痕，避免刷爆）
	if s.tc.runs != nil && !d.QuietOnSuccess {
		r := &entity.TaskRun{
			Kind: "builtin", Type: d.Type, Name: d.Name,
			Status: entity.TaskRunRunning, StartedAt: start.UnixMilli(), CreatedAt: start.UnixMilli(),
		}
		if err := s.tc.runs.Create(ctx, r); err == nil {
			runID = r.Id
		}
	}

	err := safeRunTask(ctx, d)
	cost := time.Since(start).Round(time.Second)

	// 高频任务失败：补落一条终态台账，让"异常"页能看见并支持重跑/忽略
	if s.tc.runs != nil && d.QuietOnSuccess && err != nil {
		r := &entity.TaskRun{
			Kind: "builtin", Type: d.Type, Name: d.Name,
			Status: entity.TaskRunFailed, StartedAt: start.UnixMilli(),
			EndedAt: time.Now().UnixMilli(), CreatedAt: start.UnixMilli(),
			Error: errclass.Format(err),
		}
		if cerr := s.tc.runs.Create(ctx, r); cerr != nil {
			log.Printf("[task-scheduler] %s 失败台账写入失败: %v", d.Key, cerr)
		}
	}

	if s.tc.runs != nil && runID > 0 {
		if cur, gerr := s.tc.runs.Get(ctx, runID); gerr == nil && cur != nil {
			cur.EndedAt = time.Now().UnixMilli()
			if err != nil {
				cur.Status = entity.TaskRunFailed
				cur.Error = errclass.Format(err)
			} else {
				cur.Status = entity.TaskRunSuccess
				cur.Message = "完成 · 耗时 " + cost.String()
			}
			if uerr := s.tc.runs.Update(ctx, cur); uerr != nil {
				log.Printf("[task-scheduler] %s 台账收尾失败: %v", d.Key, uerr)
			}
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

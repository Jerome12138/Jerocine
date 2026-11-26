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
	"sync"
	"time"

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
	if s.tc.runs != nil {
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

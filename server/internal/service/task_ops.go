package service

// 任务中心 · 操作面：改配置 / 手动执行 / 重跑 / 忽略 / 批量启停 / 单任务暂停恢复。
// 拆分自 task_center_service.go（P3-1），行为不变。

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"server/internal/cache"
	"server/internal/domain/entity"
	"server/internal/errclass"
	"server/internal/taskcenter"
)

// EnsureTaskKeyRegistered 供 PR2 的写接口复用：key 必须是注册表里的内置任务。
func (s *TaskCenterService) EnsureTaskKeyRegistered(key string) (taskcenter.Def, error) {
	d, ok := s.reg.Get(key)
	if !ok {
		return taskcenter.Def{}, fmt.Errorf("未知任务：%s", key)
	}
	return d, nil
}

var _ = fmt.Sprintf

// ---------------------------------------------------------------------------
// PR2：编辑配置 + 手动执行 / 控制
// ---------------------------------------------------------------------------

// TaskSettingInput 部分更新：用指针区分"未提供"与"显式设为零值"。
type TaskSettingInput struct {
	Enabled    *bool   `json:"enabled"`
	Schedule   *string `json:"schedule"`
	Remark     *string `json:"remark"`
	IntervalMs *int    `json:"intervalMs"`
}

func jobKeyOf(key string) (string, bool) {
	if !strings.HasPrefix(key, "job:") {
		return "", false
	}
	id := strings.TrimPrefix(key, "job:")
	return id, id != ""
}

// UpdateSetting 编辑任务配置（内置任务走 task_setting；cron 任务写 cron_task）。
//
// 返回值 deferred=true 表示"该任务此刻有一轮正在跑"：停用**只影响下次触发**，
// 不中断在跑的这一轮（短任务跑完即停），前端据此对用户说真话 ——
// 否则会出现"点了停用、页面显示已停用、后台还在跑"的假象。
func (s *TaskCenterService) UpdateSetting(ctx context.Context, key string, in TaskSettingInput) (deferred bool, err error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return false, fmt.Errorf("缺少任务标识")
	}

	// ---- 采集(cron) 任务：启用 / 计划 / 备注 ----
	if id, ok := cronIDOf(key); ok {
		if s.crons == nil {
			return false, fmt.Errorf("cron 仓储未装配")
		}
		t, err := s.crons.Get(ctx, id)
		if err != nil {
			return false, err
		}
		if in.IntervalMs != nil {
			return false, fmt.Errorf("采集任务的请求间隔在「采集源」里按源配置，不在这里改")
		}
		if in.Enabled != nil {
			if *in.Enabled {
				t.State = entity.StateEnabled
			} else {
				deferred = s.isTaskRunning(ctx, key)
				t.State = entity.StateDisabled
			}
		}
		if in.Schedule != nil {
			sched := strings.TrimSpace(*in.Schedule)
			if sched == "" {
				return false, fmt.Errorf("执行周期不能为空（停用请用 enabled=false）")
			}
			if verr := taskcenter.ValidateSchedule(sched); verr != nil {
				return false, verr
			}
			t.Schedule = sched // 统一时间语义：写友好文本，不再写 cron 表达式
		}
		if in.Remark != nil {
			t.Remark = strings.TrimSpace(*in.Remark)
		}
		return deferred, s.crons.Upsert(ctx, t)
	}

	// ---- 内置任务 ----
	def, err := s.EnsureTaskKeyRegistered(key)
	if err != nil {
		return false, err
	}
	allowed := make(map[string]bool, len(def.Editable))
	for _, f := range def.Editable {
		allowed[f] = true
	}
	deny := func(field, label string) error {
		return fmt.Errorf("该任务不支持修改%s", label)
	}
	if in.Enabled != nil && !allowed[taskcenter.FieldEnabled] {
		return false, deny("enabled", "启用状态")
	}
	if in.Schedule != nil && !allowed[taskcenter.FieldSchedule] {
		return false, deny("schedule", "执行周期")
	}
	if in.Remark != nil && !allowed[taskcenter.FieldRemark] {
		return false, deny("remark", "备注")
	}
	if in.IntervalMs != nil && !allowed[taskcenter.FieldInterval] {
		return false, deny("interval", "接口间隔")
	}

	if in.Schedule != nil {
		if err := taskcenter.ValidateSchedule(*in.Schedule); err != nil {
			return false, err
		}
	}
	if in.IntervalMs != nil && *in.IntervalMs > 0 && *in.IntervalMs < def.MinIntervalMs {
		return false, fmt.Errorf("接口间隔不能小于 %dms（%s）；这是执行层下限，防止被源站拉黑",
			def.MinIntervalMs, def.IntervalLabel)
	}

	now := time.Now().UnixMilli()
	if s.settings == nil {
		return false, fmt.Errorf("配置仓储未装配")
	}
	// 启用位**每次都显式写一次（写的是合并后的值）**，不再"只在传了 enabled 时才写"。
	// 原因：Upsert 的 INSERT 分支会用实体的零值建行，而实体零值 enabled=false ——
	// "只改周期/备注"这类局部更新会把默认启用的任务建成停用（静默不跑、零报错）。
	// 合并优先级：入参 enabled → 现有行 → 注册表默认。
	enabledNow := def.Enabled
	if cur, ok, gerr := s.settings.Get(ctx, key); gerr != nil {
		return false, gerr
	} else if ok {
		enabledNow = cur.Enabled
	}
	if in.Enabled != nil {
		enabledNow = *in.Enabled
	}
	// deferred 只在"本次请求是要停用"时才算（只改周期时提示"运行中"会误导）
	if in.Enabled != nil && !*in.Enabled {
		deferred = s.isTaskRunning(ctx, key)
	}
	if err := s.settings.SetEnabled(ctx, key, enabledNow, now); err != nil {
		return false, err
	}
	set := entity.TaskSetting{TaskKey: key, UpdatedAt: now, Enabled: enabledNow}
	if in.Schedule != nil {
		set.Schedule = strings.TrimSpace(*in.Schedule)
	}
	if in.Remark != nil {
		set.Remark = strings.TrimSpace(*in.Remark)
	}
	if in.IntervalMs != nil {
		set.IntervalMs = *in.IntervalMs
	}
	// Upsert 只覆盖非空字段（不会清空备注；enabled 由上面的 SetEnabled 负责）
	return deferred, s.settings.Upsert(ctx, set)
}

// isTaskRunning 该任务此刻是否有一轮在跑。
//
// 口径与列表页 state=running 完全一致（用户看到的"运行中"就是这个）：
// 内置任务先看内存状态（如豆瓣批量的 Task 进度），再退回最近一条台账；
// 采集任务看最近一条台账。查不到一律当"没在跑"，避免把停用变成需要等待的操作。
func (s *TaskCenterService) isTaskRunning(ctx context.Context, key string) bool {
	if id, ok := cronIDOf(key); ok {
		// ① 任务级单飞锁在 = 这一轮正在跑。**先看锁再看台账**：锁是在写台账之前抢的
		// （spider.RunCronTask），只看台账会漏掉"刚起轮次、台账还没落"的那一小段，
		// 导致停用时误报"没在跑"（deferred 提示错）。
		if cache.LockHeld(ctx, cache.KeyLockCronTask(id)) {
			return true
		}
		if s.crons == nil || s.runs == nil {
			return false
		}
		latest, err := s.runs.LatestByCron(ctx, []int64{id})
		if err != nil {
			return false
		}
		return latest[id].Status == entity.TaskRunRunning
	}
	def, err := s.EnsureTaskKeyRegistered(key)
	if err != nil {
		return false
	}
	if def.Status != nil {
		if st := def.Status(ctx); st.State == taskcenter.StateRunning {
			return true
		}
	}
	if run, ok := s.latestBuiltinRun(ctx, def.Type); ok {
		return run.Status == entity.TaskRunRunning
	}
	return false
}

// BatchSetEnabled 批量启用/停用（采集任务写 cron_task.state，内置任务写 task_setting.enabled）。
//
// 返回成功条数、**延迟生效的任务 key**（正在跑、等本轮结束才真正停下）与失败明细
// （部分失败不整体回滚：用户能看到哪些没成功）。
func (s *TaskCenterService) BatchSetEnabled(ctx context.Context, keys []string, enabled bool) (int, []string, []string) {
	ok, errs := 0, []string{}
	deferred := []string{}
	now := time.Now().UnixMilli()
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		// 停用前先探"此刻是否在跑"：在跑就记进 deferred，前端如实提示"本轮结束后停止"
		// （停用本身不会中断在跑的那一轮，见 UpdateSetting 注释）
		running := !enabled && s.isTaskRunning(ctx, key)
		if id, isCron := cronIDOf(key); isCron {
			if s.crons == nil {
				errs = append(errs, key+": cron 仓储未装配")
				continue
			}
			t, err := s.crons.Get(ctx, id)
			if err != nil {
				errs = append(errs, key+": "+err.Error())
				continue
			}
			if enabled {
				t.State = entity.StateEnabled
			} else {
				t.State = entity.StateDisabled
			}
			if err := s.crons.Upsert(ctx, t); err != nil {
				errs = append(errs, key+": "+err.Error())
				continue
			}
			if running {
				deferred = append(deferred, key)
			}
			ok++
			continue
		}
		if _, err := s.EnsureTaskKeyRegistered(key); err != nil {
			errs = append(errs, key+": 未知任务")
			continue
		}
		if s.settings == nil {
			errs = append(errs, key+": 配置仓储未装配")
			continue
		}
		if err := s.settings.SetEnabled(ctx, key, enabled, now); err != nil {
			errs = append(errs, key+": "+err.Error())
			continue
		}
		if running {
			deferred = append(deferred, key)
		}
		ok++
	}
	return ok, deferred, errs
}

// RerunRun 按一条台账记录重跑对应任务（异常处理用）。
func (s *TaskCenterService) RerunRun(ctx context.Context, runID int64) error {
	if s.runs == nil {
		return fmt.Errorf("台账仓储未装配")
	}
	r, err := s.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	// ① 状态判断：还在跑的没有"重跑"可言
	if r.Status == entity.TaskRunRunning {
		return fmt.Errorf("该任务仍在运行中，无需重跑")
	}
	// ② 幂等：同一任务已有在执行中的实例时拒绝（重复点"重新执行"不会叠加）
	if n, cerr := s.runs.CountRunningByType(ctx, r.Type, r.CronId); cerr == nil && n > 0 {
		return fmt.Errorf("该任务已有执行中的实例，请等它结束后再重跑")
	}
	// 定时采集任务：按 cron id 重跑（走 TriggerCron，保留其既有限制）
	if r.CronId > 0 {
		return s.RunTask(ctx, taskcenter.RefCron(r.CronId))
	}
	// 内置任务：按 type 反查注册表（同 type 只可能有一个 key）
	for _, d := range s.reg.All() {
		if d.Type == r.Type && d.Run != nil {
			return s.RunTask(ctx, d.Key)
		}
	}
	if r.Type == entity.TaskRunCollect && r.SourceId != "" {
		// 手动单源采集：没有注册表项，直接复用采集入口
		if s.spider == nil {
			return fmt.Errorf("采集服务未装配")
		}
		return s.spider.StartCollect(r.SourceId, r.Hours)
	}
	return fmt.Errorf("该类型（%s）暂不支持重跑", r.Type)
}

// SetRunIgnored 忽略/恢复一条异常台账（不删行，保留审计）。
func (s *TaskCenterService) SetRunIgnored(ctx context.Context, runID int64, ignored bool) error {
	if s.runs == nil {
		return fmt.Errorf("台账仓储未装配")
	}
	return s.runs.SetIgnored(ctx, runID, ignored)
}

// RunTask 手动执行一次（**同样写台账**：手动与定时在历史/异常里一视同仁）。
func (s *TaskCenterService) RunTask(ctx context.Context, key string) error {
	// 维护窗口内不接新活（闸门判断单点在 GuardTrigger）
	if err := GuardTrigger(ctx); err != nil {
		return err
	}
	if id, ok := cronIDOf(key); ok {
		if s.spider == nil {
			return fmt.Errorf("采集服务未装配")
		}
		return s.spider.TriggerCron(ctx, id) // 采集侧自带 task_run(kind=manual)
	}
	def, err := s.EnsureTaskKeyRegistered(key)
	if err != nil {
		return err
	}
	if def.Run == nil {
		return fmt.Errorf("该任务暂不支持手动执行")
	}
	switch key {
	case "builtin:douban_score":
		return s.runManualBuiltin(ctx, def, func(c context.Context) error { return s.film.StartDoubanScoreSyncOnce(c) })
	case "builtin:douban_backfill":
		return s.runManualBuiltin(ctx, def, func(c context.Context) error { return s.film.StartDoubanScoreBackfill(c, 0) })
	case "builtin:domain_fetch":
		return s.runManualBuiltin(ctx, def, func(c context.Context) error {
			bg := context.WithoutCancel(c)
			go func() {
				if _, ferr := s.film.FetchDomainRules(bg, ""); ferr != nil {
					log.Printf("[task-center] 域名公告拉取失败: %v", ferr)
				}
			}()
			return nil
		})
	}
	// 其余内置任务（热度/横图/巡检/年份核实/台账清理）直接调 Run，同步收尾
	return s.runManualBuiltin(ctx, def, def.Run)
}

// runManualBuiltin 执行一次内置任务并写台账：
//   - 同步任务：跑完即收尾（成功/失败都落）；
//   - 异步任务(Async)：先落"运行中"，再由观察者轮询 Status 到终态收尾（最长 3 小时）。
//
// 台账写入统一走 TaskRunService（P2-4 的唯一写入口）—— 之前这里直接调 repo 的
// Create/Get/Update，自己拼实体、自己写 Message/Error，既不截断（超列宽会让收尾失败、
// 行卡在 running 变幽灵），也与调度器那边的口径两套。现在两边同一条实现。
func (s *TaskCenterService) runManualBuiltin(ctx context.Context, def taskcenter.Def, run func(context.Context) error) error {
	led := s.ledger()
	runID := int64(0)
	if led != nil {
		runID = led.Start(entity.TaskRunKindManual, def.Type, 0, "", "手动执行 · "+def.Name, 0)
	}

	err := run(ctx)

	if runID == 0 || led == nil {
		return err
	}
	if def.Async {
		// 启动失败要立刻收尾；启动成功则交给观察者（真跑完才知道结果）
		if err != nil {
			led.Finish(runID, entity.TaskRunFailed, 0, 0, 0, "", errclass.Format(err))
			return err
		}
		go s.watchBuiltinRun(runID, def)
		return nil
	}
	if err != nil {
		led.Finish(runID, entity.TaskRunFailed, 0, 0, 0, "", errclass.Format(err))
		return err
	}
	led.Finish(runID, entity.TaskRunSuccess, 0, 0, 0, "完成", "")
	return nil
}

// watchBuiltinRun 异步任务的手动执行观察者：轮询 Status 到终态后收尾台账。
func (s *TaskCenterService) watchBuiltinRun(runID int64, def taskcenter.Def) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[task-center] 观察者 panic(run=%d): %v", runID, r)
		}
	}()
	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Hour)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Second)
		if def.Status == nil {
			return
		}
		st := def.Status(ctx)
		switch st.State {
		case taskcenter.StateSucceeded:
			s.finishRun(runID, entity.TaskRunSuccess, firstNonEmpty(st.Message, "完成"))
			return
		case taskcenter.StateFailed:
			s.finishRun(runID, entity.TaskRunFailed, firstNonEmpty(st.Error, st.Message, "失败"))
			return
		}
	}
	s.finishRun(runID, entity.TaskRunFailed, "[timeout] 超过 3 小时仍未结束，已按超时收尾")
}

// finishRun 收尾一条手工触发的台账（走唯一写入口；成功写 Message、失败写 Error）。
func (s *TaskCenterService) finishRun(runID int64, status, msg string) {
	led := s.ledger()
	if led == nil || runID <= 0 {
		return
	}
	if status == entity.TaskRunFailed {
		led.Finish(runID, status, 0, 0, 0, "", msg)
		return
	}
	led.Finish(runID, status, 0, 0, 0, msg, "")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// PauseTask / ResumeTask / StopTask：目前只对"正在运行的采集任务"（key=job:<sourceId>）生效。
func (s *TaskCenterService) PauseTask(ctx context.Context, key string) error {
	id, ok := jobKeyOf(key)
	if !ok || s.spider == nil {
		return fmt.Errorf("只有正在运行的采集任务支持暂停")
	}
	return s.spider.PauseJob(ctx, id)
}

func (s *TaskCenterService) ResumeTask(ctx context.Context, key string) error {
	id, ok := jobKeyOf(key)
	if !ok || s.spider == nil {
		return fmt.Errorf("只有暂停中的采集任务支持继续")
	}
	return s.spider.ResumeJob(ctx, id)
}

func (s *TaskCenterService) StopTask(ctx context.Context, key string) error {
	id, ok := jobKeyOf(key)
	if !ok || s.spider == nil {
		return fmt.Errorf("只有正在运行的采集任务支持停止")
	}
	return s.spider.CancelJob(ctx, id)
}

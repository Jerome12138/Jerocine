package service

// 统一任务中心（只读目录）· PR1
//
// 目标：把「采集类（cron 动态展开）」与「内置类（代码注册）」合成一份列表，
// 统一状态与进度口径，供管理端一个页面看全部任务。
//
// 分层约定（详见 docs/功能方案/方案-统一任务中心-2026-11-25.md）：
//   - 调度层 schedule  = 多久跑一次（友好文本，storage 在 task_setting）
//   - 执行层 interval  = 一次运行内相邻外部请求隔多久（有硬下限，防被源站拉黑）
//
// 本 PR 只做"读"：状态来自 task_run 最近一条 / 任务自身的内存进度；
// 手动执行、暂停、落库编辑在 PR2。

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/taskcenter"
)

// TaskView 任务中心的一行。
type TaskView struct {
	Key           string               `json:"key"`
	Kind          string               `json:"kind"`
	Type          string               `json:"type"`
	Name          string               `json:"name"`
	Target        string               `json:"target"`
	Detail        string               `json:"detail,omitempty"`
	Schedule      string               `json:"schedule"`     // 原始（友好文本 / cron 表达式）
	ScheduleText  string               `json:"scheduleText"` // 中文展示
	Enabled       bool                 `json:"enabled"`
	IntervalMs    int                  `json:"intervalMs"`
	MinIntervalMs int                  `json:"minIntervalMs"` // 执行层下限（前端据此提示，低于它会被 422）
	IntervalLabel string               `json:"intervalLabel"`
	Editable      []string             `json:"editable"`
	Caps          []string             `json:"caps"`
	State         string               `json:"state"`
	Progress      *taskcenter.Progress `json:"progress"` // null = 总数未知
	StartedAt     int64                `json:"startedAt"`
	ElapsedMs     int64                `json:"elapsedMs"`
	Message       string               `json:"message"`
	Error         string               `json:"error"`
	LastRunAt     int64                `json:"lastRunAt"`
	// NextAt 下次计划触发时刻（unix 毫秒；0=调度器还没给它算，如停用/周期待配置）。
	// 排查"这个任务怎么没跑"第一眼就看它：有值说明只是还没到点。
	NextAt int64 `json:"nextAt"`
}

// TaskCenterOverview 概览。
type TaskCenterOverview struct {
	Running   int `json:"running"`
	Builtin   int `json:"builtin"`
	Collect   int `json:"collect"`
	Failed24h int `json:"failed24h"`
}

// TaskCenterService 只读聚合。
type TaskCenterService struct {
	film       *FilmService
	crons      repository.CronTaskRepository
	runs       repository.TaskRunRepository
	settings   repository.TaskSettingRepository
	sources    repository.CollectSourceRepository
	spider     *SpiderService
	hot        *HotService
	backdrop   *BackdropService
	manage     *ManageService
	yearVerify *YearVerifyService
	reg        *taskcenter.Registry

	// 台账的**唯一写入口**（与采集任务共用同一条实现）；nil 时降级为不写台账。
	ledgerSvc *TaskRunService

	// 调度器实例（Start 时挂回来），仅用于只读诊断接口；nil = 未启动。
	schedMu sync.RWMutex
	sched   *TaskScheduler

	// 运行时策略（原先写死）：台账保留天数、调度轮询间隔。0 表示用默认值。
	retentionDays int
	tickEvery     time.Duration
}

// 策略默认值（配置未给时兜底；抽出来是为了单测能直接构造 &TaskCenterService{} 而不炸）。
const (
	defaultRetentionDays = 30
	defaultSchedulerTick = 30 * time.Second
)

// TaskCenterOptions 任务中心的运行时策略（来自 config.Tasks）。
type TaskCenterOptions struct {
	RetentionDays int
	TickEvery     time.Duration
	// Ledger 台账写入口（TaskRunService）。给 nil 时任务中心仍可只读浏览，但不写台账。
	Ledger *TaskRunService
}

// ---- 调度器三接口的实现（SchedCatalog / SchedRunner / SchedLedger）----
//
// 调度器只认这三个能力；这里做**薄适配**，不掺杂业务判断。
// 好处：调度器可被纯替身单测；以后换任务来源（多库/多租户）只需另一个 Catalog 实现。

// Definitions 全部内置任务定义（SchedCatalog）。
func (s *TaskCenterService) Definitions() []taskcenter.Def {
	if s == nil || s.reg == nil {
		return nil
	}
	return s.reg.All()
}

// Settings 内置任务的用户配置（SchedCatalog）。读失败降级为空 map（= 全用默认），不阻断调度。
func (s *TaskCenterService) Settings(ctx context.Context) map[string]entity.TaskSetting {
	if s == nil || s.settings == nil {
		return map[string]entity.TaskSetting{}
	}
	m, err := s.settings.List(ctx)
	if err != nil {
		log.Printf("[task-center] 读取任务配置失败（按默认值调度）: %v", err)
		return map[string]entity.TaskSetting{}
	}
	return m
}

// Crons 采集定时任务清单（SchedCatalog）。
func (s *TaskCenterService) Crons(ctx context.Context) ([]entity.CronTask, error) {
	if s == nil || s.crons == nil {
		return nil, nil
	}
	return s.crons.List(ctx)
}

// RunDefinition 跑一个内置任务（SchedRunner）：带 recover，任务自己 panic 不会打挂进程。
//
// 顺带把"接口间隔"真正接进执行：此前 IntervalMs 只存库/只校验、没有任何代码拿它配速
// （用户实测："我调了频次，界面还显示 3~5s"）。豆瓣批次的节奏由客户端抖动控制，
// 这里按**合并后的生效值**在每次触发前设置一次。
func (s *TaskCenterService) RunDefinition(ctx context.Context, d taskcenter.Def) error {
	if d.Key == "builtin:douban_score" || d.Key == "builtin:douban_backfill" {
		if s.film != nil {
			rc := taskcenter.Resolve(d, userConfigOf(s.Settings(ctx)[d.Key]))
			s.film.SetDoubanJitter(rc.IntervalMs)
		}
	}
	return safeRunTask(ctx, d)
}

// RunCronRound 跑一轮采集定时任务（SchedRunner）：复用采集侧 RunCronTask（单飞锁/轮次标记都在里面）。
func (s *TaskCenterService) RunCronRound(ctx context.Context, t entity.CronTask) error {
	if s == nil || s.spider == nil {
		return fmt.Errorf("采集服务未装配")
	}
	task := t
	return s.spider.RunCronTask(ctx, &task, entity.TaskRunKindCron)
}

// attachScheduler 由调度器 Start 时回调挂上（只读诊断用）。
func (s *TaskCenterService) attachScheduler(sched *TaskScheduler) {
	if s == nil {
		return
	}
	s.schedMu.Lock()
	s.sched = sched
	s.schedMu.Unlock()
}

// scheduler 当前调度器（未启动返回 nil）。
func (s *TaskCenterService) scheduler() *TaskScheduler {
	if s == nil {
		return nil
	}
	s.schedMu.RLock()
	defer s.schedMu.RUnlock()
	return s.sched
}

// runningBuiltins 当前在跑的内置任务 key（调度器 busy 快照）。
func (s *TaskCenterService) runningBuiltins() []string {
	if sched := s.scheduler(); sched != nil {
		return sched.Running()
	}
	return nil
}

// nextRuns 调度器内存里的"下次触发时刻"快照（未启动时为空 map）。
func (s *TaskCenterService) nextRuns() map[string]int64 {
	if sched := s.scheduler(); sched != nil {
		return sched.NextRuns()
	}
	return map[string]int64{}
}

// ledger 台账写入口（可能为 nil：未注入时各调用点自行跳过）。
func (s *TaskCenterService) ledger() *TaskRunService {
	if s == nil {
		return nil
	}
	return s.ledgerSvc
}

// retention 实际生效的台账保留天数。
func (s *TaskCenterService) retention() int {
	if s != nil && s.retentionDays > 0 {
		return s.retentionDays
	}
	return defaultRetentionDays
}

// schedulerTick 实际生效的调度轮询间隔。
func (s *TaskCenterService) schedulerTick() time.Duration {
	if s != nil && s.tickEvery > 0 {
		return s.tickEvery
	}
	return defaultSchedulerTick
}

func NewTaskCenterService(
	film *FilmService,
	crons repository.CronTaskRepository,
	runs repository.TaskRunRepository,
	settings repository.TaskSettingRepository,
	sources repository.CollectSourceRepository,
	spider *SpiderService,
	hot *HotService,
	backdrop *BackdropService,
	manage *ManageService,
	yearVerify *YearVerifyService,
	opts TaskCenterOptions,
) *TaskCenterService {
	s := &TaskCenterService{film: film, crons: crons, runs: runs, settings: settings, sources: sources,
		spider: spider, hot: hot, backdrop: backdrop, manage: manage, yearVerify: yearVerify,
		retentionDays: opts.RetentionDays, tickEvery: opts.TickEvery,
		ledgerSvc: opts.Ledger,
		reg:       taskcenter.NewRegistry()}
	s.registerBuiltins()
	return s
}

// registerBuiltins 装配内置任务定义：**定义本身由各域服务提供**（task_defs.go），
// 任务中心只做注册。新增任务 = 改所属服务的 TaskDefs()，不必动这里。
//
// 注意：这里只描述任务，不接管调度；调度由 TaskScheduler 按 schedule 触发。
func (s *TaskCenterService) registerBuiltins() {
	// 各域服务可能未装配（单测/精简装配）——按非 nil 逐个装配，缺谁不影响其它任务
	if s.film != nil {
		s.registerDefs(s.film.TaskDefs())
	}
	if s.hot != nil {
		s.registerDefs(s.hot.TaskDefs())
	}
	if s.backdrop != nil {
		s.registerDefs(s.backdrop.TaskDefs())
	}
	if s.manage != nil {
		s.registerDefs(s.manage.TaskDefs())
	}
	if s.yearVerify != nil {
		s.registerDefs(s.yearVerify.TaskDefs())
	}
	// 任务中心自己的内务任务（台账清理，依赖 PruneHistory）
	s.registerDefs(s.ownTaskDefs())
}

func (s *TaskCenterService) registerDefs(defs []taskcenter.Def) {
	for _, d := range defs {
		s.reg.Register(d)
	}
}

// statusFromRun 把一条台账映射成任务状态（与 cron 任务同一套口径）。
func statusFromRun(r entity.TaskRun) taskcenter.Status {
	st := taskcenter.Status{Message: r.Message, Error: r.Error, StartedAt: r.StartedAt}
	switch r.Status {
	case entity.TaskRunRunning:
		st.State = taskcenter.StateRunning
	case entity.TaskRunSuccess:
		st.State = taskcenter.StateSucceeded
	case entity.TaskRunFailed:
		st.State = taskcenter.StateFailed
	case entity.TaskRunCanceled:
		st.State = taskcenter.StateCanceled
	default:
		st.State = taskcenter.StateIdle
	}
	if r.EndedAt > 0 && r.StartedAt > 0 {
		st.ElapsedMs = r.EndedAt - r.StartedAt
	} else if st.State == taskcenter.StateRunning && r.StartedAt > 0 {
		st.ElapsedMs = time.Now().UnixMilli() - r.StartedAt
	}
	return st
}

// cronIDOf "cron:<id>" → id（解析单点在 taskcenter.ParseCronRef）。
func cronIDOf(key string) (int64, bool) { return taskcenter.ParseCronRef(key) }

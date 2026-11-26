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
	"sort"
	"strings"
	"sync"
	"time"

	"server/internal/cache"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/errclass"
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

// registerBuiltins 注册内置任务（元数据 + 状态来源）。
//
// 注意：这里**只描述**任务，不接管调度；各任务仍由自己的 ticker/cron 触发（Phase 2 再收敛）。
func (s *TaskCenterService) registerBuiltins() {
	editAll := []string{taskcenter.FieldEnabled, taskcenter.FieldSchedule, taskcenter.FieldRemark, taskcenter.FieldInterval}

	s.reg.Register(taskcenter.Def{
		Key: "builtin:douban_score", Kind: taskcenter.KindBuiltin, Type: "douban_score",
		Name: "豆瓣评分同步", Target: "上线月份近半年、已有豆瓣 ID 的影片",
		Detail:          "每周固定时间刷新这批影片的豆瓣评分（区间内分数会变）。",
		DefaultSchedule: "weekly:1 03:00", Enabled: true,
		DefaultIntervalMs: 4000, MinIntervalMs: 2000, IntervalLabel: "3~5s 随机",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: s.doubanStatus,
		Run: func(ctx context.Context) error {
			return s.film.RunDoubanScoreSyncOnce(ctx)
		},
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:douban_backfill", Kind: taskcenter.KindBuiltin, Type: "douban_backfill",
		Name: "豆瓣评分欠账补查", Target: "有豆瓣 ID 但一直没评分的影片",
		Detail:          "默认不启用（不参与定时）；需要时手动触发，单次上限可配。",
		DefaultSchedule: "", Enabled: false,
		DefaultIntervalMs: 4000, MinIntervalMs: 2000, IntervalLabel: "3~5s 随机",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: s.doubanStatus,
		Run: func(ctx context.Context) error {
			return s.film.StartDoubanScoreBackfill(ctx, 0)
		},
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:domain_fetch", Kind: taskcenter.KindBuiltin, Type: "domain_fetch",
		Name: "域名替换公告拉取", Target: "公告站（lz 等）",
		Detail:          "按注册表里的来源逐个拉取公告并入库（只写规则表，不改业务数据）。",
		DefaultSchedule: "every:24h", Enabled: true,
		IntervalLabel: "单请求 · 无间隔",
		Editable:      []string{taskcenter.FieldEnabled, taskcenter.FieldSchedule, taskcenter.FieldRemark}, Caps: []string{taskcenter.CapRun},
		Status: func(context.Context) taskcenter.Status {
			return taskcenter.Status{State: taskcenter.StateIdle, Message: "每 24 小时自动拉取一次"}
		},
		Run: s.film.RunDomainFetchOnce,
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:hot_refresh", Kind: taskcenter.KindBuiltin, Type: "hot_refresh",
		Name: "榜单热度刷新", Target: "豆瓣榜单（按分类）",
		Detail:          "每天抓一轮榜单并把热度分写回影片（服务内部仍按 04:00 判定 + 当日锁）。",
		DefaultSchedule: "daily:04:00", Enabled: true,
		DefaultIntervalMs: 5000, MinIntervalMs: 2000, IntervalLabel: "5s/请求",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: idleWithMessage("每天 04:00 刷新一轮"),
		Run: func(ctx context.Context) error {
			if s.hot == nil {
				return fmt.Errorf("热度服务未装配")
			}
			s.hot.SweepOnce(ctx)
			return nil
		},
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:backdrop", Kind: taskcenter.KindBuiltin, Type: "backdrop",
		Name: "横图补采", Target: "缺横图的影片（TMDB）",
		Detail:          "扫描缺横图的影片并从 TMDB 取图（每请求固定延迟）；频率高，只执行不记台账。",
		DefaultSchedule: "every:20m", Enabled: true, QuietOnSuccess: true,
		DefaultIntervalMs: 300, MinIntervalMs: 100, IntervalLabel: "300ms/请求",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: idleWithMessage("每 20 分钟扫描一次"),
		Run: func(ctx context.Context) error {
			if s.backdrop == nil {
				return fmt.Errorf("横图服务未装配")
			}
			s.backdrop.SweepOnce(ctx)
			return nil
		},
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:health", Kind: taskcenter.KindBuiltin, Type: "health",
		Name: "采集源健康巡检", Target: "全部启用的采集源",
		Detail:          "探测各源可用性与延迟（并发 6，按间隔限速）；频率高，只执行不记台账。",
		DefaultSchedule: "every:1h", Enabled: true, QuietOnSuccess: true,
		DefaultIntervalMs: 200, MinIntervalMs: 200, IntervalLabel: "200ms/探针",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: idleWithMessage("每 1 小时巡检一次"),
		Run: func(ctx context.Context) error {
			if s.manage == nil {
				return fmt.Errorf("管理服务未装配")
			}
			s.manage.RunHealthCheckOnce(ctx)
			return nil
		},
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:year_verify", Kind: taskcenter.KindBuiltin, Type: "year_verify",
		Name: "年份核实", Target: "年份越界的影片（TMDB 校验）",
		Detail:          "用外部权威纠正**确凿错误**的年份：同名命中且年份不同才写库，查不到一律不动（宁缺勿错）。",
		DefaultSchedule: "every:30m", Enabled: true, QuietOnSuccess: true,
		DefaultIntervalMs: 300, MinIntervalMs: 100, IntervalLabel: "300ms/请求",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: idleWithMessage("每 30 分钟扫描一次（高频，不记台账）"),
		Run: func(ctx context.Context) error {
			if s.yearVerify == nil {
				return fmt.Errorf("年份核实服务未装配")
			}
			s.yearVerify.RunOnce(ctx)
			return nil
		},
	})

	// 历史台账保留 30 天（定时任务之一，周期可在任务中心改）
	s.reg.Register(taskcenter.Def{
		Key: "builtin:history_prune", Kind: taskcenter.KindBuiltin, Type: "history_prune",
		Name: "历史台账清理", Target: "task_run（保留 30 天）",
		Detail:          "删除 30 天前的终态记录（分批，不删 running）。",
		DefaultSchedule: "daily:04:00", Enabled: true,
		Editable: []string{taskcenter.FieldEnabled, taskcenter.FieldSchedule, taskcenter.FieldRemark},
		Status:   idleWithMessage("每天清理一次"),
		Run:      s.PruneHistory,
	})
}

func idleWithMessage(msg string) func(context.Context) taskcenter.Status {
	return func(context.Context) taskcenter.Status {
		return taskcenter.Status{State: taskcenter.StateIdle, Message: msg}
	}
}

// doubanStatus 豆瓣两个任务共用一份进度（同一把单飞锁）。
func (s *TaskCenterService) doubanStatus(ctx context.Context) taskcenter.Status {
	if s.film == nil {
		// 未装配（单测/精简装配）时按"空闲"处理：状态探针不能把调用方（如停用前的
		// isTaskRunning）打挂 —— 探针炸掉只是不知道状态，不该拦下停用。
		return taskcenter.Status{State: taskcenter.StateIdle}
	}
	p := s.film.DoubanBatchProgress()
	st := taskcenter.Status{State: taskcenter.StateIdle, Message: p.Message, StartedAt: p.StartedAt}
	switch p.State {
	case "running":
		st.State = taskcenter.StateRunning
		st.Progress = &taskcenter.Progress{Total: p.Total, Done: p.Done, Failed: p.Failed, Unit: "部"}
		if p.StartedAt > 0 {
			st.ElapsedMs = time.Now().UnixMilli() - p.StartedAt
		}
	case "done":
		st.State = taskcenter.StateSucceeded
	case "failed":
		st.State = taskcenter.StateFailed
		st.Error = p.Message
	}
	return st
}

// List 合并「内置任务 + cron 任务」。
func (s *TaskCenterService) List(ctx context.Context, kind string) ([]TaskView, error) {
	settings := map[string]entity.TaskSetting{}
	if s.settings != nil {
		if m, err := s.settings.List(ctx); err == nil {
			settings = m
		}
	}

	out := make([]TaskView, 0, 16)
	for _, d := range s.reg.All() {
		if d.Internal {
			continue
		}
		if kind != "" && d.Kind != kind {
			continue
		}
		out = append(out, s.viewOfDef(ctx, d, settings[d.Key]))
	}

	if kind == "" || kind == taskcenter.KindCollect {
		crons, err := s.crons.List(ctx)
		if err != nil {
			return nil, err
		}
		if len(crons) > 0 {
			ids := make([]int64, 0, len(crons))
			for i := range crons {
				ids = append(ids, crons[i].Id)
			}
			latest, _ := s.runs.LatestByCron(ctx, ids)
			names, allIDs := s.sourcesOnce(ctx)
			// Redis 采集进度**只取一次全量快照**再按源聚合：
			// 此前是"每 cron × 每源"各往返一次（20 源 × 3 任务 = 60 次/刷新，页面还并发拉
			// /tasks 与 /overview ⇒ 翻倍）。JobList 本来就是一次 SCAN，用它喂进度最省。
			snaps := cache.JobSnapshots(ctx)
			for i := range crons {
				out = append(out, s.viewOfCron(ctx, crons[i], latest[crons[i].Id], names, allIDs, snaps))
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind == taskcenter.KindBuiltin
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

// latestBuiltinRun 取某内置任务类型最近一条台账（没有则 ok=false）。
func (s *TaskCenterService) latestBuiltinRun(ctx context.Context, typ string) (entity.TaskRun, bool) {
	if s.runs == nil || typ == "" {
		return entity.TaskRun{}, false
	}
	list, _, err := s.runs.List(ctx, repository.TaskRunFilter{Type: typ}, repository.Page{Current: 1, Size: 1})
	if err != nil || len(list) == 0 {
		return entity.TaskRun{}, false
	}
	return list[0], true
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

func (s *TaskCenterService) viewOfDef(ctx context.Context, d taskcenter.Def, set entity.TaskSetting) TaskView {
	// 合并逻辑单点在 taskcenter.Resolve（与调度器 effective 同一实现，不会两边漂移）
	rc := taskcenter.Resolve(d, userConfigOf(set))
	enabled, schedule, interval := rc.Enabled, rc.Schedule, rc.IntervalMs
	st := taskcenter.Status{State: taskcenter.StateIdle}
	if d.Status != nil {
		st = d.Status(ctx)
	}
	// 台账兜底：hot/backdrop/health/domain 这类没有内存态的任务，运行结果只在 task_run 里。
	// 不补这一步，页面上它们永远显示"空闲"（PR4 之后它们已由中心调度并写台账）。
	if st.State == taskcenter.StateIdle {
		if run, ok := s.latestBuiltinRun(ctx, d.Type); ok {
			st = statusFromRun(run)
		}
	}
	if st.Message == "" && set.Remark != "" {
		st.Message = set.Remark
	}
	return TaskView{
		Key: d.Key, Kind: d.Kind, Type: d.Type, Name: d.Name, Target: d.Target, Detail: d.Detail,
		Schedule: schedule, ScheduleText: taskcenter.DescribeSchedule(schedule),
		Enabled: enabled, IntervalMs: interval, MinIntervalMs: d.MinIntervalMs, IntervalLabel: d.IntervalLabel,
		Editable: d.Editable, Caps: d.Caps,
		State: string(st.State), Progress: st.Progress,
		StartedAt: st.StartedAt, ElapsedMs: st.ElapsedMs, Message: st.Message, Error: st.Error,
	}
}

func (s *TaskCenterService) viewOfCron(ctx context.Context, t entity.CronTask, last entity.TaskRun,
	names map[string]string, allIDs []string, snaps map[string]cache.JobProgress) TaskView {
	v := TaskView{
		Key: taskcenter.RefCron(t.Id), Kind: taskcenter.KindCollect,
		Type: cronTaskType(t), Name: cronTaskName(t, names),
		Target:   cronTaskTarget(t, names),
		Schedule: t.Schedule, ScheduleText: taskcenter.DescribeSchedule(t.Schedule),
		Enabled:  t.State == entity.StateEnabled,
		Editable: []string{taskcenter.FieldEnabled, taskcenter.FieldSchedule, taskcenter.FieldRemark},
		Caps:     []string{taskcenter.CapRun},
		State:    string(taskcenter.StateIdle),
	}
	if last.Id > 0 {
		v.LastRunAt = last.StartedAt
		v.Message = last.Message
		v.Error = last.Error
		v.StartedAt = last.StartedAt
		if last.EndedAt > 0 && last.StartedAt > 0 {
			v.ElapsedMs = last.EndedAt - last.StartedAt
		}
		switch last.Status {
		case entity.TaskRunRunning:
			v.State = string(taskcenter.StateRunning)
		case entity.TaskRunSuccess:
			v.State = string(taskcenter.StateSucceeded)
		case entity.TaskRunFailed:
			v.State = string(taskcenter.StateFailed)
		case entity.TaskRunCanceled:
			v.State = string(taskcenter.StateCanceled)
		}
		// 进度：总数未知（0）时给 null，前端不画进度条
		if last.Total > 0 {
			v.Progress = &taskcenter.Progress{Total: last.Total, Done: last.Done, Failed: last.Failed, Unit: "页"}
		}
		// 运行中且台账还没页数（刚开始/快照缺失）→ 用 Redis 采集进度兜底，让页面能实时看到进度
		if v.Progress == nil && v.State == string(taskcenter.StateRunning) {
			v.Progress = s.cronProgress(t, allIDs, snaps)
		}
		if v.State == string(taskcenter.StateRunning) && v.ElapsedMs == 0 && last.StartedAt > 0 {
			v.ElapsedMs = time.Now().UnixMilli() - last.StartedAt
		}
	}
	return v
}

// cronProgress 采集任务运行中的进度：聚合各目标源的 Redis 采集进度（页数）。
// 只有"知道总数"的行才给进度对象，否则返回 nil（前端不画进度条）。
// cronProgress 用**调用方预先取好的** Redis 快照聚合进度（纯函数：不碰 IO、不 N+1）。
// 之前是"每 cron × 每源"各一次 JobSnapshot —— 20 源 × 3 任务 = 60 次往返/刷新，
// 页面还并发拉 /tasks 与 /overview 再翻一倍。现在快照在 List 入口取一次。
func (s *TaskCenterService) cronProgress(t entity.CronTask, allIDs []string, snaps map[string]cache.JobProgress) *taskcenter.Progress {
	siteIds := make([]string, 0, 8)
	switch t.Model {
	case entity.CronModelRecover:
		return nil // 失败页补采没有"页"的概念
	case entity.CronModelSomeSrc:
		siteIds = append(siteIds, t.SourceIds...)
	default:
		siteIds = allIDs
	}
	var total, done, failed int
	for _, id := range siteIds {
		p, ok := snaps[id]
		if !ok {
			continue
		}
		total += p.Total
		done += p.Done
		failed += p.Failed
	}
	if total <= 0 {
		return nil
	}
	return &taskcenter.Progress{Total: total, Done: done, Failed: failed, Unit: "页"}
}

// Overview 概览（运行中 / 内置数 / 采集数 / 24h 失败）。
func (s *TaskCenterService) Overview(ctx context.Context) (TaskCenterOverview, error) {
	list, err := s.List(ctx, "")
	if err != nil {
		return TaskCenterOverview{}, err
	}
	ov := TaskCenterOverview{}
	for _, v := range list {
		if v.Kind == taskcenter.KindBuiltin {
			ov.Builtin++
		} else {
			ov.Collect++
		}
		if v.State == string(taskcenter.StateRunning) {
			ov.Running++
		}
	}
	if s.runs != nil {
		since := time.Now().Add(-24 * time.Hour).UnixMilli()
		_, failed, ferr := s.runs.CountSince(ctx, since)
		if ferr == nil {
			ov.Failed24h = int(failed)
		}
	}
	return ov, nil
}

// ---- 采集任务展示辅助 ----

func cronTaskType(t entity.CronTask) string {
	switch t.Model {
	case entity.CronModelRecover:
		return "recover"
	case entity.CronModelAutoAll:
		return "collect_all"
	default:
		return "collect"
	}
}

func cronTaskName(t entity.CronTask, names map[string]string) string {
	if strings.TrimSpace(t.Remark) != "" {
		return t.Remark
	}
	switch t.Model {
	case entity.CronModelRecover:
		return "失败页补采"
	case entity.CronModelAutoAll:
		return "全站采集"
	default:
		return "定时采集：" + cronTaskTarget(t, names)
	}
}

func cronTaskTarget(t entity.CronTask, names map[string]string) string {
	switch t.Model {
	case entity.CronModelRecover:
		return "失败页"
	case entity.CronModelAutoAll:
		return "全部启用源"
	}
	if len(t.SourceIds) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(t.SourceIds))
	for _, id := range t.SourceIds {
		if n, ok := names[id]; ok && n != "" {
			parts = append(parts, n)
			continue
		}
		parts = append(parts, id)
	}
	return strings.Join(parts, "、")
}

// sourcesOnce 一次取回"源名映射 + 全部源 id 列表"。
// List 里既要显示源名、又要按源聚合采集进度 —— 同一次查询喂两处，避免重复查库。
func (s *TaskCenterService) sourcesOnce(ctx context.Context) (map[string]string, []string) {
	out := map[string]string{}
	ids := make([]string, 0, 16)
	if s.sources == nil {
		return out, ids
	}
	list, err := s.sources.List(ctx, false)
	if err != nil {
		return out, ids
	}
	for _, src := range list {
		out[src.Id] = src.Name
		ids = append(ids, src.Id)
	}
	return out, ids
}

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

// cronIDOf "cron:<id>" → id（解析单点在 taskcenter.ParseCronRef）。
func cronIDOf(key string) (int64, bool) { return taskcenter.ParseCronRef(key) }

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

// NormalizeCronSchedules 把存量 cron 表达式幂等转换为友好文本（统一时间语义）。
//
//   - 能转：写入 schedule，之后由任务中心统一调度；
//   - 转不动（步长/区间/列表等复杂语法）：**停用并标注**，等人工在任务中心改周期，
//     绝不按旧表达式静默继续跑（避免"看着在跑、实际周期错"）。
func (s *TaskCenterService) NormalizeCronSchedules(ctx context.Context) {
	if s.crons == nil {
		return
	}
	list, err := s.crons.List(ctx)
	if err != nil {
		log.Printf("[task-center] 读取采集定时任务失败: %v", err)
		return
	}
	for i := range list {
		t := list[i]
		if strings.TrimSpace(t.Schedule) != "" {
			continue
		}
		if sched, ok := taskcenter.CronSpecToSchedule(t.Spec); ok {
			t.Schedule = sched
			if uerr := s.crons.Upsert(ctx, &t); uerr != nil {
				log.Printf("[task-center] 周期转换写库失败 #%d: %v", t.Id, uerr)
				continue
			}
			log.Printf("[task-center] 采集任务 #%d 周期已统一: %q → %s", t.Id, t.Spec, sched)
			continue
		}
		if t.State != entity.StateDisabled {
			t.State = entity.StateDisabled
			if !strings.Contains(t.Remark, "周期待人工配置") {
				t.Remark = strings.TrimSpace(t.Remark + "（周期待人工配置）")
			}
			if uerr := s.crons.Upsert(ctx, &t); uerr != nil {
				log.Printf("[task-center] 停用待配置任务失败 #%d: %v", t.Id, uerr)
			}
			log.Printf("[task-center] 采集任务 #%d 周期无法自动转换，已停用待人工配置: spec=%q", t.Id, t.Spec)
		}
	}
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
func (s *TaskCenterService) runManualBuiltin(ctx context.Context, def taskcenter.Def, run func(context.Context) error) error {
	runID := int64(0)
	now := time.Now().UnixMilli()
	if s.runs != nil {
		r := &entity.TaskRun{
			Kind: "manual", Type: def.Type, Name: "手动执行 · " + def.Name,
			Status: entity.TaskRunRunning, StartedAt: now, CreatedAt: now,
		}
		if err := s.runs.Create(context.WithoutCancel(ctx), r); err == nil {
			runID = r.Id
		}
	}

	err := run(ctx)

	if runID == 0 || s.runs == nil {
		return err
	}
	if def.Async {
		// 启动失败要立刻收尾；启动成功则交给观察者（真跑完才知道结果）
		if err != nil {
			s.finishRun(runID, entity.TaskRunFailed, errclass.Format(err))
			return err
		}
		go s.watchBuiltinRun(runID, def)
		return nil
	}
	if err != nil {
		s.finishRun(runID, entity.TaskRunFailed, errclass.Format(err))
		return err
	}
	s.finishRun(runID, entity.TaskRunSuccess, "完成")
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

func (s *TaskCenterService) finishRun(runID int64, status, msg string) {
	ctx := context.Background()
	cur, err := s.runs.Get(ctx, runID)
	if err != nil || cur == nil {
		return
	}
	cur.Status = status
	cur.EndedAt = time.Now().UnixMilli()
	if status == entity.TaskRunFailed {
		cur.Error = msg
	} else {
		cur.Message = msg
	}
	if uerr := s.runs.Update(ctx, cur); uerr != nil {
		log.Printf("[task-center] 台账收尾失败(run=%d): %v", runID, uerr)
	}
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

// PruneHistory 历史台账保留 30 天（分批删除终态记录；running 不动）。
func (s *TaskCenterService) PruneHistory(ctx context.Context) error {
	if s.runs == nil {
		return fmt.Errorf("台账仓储未装配")
	}
	before := time.Now().AddDate(0, 0, -s.retention()).UnixMilli()
	total := int64(0)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := s.runs.PruneBefore(ctx, before, 5000)
		if err != nil {
			return err
		}
		total += n
		if n == 0 {
			break
		}
	}
	log.Printf("[task-center] 历史台账清理：删除 %d 行（早于 %s）", total, time.UnixMilli(before).Format("2006-01-02"))
	return nil
}

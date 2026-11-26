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
	"strconv"
	"strings"
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
	film     *FilmService
	crons    repository.CronTaskRepository
	runs     repository.TaskRunRepository
	settings repository.TaskSettingRepository
	sources  repository.CollectSourceRepository
	spider   *SpiderService
	reg      *taskcenter.Registry
}

func NewTaskCenterService(
	film *FilmService,
	crons repository.CronTaskRepository,
	runs repository.TaskRunRepository,
	settings repository.TaskSettingRepository,
	sources repository.CollectSourceRepository,
	spider *SpiderService,
) *TaskCenterService {
	s := &TaskCenterService{film: film, crons: crons, runs: runs, settings: settings, sources: sources,
		spider: spider, reg: taskcenter.NewRegistry()}
	s.registerBuiltins()
	return s
}

// registerBuiltins 注册内置任务（元数据 + 状态来源）。
//
// 注意：这里**只描述**任务，不接管调度；各任务仍由自己的 ticker/cron 触发（Phase 2 再收敛）。
func (s *TaskCenterService) registerBuiltins() {
	editAll := []string{"enabled", "schedule", "remark", "interval"}

	s.reg.Register(taskcenter.Def{
		Key: "builtin:douban_score", Kind: taskcenter.KindBuiltin, Type: "douban_score",
		Name: "豆瓣评分同步", Target: "上线月份近半年、已有豆瓣 ID 的影片",
		Detail:          "每周固定时间刷新这批影片的豆瓣评分（区间内分数会变）。",
		DefaultSchedule: "weekly:1 03:00", Enabled: true,
		DefaultIntervalMs: 4000, MinIntervalMs: 2000, IntervalLabel: "3~5s 随机",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: s.doubanStatus,
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:douban_backfill", Kind: taskcenter.KindBuiltin, Type: "douban_backfill",
		Name: "豆瓣评分欠账补查", Target: "有豆瓣 ID 但一直没评分的影片",
		Detail:          "默认不启用（不参与定时）；需要时手动触发，单次上限可配。",
		DefaultSchedule: "", Enabled: false,
		DefaultIntervalMs: 4000, MinIntervalMs: 2000, IntervalLabel: "3~5s 随机",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: s.doubanStatus,
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:domain_fetch", Kind: taskcenter.KindBuiltin, Type: "domain_fetch",
		Name: "域名替换公告拉取", Target: "公告站（lz 等）",
		Detail:          "按注册表里的来源逐个拉取公告并入库（只写规则表，不改业务数据）。",
		DefaultSchedule: "every:24h", Enabled: true,
		IntervalLabel: "单请求 · 无间隔",
		Editable:      []string{"enabled", "schedule", "remark"}, Caps: []string{taskcenter.CapRun},
		Status: func(context.Context) taskcenter.Status {
			return taskcenter.Status{State: taskcenter.StateIdle, Message: "每 24 小时自动拉取一次"}
		},
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:hot_refresh", Kind: taskcenter.KindBuiltin, Type: "hot_refresh",
		Name: "榜单热度刷新", Target: "豆瓣榜单（按分类）",
		Detail:          "定期抓榜单并把热度分写回影片（豆瓣接口，注意间隔）。",
		DefaultSchedule: "every:10m", Enabled: true,
		DefaultIntervalMs: 5000, MinIntervalMs: 2000, IntervalLabel: "5s/请求",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: idleWithMessage("每 10 分钟扫描一次"),
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:backdrop", Kind: taskcenter.KindBuiltin, Type: "backdrop",
		Name: "横图补采", Target: "缺横图的影片（TMDB）",
		Detail:          "扫描缺横图的影片并从 TMDB 取图（每请求有固定延迟）。",
		DefaultSchedule: "every:20m", Enabled: true,
		DefaultIntervalMs: 300, MinIntervalMs: 100, IntervalLabel: "300ms/请求",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: idleWithMessage("每 20 分钟扫描一次"),
	})

	s.reg.Register(taskcenter.Def{
		Key: "builtin:health", Kind: taskcenter.KindBuiltin, Type: "health",
		Name: "采集源健康巡检", Target: "全部启用的采集源",
		Detail:          "定期探测各源可用性与延迟（并发 6，建议配间隔避免打爆源站）。",
		DefaultSchedule: "every:1h", Enabled: true,
		DefaultIntervalMs: 200, MinIntervalMs: 200, IntervalLabel: "200ms/探针",
		Editable: editAll, Caps: []string{taskcenter.CapRun},
		Status: idleWithMessage("每 1 小时巡检一次"),
	})
}

func idleWithMessage(msg string) func(context.Context) taskcenter.Status {
	return func(context.Context) taskcenter.Status {
		return taskcenter.Status{State: taskcenter.StateIdle, Message: msg}
	}
}

// doubanStatus 豆瓣两个任务共用一份进度（同一把单飞锁）。
func (s *TaskCenterService) doubanStatus(ctx context.Context) taskcenter.Status {
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
			names := s.sourceNames(ctx)
			for i := range crons {
				out = append(out, s.viewOfCron(ctx, crons[i], latest[crons[i].Id], names))
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

func (s *TaskCenterService) viewOfDef(ctx context.Context, d taskcenter.Def, set entity.TaskSetting) TaskView {
	enabled := d.Enabled
	schedule := d.DefaultSchedule
	interval := d.DefaultIntervalMs
	if set.TaskKey != "" {
		enabled = set.Enabled
		if set.Schedule != "" {
			schedule = set.Schedule
		}
		if set.IntervalMs > 0 {
			interval = set.IntervalMs
		}
	}
	if interval < d.MinIntervalMs {
		interval = d.MinIntervalMs
	}
	st := taskcenter.Status{State: taskcenter.StateIdle}
	if d.Status != nil {
		st = d.Status(ctx)
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

func (s *TaskCenterService) viewOfCron(ctx context.Context, t entity.CronTask, last entity.TaskRun, names map[string]string) TaskView {
	v := TaskView{
		Key: "cron:" + strconv.FormatInt(t.Id, 10), Kind: taskcenter.KindCollect,
		Type: cronTaskType(t), Name: cronTaskName(t, names),
		Target:   cronTaskTarget(t, names),
		Schedule: t.Spec, ScheduleText: t.Spec,
		Enabled:  t.State == entity.StateEnabled,
		Editable: []string{"enabled", "schedule", "remark"},
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
		if v.State == string(taskcenter.StateRunning) && v.ElapsedMs == 0 && last.StartedAt > 0 {
			v.ElapsedMs = time.Now().UnixMilli() - last.StartedAt
		}
	}
	return v
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

func (s *TaskCenterService) sourceNames(ctx context.Context) map[string]string {
	out := map[string]string{}
	if s.sources == nil {
		return out
	}
	list, err := s.sources.List(ctx, false)
	if err != nil {
		return out
	}
	for _, src := range list {
		out[src.Id] = src.Name
	}
	return out
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

func cronIDOf(key string) (int64, bool) {
	if !strings.HasPrefix(key, "cron:") {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(key, "cron:"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func jobKeyOf(key string) (string, bool) {
	if !strings.HasPrefix(key, "job:") {
		return "", false
	}
	id := strings.TrimPrefix(key, "job:")
	return id, id != ""
}

// UpdateSetting 编辑任务配置（内置任务走 task_setting；cron 任务写 cron_task）。
func (s *TaskCenterService) UpdateSetting(ctx context.Context, key string, in TaskSettingInput) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("缺少任务标识")
	}

	// ---- 采集(cron) 任务：启用 / 计划 / 备注 ----
	if id, ok := cronIDOf(key); ok {
		if s.crons == nil {
			return fmt.Errorf("cron 仓储未装配")
		}
		t, err := s.crons.Get(ctx, id)
		if err != nil {
			return err
		}
		if in.IntervalMs != nil {
			return fmt.Errorf("采集任务的请求间隔在「采集源」里按源配置，不在这里改")
		}
		if in.Enabled != nil {
			if *in.Enabled {
				t.State = entity.StateEnabled
			} else {
				t.State = entity.StateDisabled
			}
		}
		if in.Schedule != nil {
			spec := strings.TrimSpace(*in.Schedule)
			if spec == "" {
				return fmt.Errorf("cron 计划不能为空（停用请用 enabled=false）")
			}
			t.Spec = spec
		}
		if in.Remark != nil {
			t.Remark = strings.TrimSpace(*in.Remark)
		}
		return s.crons.Upsert(ctx, t)
	}

	// ---- 内置任务 ----
	def, err := s.EnsureTaskKeyRegistered(key)
	if err != nil {
		return err
	}
	allowed := make(map[string]bool, len(def.Editable))
	for _, f := range def.Editable {
		allowed[f] = true
	}
	deny := func(field, label string) error {
		return fmt.Errorf("该任务不支持修改%s", label)
	}
	if in.Enabled != nil && !allowed["enabled"] {
		return deny("enabled", "启用状态")
	}
	if in.Schedule != nil && !allowed["schedule"] {
		return deny("schedule", "执行周期")
	}
	if in.Remark != nil && !allowed["remark"] {
		return deny("remark", "备注")
	}
	if in.IntervalMs != nil && !allowed["interval"] {
		return deny("interval", "接口间隔")
	}

	if in.Schedule != nil {
		if err := taskcenter.ValidateSchedule(*in.Schedule); err != nil {
			return err
		}
	}
	if in.IntervalMs != nil && *in.IntervalMs > 0 && *in.IntervalMs < def.MinIntervalMs {
		return fmt.Errorf("接口间隔不能小于 %dms（%s）；这是执行层下限，防止被源站拉黑",
			def.MinIntervalMs, def.IntervalLabel)
	}

	now := time.Now().UnixMilli()
	if in.Enabled != nil {
		if s.settings == nil {
			return fmt.Errorf("配置仓储未装配")
		}
		if err := s.settings.SetEnabled(ctx, key, *in.Enabled, now); err != nil {
			return err
		}
	}
	if s.settings == nil {
		return fmt.Errorf("配置仓储未装配")
	}
	set := entity.TaskSetting{TaskKey: key, UpdatedAt: now}
	if in.Schedule != nil {
		set.Schedule = strings.TrimSpace(*in.Schedule)
	}
	if in.Remark != nil {
		set.Remark = strings.TrimSpace(*in.Remark)
	}
	if in.IntervalMs != nil {
		set.IntervalMs = *in.IntervalMs
	}
	// Upsert 只覆盖非空字段（不会清空备注，也不会动 enabled）
	return s.settings.Upsert(ctx, set)
}

// RunTask 手动执行一次。
func (s *TaskCenterService) RunTask(ctx context.Context, key string) error {
	if id, ok := cronIDOf(key); ok {
		if s.spider == nil {
			return fmt.Errorf("采集服务未装配")
		}
		return s.spider.TriggerCron(ctx, id)
	}
	switch key {
	case "builtin:douban_score":
		return s.film.StartDoubanScoreSyncOnce(ctx)
	case "builtin:douban_backfill":
		return s.film.StartDoubanScoreBackfill(ctx, 0)
	case "builtin:domain_fetch":
		bg := context.WithoutCancel(ctx)
		go func() {
			if _, err := s.film.FetchDomainRules(bg, ""); err != nil {
				log.Printf("[task-center] 域名公告拉取失败: %v", err)
			}
		}()
		return nil
	}
	return fmt.Errorf("该任务暂不支持手动执行（可在对应页面操作或等待后续版本接入）")
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

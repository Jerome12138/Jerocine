package service

// 任务中心 · 目录与视图（只读装配）：把「注册表默认 ⊕ 用户配置 ⊕ 最近台账 ⊕ Redis 进度」
// 拼成页面要的 TaskView。拆分自 task_center_service.go（P3-1），行为不变。

import (
	"context"
	"sort"
	"strings"
	"time"

	"server/internal/cache"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/taskcenter"
)

// List 合并「内置任务 + cron 任务」。
func (s *TaskCenterService) List(ctx context.Context, kind string) ([]TaskView, error) {
	// 两个一次性快照（避免每个任务各查一次）：
	//   lastRuns — 高频任务成功不写台账，靠这个轻量记录显示"上次运行"
	//   nextAt   — 调度器内存里的"下次触发时刻"，用来回答"为什么还没跑"
	lastRuns := cache.TaskLastRuns(ctx)
	nextAt := s.nextRuns()

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
		out = append(out, s.viewOfDef(ctx, d, settings[d.Key], lastRuns, nextAt))
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
				out = append(out, s.viewOfCron(ctx, crons[i], latest[crons[i].Id], names, allIDs, snaps,
					nextAt[taskcenter.RefCron(crons[i].Id)]))
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

func (s *TaskCenterService) viewOfDef(ctx context.Context, d taskcenter.Def, set entity.TaskSetting,
	lastRuns map[string]cache.TaskLastRun, nextAt map[string]int64) TaskView {
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
		} else if lr, ok := lastRuns[d.Type]; ok {
			// 高频任务(QuietOnSuccess)成功不写台账 ⇒ 用轻量记录兜底，
			// 否则列表永远显示"未运行"，让人以为任务没在跑
			st.State = taskcenter.StateSucceeded
			if !lr.OK {
				st.State = taskcenter.StateFailed
			}
			st.StartedAt = lr.AtMs
		}
	}
	// 行尾文案的取舍（2026-11-26 用户反馈"每 20 分钟扫描一次"与左侧"计划"列重复）：
	//   · idle 的静态提示本质是"周期说明"，已在计划列展示 ⇒ 不再当正文；
	//   · idle 时优先展示用户备注（此前被静态提示盖住，等于改备注看不到效果）；
	//   · 有台账结果（成功/失败）时保留结果文案。
	if st.State == taskcenter.StateIdle {
		st.Message = rc.Remark
	} else if st.Message == "" && rc.Remark != "" {
		st.Message = rc.Remark
	}
	return TaskView{
		Key: d.Key, Kind: d.Kind, Type: d.Type, Name: d.Name, Target: d.Target, Detail: d.Detail,
		Schedule: schedule, ScheduleText: taskcenter.DescribeSchedule(schedule),
		Enabled: enabled, IntervalMs: interval, MinIntervalMs: d.MinIntervalMs, IntervalLabel: d.IntervalLabel,
		Editable: d.Editable, Caps: d.Caps,
		State: string(st.State), Progress: st.Progress,
		StartedAt: st.StartedAt, ElapsedMs: st.ElapsedMs, Message: st.Message, Error: st.Error,
		NextAt: nextAt[d.Key],
	}
}

func (s *TaskCenterService) viewOfCron(ctx context.Context, t entity.CronTask, last entity.TaskRun,
	names map[string]string, allIDs []string, snaps map[string]cache.JobProgress, nextAtMs int64) TaskView {
	v := TaskView{
		Key: taskcenter.RefCron(t.Id), Kind: taskcenter.KindCollect,
		Type: cronTaskType(t), Name: cronTaskName(t, names),
		Target:   cronTaskTarget(t, names),
		Schedule: t.Schedule, ScheduleText: taskcenter.DescribeSchedule(t.Schedule),
		Enabled:  t.State == entity.StateEnabled,
		Editable: []string{taskcenter.FieldEnabled, taskcenter.FieldSchedule, taskcenter.FieldRemark},
		Caps:     []string{taskcenter.CapRun},
		State:    string(taskcenter.StateIdle),
		NextAt:   nextAtMs,
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

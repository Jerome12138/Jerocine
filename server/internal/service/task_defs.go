package service

// 内置任务定义（**按所属域拆分**）—— 2026-11-26 P3-2
//
// 为什么这么做：原来 8 个任务的元数据全塞在 `TaskCenterService.registerBuiltins()` 里（一个 1059 行的
// 胖文件），新增/调整一个任务必须改任务中心，而且闭包直接抓 5 个 service 的字段，注册表本身看不出
// 任务依赖谁。现在每条定义由**拥有它的服务**提供（`TaskDefs()`），任务中心只做装配：
//   · 改豆瓣任务的周期/文案 → 只动 FilmService；
//   · 换某个任务的执行方式 → 只动它自己的服务；
//   · 任务中心不再认识 film/hot/backdrop/manage/yearVerify 的细节（只剩装配 + 视图）。
//
// 约定（新增任务时照抄）：
//   - Key 用 "builtin:<type>"，Type 与台账 task_type 一致（历史/异常页按它聚合）
//   - 有周期的任务必须有 Run（`TestBuiltinRegistry_SchedulingContract` 兜底）
//   - 高频任务标 QuietOnSuccess（成功不记、失败记）
//   - DefaultSchedule 必须是友好文本（同一个用例会跑 ValidateSchedule）

import (
	"context"
	"fmt"
	"time"

	"server/internal/taskcenter"
)

// ⚠ IntervalLabel 是**说明性静态文案**，不是实时值：
//   有可配间隔的任务，界面一律显示 `intervalMs` 对应的真实值（如 "间隔 5s"）；
//   IntervalLabel 只在"没有可配间隔"时兜底（如固定 5s/请求的榜单任务）。
//   别在这里写"默认 10s"这类会被当成当前值的措辞（2026-11-26 用户实测踩过）。

// editAllFields 全部可编辑字段（大多数任务的默认值）。
var editAllFields = []string{
	taskcenter.FieldEnabled, taskcenter.FieldSchedule, taskcenter.FieldRemark, taskcenter.FieldInterval,
}

// editBasicFields 只有"启用/周期/备注"可编辑（不涉及执行层间隔）。
var editBasicFields = []string{
	taskcenter.FieldEnabled, taskcenter.FieldSchedule, taskcenter.FieldRemark,
}

var capsRun = []string{taskcenter.CapRun}

// idleWithMessage 静态"空闲"状态探针（没有内存进度的任务用）。
func idleWithMessage(msg string) func(context.Context) taskcenter.Status {
	return func(context.Context) taskcenter.Status {
		return taskcenter.Status{State: taskcenter.StateIdle, Message: msg}
	}
}

// ---- 豆瓣 / 域名公告（FilmService 拥有）----

// doubanStatusFor 返回"只看本任务"的状态探针。
//
// 豆瓣两条任务共用一把单飞锁和**一份**进度结构（p.Task 标明当前是哪个任务在跑）。
// 探针必须按归属判断，否则一条在跑时另一条也会显示"运行中"并带上进度
// （用户实测：手动跑「评分同步」，「欠账补查」也被显示成运行中，还挂了"本轮结束后停止"）。
func (s *FilmService) doubanStatusFor(task string) func(context.Context) taskcenter.Status {
	return func(ctx context.Context) taskcenter.Status {
		if s == nil {
			// 未装配（单测/精简装配）时按"空闲"处理：状态探针不能把调用方打挂
			return taskcenter.Status{State: taskcenter.StateIdle}
		}
		p := s.DoubanBatchProgress()
		if p.Task != "" && p.Task != task {
			// 另一条豆瓣任务在跑：本任务就是空闲，既不显示别人的进度也不要别人的文案
			return taskcenter.Status{State: taskcenter.StateIdle}
		}
		st := taskcenter.Status{State: taskcenter.StateIdle, Message: p.Message,
			StartedAt: p.StartedAt, UpdatedAt: p.UpdatedAt}
		// 进度与状态解耦：**完成后也要带上 x/y**（用户要求列表对已完成任务也显示总数/完成数）
		if p.Total > 0 {
			st.Progress = &taskcenter.Progress{Total: p.Total, Done: p.Done, Failed: p.Failed, Unit: "部"}
		}
		switch p.State {
		case "running":
			st.State = taskcenter.StateRunning
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
}

// TaskDefs 豆瓣评分同步 / 欠账补查 / 域名公告拉取。
func (s *FilmService) TaskDefs() []taskcenter.Def {
	return []taskcenter.Def{
		{
			Key: "builtin:douban_score", Kind: taskcenter.KindBuiltin, Type: "douban_score",
			Name: "豆瓣评分同步", Target: "上线月份近半年、已有豆瓣 ID 的影片",
			Detail:          "每周固定时间刷新这批影片的豆瓣评分（区间内分数会变）。",
			DefaultSchedule: "weekly:1 03:00", Enabled: true,
			DefaultIntervalMs: doubanDefaultIntervalMs, MinIntervalMs: doubanMinIntervalMs,
			IntervalLabel: "±20% 抖动，按「接口间隔」执行",
			Editable:      editAllFields, Caps: capsRun,
			Status: s.doubanStatusFor("douban_score"),
			// 豆瓣刷分是"启动后台批 + 内存进度"：Run 必须用 Start 变体并标 Async，
			// 否则手动/定时触发都会**立刻记成功**（用户实测："一触发就完成、没有进度"）。
			Async: true,
			// 实测：近半年候选 ≈1941 部 × 4s ≈ 2.2h；给 8h 余量 + 30 分钟停滞判定
			WatchTimeout: 8 * time.Hour,
			WatchStall:   30 * time.Minute,
			Run: func(ctx context.Context) error {
				if s == nil {
					return fmt.Errorf("影片服务未装配")
				}
				return s.StartDoubanScoreSyncOnce(ctx)
			},
		},
		{
			Key: "builtin:douban_backfill", Kind: taskcenter.KindBuiltin, Type: "douban_backfill",
			Name: "豆瓣评分欠账补查", Target: "有豆瓣 ID 但一直没评分的影片",
			Detail:          "默认不启用（不参与定时）；需要时手动触发，单次上限可配。",
			DefaultSchedule: "", Enabled: false,
			DefaultIntervalMs: doubanDefaultIntervalMs, MinIntervalMs: doubanMinIntervalMs,
			IntervalLabel: "±20% 抖动，按「接口间隔」执行",
			Editable:      editAllFields, Caps: capsRun,
			Status: s.doubanStatusFor("douban_backfill"),
			Async:  true, // 同上：后台批 + 内存进度
			// 实测欠账 ≈2.8 万部 × 4s ≈ 31h；给 48h 上限，停滞 30 分钟才判失败
			WatchTimeout: 48 * time.Hour,
			WatchStall:   30 * time.Minute,
			Run: func(ctx context.Context) error {
				if s == nil {
					return fmt.Errorf("影片服务未装配")
				}
				return s.StartDoubanScoreBackfill(ctx, 0)
			},
		},
		{
			Key: "builtin:domain_fetch", Kind: taskcenter.KindBuiltin, Type: "domain_fetch",
			Name: "域名替换公告拉取", Target: "公告站（lz 等）",
			Detail:          "按注册表里的来源逐个拉取公告并入库（只写规则表，不改业务数据）。",
			DefaultSchedule: "every:24h", Enabled: true,
			IntervalLabel: "单请求 · 无间隔",
			Editable:      editBasicFields, Caps: capsRun,
			Status: idleWithMessage("每 24 小时自动拉取一次"),
			Run:    s.RunDomainFetchOnce,
		},
	}
}

// ---- 榜单热度（HotService）----

func (s *HotService) TaskDefs() []taskcenter.Def {
	return []taskcenter.Def{
		{
			Key: "builtin:hot_refresh", Kind: taskcenter.KindBuiltin, Type: "hot_refresh",
			Name: "榜单热度刷新", Target: "豆瓣榜单（按分类）",
			Detail:          "每天抓一轮榜单并把热度分写回影片（服务内部仍按 04:00 判定 + 当日锁）。",
			DefaultSchedule: "daily:04:00", Enabled: true,
			DefaultIntervalMs: 5000, MinIntervalMs: 2000, IntervalLabel: "5s/请求",
			// 间隔由服务内部常量控制（5s/请求），未接任务中心配置 ⇒ 不暴露可编辑项，避免"改了不生效"
			Editable: editBasicFields, Caps: capsRun,
			Status: idleWithMessage("每天 04:00 刷新一轮"),
			Run: func(ctx context.Context) error {
				if s == nil {
					return fmt.Errorf("热度服务未装配")
				}
				s.SweepOnce(ctx)
				return nil
			},
		},
	}
}

// ---- 横图补采（BackdropService）----

func (s *BackdropService) TaskDefs() []taskcenter.Def {
	return []taskcenter.Def{
		{
			Key: "builtin:backdrop", Kind: taskcenter.KindBuiltin, Type: "backdrop",
			Name: "横图补采", Target: "缺横图的影片（TMDB）",
			Detail:          "扫描缺横图的影片并从 TMDB 取图（每请求固定延迟）；频率高，只执行不记台账。",
			DefaultSchedule: "every:20m", Enabled: true, QuietOnSuccess: true,
			DefaultIntervalMs: 300, MinIntervalMs: 100, IntervalLabel: "300ms/请求",
			Editable: editBasicFields, Caps: capsRun, // 同上：间隔为内部常量
			Status: idleWithMessage("每 20 分钟扫描一次"),
			Run: func(ctx context.Context) error {
				if s == nil {
					return fmt.Errorf("横图服务未装配")
				}
				s.SweepOnce(ctx)
				return nil
			},
		},
	}
}

// ---- 采集源健康巡检（ManageService）----

func (s *ManageService) TaskDefs() []taskcenter.Def {
	return []taskcenter.Def{
		{
			Key: "builtin:health", Kind: taskcenter.KindBuiltin, Type: "health",
			Name: "采集源健康巡检", Target: "全部启用的采集源",
			Detail:          "探测各源可用性与延迟（并发 6，按间隔限速）；频率高，只执行不记台账。",
			DefaultSchedule: "every:1h", Enabled: true, QuietOnSuccess: true,
			DefaultIntervalMs: 200, MinIntervalMs: 200, IntervalLabel: "200ms/探针",
			Editable: editBasicFields, Caps: capsRun, // 同上
			Status: idleWithMessage("每 1 小时巡检一次"),
			Run: func(ctx context.Context) error {
				if s == nil {
					return fmt.Errorf("管理服务未装配")
				}
				s.RunHealthCheckOnce(ctx)
				return nil
			},
		},
	}
}

// ---- 年份核实（YearVerifyService）----

func (s *YearVerifyService) TaskDefs() []taskcenter.Def {
	return []taskcenter.Def{
		{
			Key: "builtin:year_verify", Kind: taskcenter.KindBuiltin, Type: "year_verify",
			Name: "年份核实", Target: "年份越界的影片（TMDB 校验）",
			Detail:          "用外部权威纠正**确凿错误**的年份：同名命中且年份不同才写库，查不到一律不动（宁缺勿错）。",
			DefaultSchedule: "every:30m", Enabled: true, QuietOnSuccess: true,
			DefaultIntervalMs: 300, MinIntervalMs: 100, IntervalLabel: "300ms/请求",
			Editable: editBasicFields, Caps: capsRun, // 同上
			Status: idleWithMessage("每 30 分钟扫描一次（高频，不记台账）"),
			Run: func(ctx context.Context) error {
				if s == nil {
					return fmt.Errorf("年份核实服务未装配")
				}
				s.RunOnce(ctx)
				return nil
			},
		},
	}
}

// ---- 任务中心自己的内务任务（台账清理）----

// ownTaskDefs 任务台账自身的维护任务（依赖 TaskCenterService 的 PruneHistory）。
func (s *TaskCenterService) ownTaskDefs() []taskcenter.Def {
	return []taskcenter.Def{
		{
			Key: "builtin:history_prune", Kind: taskcenter.KindBuiltin, Type: "history_prune",
			Name: "历史台账清理", Target: "任务历史台账（保留期默认 30 天，可用 TASK_RETENTION_DAYS 配）",
			Detail:          "删除保留期之前的已结束记录（成功/失败/取消），分批执行；运行中的不删。",
			DefaultSchedule: "daily:04:00", Enabled: true,
			Editable: editBasicFields,
			Status:   idleWithMessage("每天清理一次"),
			Run:      s.PruneHistory,
		},
	}
}

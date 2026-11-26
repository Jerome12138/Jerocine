package service

// 长耗时任务的统一暂停/恢复（维护窗口）· 2026-11-26
//
// 场景：部署 / 迁移 / 备份 / 重建派生表这类"需要一个安静窗口"的动作，由外部 agent 在动作前
// 调 POST /manage/task-center/pause，动作完成后调 resume。语义分三层，**分层写清楚是为了不吹牛**：
//
//	① 闸门：窗口内不再触发新任务（调度器自动触发 + 手动"立即执行" + 单源启动采集）；
//	② 协作暂停：此刻正在跑的**采集任务**停在页边界（不中断事务、不丢已完成的进度，
//	   resume 后同进程内继续）——复用既有 per-source job 控制面；
//	③ 如实回报：没有协作暂停点的长任务（豆瓣批量、全量派生等）标 pauseable=false + 原因，
//	   **绝不假装"已暂停"**（假阳性会让 agent 以为窗口安静了，实际还在写库）。
//
// 注意语义边界（写进返回体，别让调用方猜）：
//   - 暂停是**协作式**的：真正停下要等当前页跑完（秒级~分钟级），不是立刻冻结；
//   - 暂停**不能跨进程续跑**：采集轮次是进程内 goroutine，容器重建后那一轮就没了 ——
//     所以 pause 的价值是"关掉新触发 + 让在跑的停在安全点"，不是"存档"。

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"server/internal/cache"
	"server/internal/taskcenter"
)

// TaskGateItem 报告里的一行：这一个任务/采集源被怎么处理了。
type TaskGateItem struct {
	Key       string `json:"key"`
	Name      string `json:"name,omitempty"`
	Kind      string `json:"kind"`   // collect(采集源/轮次) | builtin(内置任务)
	State     string `json:"state"`  // 动作前的状态
	Action    string `json:"action"` // paused | resumed | skipped
	Pauseable bool   `json:"pauseable"`
	Reason    string `json:"reason,omitempty"`
}

// TaskGateReport pause / resume 的返回体。
type TaskGateReport struct {
	By     string         `json:"by,omitempty"`
	Until  int64          `json:"until,omitempty"` // unix 秒：闸门自动关闭时刻（0=未开）
	Items  []TaskGateItem `json:"items"`
	Errors []string       `json:"errors,omitempty"`
	Note   string         `json:"note,omitempty"`
}

// TaskGateState GET /manage/task-center/control 的返回体（给 agent 轮询确认）。
type TaskGateState struct {
	Open       bool     `json:"open"`
	By         string   `json:"by,omitempty"`
	Until      int64    `json:"until,omitempty"`
	PausedJobs []string `json:"pausedJobs"` // 因闸门而暂停、待 resume 恢复的采集源
	// RunningJobs 仍在跑的采集源：暂停是**协作式**的（引擎在页边界才检查状态），
	// 部署脚本据此轮询"安静点到了没有"——为空即可动手迁移/重建容器。
	RunningJobs []string `json:"runningJobs"`
	Note        string   `json:"note,omitempty"`
}

// 返回体里的固定说明（接口自解释，免得 agent 误判"暂停=立刻冻结/可跨重启"）。
const gateNote = "闸门只拦「新触发」并协作暂停正在跑的采集源（停在页边界，非立刻冻结）；" +
	"豆瓣批量/全量派生等长任务没有协作暂停点（pauseable=false），如需立即中断请用对应 stop。" +
	"闸门带 TTL 自动关闭，不会把任务永久停住。"

// PauseAll 打开维护窗口：拦新触发 + 协作暂停此刻在跑的采集任务。
func (s *TaskCenterService) PauseAll(ctx context.Context, by string, ttlMinutes int) (TaskGateReport, error) {
	if err := cache.TaskGateOpen(ctx, by, cache.GateTTL(ttlMinutes)); err != nil {
		// 闸门都开不起来就别继续：否则"暂停了"是假的，后续动作会在没保护的窗口里跑
		return TaskGateReport{}, fmt.Errorf("打开维护窗口失败: %w", err)
	}
	byNow, until, _, _ := cache.TaskGateInfo(ctx) // 回读：拿到归一化后的真实关闭时刻
	rep := TaskGateReport{
		By: byNow, Until: until.Unix(), Items: []TaskGateItem{}, Note: gateNote,
	}

	jobs, err := cache.JobList(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, "读取采集任务列表失败: "+err.Error())
	}
	paused := s.pauseRunningJobs(ctx, jobs, &rep)
	rep.Items = append(rep.Items, paused...)
	rep.Items = append(rep.Items, s.nonPauseableRunning(ctx)...)

	log.Printf("[task-center] 维护窗口已打开: by=%s until=%s · 暂停采集源 %d 个, 跳过/不可暂停 %d 个",
		byNow, until.Format(time.RFC3339), countAction(rep.Items, "paused"), countAction(rep.Items, "skipped"))
	return rep, nil
}

// pauseRunningJobs 把"正在跑"的采集源逐个暂停，并记进 resume 白名单。
func (s *TaskCenterService) pauseRunningJobs(ctx context.Context, jobs []cache.JobProgress, rep *TaskGateReport) []TaskGateItem {
	out := make([]TaskGateItem, 0, len(jobs))
	for _, j := range jobs {
		if j.State != cache.JobRunning {
			continue // 已暂停/已结束的不动（尤其别把"用户手工暂停"的算成闸门暂停）
		}
		it := TaskGateItem{
			Key: taskcenter.RefJob(j.SourceId), Name: j.Name, Kind: taskcenter.KindCollect,
			State: j.State, Pauseable: true,
		}
		switch err := cache.JobControl(ctx, j.SourceId, cache.JobPaused, cache.JobRunning); {
		case err == nil:
			cache.TaskGateMarkJob(ctx, j.SourceId) // 只有真的暂停成功才进白名单
			it.Action = "paused"
		case errors.Is(err, cache.ErrJobStateMismatch):
			it.Action, it.Reason = "skipped", "状态已变化（可能刚结束）"
		default:
			it.Action, it.Reason = "skipped", err.Error()
			rep.Errors = append(rep.Errors, it.Key+": "+err.Error())
		}
		out = append(out, it)
	}
	return out
}

// nonPauseableRunning 列出"正在跑但没有协作暂停点"的内置长任务：如实回报，不假装暂停。
func (s *TaskCenterService) nonPauseableRunning(ctx context.Context) []TaskGateItem {
	out := []TaskGateItem{}
	if s.reg == nil {
		return out
	}
	for _, d := range s.reg.All() {
		if d.Status == nil || d.Internal {
			continue
		}
		st := d.Status(ctx)
		if st.State != taskcenter.StateRunning {
			continue
		}
		out = append(out, TaskGateItem{
			Key: d.Key, Name: d.Name, Kind: d.Kind, State: string(st.State),
			Action: "skipped", Pauseable: false,
			Reason: "该任务没有协作暂停点，暂停只能拦住它的下一次触发；本轮请等它跑完，或先停用该任务",
		})
	}
	return out
}

// ResumeAll 关闭维护窗口，并只恢复"因闸门而暂停"的采集源。
func (s *TaskCenterService) ResumeAll(ctx context.Context) (TaskGateReport, error) {
	ids := cache.TaskGateJobs(ctx) // 先读白名单（关闸门不动它）
	if err := cache.TaskGateClose(ctx); err != nil {
		return TaskGateReport{}, fmt.Errorf("关闭维护窗口失败: %w", err)
	}
	rep := TaskGateReport{Items: []TaskGateItem{}, Note: gateNote}
	// keepFailed：恢复**真的失败**（协调存储抖动等）的源必须留在白名单里，
	// 否则"关窗成功 + 清空白名单 + 源还停在 paused" ⇒ 没人再管它（要等 job hash 24h TTL 才自愈）。
	// 已结束/已中断/已在跑（错失状态）的这些不算失败：它们本来就不需要恢复。
	var keepFailed []string
	for _, id := range ids {
		it := TaskGateItem{Key: taskcenter.RefJob(id), Kind: taskcenter.KindCollect, State: cache.JobPaused, Pauseable: true}
		switch err := cache.JobControl(ctx, id, cache.JobRunning, cache.JobPaused); {
		case err == nil:
			it.Action = "resumed"
		case errors.Is(err, cache.ErrJobNotFound):
			it.Action, it.Reason = "skipped", "任务已结束/已回收，无需恢复"
		case errors.Is(err, cache.ErrJobStateMismatch):
			// 状态已不是 paused：读一眼说清楚是什么，别笼统写"已变化"。
			//   error + 「服务重启中断」= 部署重启把这一轮掐掉了（进程内的轮次不可能跨重启续跑），
			//   这不是异常而是预期：下一轮按增量窗口重采，数据不会丢（已落库的页都在事务里）。
			switch cur := cache.JobReadState(ctx, id); cur {
			case cache.JobError:
				it.Action, it.Reason = "skipped", "本轮已随服务重启中断（进程内轮次不跨重启续跑），下一轮会自动重采"
			case cache.JobDone:
				it.Action, it.Reason = "skipped", "本轮已跑完，无需恢复"
			case cache.JobCanceled:
				it.Action, it.Reason = "skipped", "已被取消，无需恢复"
			case cache.JobRunning:
				it.Action, it.Reason = "skipped", "已在跑（可能被手工恢复）"
			default:
				it.Action, it.Reason = "skipped", "状态已变化（当前 "+cur+"）"
			}
		default:
			it.Action, it.Reason = "skipped", err.Error()
			if keepOnResumeError(err) {
				keepFailed = append(keepFailed, id)
			}
			rep.Errors = append(rep.Errors, it.Key+": "+err.Error())
		}
		rep.Items = append(rep.Items, it)
	}
	cache.TaskGateJobsClear(ctx)
	if len(keepFailed) > 0 {
		for _, id := range keepFailed {
			cache.TaskGateMarkJob(ctx, id) // 留白名单：再调一次 resume 就能补上
		}
		log.Printf("[task-center] resume 有 %d 个源恢复失败, 已留在白名单待重试: %v", len(keepFailed), keepFailed)
	}
	log.Printf("[task-center] 维护窗口已关闭 · 恢复采集源 %d 个", countAction(rep.Items, "resumed"))
	return rep, nil
}

// ControlState 当前维护窗口状态（给 agent 轮询：开没开、谁开的、还有多久、暂停了哪些源、
// 还有哪些源没停到页边界）。
func (s *TaskCenterService) ControlState(ctx context.Context) (TaskGateState, error) {
	by, until, open, err := cache.TaskGateInfo(ctx)
	if err != nil {
		return TaskGateState{}, err
	}
	st := TaskGateState{
		Open: open, By: by, PausedJobs: cache.TaskGateJobs(ctx),
		RunningJobs: []string{}, Note: gateNote,
	}
	if open {
		st.Until = until.Unix()
	}
	if jobs, jerr := cache.JobList(ctx); jerr == nil {
		for _, j := range jobs {
			if j.State == cache.JobRunning {
				st.RunningJobs = append(st.RunningJobs, j.SourceId)
			}
		}
	}
	return st, nil
}

func countAction(items []TaskGateItem, action string) int {
	n := 0
	for _, it := range items {
		if it.Action == action {
			n++
		}
	}
	return n
}

// keepOnResumeError 恢复失败时是否要留在白名单待重试。
//   - 已回收（job 键没了）/ 状态已变化（已中断、已跑完、已在跑）→ 本来就不需要恢复，不留；
//   - 其余（协调存储抖动等真失败）→ 必须留，否则"关窗 + 清空白名单 + 源仍 paused"没人再管，
//     要等 job hash 的 24h TTL 才自愈（这期间页面一直显示"暂停中"，也不会再被采集）。
func keepOnResumeError(err error) bool {
	return !errors.Is(err, cache.ErrJobNotFound) && !errors.Is(err, cache.ErrJobStateMismatch)
}

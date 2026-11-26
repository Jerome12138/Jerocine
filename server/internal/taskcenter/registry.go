// Package taskcenter 统一任务中心的**元数据层**：任务的类型、计划、能力与状态。
//
// 分层（见 docs/功能方案/方案-统一任务中心-2026-11-25.md）：
//   - 调度层 schedule：多久跑一次（友好文本，本包负责解析/校验）
//   - 执行层 interval_ms：一次运行内相邻外部请求隔多久（有硬下限，防被源站拉黑）
//
// 本包只做"是什么、能不能做、现在什么状态"，不负责触发调度（收敛在后续 PR）。
package taskcenter

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// State 任务状态（统一口径，前端按此渲染）。
type State string

const (
	StateIdle      State = "idle"      // 空闲（从未运行 / 上轮已结束且已过展示窗口）
	StateRunning   State = "running"   // 运行中
	StatePaused    State = "paused"    // 已暂停（仅采集类支持）
	StateSucceeded State = "succeeded" // 已完成
	StateFailed    State = "failed"    // 失败
	StateCanceled  State = "canceled"  // 已取消
)

// Progress 进度；Total<=0 表示"总数未知"，此时前端**不画进度条**（避免假进度）。
type Progress struct {
	Total  int    `json:"total"`
	Done   int    `json:"done"`
	Failed int    `json:"failed"`
	Unit   string `json:"unit"` // 页 / 部 / 条
}

// Status 一次状态快照。
type Status struct {
	State     State
	Progress  *Progress
	Message   string
	Error     string
	StartedAt int64 // ms
	ElapsedMs int64
}

// 能力
const (
	CapRun    = "run"
	CapPause  = "pause"
	CapResume = "resume"
	CapStop   = "stop"
)

// 任务大类
const (
	KindCollect = "collect"
	KindBuiltin = "builtin"
)

// Def 任务定义（内置任务静态注册；采集类由 cron 动态展开）。
type Def struct {
	Key    string // builtin:douban_score / cron:<id>
	Kind   string
	Type   string // collect / recover / domain_fetch / douban_score / douban_backfill / hot_refresh / backdrop / health / history_prune
	Name   string
	Target string // 目标描述（可动态填）
	Detail string // 长描述（详情抽屉用）

	// 调度层
	DefaultSchedule string // 友好文本；空 = 非定时
	Enabled         bool   // 默认是否启用（可被 task_setting 覆盖）

	// 执行层
	DefaultIntervalMs int
	MinIntervalMs     int
	IntervalLabel     string // 展示用："3~5s 随机" / "300ms/页"

	Editable []string // enabled | schedule | remark | interval
	Caps     []string

	Internal bool // 不在列表展示（如 history_prune）
	// QuietOnSuccess 高频任务：照常执行，**成功不写台账、失败照写**。
	// 这样既不把台账刷爆（20~30 分钟一条），又能保证"异常留痕 + 可重跑/可忽略"。
	QuietOnSuccess bool

	Status func(ctx context.Context) Status
	// Async 该任务的 Run 是"启动即返回"（真正跑完要轮询 Status 才知道）。
	// 手动执行时据此决定：立即收尾（同步）还是挂观察者等它结束（异步）。
	Async bool
	// Run 触发一次（Phase 2 收敛后由中心调度调用）。
	// 为 nil 表示该任务只读展示（还没有可复用的执行入口）。
	Run func(ctx context.Context) error
}

// Registry 任务注册表（读多写少）。
type Registry struct {
	mu    sync.RWMutex
	defs  map[string]Def
	order []string
}

func NewRegistry() *Registry {
	return &Registry{defs: map[string]Def{}}
}

// Register 注册（同 key 覆盖，便于测试与热更新）。
// Register 注册/覆盖任务定义。
// 重复 key 会**告警**（不 panic：注册在启动期，但测试替身可能故意覆盖）——
// 静默覆盖的后果是两个域误用同一个 key 时，后注册者悄悄接管，排查时看不出任何线索。
func (r *Registry) Register(d Def) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev, ok := r.defs[d.Key]; ok {
		log.Printf("[taskcenter] 重复注册任务 key=%q（原 name=%q → 新 name=%q），已覆盖",
			d.Key, prev.Name, d.Name)
	} else {
		r.order = append(r.order, d.Key)
	}
	r.defs[d.Key] = d
}

// All 按注册顺序返回副本。
func (r *Registry) All() []Def {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Def, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.defs[k])
	}
	return out
}

func (r *Registry) Get(key string) (Def, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.defs[key]
	return d, ok
}

// ---------------------------------------------------------------------------
// 调度层：友好文本周期
// ---------------------------------------------------------------------------

// scheduleRe 支持的四种写法（文档 §3.1）：
//
//	every:30m / every:6h      间隔型
//	daily:03:00               每天定点
//	weekly:1 03:00            每周定点（1=周一 … 7=周日）
//	monthly:1 04:00           每月定点
var scheduleRe = regexp.MustCompile(`^(every:(\d{1,3})(m|h)|daily:(\d{2}):(\d{2})|weekly:([1-7]) (\d{2}):(\d{2})|monthly:(\d{1,2}) (\d{2}):(\d{2}))$`)

// ValidateSchedule 校验友好文本周期；空串合法（表示"不定时"）。
func ValidateSchedule(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	m := scheduleRe.FindStringSubmatch(s)
	if m == nil {
		return fmt.Errorf("周期格式不支持：%s（可用 every:30m / daily:03:00 / weekly:1 03:00 / monthly:1 04:00）", s)
	}
	switch {
	case m[2] != "": // every:Nm|h
		n, _ := strconv.Atoi(m[2])
		if n < 1 {
			return fmt.Errorf("间隔必须 ≥1")
		}
		if m[3] == "m" && n > 59 {
			return fmt.Errorf("分钟间隔最大 59，更长的请用小时（如 every:6h）")
		}
		return nil
	case m[4] != "": // daily:HH:MM
		return checkHHMM(m[4], m[5])
	case m[6] != "": // weekly:D HH:MM
		return checkHHMM(m[7], m[8])
	default: // monthly:D HH:MM
		d, _ := strconv.Atoi(m[9])
		if d < 1 || d > 28 {
			return fmt.Errorf("monthly 的日期只能是 1~28（避开月末不存在）")
		}
		return checkHHMM(m[10], m[11])
	}
}

func checkHHMM(hh, mm string) error {
	h, _ := strconv.Atoi(hh)
	m, _ := strconv.Atoi(mm)
	if h > 23 || m > 59 {
		return fmt.Errorf("时间不合法：%s:%s（小时 00~23，分钟 00~59）", hh, mm)
	}
	return nil
}

// DescribeSchedule 把友好文本翻成中文（空串返回"仅手动"）。
func DescribeSchedule(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "仅手动"
	}
	m := scheduleRe.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	switch {
	case m[2] != "":
		if m[3] == "m" {
			return "每 " + m[2] + " 分钟"
		}
		return "每 " + m[2] + " 小时"
	case m[4] != "":
		return "每天 " + m[4] + ":" + m[5]
	case m[6] != "":
		return "每周" + weekdayCN(m[6]) + " " + m[7] + ":" + m[8]
	default:
		return "每月 " + m[9] + " 日 " + m[10] + ":" + m[11]
	}
}

func weekdayCN(d string) string {
	names := []string{"一", "二", "三", "四", "五", "六", "日"}
	i, err := strconv.Atoi(d)
	if err != nil || i < 1 || i > 7 {
		return d
	}
	return names[i-1]
}

// NextRun 计算下一次触发时间（schedule 为空返回零值）。
func NextRun(schedule string, now time.Time) time.Time {
	s := strings.TrimSpace(schedule)
	m := scheduleRe.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}
	}
	switch {
	case m[2] != "": // every:Nm|h
		n, _ := strconv.Atoi(m[2])
		d := time.Duration(n) * time.Minute
		if m[3] == "h" {
			d = time.Duration(n) * time.Hour
		}
		return now.Add(d)
	case m[4] != "":
		return nextDaily(now, atoi(m[4]), atoi(m[5]))
	case m[6] != "":
		return nextWeekly(now, atoi(m[6]), atoi(m[7]), atoi(m[8]))
	default:
		return nextMonthly(now, atoi(m[9]), atoi(m[10]), atoi(m[11]))
	}
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func nextDaily(now time.Time, hh, mm int) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func nextWeekly(now time.Time, weekday, hh, mm int) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
	target := time.Weekday(weekday % 7) // 1=周一 → Monday(1)；7=周日 → Sunday(0)
	for i := 0; i < 8; i++ {
		if t.Weekday() == target && t.After(now) {
			return t
		}
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func nextMonthly(now time.Time, day, hh, mm int) time.Time {
	t := time.Date(now.Year(), now.Month(), day, hh, mm, 0, 0, now.Location())
	if !t.After(now) {
		t = time.Date(now.Year(), now.Month()+1, day, hh, mm, 0, 0, now.Location())
	}
	return t
}

// ---- 采集 job 状态机（纯规则）----
//
// 为什么放在规则层：这些"从哪个状态可以走到哪个状态"是**业务语义**，不是存储细节。
// 原先这段判断埋在 cache.JobControl 里（存储层知道状态机），取消/暂停语义遂扩散到缓存包。
// 现在规则在这里、读写原语仍在 cache —— cache 侧有等价性用例兜住（service 包
// TestJobStateMachineMatchesCache）。

// JobAction 采集任务的三个控制动作。
type JobAction string

const (
	JobActionPause  JobAction = "pause"
	JobActionResume JobAction = "resume"
	JobActionCancel JobAction = "cancel"
)

// 采集 job 的状态常量（与 cache 侧的字符串取值一一对应）。
const (
	JobRunning  = "running"
	JobPaused   = "paused"
	JobCanceled = "canceled"
	JobDone     = "done"
	JobError    = "error"
)

// NextJobState 计算动作后的状态；ok=false 表示"当前状态不允许该动作"。
//
// 允许的迁移（与 cache.JobControl 的 from 前置一致）：
//
//	run   --pause-->  paused        （只能暂停在跑的）
//	paused --resume--> running      （只能恢复暂停的）
//	run|paused --cancel--> canceled （在跑/暂停都能取消）
//
// done/error 是终态，任何动作都不允许（避免把已结束的任务"复活"成 running 造成幽灵运行）。
func NextJobState(cur string, act JobAction) (next string, ok bool) {
	switch act {
	case JobActionPause:
		if cur == JobRunning {
			return JobPaused, true
		}
	case JobActionResume:
		if cur == JobPaused {
			return JobRunning, true
		}
	case JobActionCancel:
		if cur == JobRunning || cur == JobPaused {
			return JobCanceled, true
		}
	}
	return "", false
}

// JobTerminal 是否终态（done/error/canceled）。
func JobTerminal(state string) bool {
	return state == JobDone || state == JobError || state == JobCanceled
}

// ---- 生效配置合并（单点）----
//
// 为什么单独成类型：内置任务的"实际生效值"= 注册表默认值 ⊕ 用户改过的值。
// 这段合并原先在 `TaskScheduler.effective()` 与 `TaskCenterService.viewOfDef()` **各写一遍**
// （`if set.Schedule != "" { schedule = set.Schedule }` …）。两处必须永远同步，否则出现
// "页面显示启用、调度器认为停用"这种最难查的分歧。现在合并只在这里做，两边都调它。

// UserConfig 用户改过的值（空串/0 表示"没改"，与 task_setting 的"只存用户改过的值"同语义）。
type UserConfig struct {
	Enabled    *bool
	Schedule   string
	IntervalMs int
	Remark     string
}

// ResolvedConfig 合并后的生效配置 + 每个字段的来源（default / user），便于排查。
type ResolvedConfig struct {
	Enabled    bool
	Schedule   string
	IntervalMs int
	Remark     string

	FromEnabled    string
	FromSchedule   string
	FromIntervalMs string
}

// Resolve 把注册表默认值与用户配置合并成生效配置（唯一实现）。
// 规则：
//   - Enabled：用户显式给过就用它，否则用注册表默认（默认停用的任务不会因为"只改备注"被打开）；
//   - Schedule：用户非空则覆盖，否则用默认；
//   - IntervalMs：用户 >0 则覆盖，否则用默认；最后**统一夹到执行层下限**（防被源站拉黑）；
//   - Remark：用户值（注册表没有备注概念）。
func Resolve(d Def, u UserConfig) ResolvedConfig {
	rc := ResolvedConfig{
		Enabled: d.Enabled, Schedule: d.DefaultSchedule,
		IntervalMs: d.DefaultIntervalMs, Remark: u.Remark,
		FromEnabled: "default", FromSchedule: "default", FromIntervalMs: "default",
	}
	if u.Enabled != nil {
		rc.Enabled = *u.Enabled
		rc.FromEnabled = "user"
	}
	if u.Schedule != "" {
		rc.Schedule = u.Schedule
		rc.FromSchedule = "user"
	}
	if u.IntervalMs > 0 {
		rc.IntervalMs = u.IntervalMs
		rc.FromIntervalMs = "user"
	}
	if rc.IntervalMs < d.MinIntervalMs {
		rc.IntervalMs = d.MinIntervalMs // 下限永远优先（比默认值和用户值都硬）
	}
	return rc
}

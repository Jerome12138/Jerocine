package entity

// 任务运行台账 (table: task_run) — 每次任务触发一行, 覆盖定时(cron)与手动(manual)两类来源。
// 记录任务级状态/结果/失败原因, 支持后台"任务管理"页展示历史、失败提示与重跑。

// TaskRunKind 触发来源
const (
	TaskRunKindCron   = "cron"   // 定时任务触发
	TaskRunKindManual = "manual" // 手动触发
)

// TaskRunType 任务类型
const (
	TaskRunCollect      = "collect"        // 采集(单源: 手动或定时)
	TaskRunRecover      = "recover"        // 失败页补采
	TaskRunCategoryCover = "category_cover" // 分类覆盖
	TaskRunHotRefresh   = "hot_refresh"    // 榜单热度刷新
)

// TaskRunStatus 运行状态
const (
	TaskRunRunning  = "running"
	TaskRunSuccess  = "success"
	TaskRunFailed   = "failed"
	TaskRunCanceled = "canceled"
)

// TaskRun 一条任务运行记录。
type TaskRun struct {
	Id        int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Kind      string `gorm:"column:kind" json:"kind"`           // cron | manual
	Type      string `gorm:"column:task_type" json:"type"`      // collect | recover | category_cover | hot_refresh
	CronId    int64  `gorm:"column:cron_id" json:"cronId"`      // 定时任务 id(非定时触发为 0)
	SourceId  string `gorm:"column:source_id" json:"sourceId"`  // 采集源 id(采集/覆盖类)
	Name      string `gorm:"column:name" json:"name"`           // 任务显示名
	Hours     int    `gorm:"column:hours" json:"hours"`         // 采集时长: -1 全量 / >0 增量小时
	Status    string `gorm:"column:status" json:"status"`       // running | success | failed | canceled
	Total     int    `gorm:"column:total" json:"total"`         // 总页数
	Done      int    `gorm:"column:done" json:"done"`           // 成功页数
	Failed    int    `gorm:"column:failed" json:"failed"`       // 失败页数
	Message   string `gorm:"column:message" json:"message"`     // 结果摘要
	Error     string `gorm:"column:error" json:"error"`         // 失败原因(截断)
	StartedAt int64  `gorm:"column:started_at" json:"startedAt"`
	EndedAt   int64  `gorm:"column:ended_at" json:"endedAt"`
	CreatedAt int64  `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt int64  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (TaskRun) TableName() string { return "task_run" }

// IsTerminal 是否已结束(不再展示为运行中)。
func (r *TaskRun) IsTerminal() bool {
	return r.Status == TaskRunSuccess || r.Status == TaskRunFailed || r.Status == TaskRunCanceled
}

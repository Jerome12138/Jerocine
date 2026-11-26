package entity

// TaskSetting 内置任务的可编辑配置 (table: task_setting) —— 只存"用户改过的值"。
//
// 读取优先级：task_setting（非空）→ 代码注册表默认值。
// schedule 与 interval_ms 属于**两个层**：
//   - schedule    调度层（多久跑一次）
//   - interval_ms 执行层（一次运行内相邻请求隔多久，防拉黑，有硬下限）
type TaskSetting struct {
	TaskKey string `gorm:"column:task_key;primaryKey" json:"taskKey"`
	// Enabled 必须带 default:1 —— GORM 对"带 default 标签的零值字段"在 INSERT 时会**省略**，
	// 交给 DB 默认值；不带标签就会把零值 false 显式写进去。
	// 踩过的坑：只改周期（不传 enabled）时 Upsert 走 INSERT 分支，把 enabled=0 写进新行 ⇒
	// 默认启用的内置任务被**静默停用**（页面显示停用、零报错）；前端对话框总带 enabled 才没暴露。
	Enabled    bool   `gorm:"column:enabled;default:1" json:"enabled"`
	Schedule   string `gorm:"column:schedule" json:"schedule"`
	Remark     string `gorm:"column:remark" json:"remark"`
	IntervalMs int    `gorm:"column:interval_ms" json:"intervalMs"`
	UpdatedAt  int64  `gorm:"column:updated_at" json:"updatedAt"`
}

func (TaskSetting) TableName() string { return "task_setting" }

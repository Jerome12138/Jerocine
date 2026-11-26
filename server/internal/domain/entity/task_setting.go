package entity

// TaskSetting 内置任务的可编辑配置 (table: task_setting) —— 只存"用户改过的值"。
//
// 读取优先级：task_setting（非空）→ 代码注册表默认值。
// schedule 与 interval_ms 属于**两个层**：
//   - schedule    调度层（多久跑一次）
//   - interval_ms 执行层（一次运行内相邻请求隔多久，防拉黑，有硬下限）
type TaskSetting struct {
	TaskKey    string `gorm:"column:task_key;primaryKey" json:"taskKey"`
	Enabled    bool   `gorm:"column:enabled" json:"enabled"`
	Schedule   string `gorm:"column:schedule" json:"schedule"`
	Remark     string `gorm:"column:remark" json:"remark"`
	IntervalMs int    `gorm:"column:interval_ms" json:"intervalMs"`
	UpdatedAt  int64  `gorm:"column:updated_at" json:"updatedAt"`
}

func (TaskSetting) TableName() string { return "task_setting" }

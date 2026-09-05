package entity

import "time"

// TelemetryEvent 埋点事件 (table: telemetry_event) — server_ts 用 datetime(3) 便于窗口聚合。
type TelemetryEvent struct {
	Id        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Category  string    `gorm:"column:category" json:"category"`
	Path      string    `gorm:"column:path" json:"path"`
	Label     string    `gorm:"column:label" json:"label"`
	Value     float64   `gorm:"column:value" json:"value"`
	Duration  int       `gorm:"column:duration" json:"duration"`
	UserId    *int64    `gorm:"column:user_id" json:"userId"`
	SessionId string    `gorm:"column:session_id" json:"sessionId"`
	Ip        string    `gorm:"column:ip" json:"ip"`
	Ua        string    `gorm:"column:ua" json:"ua"`
	Extra     JSONMap   `gorm:"column:extra;type:json" json:"extra"`
	ClientTs  int64     `gorm:"column:client_ts" json:"clientTs"`
	ServerTs  time.Time `gorm:"column:server_ts" json:"serverTs"`
}

func (TelemetryEvent) TableName() string { return "telemetry_event" }

// TelemetryIssueResolution 问题闭环 (table: telemetry_issue_resolution)。
// KeyHash = sha1(category|path|label)。
type TelemetryIssueResolution struct {
	Id               int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	KeyHash          string `gorm:"column:key_hash" json:"keyHash"`
	Category         string `gorm:"column:category" json:"category"`
	Path             string `gorm:"column:path" json:"path"`
	Label            string `gorm:"column:label" json:"label"`
	ResolvedCommitId string `gorm:"column:resolved_commit_id" json:"resolvedCommitId"`
	Resolved         bool   `gorm:"column:resolved" json:"resolved"`
	ReopenedCount    int    `gorm:"column:reopened_count" json:"reopenedCount"`
	CreatedAt        int64  `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt        int64  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (TelemetryIssueResolution) TableName() string { return "telemetry_issue_resolution" }

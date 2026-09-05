package entity

// 采集源等级
const (
	GradeMaster int8 = 0 // 主站
	GradeSlave  int8 = 1 // 附属
)

// 采集源返回类型
const (
	ResultJSON int8 = 0
	ResultXML  int8 = 1
)

// 启用状态 (沿用旧语义: 0 启用 / 1 停用)
const (
	StateEnabled  int8 = 0
	StateDisabled int8 = 1
)

// CollectSource 采集源 (table: collect_source) — id 为站点字符串标识。
type CollectSource struct {
	Id           string `gorm:"column:id;primaryKey" json:"id"`
	Name         string `gorm:"column:name" json:"name"`
	Uri          string `gorm:"column:uri" json:"uri"`
	ResultModel  int8   `gorm:"column:result_model" json:"resultModel"`
	Grade        int8   `gorm:"column:grade" json:"grade"`
	SyncPictures bool   `gorm:"column:sync_pictures" json:"syncPictures"`
	CollectType  int    `gorm:"column:collect_type" json:"collectType"`
	GradeScore   int    `gorm:"column:grade_score" json:"gradeScore"`
	IntervalMs   int    `gorm:"column:interval_ms" json:"intervalMs"`
	State        int8   `gorm:"column:state" json:"state"`
	// ClientOnly 仅端侧可达: 服务器(被地域封等)访问不到该源 → 服务端不测速/不计失败/不自动停采, 端侧另做测速。
	ClientOnly   bool   `gorm:"column:client_only" json:"clientOnly"`
	CreatedAt    int64  `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt    int64  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (CollectSource) TableName() string { return "collect_source" }

// CronTask 采集定时任务 (table: cron_task)。
type CronTask struct {
	Id        int64       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	SourceIds StringSlice `gorm:"column:source_ids;type:json" json:"sourceIds"`
	Spec      string      `gorm:"column:spec" json:"spec"`
	Time      int         `gorm:"column:time" json:"time"`
	Model     int8        `gorm:"column:model" json:"model"` // 0 自动全站 / 1 指定源
	State     int8        `gorm:"column:state" json:"state"`
	Remark    string      `gorm:"column:remark" json:"remark"`
	EntryId   int         `gorm:"column:entry_id" json:"entryId"`
	LastRunAt int64       `gorm:"column:last_run_at" json:"lastRunAt"`
	CreatedAt int64       `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt int64       `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (CronTask) TableName() string { return "cron_task" }

// 采集源健康状态
const (
	HealthUnknown  = "unknown"
	HealthHealthy  = "healthy"
	HealthDegraded = "degraded"
	HealthDown     = "down"
)

// SourceHealth 采集源健康度 (table: source_health) — 一源一行最新快照 + 连续失败计数。
// suppressed=true 表示连续失败达阈值被自动停采(与 CollectSource.State 正交, 不互相覆盖)。
type SourceHealth struct {
	SourceId         string `gorm:"column:source_id;primaryKey" json:"sourceId"`
	Status           string `gorm:"column:status" json:"status"`
	ConsecutiveFails int    `gorm:"column:consecutive_fails" json:"consecutiveFails"`
	Suppressed       bool   `gorm:"column:suppressed" json:"suppressed"`
	LastOk           bool   `gorm:"column:last_ok" json:"lastOk"`
	LatencyMs        int64  `gorm:"column:latency_ms" json:"latencyMs"`
	BestMs           int64  `gorm:"column:best_ms" json:"bestMs"`
	Films            int    `gorm:"column:films" json:"films"`
	PageCount        int    `gorm:"column:page_count" json:"pageCount"`           // 分页总数(资源最全判定)
	Total            int    `gorm:"column:total" json:"total"`                    // 目录总片数
	PlayLatencyMs    int64  `gorm:"column:play_latency_ms" json:"playLatencyMs"`  // 抽样 m3u8 播放延时(0=未测)
	OkCount          int    `gorm:"column:ok_count" json:"okCount"`
	Probes           int    `gorm:"column:probes" json:"probes"`
	Message          string `gorm:"column:message" json:"message"`
	CheckedAt        int64  `gorm:"column:checked_at" json:"checkedAt"`
	UpdatedAt        int64  `gorm:"column:updated_at" json:"updatedAt"`
}

func (SourceHealth) TableName() string { return "source_health" }

// SiteConfig 站点基础配置 (table: site_config) — 单行, id 固定 1。
type SiteConfig struct {
	Id          int8   `gorm:"column:id;primaryKey" json:"id"`
	SiteName    string `gorm:"column:site_name" json:"siteName"`
	Domain      string `gorm:"column:domain" json:"domain"`
	Logo        string `gorm:"column:logo" json:"logo"`
	Keyword     string `gorm:"column:keyword" json:"keyword"`
	Description string `gorm:"column:description" json:"description"`
	State       int8   `gorm:"column:state" json:"state"`
	Hint        string `gorm:"column:hint" json:"hint"`
	UpdatedAt   int64  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (SiteConfig) TableName() string { return "site_config" }

// AppVersion APK 自升级版本 (table: app_version)。
type AppVersion struct {
	Id          int64       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Channel     int8        `gorm:"column:channel" json:"channel"` // 0 stable / 1 beta
	VersionCode int         `gorm:"column:version_code" json:"versionCode"`
	VersionName string      `gorm:"column:version_name" json:"versionName"`
	ApkUrl      string      `gorm:"column:apk_url" json:"apkUrl"`
	Changelog   string      `gorm:"column:changelog" json:"changelog"`
	Force       bool        `gorm:"column:force" json:"force"`
	Whitelist   StringSlice `gorm:"column:whitelist;type:json" json:"whitelist"`
	CreatedAt   int64       `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
}

func (AppVersion) TableName() string { return "app_version" }

// 文件类型
const (
	FileTypeCover  int = 0 // 影片封面
	FileTypeAvatar int = 1 // 用户头像
)

// FileInfo 图库/二进制元数据 (table: files) — BlobStore 管二进制, 此表只管元数据。
type FileInfo struct {
	Id          int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Link        string `gorm:"column:link" json:"link"`
	ObjectKey   string `gorm:"column:object_key" json:"objectKey"`
	Uid         int    `gorm:"column:uid" json:"uid"`
	RelevanceId int64  `gorm:"column:relevance_id" json:"relevanceId"`
	Type        int    `gorm:"column:type" json:"type"`
	Fid         string `gorm:"column:fid" json:"fid"`
	FileType    string `gorm:"column:file_type" json:"fileType"`
	CreatedAt   int64  `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
}

func (FileInfo) TableName() string { return "files" }

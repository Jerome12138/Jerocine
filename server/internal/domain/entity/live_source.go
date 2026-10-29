package entity

// LiveSource 直播源（m3u 播放列表）(table: live_source)。
// 一条直播源 = 一个 m3u 地址；频道与分组由服务端按需拉取解析并缓存（键含源地址 hash，
// 地址变更即自然换键，无需显式失效）。
type LiveSource struct {
	Id        int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name      string `gorm:"column:name" json:"name"`
	URL       string `gorm:"column:url" json:"url"`
	Remark    string `gorm:"column:remark" json:"remark"`
	Sort      int    `gorm:"column:sort" json:"sort"`
	State     int8   `gorm:"column:state" json:"state"`
	CreatedAt int64  `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt int64  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (LiveSource) TableName() string { return "live_source" }

// 状态: 与站内其它开关同口径(0 启用 / 1 停用)。
const (
	LiveSourceEnabled  int8 = 0
	LiveSourceDisabled int8 = 1
)

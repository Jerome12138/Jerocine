package entity

// LiveChannelState 直播频道状态（覆盖层）(table: live_channel_state)。
// 频道本体由 m3u 解析产生，本表按 (source_id, url) 对齐做**覆盖**：
//   - hidden: 前台隐藏（解析频道的"删除"即置位，源重解析后覆盖层仍生效）
//   - display_name: 显示名覆盖（空 = 用解析名）
//   - is_custom=1: 后台手工添加的自定义频道（url/name/group 全来自本表，物理删可用）
//
// CORS/活性探源结果不入库（易失可重建），存 Redis 见 cache.KeyLiveHealth。
type LiveChannelState struct {
	Id          int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	SourceId    int64  `gorm:"column:source_id" json:"sourceId"`
	URL         string `gorm:"column:url" json:"url"`
	Name        string `gorm:"column:name" json:"name"`
	GroupName   string `gorm:"column:group_name" json:"groupName"`
	Hidden      int8   `gorm:"column:hidden" json:"hidden"`
	DisplayName string `gorm:"column:display_name" json:"displayName"`
	IsCustom    int8   `gorm:"column:is_custom" json:"isCustom"`
	CreatedAt   int64  `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt   int64  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (LiveChannelState) TableName() string { return "live_channel_state" }

const (
	LiveChannelVisible int8 = 0
	LiveChannelHidden  int8 = 1
	LiveChannelParsed  int8 = 0 // 覆盖层条目对齐解析结果
	LiveChannelCustom  int8 = 1 // 后台手工添加
)

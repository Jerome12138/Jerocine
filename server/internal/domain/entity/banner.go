package entity

// Banner 首页轮播配置 (table: banner)。
// image 是横图(宽幅主视觉), poster 是竖图(窄屏/兜底); 跳转优先 link(可外链), 否则关联影片 mid。
type Banner struct {
	Id        int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Title     string `gorm:"column:title" json:"title"`
	Subtitle  string `gorm:"column:subtitle" json:"subtitle"`
	Image     string `gorm:"column:image" json:"image"`
	Poster    string `gorm:"column:poster" json:"poster"`
	Mid       int64  `gorm:"column:mid" json:"mid"`
	Link      string `gorm:"column:link" json:"link"`
	Sort      int    `gorm:"column:sort" json:"sort"`
	State     int8   `gorm:"column:state" json:"state"`
	StartAt   int64  `gorm:"column:start_at" json:"startAt"`
	EndAt     int64  `gorm:"column:end_at" json:"endAt"`
	CreatedAt int64  `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt int64  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (Banner) TableName() string { return "banner" }

// Banner 状态: 与站内其它开关同口径(0 启用 / 1 停用)。
const (
	BannerEnabled  int8 = 0
	BannerDisabled int8 = 1
)

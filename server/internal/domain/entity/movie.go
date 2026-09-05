package entity

// Movie 影片详情主表 (table: movie)。
// mid 全局唯一主键, 不嵌 cid → 详情可凭 mid 一跳直查 (Redis key v1:movie:detail:{mid})。
type Movie struct {
	Mid          int64       `gorm:"column:mid;primaryKey" json:"mid"`
	Cid          int64       `gorm:"column:cid" json:"cid"`
	Pid          int64       `gorm:"column:pid" json:"pid"`
	Name         string      `gorm:"column:name" json:"name"`
	SubTitle     string      `gorm:"column:sub_title" json:"subTitle"`
	CName        string      `gorm:"column:c_name" json:"cName"`
	EnName       string      `gorm:"column:en_name" json:"enName"`
	Initial      string      `gorm:"column:initial" json:"initial"`
	ClassTag     string      `gorm:"column:class_tag" json:"classTag"`
	Area         string      `gorm:"column:area" json:"area"`
	Language     string      `gorm:"column:language" json:"language"`
	Year         int         `gorm:"column:year" json:"year"`
	Actor        string      `gorm:"column:actor" json:"actor"`
	Director     string      `gorm:"column:director" json:"director"`
	Writer       string      `gorm:"column:writer" json:"writer"`
	Content      string      `gorm:"column:content" json:"content"`
	DbId         int64       `gorm:"column:db_id" json:"dbId"`
	DbScore      float64     `gorm:"column:db_score;type:decimal(3,1)" json:"dbScore"`
	Hits         int64       `gorm:"column:hits" json:"hits"`
	State        string      `gorm:"column:state" json:"state"`
	Remarks      string      `gorm:"column:remarks" json:"remarks"`
	Cover        string      `gorm:"column:cover" json:"cover"`
	PlayFrom     StringSlice `gorm:"column:play_from;type:json" json:"playFrom"`
	DownFrom     string      `gorm:"column:down_from" json:"downFrom"`
	ReleaseStamp int64       `gorm:"column:release_stamp" json:"releaseStamp"`
	UpdateStamp  int64       `gorm:"column:update_stamp" json:"updateStamp"`
	CreatedAt    int64       `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt    int64       `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (Movie) TableName() string { return "movie" }

// MovieSearch 物化卡片/检索宽表 (table: movie_search) — CQRS 读模型。
// cover 进表 → 列表/卡片一次查询出, 无需回填 Redis basic 或 join files。
type MovieSearch struct {
	Mid          int64   `gorm:"column:mid;primaryKey" json:"mid"`
	Cid          int64   `gorm:"column:cid" json:"cid"`
	Pid          int64   `gorm:"column:pid" json:"pid"`
	Name         string  `gorm:"column:name" json:"name"`
	SubTitle     string  `gorm:"column:sub_title" json:"subTitle"`
	CName        string  `gorm:"column:c_name" json:"cName"`
	ClassTag     string  `gorm:"column:class_tag" json:"classTag"`
	Area         string  `gorm:"column:area" json:"area"`
	Language     string  `gorm:"column:language" json:"language"`
	Year         int     `gorm:"column:year" json:"year"`
	Initial      string  `gorm:"column:initial" json:"initial"`
	NamePinyin   string  `gorm:"column:name_pinyin" json:"namePinyin"` // 片名拼音首字母串(大写), 首字母搜索用
	State        string  `gorm:"column:state" json:"state"`
	Remarks      string  `gorm:"column:remarks" json:"remarks"`
	DbScore      float64 `gorm:"column:db_score;type:decimal(3,1)" json:"dbScore"`
	Hits         int64   `gorm:"column:hits" json:"hits"`
	Cover        string  `gorm:"column:cover" json:"cover"`
	ReleaseStamp int64   `gorm:"column:release_stamp" json:"releaseStamp"`
	UpdateStamp  int64   `gorm:"column:update_stamp" json:"updateStamp"`
	CreatedAt    int64   `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt    int64   `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (MovieSearch) TableName() string { return "movie_search" }

// MoviePlaySource 多源播放 (table: movie_play_source) — episodes_json 整源一行。
// match_key = GenerateHashKey(片名/dbId), 用于跨站点匹配同一影片的播放源。
type MoviePlaySource struct {
	Id       int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Mid      *int64 `gorm:"column:mid" json:"mid"`
	SiteId   string `gorm:"column:site_id" json:"siteId"`
	MatchKey string `gorm:"column:match_key" json:"matchKey"`
	// SourceVodID 源站影片ID(maccms vod_id / xml <id>)。补充源同片按 hash(片名)/hash(豆瓣ID)
	// 双键各存一行, 仅 (site_id, source_vod_id) 能唯一定位一部片, 用于去重统计(0=旧行未知)。
	SourceVodID int64       `gorm:"column:source_vod_id" json:"sourceVodId"`
	PlayFrom    string      `gorm:"column:play_from" json:"playFrom"`
	Episodes    EpisodeList `gorm:"column:episodes_json;type:json" json:"episodes"`
	CreatedAt   int64       `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt   int64       `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (MoviePlaySource) TableName() string { return "movie_play_source" }

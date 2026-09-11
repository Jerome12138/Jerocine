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
	// PubDate 上映日期, 源站 vod_pubdate 规范化后的 ISO 前缀串: "2026-09-11" / "2026-07" /
	// "2007" / ""(源站未提供)。精度自描述, 字典序=时间序, 可直接参与 ORDER BY。
	PubDate      string      `gorm:"column:pub_date;size:10" json:"pubDate"`
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
	// Backdrop 横图(16:9, 详情页 hero / 轮播兜底)。由后台 TMDB worker 下载到本地 blob 后回填,
	// 采集 upsert 不覆盖该列(见 movieUpsertExclude); "-" 为"检索无果"哨兵, 对外 DTO 归一为空串。
	Backdrop string `gorm:"column:backdrop" json:"backdrop"`
	// 榜单热度列, 由豆瓣榜单刷新任务写入(见 service/hot_service.go)。属"本地计算列":
	// 采集 upsert 一律排除(见 movieUpsertExclude), movie_search 影子表换表前从 movie 回灌。
	// HotRank=当前榜位(0 不在榜) / HotRankAt=本轮抓取时间(Unix 秒) / HotScore=合成热度分 /
	// DbIdSrc=movie.db_id 来源(0 源站自带 / 1 榜单回填)。
	HotRank   int   `gorm:"column:hot_rank" json:"hotRank"`
	HotRankAt int64 `gorm:"column:hot_rank_at" json:"hotRankAt"`
	HotScore  int   `gorm:"column:hot_score" json:"hotScore"`
	DbIdSrc   int8  `gorm:"column:db_id_src" json:"dbIdSrc"`
	PlayFrom     StringSlice `gorm:"column:play_from;type:json" json:"playFrom"`
	DownFrom     string      `gorm:"column:down_from" json:"downFrom"`
	ReleaseStamp int64       `gorm:"column:release_stamp" json:"releaseStamp"`
	UpdateStamp  int64       `gorm:"column:update_stamp" json:"updateStamp"`
	CreatedAt    int64       `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt    int64       `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
	// DeletedAt 软删除时间戳(毫秒), 0 = 未删。采集 upsert 不覆盖该列(见 movieUpsertCols),
	// 所以已删影片被源站重新推回来也不会自动复活, 只有后台显式恢复才会。
	DeletedAt int64 `gorm:"column:deleted_at" json:"deletedAt"`
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
	// PubDate 上映日期(ISO 前缀串), 与 movie.pub_date 同源, 供卡片/详情展示与排序 tiebreak。
	PubDate      string  `gorm:"column:pub_date;size:10" json:"pubDate"`
	Initial      string  `gorm:"column:initial" json:"initial"`
	NamePinyin   string  `gorm:"column:name_pinyin" json:"namePinyin"` // 片名拼音首字母串(大写), 首字母搜索用
	State        string  `gorm:"column:state" json:"state"`
	Remarks      string  `gorm:"column:remarks" json:"remarks"`
	DbScore      float64 `gorm:"column:db_score;type:decimal(3,1)" json:"dbScore"`
	Hits         int64   `gorm:"column:hits" json:"hits"`
	Cover        string  `gorm:"column:cover" json:"cover"`
	// Backdrop 横图, 与 movie.backdrop 由 worker 双写同步(采集 upsert 不覆盖, 见 searchUpsertExclude)。
	Backdrop     string  `gorm:"column:backdrop" json:"backdrop"`
	// 榜单热度列, 与 movie 同列同源(由 hot_service 双写)。属"本地计算列":
	// 采集 upsert 不覆盖(见 searchUpsertExclude); 全量重采走影子表重建, 换表前从 movie 回灌。
	HotRank      int   `gorm:"column:hot_rank" json:"hotRank"`
	HotRankAt    int64 `gorm:"column:hot_rank_at" json:"hotRankAt"`
	HotScore     int   `gorm:"column:hot_score" json:"hotScore"`
	DbIdSrc      int8  `gorm:"column:db_id_src" json:"dbIdSrc"`
	ReleaseStamp int64   `gorm:"column:release_stamp" json:"releaseStamp"`
	UpdateStamp  int64   `gorm:"column:update_stamp" json:"updateStamp"`
	CreatedAt    int64   `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt    int64   `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
	// DeletedAt 软删除标记的读模型镜像; 由 movie.deleted_at 同步而来(见 SearchRepository.SyncDeletedFromMovie)。
	// 公开列表/检索/推荐全部带 deleted_at = 0 过滤, 后台列表可显式查已删。
	DeletedAt int64 `gorm:"column:deleted_at" json:"deletedAt"`
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

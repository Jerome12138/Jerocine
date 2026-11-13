package entity

// MovieEpisodeEpid 集身份派生表（P1-1, 2026-11-10）—— **一集一行**，**单向派生**自
// `movie_play_source.episodes_json`（原始真相永不改）。
//
// 口径见 docs/方案-集身份epid落地路线与弹幕准确性-2026-11-09.md（v5.0 §5.2）：
//   - 受控冗余 `mid/match_key/site_id/play_from`：目的只有一个 —— **免 JOIN 1GB 巨表**（性能铁律 1）
//   - **不冗余 `link`**（大字段 + 最热变动）
//   - `Epid` 空串 = **不产身份**（L6：裸「第N集」/推不出期号的集）⇒ 下游走位置兜底
//   - `FromSrc` = 期号来源（self|bridge|anchor|none），供统计（P1-3 的 from= 同源）
//   - `SrcEpHash` = 派生时该源行的 `ep_hash` ⇒ **派生状态锚**（与源行当前 ep_hash 不等 ⇒ 该源行脏）
type MovieEpisodeEpid struct {
	Id        int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	SrcRowId  int64  `gorm:"column:src_row_id" json:"srcRowId"`
	Mid       *int64 `gorm:"column:mid" json:"mid"`
	MatchKey  string `gorm:"column:match_key" json:"matchKey"`
	SiteId    string `gorm:"column:site_id" json:"siteId"`
	PlayFrom  string `gorm:"column:play_from" json:"playFrom"`
	EpIndex   int    `gorm:"column:ep_index" json:"epIndex"`
	Epid      string `gorm:"column:epid" json:"epid"`
	FromSrc   string `gorm:"column:from_src" json:"fromSrc"`
	RawName   string `gorm:"column:raw_name" json:"rawName"`
	SrcEpHash string `gorm:"column:src_ep_hash" json:"srcEpHash"`
	// DupCopy 双写副本标记（2026-11-13）—— 与源行一致（0 = 片名键主份 / 1 = 豆瓣键副本）。
	// 对齐率系列指标按 `dup_copy = 0` **排除双写副本**（口径见
	// docs/epid-集身份/分析-采集侧键口径与匹配度-2026-11-13.md §10.5）。
	DupCopy    int8  `gorm:"column:dup_copy" json:"dupCopy"`
	ComputedAt int64 `gorm:"column:computed_at" json:"computedAt"`
}

func (MovieEpisodeEpid) TableName() string { return "movie_episode_epid" }

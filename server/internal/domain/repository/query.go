package repository

// 查询值对象 (纯领域, 无框架依赖)。

import "server/internal/domain/entity"

// Page 分页请求。
type Page struct {
	Current int
	Size    int
}

// Normalize 用默认页大小修正非法分页参数。
func (p Page) Normalize(defSize int) Page {
	c, s := p.Current, p.Size
	if c <= 0 {
		c = 1
	}
	if s <= 0 {
		s = defSize
	}
	if s > 200 {
		s = 200 // 硬上限防滥用
	}
	if c > 10000 {
		c = 10000 // 页码上界: 防超大值 Offset 溢出为负(500)与深翻页慢查询
	}
	return Page{Current: c, Size: s}
}

func (p Page) Offset() int { return (p.Current - 1) * p.Size }
func (p Page) Limit() int  { return p.Size }

// ClassifySort 分类首页/筛选页排序维度。
type ClassifySort int

const (
	SortLatest ClassifySort = iota // 最新上线 (year DESC, pub_date DESC, update_stamp DESC)
	SortHot                        // 热度优先 (hot_score DESC, year DESC, update_stamp DESC, mid DESC)
	SortRecent                     // 最近更新 (update_stamp DESC)
	SortScore                      // 评分优先 (db_score DESC, year DESC, mid DESC)
)

// 软删除态过滤维度。
const (
	DeletedExclude = 0  // 仅未删 (公开读路径默认, 也是零值)
	DeletedOnly    = 1  // 仅已删 (后台回收站)
	DeletedInclude = -1 // 不限 (后台"全部")
)

// FilterSpec /films 多维筛选条件 (空字段忽略)。
type FilterSpec struct {
	Keyword  string
	Pid      int64
	Cid      int64
	Plot     string
	Area     string
	Language string
	Year     int
	Sort     string // update_stamp | hot(别名 hits) | score(别名 db_score) | latest(别名 release_stamp)
	// Deleted 软删除态过滤: DeletedExclude(0, 公开默认) / DeletedOnly(1) / DeletedInclude(-1)。
	// 零值即"仅未删", 所以公开调用方无需关心该字段。
	Deleted int
}

// RelatedSeed 相关推荐种子 (来自当前影片)。
type RelatedSeed struct {
	Mid      int64
	Cid      int64
	Name     string
	ClassTag string
	Area     string
	Language string
}

// ---- 榜单热度值对象 (见 docs/榜单热度方案-2026-09-11.md) ----

// HotCandidate 榜单匹配/算分所需的本地影片行 —— 只取匹配与算分要用的列,
// 不拉 content/play_from 这类大字段(整表扫一遍也扛得住)。
type HotCandidate struct {
	Mid       int64
	DbId      int64
	Name      string
	Area      string
	Year      int
	Remarks   string
	Actor     string
	Director  string
	DbScore   float64
	HotRank   int   // 仅 ListHotBoard 填充
	HotRankAt int64 // 仅 ListHotBoard 填充
}

// HotRow 一次热度刷新要写的一行, 事务内双写 movie 与 movie_search。
//
// 零值语义: HotRank=0 且 HotRankAt=0 表示"落榜", 该行的 hot_score 回落兜底分、hot_board 清空;
// DbId / DbScore 为 0 表示不回填该列(db_score=0 在库里就是"无评分", 不存在"写 0 是有效值"的情形)。
type HotRow struct {
	Mid       int64
	HotRank   int
	HotRankAt int64
	HotBoard  string // 榜位来源集合名(如 movie_hot_gaia), 落榜行为空串
	HotScore  int
	DbId      int64
	DbScore   float64
}

// ---- 热榜快照 / 轮播候选 (2026-10-23, 见 docs/热榜页面与轮播候选方案) ----

// HotBoardItem 热榜快照一行: 某个榜(board)的第 Rank 位影片 mid。
// 同一部片命中多个榜会保留多行(候选派生据此计算多榜回血)。每日榜单刷新成功后全量替换。
type HotBoardItem struct {
	Board     string
	Rank      int
	Mid       int64
	RefreshAt int64
}

// HotBoardCandidate 轮播候选池一行: 每日刷新算好排序与分数后落库(恒 16 位),
// 轮播 compose 时一次 SELECT 取走, 不再实时重算。
type HotBoardCandidate struct {
	WeightPos  int    // 候选池排序位(0..15)
	Board      string // 来源榜(该候选取位的榜)
	Rank       int    // 在来源榜的位次
	Mid        int64
	Score      int  // 候选分(含 TC 降级/多榜回血/大陆保底)
	Guaranteed bool // 是否为"中国大陆保底"替换位
	RefreshAt  int64
}

// HotBoardMember 热榜快照 JOIN 影片读模型后的一行 —— 候选派生与热榜页共用。
// 嵌入 MovieSearch 拿到影片字段(area/year/pid/remarks/cover/backdrop 等),
// SBoard/SRank 为该成员在快照中的榜与位次(无 JOIN 全榜视图时取影片自身的代表榜)。
type HotBoardMember struct {
	entity.MovieSearch `gorm:"embedded"`
	SBoard             string `gorm:"column:sb_board"`
	SBRank             int    `gorm:"column:sb_rank"`
}

// HotBoardFilterFacets 热榜筛选维度的服务端聚合部分(年份/地区/语言)。
// 榜单维度(boards)由 service 以 douban.Collections 为唯一来源拼装, 不查库。
type HotBoardFilterFacets struct {
	Years     []int
	Areas     []string
	Languages []string
}

// TagOption 单个筛选标签。
type TagOption struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FilterOptions 某一级分类的全部筛选维度 (7 维: Category/Plot/Area/Language/Year/Initial/Sort)。
type FilterOptions struct {
	Titles   map[string]string      `json:"titles"`
	Tags     map[string][]TagOption `json:"tags"`
	SortList []string               `json:"sortList"`
}

// ---- 埋点分析值对象 ----

// CategoryCount 按类别计数。
type CategoryCount struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

// PathCount 按路径计数 (top paths)。
type PathCount struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

// MidCount 按影片 mid 计数 (收藏最多 / 热点视频聚合)。
type MidCount struct {
	Mid   int64 `json:"mid"`
	Count int64 `json:"count"`
}

// ApiPerf 接口性能聚合 (按 path)。
type ApiPerf struct {
	Path  string  `json:"path"`
	Count int64   `json:"count"`
	AvgMs float64 `json:"avgMs"`
	MaxMs int64   `json:"maxMs"`
}

// OverviewStats 概览原始聚合 (service 再组装成前端 OverviewResp)。
type OverviewStats struct {
	PV          int64
	ApiCount    int64
	ErrorCount  int64
	UV          int64 // distinct session_id
	UniqueUsers int64 // distinct user_id(非空非0)
	AvgApiMs    float64
}

// ActionPerfAgg 按 API action(埋点 extra.action=接口路径) 的聚合, 百分位由 service 计算。
type ActionPerfAgg struct {
	Action string
	Count  int64
	Avg    float64
}

// TelemetryFilter 事件列表筛选。
type TelemetryFilter struct {
	Category  string
	Path      string
	SinceUnix int64 // server_ts 下界 (秒); 0 表示不限
}

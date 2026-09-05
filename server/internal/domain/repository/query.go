package repository

// 查询值对象 (纯领域, 无框架依赖)。

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
	return Page{Current: c, Size: s}
}

func (p Page) Offset() int { return (p.Current - 1) * p.Size }
func (p Page) Limit() int  { return p.Size }

// ClassifySort 分类首页三榜排序维度。
type ClassifySort int

const (
	SortLatest ClassifySort = iota // 最新上映 (release_stamp)
	SortHot                        // 排行榜 (hits)
	SortRecent                     // 最近更新 (update_stamp)
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
	Sort     string // update_stamp | hits | db_score | release_stamp
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

package entity

// Category 分类 (table: category) — 父子用 pid 关联, 行级 CRUD。
// 终态: category 是 MySQL 权威真相, Redis v1:cat:tree 仅缓存整树, miss 回源重建。
type Category struct {
	Id   int64  `gorm:"column:id;primaryKey" json:"id"`
	Pid  int64  `gorm:"column:pid" json:"pid"`
	Name string `gorm:"column:name" json:"name"`
	Show bool   `gorm:"column:show" json:"show"`
	// FilmVisible 影片可见性: false = 该分类影片从公开列表/搜索全部剔除(详情直链仍可达)。
	// 与 Show(菜单显隐)拆分: 菜单藏了影片仍可被搜到; 影片藏了才是根本上看不到。
	// Category 经公开 GET /categories 直出(无鉴权), 此列为普通布尔, 无敏感泄露面。
	FilmVisible bool  `gorm:"column:film_visible" json:"filmVisible"`
	Sort        int   `gorm:"column:sort" json:"sort"`
	CreatedAt   int64 `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt   int64 `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (Category) TableName() string { return "category" }

// CategoryNode 分类树节点 (内存结构, 非表) — 由 category 行装配, 缓存到 v1:cat:tree。
type CategoryNode struct {
	Category
	Children []*CategoryNode `json:"children"`
	// FilmCount 该分类 pid/cid 直挂在架影片数(后台分类管理专用, 公开口不填)。
	// 指针区分「真 0 部」(输出 0, 前端显示 "0 部")与「计数失败降级」(nil 缺省,
	// 前端显示 —) —— int64+omitempty 会把真 0 也吞成缺省, 两种状态混淆。
	FilmCount *int64 `json:"filmCount,omitempty"`
}

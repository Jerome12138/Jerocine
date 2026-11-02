package entity

// Category 分类 (table: category) — 父子用 pid 关联, 行级 CRUD。
// 终态: category 是 MySQL 权威真相, Redis v1:cat:tree 仅缓存整树, miss 回源重建。
type Category struct {
	Id   int64  `gorm:"column:id;primaryKey" json:"id"`
	Pid  int64  `gorm:"column:pid" json:"pid"`
	Name string `gorm:"column:name" json:"name"`
	// Show 顶栏导航显示(web+TV 顶部一级导航; 000037 起仅一级有意义, 二级不再消费此列)。
	Show bool `gorm:"column:show" json:"show"`
	// SidebarShow 侧边栏显示(桌面常驻侧栏 + 移动抽屉, 一级与二级各自独立可设)。
	// 000037 从 show 拆出: 顶栏与侧边栏显隐解耦, 存量值 = 原 show。
	SidebarShow bool `gorm:"column:sidebar_show" json:"sidebarShow"`
	// FilmVisible 影片可见性: false = 该分类影片从公开列表/搜索/推荐全部剔除(详情直链仍可达)。
	// 与 Show/SidebarShow(菜单显隐)拆分: 菜单藏了影片仍可被搜到; 影片藏了才是根本上看不到。
	// 置 false 时服务层强制联动: 本节点与全部子孙的 show/sidebar_show 一并置 false。
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

package entity

// Category 分类 (table: category) — 父子用 pid 关联, 行级 CRUD。
// 终态: category 是 MySQL 权威真相, Redis v1:cat:tree 仅缓存整树, miss 回源重建。
type Category struct {
	Id        int64  `gorm:"column:id;primaryKey" json:"id"`
	Pid       int64  `gorm:"column:pid" json:"pid"`
	Name      string `gorm:"column:name" json:"name"`
	Show      bool   `gorm:"column:show" json:"show"`
	Sort      int    `gorm:"column:sort" json:"sort"`
	CreatedAt int64  `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt int64  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (Category) TableName() string { return "category" }

// CategoryNode 分类树节点 (内存结构, 非表) — 由 category 行装配, 缓存到 v1:cat:tree。
type CategoryNode struct {
	Category
	Children []*CategoryNode `json:"children"`
}

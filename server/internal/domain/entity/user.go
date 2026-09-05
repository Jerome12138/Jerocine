package entity

import "gorm.io/gorm"

// 用户角色
const (
	RoleUser  int = 0 // 普通用户
	RoleAdmin int = 1 // 管理员
)

// User 用户 (table: users) — 保留软删除(管理员维护、极少删)。
type User struct {
	gorm.Model        // ID / CreatedAt / UpdatedAt / DeletedAt(软删)
	UserName   string `gorm:"column:user_name" json:"userName"`
	Password   string `gorm:"column:password" json:"-"`
	Role       int    `gorm:"column:role" json:"role"`
}

func (User) TableName() string { return "users" }

// UserHistory 观看历史 (table: user_history) — 硬删, (user_id, mid) 唯一。
type UserHistory struct {
	Id        int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserId    int64  `gorm:"column:user_id" json:"userId"`
	Mid       int64  `gorm:"column:mid" json:"mid"`
	PlayFrom  string `gorm:"column:play_from" json:"playFrom"`
	Episode   int    `gorm:"column:episode" json:"episode"`
	Progress  int    `gorm:"column:progress" json:"progress"`
	Duration  int    `gorm:"column:duration" json:"duration"`
	CreatedAt int64  `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
	UpdatedAt int64  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (UserHistory) TableName() string { return "user_history" }

// UserFavorite 收藏 (table: user_favorite) — 硬删, (user_id, mid) 唯一。
type UserFavorite struct {
	Id        int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserId    int64 `gorm:"column:user_id" json:"userId"`
	Mid       int64 `gorm:"column:mid" json:"mid"`
	CreatedAt int64 `gorm:"column:created_at;autoCreateTime:milli" json:"createdAt"`
}

func (UserFavorite) TableName() string { return "user_favorite" }

// UserSkipSetting 每用户每片的片头/片尾跳过秒数 (table: user_skip_setting) — (user_id, mid) 唯一(upsert)。
type UserSkipSetting struct {
	Id        int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserId    int64 `gorm:"column:user_id" json:"userId"`
	Mid       int64 `gorm:"column:mid" json:"mid"`
	IntroSec  int   `gorm:"column:intro_sec" json:"intro"`
	OutroSec  int   `gorm:"column:outro_sec" json:"outro"`
	UpdatedAt int64 `gorm:"column:updated_at;autoUpdateTime:milli" json:"updatedAt"`
}

func (UserSkipSetting) TableName() string { return "user_skip_setting" }

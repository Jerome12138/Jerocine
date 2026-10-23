package entity

import "time"

// 注册申请状态 (table: register_apply)。
const (
	ApplyStatusPending  int8 = 0 // 待审
	ApplyStatusApproved int8 = 1 // 已通过(已发激活邮件, 待用户激活)
	ApplyStatusRejected int8 = 2 // 已驳回(必填理由, 可重申)
)

// ApplyStatusAny 后台列表筛选哨兵: 不限状态(与 entity.FailureStatusAny 同约定)。
const ApplyStatusAny int8 = -1

// RegisterApply 申请注册审核行 (table: register_apply)。
// 刻意不存密码: 申请表单不收密码 —— 审批通过后发一次性激活链接, 用户到站设密码才落 users
// (设计 §9.2: 申请→审批可能隔数天, 激活时设当场记得住; pending 行少一类敏感数据)。
// user_name 为申请时填报值, 审批通过到激活的窗口期可能被 open 注册者占用 → 激活页允许改名。
// ReviewedBy/ReviewedAt/ActivatedAt 审核与激活时间: 未发生时库列为 NULL(migration 定义),
// 必须用 *time.Time —— 值类型零值会被 GORM 写成 '0000-00-00', MySQL 严格模式直接拒绝
// (2026-10-23 实测: 提交注册申请报 Error 1292 Incorrect datetime value)。
type RegisterApply struct {
	Id           int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserName     string     `gorm:"column:user_name" json:"userName"`
	Email        string     `gorm:"column:email" json:"email"`
	Reason       string     `gorm:"column:reason" json:"reason"`
	SourceIp     string     `gorm:"column:source_ip" json:"sourceIp"`
	Status       int8       `gorm:"column:status" json:"status"`
	RejectReason string     `gorm:"column:reject_reason" json:"rejectReason"`
	ReviewedBy   int64      `gorm:"column:reviewed_by" json:"reviewedBy"`
	ReviewedAt   *time.Time `gorm:"column:reviewed_at" json:"reviewedAt"`
	ActivatedAt  *time.Time `gorm:"column:activated_at" json:"activatedAt"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
}

func (RegisterApply) TableName() string { return "register_apply" }

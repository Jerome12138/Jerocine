// Package mysql 实现 domain/repository 端口。唯一写 SQL 的地方。
// 事务经 context 传播: TxManager.WithinTx 把 *gorm.DB 挂到 ctx, dbFrom 解析出来,
// 故所有 repo 方法只需 ctx, 不在签名里泄漏 *gorm.DB。
package mysql

import (
	"context"

	"gorm.io/gorm"

	"server/internal/domain/repository"
)

type ctxTxKey struct{}

// dbFrom 返回当前 ctx 应使用的 *gorm.DB: 事务中用事务连接, 否则用基础连接(均绑定 ctx)。
func dbFrom(ctx context.Context, base *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(ctxTxKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return base.WithContext(ctx)
}

type txManager struct{ db *gorm.DB }

// NewTxManager 构造事务管理器。
func NewTxManager(db *gorm.DB) repository.TxManager { return &txManager{db: db} }

func (m *txManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(ctxTxKey{}).(*gorm.DB); ok {
		return fn(ctx) // 已在事务中, 复用(嵌套调用安全)
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, ctxTxKey{}, tx))
	})
}

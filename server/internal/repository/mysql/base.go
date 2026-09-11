// Package mysql 实现 domain/repository 端口。唯一写 SQL 的地方。
// 事务经 context 传播: TxManager.WithinTx 把 *gorm.DB 挂到 ctx, dbFrom 解析出来,
// 故所有 repo 方法只需 ctx, 不在签名里泄漏 *gorm.DB。
package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"server/internal/domain/repository"
)

// nowMilli 当前毫秒时间戳。库内 BIGINT 时间列统一用毫秒口径(软删除戳、失败台账等), 故下沉到共享层。
func nowMilli() int64 { return time.Now().UnixMilli() }

// conditionalUpsert "源站给不出时保留本地旧值"的列 → 冲突时的赋值表达式。
//
// 背景: 采集 upsert 原先是齐刷刷的 `col = VALUES(col)` 覆盖写, 于是一旦源站这次没给数据
// (vod_pubdate 覆盖仅 38%、vod_douban_id 约 59%), 本地已有的值(榜单回填的 db_id、补上的
// db_score、上一轮采到的 pub_date)就会被一条空值/0 抹掉 —— 白做。
//
// 这些列都是"源站与本地都会写同一列、本地只在源站给不出时才补"的性质, 故冲突时按
// IF(VALUES(col) 有效, VALUES(col), col) 取舍: 源站给了就用源站的, 没给就保留已有的。
// 判定口径按列类型: 数值列 > 0; 字符串列 <> ''。
//
// 刻意不做"全表所有列都加保护": 那会让源站主动清空 / 失效的字段(过期简介、失效封面)
// 永远滞留旧值。当前只有下面这四列存在"源站空、本地有"的情形。
var conditionalUpsert = map[string]string{
	"db_id":    "IF(VALUES(db_id) > 0, VALUES(db_id), db_id)",
	"db_score": "IF(VALUES(db_score) > 0, VALUES(db_score), db_score)",
	"year":     "IF(VALUES(year) > 0, VALUES(year), year)",
	"pub_date": "IF(VALUES(pub_date) <> '', VALUES(pub_date), pub_date)",
}

// upsertAssignments 构造 ON DUPLICATE KEY UPDATE 的赋值列表:
// 常规列 `col = VALUES(col)` 覆盖, conditionalUpsert 里的列改走上述条件表达式。
// 入参切片本身仍是 upsert 列清单的唯一来源(反射测试校验的也是它), 此处不改变其口径。
//
// 用带序的 clause.Set 而非 clause.Assignments(map 遍历顺序随机), 让生成的 SQL 稳定可核对。
func upsertAssignments(cols []string) clause.Set {
	out := make([]clause.Assignment, 0, len(cols))
	for _, c := range cols {
		expr := "VALUES(" + c + ")"
		if e, ok := conditionalUpsert[c]; ok {
			expr = e
		}
		out = append(out, clause.Assignment{Column: clause.Column{Name: c}, Value: gorm.Expr(expr)})
	}
	return clause.Set(out)
}

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

package repository

import "context"

// TxManager 事务边界抽象。
//
// 约定: 事务通过 context 传播 —— WithinTx 在 ctx 上挂一个事务句柄,
// 各 repository 方法内部从 ctx 解析事务句柄(命中则用事务连接, 否则用基础连接)。
// 这样 repository 方法签名只需 ctx, 不泄漏 *gorm.DB / Tx 类型, 领域层保持纯净。
//
// 采集"事务先行双写"即: txm.WithinTx(ctx, func(ctx) error { movieRepo.Upsert(ctx,..); searchRepo.Upsert(ctx,..); playRepo.UpsertBatch(ctx,..) })。
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

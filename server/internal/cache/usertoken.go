package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// 多设备 token(协调态 db1): 每用户一个 ZSET, member=token, score=登录时间戳。
// 支持最多 maxDevices 个设备, 超出按最旧剔除。从旧 model/system/Jwt.go 平移到 cache 层。
//
// 重部署不掉登录 (2026-06): 主因是 token 集合曾带 TTL → 被 Redis volatile-lru 在内存压力下
// 淘汰, 致用户集体掉登录。修复 = 集合不再设 TTL(volatile-lru 只淘汰带 TTL 的键;集合已按
// maxDevices 截断, 总量有界; token 过期由 JWT 自身把关, 无需 TTL 清理), 加 AOF 持久化即可
// 跨重启存活。**安全**: 校验保持 fail-closed —— 集合/成员不存在(登出/踢人/被清)一律判无效,
// 绝不"放行+回填"(那会把会话撤销永久绕过); 仅当 Redis 连接不可达(临时网络错误)才暂放行靠
// JWT 兜底, 且不回填。整库被清(down -v/flush)按安全默认强制重新登录。
//
// SaveUserToken 登录成功后加入 token 集合, 超出 maxDevices 剔除最旧。
func SaveUserToken(ctx context.Context, userId int64, token string, maxDevices int, ttl time.Duration) error {
	if coordRdb == nil {
		return nil
	}
	key := KeyToken(userId)
	now := float64(time.Now().Unix())
	if err := coordRdb.ZAdd(ctx, key, redis.Z{Score: now, Member: token}).Err(); err != nil {
		return err
	}
	if maxDevices > 0 {
		if cnt, err := coordRdb.ZCard(ctx, key).Result(); err == nil && cnt > int64(maxDevices) {
			coordRdb.ZRemRangeByRank(ctx, key, 0, cnt-int64(maxDevices)-1)
		}
	}
	// 故意不设 TTL(见上方说明): 防 volatile-lru 淘汰致掉登录。ttl 参数保留兼容签名。
	// 每次登录都 Persist: 清除集合上**可能残留的旧 TTL**(早期代码曾对集合 EXPIRE, 老键
	// 至今仍带过期时间 → 到期被删 / 内存压力下被 volatile-lru 淘汰 → 用户集体掉线)。
	// Persist 是幂等的(无 TTL 时返回 0/无副作用), 每次登录顺手把这把"定时炸弹"拆掉。
	coordRdb.Persist(ctx, key)
	_ = ttl
	return nil
}

// IsUserTokenValid token 是否在该用户有效集合内。
// fail-closed: 成员/集合不存在(登出/踢人/被清)一律判无效; 仅 Redis 连接错误时暂放行(不回填)。
func IsUserTokenValid(ctx context.Context, userId int64, token string) bool {
	if coordRdb == nil {
		return true // 测试/未接 coord
	}
	err := coordRdb.ZScore(ctx, KeyToken(userId), token).Err()
	if err == nil {
		return true // 在有效集合内
	}
	if errors.Is(err, redis.Nil) {
		return false // 集合/成员不存在(登出/踢人/整库被清) → 拒, 撤销永远生效
	}
	// 其它错误 = Redis 连接不可达(网络/超时) → 临时放行靠 JWT 签名+过期兜底, 不回填、不持久化
	return true
}

// ClearUserTokens 清掉该用户全部设备 token。
func ClearUserTokens(ctx context.Context, userId int64) {
	if coordRdb != nil {
		coordRdb.Del(ctx, KeyToken(userId))
	}
}

// ClearOneToken 仅清当前设备 token。
func ClearOneToken(ctx context.Context, userId int64, token string) {
	if coordRdb != nil {
		coordRdb.ZRem(ctx, KeyToken(userId), token)
	}
}

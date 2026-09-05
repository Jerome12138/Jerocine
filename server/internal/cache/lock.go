package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"
)

// 分布式锁(协调态 db1): SET NX PX + 随机 token, 释放时 Lua 校验 token 防误删他人锁。
// 替代旧代码进程内 sync.Map 重入锁 / atomic, 多副本一致。token 用 crypto/rand 避免引入 uuid 依赖。

var unlockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
else
  return 0
end`)

// TryLock 尝试抢锁; 抢到返回 token(用于安全释放)与 true。
func TryLock(ctx context.Context, key string, ttl time.Duration) (token string, ok bool, err error) {
	if coordRdb == nil {
		return "", true, nil // 无协调端(测试) → 视为单机直接放行
	}
	token = randToken()
	ok, err = coordRdb.SetNX(ctx, key, token, ttl).Result()
	if err != nil || !ok {
		return "", ok, err
	}
	return token, true, nil
}

// Unlock 释放锁(仅当持有的 token 匹配)。
func Unlock(ctx context.Context, key, token string) {
	if coordRdb == nil || token == "" {
		return
	}
	_ = unlockScript.Run(ctx, coordRdb, []string{key}, token).Err()
}

func randToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// 令牌桶限流(协调态 db1)。now 由 Go 传入(不依赖 redis TIME, miniredis 兼容),
// 用 HGET/HSET 而非 HMSET(更广兼容)。替代旧代码三套散落的进程内令牌桶, 多副本一致。
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refillPerMs = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local cost = tonumber(ARGV[4])
local tokens = tonumber(redis.call('HGET', key, 'tokens'))
local ts = tonumber(redis.call('HGET', key, 'ts'))
if tokens == nil then
  tokens = capacity
  ts = now
end
local elapsed = now - ts
if elapsed < 0 then elapsed = 0 end
tokens = math.min(capacity, tokens + elapsed * refillPerMs)
local allowed = 0
if tokens >= cost then
  allowed = 1
  tokens = tokens - cost
end
redis.call('HSET', key, 'tokens', tokens, 'ts', now)
local ttlMs = math.ceil(capacity / refillPerMs) + 2000
redis.call('PEXPIRE', key, ttlMs)
return allowed`)

// Allow 令牌桶判定: scope+ip 维度, 桶容量 capacity, 每秒补充 refillPerSec 个令牌。
// 返回是否放行。协调端缺失(测试)时默认放行。
func Allow(ctx context.Context, scope, ip string, capacity int, refillPerSec float64) (bool, error) {
	if coordRdb == nil {
		return true, nil
	}
	if refillPerSec <= 0 {
		refillPerSec = 1
	}
	now := time.Now().UnixMilli()
	refillPerMs := refillPerSec / 1000.0
	res, err := tokenBucketScript.Run(ctx, coordRdb, []string{KeyRate(scope, ip)},
		capacity, refillPerMs, now, 1).Int()
	if err != nil {
		return true, err // fail-open: 限流故障不应打垮接口
	}
	return res == 1, nil
}

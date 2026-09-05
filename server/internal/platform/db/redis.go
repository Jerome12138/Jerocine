package db

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"server/internal/cache"
	"server/internal/config"
)

const (
	redisPoolSize     = 64
	redisMinIdleConns = 16
	redisRetryAttempts = 5
	redisRetryBase     = time.Second
)

// InitRedis 建立两个 redis 客户端(同实例不同 DB): cache(db0) + coord(db1),
// Ping 通过后注入 cache 包。返回两个 client 供调用方关闭。
func InitRedis(cfg config.RedisConfig) (cacheRdb, coordRdb *redis.Client, err error) {
	mk := func(dbNo int) *redis.Client {
		return redis.NewClient(&redis.Options{
			Addr:         cfg.Addr,
			Password:     cfg.Password,
			DB:           dbNo,
			PoolSize:     redisPoolSize,
			MinIdleConns: redisMinIdleConns,
			DialTimeout:  10 * time.Second,
		})
	}
	cacheRdb = mk(cfg.CacheDB)
	coordRdb = mk(cfg.CoordDB)

	if err = pingWithRetry(cacheRdb); err != nil {
		return nil, nil, err
	}
	if err = pingWithRetry(coordRdb); err != nil {
		return nil, nil, err
	}
	cache.Init(cacheRdb, coordRdb)
	return cacheRdb, coordRdb, nil
}

func pingWithRetry(rdb *redis.Client) error {
	var lastErr error
	for i := 0; i < redisRetryAttempts; i++ {
		if _, err := rdb.Ping(context.Background()).Result(); err == nil {
			return nil
		} else {
			lastErr = err
			wait := redisRetryBase << i
			log.Printf("WARN redis: Ping 失败 (%d/%d): %v, %s 后重试", i+1, redisRetryAttempts, err, wait)
			time.Sleep(wait)
		}
	}
	return lastErr
}

// Package cache 是 Redis 缓存层(db0)+ 协调态(db1)的统一封装。
//
// 核心定稿(对齐 doc/重构方案-全栈终态蓝图.md §5.2):
//   - 包级 Init(cache, coord) 注入两个 redis client;
//   - 泛型 GetOrLoad[T] 实现 cache-aside: 命中即返, miss 经进程内 singleflight 防击穿后回源 loader,
//     loader 返回 (value, found, error) 清晰区分"不存在"与"错误";
//   - 负缓存(negSentinel)防穿透; 写入 TTL 叠 ±10% 抖动防雪崩;
//   - 读 redis 出错时 fail-open(降级回源), 不让缓存故障打垮主流程。
package cache

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	cacheRdb *redis.Client // db0 缓存
	coordRdb *redis.Client // db1 协调态
	sf       = &flightGroup{m: make(map[string]*flightCall)}
)

const (
	negSentinel = "\x00__nil__"     // 负缓存哨兵值
	negTTL      = 60 * time.Second  // 负缓存 TTL(固定不抖)
)

// Init 注入缓存/协调 redis 客户端(由 platform/db 在启动期调用)。
func Init(cache, coord *redis.Client) {
	cacheRdb = cache
	coordRdb = coord
}

// CacheClient / CoordClient 供健康检查等少数场景直接 Ping。
func CacheClient() *redis.Client { return cacheRdb }
func CoordClient() *redis.Client { return coordRdb }

// GetOrLoad cache-aside 读取。found=false 表示资源不存在(已写负缓存)。
func GetOrLoad[T any](ctx context.Context, key string, ttl time.Duration, loader func(context.Context) (T, bool, error)) (T, bool, error) {
	var zero T
	// 1. 先查缓存
	if cacheRdb != nil {
		s, err := cacheRdb.Get(ctx, key).Result()
		if err == nil {
			if s == negSentinel {
				return zero, false, nil
			}
			var v T
			if json.Unmarshal([]byte(s), &v) == nil {
				return v, true, nil
			}
			// 反序列化失败(脏数据) → 落到回源重建
		} else if err != redis.Nil {
			log.Printf("cache: get %s err: %v (fail-open 回源)", key, err)
		}
	}
	// 2. miss → singleflight 回源, 同 key 并发只跑一次
	res, err := sf.Do(key, func() (flightResult, error) {
		// 双检: 可能已被其它 goroutine 填好
		if cacheRdb != nil {
			if s, e := cacheRdb.Get(ctx, key).Result(); e == nil && json.Valid([]byte(s)) {
				return flightResult{data: s, found: s != negSentinel}, nil
			} else if e == nil && s == negSentinel {
				return flightResult{found: false}, nil
			}
		}
		v, found, e := loader(ctx)
		if e != nil {
			return flightResult{}, e
		}
		if !found {
			if cacheRdb != nil {
				cacheRdb.Set(ctx, key, negSentinel, negTTL)
			}
			return flightResult{found: false}, nil
		}
		b, e := json.Marshal(v)
		if e != nil {
			return flightResult{}, e
		}
		if cacheRdb != nil {
			cacheRdb.Set(ctx, key, b, JitterTTL(ttl))
		}
		return flightResult{data: string(b), found: true}, nil
	})
	if err != nil {
		return zero, false, err
	}
	if !res.found {
		return zero, false, nil
	}
	var v T
	if e := json.Unmarshal([]byte(res.data), &v); e != nil {
		return zero, false, e
	}
	return v, true, nil
}

// Set 主动写缓存(TTL 叠抖动)。供采集后预热等场景。
func Set[T any](ctx context.Context, key string, ttl time.Duration, v T) error {
	if cacheRdb == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return cacheRdb.Set(ctx, key, b, JitterTTL(ttl)).Err()
}

// Del 删除指定缓存键。
func Del(ctx context.Context, keys ...string) {
	if cacheRdb == nil || len(keys) == 0 {
		return
	}
	if err := cacheRdb.Del(ctx, keys...).Err(); err != nil {
		log.Printf("cache: del err: %v", err)
	}
}

// DelByPattern 用 SCAN(非 KEYS, 不阻塞)批量删除匹配 pattern 的缓存键, 分批 Unlink。
func DelByPattern(ctx context.Context, pattern string) {
	if cacheRdb == nil {
		return
	}
	var cursor uint64
	for {
		keys, next, err := cacheRdb.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			log.Printf("cache: scan %s err: %v", pattern, err)
			return
		}
		if len(keys) > 0 {
			if err := cacheRdb.Unlink(ctx, keys...).Err(); err != nil {
				cacheRdb.Del(ctx, keys...)
			}
		}
		if next == 0 {
			return
		}
		cursor = next
	}
}

// JitterTTL 给 TTL 叠加 ±10% 随机抖动, 防同一时刻批量过期引发雪崩。
func JitterTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return ttl
	}
	delta := int64(ttl) / 10
	if delta <= 0 {
		return ttl
	}
	return ttl + time.Duration(rand.Int63n(2*delta+1)-delta)
}

// ---- 进程内 singleflight(避免引入 golang.org/x/sync 依赖) ----

type flightResult struct {
	data  string
	found bool
}

type flightCall struct {
	wg  sync.WaitGroup
	res flightResult
	err error
}

type flightGroup struct {
	mu sync.Mutex
	m  map[string]*flightCall
}

func (g *flightGroup) Do(key string, fn func() (flightResult, error)) (flightResult, error) {
	g.mu.Lock()
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.res, c.err
	}
	c := &flightCall{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.res, c.err = fn()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()
	c.wg.Done()
	return c.res, c.err
}

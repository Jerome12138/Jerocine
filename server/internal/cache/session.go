package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// 登录会话元数据(协调态 DB1): 每个 token 一条(用户 / IP / UA / 登录时间), 另加一条按最近活跃
// 排序的索引, 供后台「在线会话」列表与踢人。
//
//   - **只做展示与撤销**, 鉴权仍以 KeyToken 集合为准(IsUserTokenValid); 元数据丢了只是列表少一行,
//     绝不能让用户掉登录, 所以元数据一切失败都静默忽略。
//   - 元数据 TTL = token 有效期, 随 token 一起过期; 索引成员在列表时按"元数据不存在"惰性清理。
//   - 令牌原文存在 HASH 里只为踢人时 ZRem 用, 任何接口都不返回它。
//   - 活跃时间写入节流(同 token 60s 一次, 进程内计数), 避免每个鉴权请求都写 Redis。

const (
	sessionTouchInterval = 60 * time.Second
	sessionPruneInterval = 512 // 每 N 次 touch 清一次进程内节流表
	sessionTouchTTL      = 10 * time.Minute
)

// SessionMeta 一条登录会话。
type SessionMeta struct {
	ID       string `json:"id"`
	UserID   int64  `json:"userId"`
	UserName string `json:"userName"`
	IP       string `json:"ip"`
	UA       string `json:"ua"`
	LoginAt  int64  `json:"loginAt"`  // 秒
	LastSeen int64  `json:"lastSeen"` // 秒
	Token    string `json:"-"`        // 内部撤销用, 不出网
}

// touchAt 进程内节流表: sessionID → 上次写 Redis 的 unix 秒。
var (
	touchAt  sync.Map
	touchSeq atomic.Uint64
)

// SessionID token → 会话 id(sha256 前 16 字节 hex): 键名不暴露凭据。
func SessionID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:16])
}

// SaveSession 登录成功后写会话元数据(同 token 幂等覆盖)。
func SaveSession(ctx context.Context, token string, uid int64, userName, ip, ua string, ttl time.Duration) {
	if coordRdb == nil || token == "" {
		return
	}
	if ttl <= 0 {
		ttl = 168 * time.Hour
	}
	id := SessionID(token)
	now := time.Now().Unix()
	pipe := coordRdb.Pipeline()
	pipe.HSet(ctx, KeySession(id), map[string]any{
		"uid": uid, "uname": userName, "ip": ip, "ua": ua,
		"loginAt": now, "token": token,
	})
	pipe.Expire(ctx, KeySession(id), ttl)
	pipe.ZAdd(ctx, KeySessionIdx, redis.Z{Score: float64(now), Member: id})
	_, _ = pipe.Exec(ctx)
}

// TouchSession 更新"最近活跃"(节流 + 只更新已存在的索引成员, 不复活过期会话)。
func TouchSession(ctx context.Context, token string) {
	if coordRdb == nil || token == "" {
		return
	}
	id := SessionID(token)
	now := time.Now().Unix()
	if v, ok := touchAt.Load(id); ok {
		if ts, _ := v.(int64); now-ts < int64(sessionTouchInterval/time.Second) {
			return
		}
	}
	touchAt.Store(id, now)
	// XX: 索引里没有这条(元数据已过期/被踢) 就不写, 避免幽灵行
	coordRdb.ZAddArgs(ctx, KeySessionIdx, redis.ZAddArgs{
		XX: true, GT: true,
		Members: []redis.Z{{Score: float64(now), Member: id}},
	})
	if touchSeq.Add(1)%sessionPruneInterval == 0 {
		pruneTouchAt(now)
	}
}

// pruneTouchAt 清理进程内节流表中久未出现的 token(防长期运行时无限增长)。
func pruneTouchAt(now int64) {
	cutoff := now - int64(sessionTouchTTL/time.Second)
	touchAt.Range(func(k, v any) bool {
		if ts, ok := v.(int64); ok && ts < cutoff {
			touchAt.Delete(k)
		}
		return true
	})
}

// DeleteSession 删除会话元数据与索引(登出 / 踢人 / 改密全端下线时)。
func DeleteSession(ctx context.Context, token string) {
	if coordRdb == nil || token == "" {
		return
	}
	id := SessionID(token)
	coordRdb.Del(ctx, KeySession(id))
	coordRdb.ZRem(ctx, KeySessionIdx, id)
	touchAt.Delete(id)
}

// SessionByID 按会话 id 取元数据(含 token 原文, 仅内部撤销用)。
func SessionByID(ctx context.Context, id string) (SessionMeta, bool) {
	if coordRdb == nil || id == "" {
		return SessionMeta{}, false
	}
	m, err := coordRdb.HGetAll(ctx, KeySession(id)).Result()
	if err != nil || len(m) == 0 {
		return SessionMeta{}, false
	}
	return metaFromHash(id, m, 0), true
}

// ListSessions 最近活跃的会话(倒序, limit 上限 500); 元数据已过期的索引成员顺手清掉。
func ListSessions(ctx context.Context, limit int) []SessionMeta {
	if coordRdb == nil {
		return nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := coordRdb.ZRevRangeWithScores(ctx, KeySessionIdx, 0, int64(limit)-1).Result()
	if err != nil || len(rows) == 0 {
		return nil
	}
	pipe := coordRdb.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(rows))
	for i, r := range rows {
		id, _ := r.Member.(string)
		cmds[i] = pipe.HGetAll(ctx, KeySession(id))
	}
	_, _ = pipe.Exec(ctx)

	out := make([]SessionMeta, 0, len(rows))
	stale := make([]any, 0, 4)
	for i, r := range rows {
		id, _ := r.Member.(string)
		m := cmds[i].Val()
		if len(m) == 0 {
			stale = append(stale, id)
			continue
		}
		out = append(out, metaFromHash(id, m, int64(r.Score)))
	}
	if len(stale) > 0 {
		coordRdb.ZRem(ctx, KeySessionIdx, stale...)
	}
	return out
}

// metaFromHash HASH → SessionMeta。lastSeen 取索引 score(缺失时回退登录时间)。
func metaFromHash(id string, m map[string]string, lastSeen int64) SessionMeta {
	uid, _ := strconv.ParseInt(m["uid"], 10, 64)
	loginAt, _ := strconv.ParseInt(m["loginAt"], 10, 64)
	if lastSeen <= 0 {
		lastSeen = loginAt
	}
	return SessionMeta{
		ID: id, UserID: uid, UserName: m["uname"], IP: m["ip"], UA: m["ua"],
		LoginAt: loginAt, LastSeen: lastSeen, Token: m["token"],
	}
}

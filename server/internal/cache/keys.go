package cache

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"time"
)

// Redis key 命名: v1:{域}:{标识}。详情/卡片去 cid(mid 全局唯一)。
// 缓存键(db0)与协调态键(db1)分库, 见 platform/db。

// ---- db0 缓存键 ----
func KeyMovieDetail(mid int64) string { return fmt.Sprintf("v1:movie:detail:%d", mid) }
func KeyMovieBasic(mid int64) string  { return fmt.Sprintf("v1:movie:basic:%d", mid) }
func KeyMovieCard(mid int64) string   { return fmt.Sprintf("v1:movie:card:%d", mid) }
func KeyClassify(pid int64) string    { return fmt.Sprintf("v1:classify:%d", pid) }
func KeyRelate(mid int64) string      { return fmt.Sprintf("v1:relate:%d", mid) }
func KeyTagOpts(pid int64) string     { return fmt.Sprintf("v1:tagopts:%d", pid) }

const (
	KeyCatTree    = "v1:cat:tree"
	KeyHomeAgg    = "v1:home:agg"
	KeyCfgSite    = "v1:cfg:site"
	KeyCfgVersion = "v1:cfg:version"
	KeyCfgSources = "v1:cfg:sources"
	KeyBanners    = "v1:cfg:banners"
)

// 弹幕相关缓存键（DB0；正缓存 TTL 7d / 负缓存短 TTL 由 service 内写，见 danmu_service.go）。
func KeyDanmuSource(mid int64) string { return fmt.Sprintf("v1:danmu:src:%d", mid) }

// KeyDanmuBiliCmt B站整集弹幕内容缓存键（D2 起：Go 不再缓存分集映射，只缓存最终整集弹幕）。
// seasonID 来自用户输入(路由参数)，经 EpKeyOf 哈希定长化 —— 限制不合法输入制造无界键
// (公开限流接口可批量造 key 撑 Redis 内存；同 episodeID 的 8.5/8.6 约定)。
func KeyDanmuBiliCmt(seasonID string, episode int) string {
	return fmt.Sprintf("v1:danmu:bili:cmt:%s:%d", EpKeyOf(seasonID), episode)
}

// KeyDanmuSeg 分段弹幕内容缓存键（腾讯/爱奇艺等分段点播，P1c 起用；先登记）。
// epKey = EpKeyOf(episodeId)：episodeId 是 provider 复合串（含 & 且长度不定），
// 不得直接拼 Redis 键（8.5/8.6 约定）。
func KeyDanmuSeg(provider, epKey, segmentID string) string {
	return fmt.Sprintf("v1:danmu:seg:%s:%s:%s", provider, epKey, segmentID)
}

// EpKeyOf 由 episodeId 复合串派生缓存键片段：sha1 前 8 字节 hex（16 字符）。
func EpKeyOf(episodeID string) string {
	h := sha1.Sum([]byte(episodeID))
	return hex.EncodeToString(h[:8])
}

// KeyLiveHealth 单频道探源健康缓存（DB0）。value = JSON{alive,cors,at}，TTL 由 service 定。
// srcKey/urlKey 均为 EpKeyOf 哈希（url 是用户可控长串，同 8.5/8.6 约定不得直拼键）。
func KeyLiveHealth(srcKey, urlKey string) string {
	return fmt.Sprintf("v1:live:health:%s:%s", srcKey, urlKey)
}

// ---- 聚合弹幕（dandanplay v2 兼容，按标题匹配；DB0，正 7d/负 2h 由 service 定） ----

// KeyDanmuDdAnime 关键词搜索结果缓存键（kwKey = EpKeyOf(归一化关键词)）。
func KeyDanmuDdAnime(kwKey string) string { return fmt.Sprintf("v1:danmu:dd:anime:%s", kwKey) }

// KeyDanmuDdEps 作品分集列表缓存键。
func KeyDanmuDdEps(animeID int64) string { return fmt.Sprintf("v1:danmu:dd:eps:%d", animeID) }

// KeyDanmuDdCmt 整集弹幕内容缓存键（episodeId 为聚合服务数值 id）。
func KeyDanmuDdCmt(episodeID int64) string { return fmt.Sprintf("v1:danmu:dd:cmt:%d", episodeID) }

// ---- 直播源频道解析（DB0） ----

// KeyLiveChannels 直播源频道解析缓存键（urlKey = EpKeyOf(源 url)，源地址变更即自然换键）。
func KeyLiveChannels(sourceID int64, urlKey string) string {
	return fmt.Sprintf("v1:live:ch:%d:%s", sourceID, urlKey)
}

// 失效用的 pattern(SCAN, 不用 KEYS)。
const (
	PatMovie       = "v1:movie:*"
	PatClassify    = "v1:classify:*"
	PatRelate      = "v1:relate:*"
	PatTagOpts     = "v1:tagopts:*"
	PatDanmuSource = "v1:danmu:src:*" // 弹幕来源候选缓存(后台改弹幕配置后全清, 见 ManageService.SaveDanmuConfig)
)

// ---- db1 协调态键 ----
func KeyLockSF(key string) string        { return "v1:lock:sf:" + key }
func KeyLockCron(taskId string) string   { return "v1:lock:cron:" + taskId }
func KeyRate(scope, ip string) string    { return fmt.Sprintf("v1:rl:%s:%s", scope, ip) }
func KeyJob(sourceId string) string      { return "v1:job:" + sourceId }
func KeyToken(userId int64) string       { return fmt.Sprintf("v1:token:%d", userId) }
func KeySpiderTemp(siteId string) string { return "v1:spider:temp:" + siteId }

// ---- 注册 / 邮件验证码(协调态, 见 cache/regcode.go) ----
// kind 区分邮件类别, 避免"申请验证码"占掉同邮箱 1 分钟窗口把"激活邮件"挡在门外(审批常紧随申请之后)。
func KeyRegCode(purpose, email string) string { return "v1:regcode:" + purpose + ":" + email }
func KeyRegTry(email string) string           { return "v1:regtry:" + email }                      // 连续校验失败计数(锁 10min)
func KeyRegSendMin(kind, email string) string { return "v1:regsend:min:" + kind + ":" + email }    // 1 分钟 1 封窗口(NX)
func KeyRegSendDay(kind, email string) string { return "v1:regsend:day:" + kind + ":" + email }    // 邮箱日配额计数
func KeyMailQuota(day string) string          { return "v1:mailquota:" + day }                     // 全局日配额计数(YYYYMMDD)
func KeyActivate(token string) string         { return "v1:activate:" + token }                    // 激活 token → apply_id (P2)
func KeyActivateID(applyID int64) string      { return fmt.Sprintf("v1:activate:id:%d", applyID) } // 反向: apply_id → 当前 token(重发作废旧)
func KeyReset(token string) string            { return "v1:reset:" + token }                       // 重置密码 token → user_id (P3)
func KeyResetID(userID int64) string          { return fmt.Sprintf("v1:reset:id:%d", userID) }     // 反向: user_id → 当前 token(重新申请作废旧)

// ---- 登录会话元数据(协调态, 见 cache/session.go) ----
// KeySession 会话 id(sha256 前 16 字节 hex) → HASH{uid,uname,ip,ua,loginAt,token};
// 键名不含原始凭据。KeySessionIdx 按最近活跃排序, 供后台会话列表。
func KeySession(id string) string { return "v1:sess:" + id }

const KeySessionIdx = "v1:sess:idx"

const KeyLockSyncPic = "v1:lock:sync:pic"

// ---- 榜单热度刷新(协调态, 见 service/hot_service.go) ----

// KeyLockHotRun 刷新互斥锁: 每日调度与后台手动触发不能同时打豆瓣(限流红线)。
const KeyLockHotRun = "v1:lock:hot:run"

// KeyLockHotDaily 每日一次的成功标记(按本地日期): 抢到即说明今天还没成功跑过。
// TTL 25h(>24h)覆盖跨天, 重启/多副本也不会当天重复拉榜单。
func KeyLockHotDaily(date string) string { return "v1:lock:hot:daily:" + date }

// LiveHealthGetAll 批量读频道探源健康（key 顺序与返回顺序一致; miss 位返回 nil）。
func LiveHealthGetAll(ctx context.Context, srcKey string, urlKeys []string) ([]string, error) {
	if cacheRdb == nil || len(urlKeys) == 0 {
		return make([]string, len(urlKeys)), nil
	}
	keys := make([]string, len(urlKeys))
	for i, u := range urlKeys {
		keys[i] = KeyLiveHealth(srcKey, u)
	}
	vals, err := cacheRdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(vals))
	for i, v := range vals {
		if str, ok := v.(string); ok {
			out[i] = str
		}
	}
	return out, nil
}

// LiveHealthSet 写单频道探源健康（JSON 由调用方序列化）。
func LiveHealthSet(ctx context.Context, srcKey, urlKey, value string, ttl time.Duration) error {
	if cacheRdb == nil {
		return nil
	}
	return cacheRdb.Set(ctx, KeyLiveHealth(srcKey, urlKey), value, ttl).Err()
}

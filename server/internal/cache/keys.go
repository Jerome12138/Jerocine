package cache

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
)

// Redis key 命名: v1:{域}:{标识}。详情/卡片去 cid(mid 全局唯一)。
// 缓存键(db0)与协调态键(db1)分库, 见 platform/db。

// ---- db0 缓存键 ----
func KeyMovieDetail(mid int64) string     { return fmt.Sprintf("v1:movie:detail:%d", mid) }
func KeyMovieBasic(mid int64) string      { return fmt.Sprintf("v1:movie:basic:%d", mid) }
func KeyMovieCard(mid int64) string       { return fmt.Sprintf("v1:movie:card:%d", mid) }
func KeyClassify(pid int64) string        { return fmt.Sprintf("v1:classify:%d", pid) }
func KeyRelate(mid int64) string          { return fmt.Sprintf("v1:relate:%d", mid) }
func KeyTagOpts(pid int64) string         { return fmt.Sprintf("v1:tagopts:%d", pid) }
func KeyPlayMulti(matchKey string) string { return fmt.Sprintf("v1:play:multi:%s", matchKey) }

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
func KeyDanmuBiliCmt(seasonID string, episode int) string {
	return fmt.Sprintf("v1:danmu:bili:cmt:%s:%d", seasonID, episode)
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

// 失效用的 pattern(SCAN, 不用 KEYS)。
const (
	PatMovie    = "v1:movie:*"
	PatClassify = "v1:classify:*"
	PatRelate   = "v1:relate:*"
	PatTagOpts  = "v1:tagopts:*"
	PatPlay     = "v1:play:*"
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

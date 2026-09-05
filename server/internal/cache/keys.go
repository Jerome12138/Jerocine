package cache

import "fmt"

// Redis key 命名: v1:{域}:{标识}。详情/卡片去 cid(mid 全局唯一)。
// 缓存键(db0)与协调态键(db1)分库, 见 platform/db。

// ---- db0 缓存键 ----
func KeyMovieDetail(mid int64) string { return fmt.Sprintf("v1:movie:detail:%d", mid) }
func KeyMovieBasic(mid int64) string  { return fmt.Sprintf("v1:movie:basic:%d", mid) }
func KeyMovieCard(mid int64) string   { return fmt.Sprintf("v1:movie:card:%d", mid) }
func KeyClassify(pid int64) string    { return fmt.Sprintf("v1:classify:%d", pid) }
func KeyRelate(mid int64) string      { return fmt.Sprintf("v1:relate:%d", mid) }
func KeyTagOpts(pid int64) string     { return fmt.Sprintf("v1:tagopts:%d", pid) }
func KeyPlayMulti(matchKey string) string { return fmt.Sprintf("v1:play:multi:%s", matchKey) }

const (
	KeyCatTree    = "v1:cat:tree"
	KeyHomeAgg    = "v1:home:agg"
	KeyCfgSite    = "v1:cfg:site"
	KeyCfgVersion = "v1:cfg:version"
	KeyCfgSources = "v1:cfg:sources"
)

// 失效用的 pattern(SCAN, 不用 KEYS)。
const (
	PatMovie    = "v1:movie:*"
	PatClassify = "v1:classify:*"
	PatRelate   = "v1:relate:*"
	PatTagOpts  = "v1:tagopts:*"
	PatPlay     = "v1:play:*"
)

// ---- db1 协调态键 ----
func KeyLockSF(key string) string       { return "v1:lock:sf:" + key }
func KeyLockCron(taskId string) string  { return "v1:lock:cron:" + taskId }
func KeyRate(scope, ip string) string   { return fmt.Sprintf("v1:rl:%s:%s", scope, ip) }
func KeyJob(sourceId string) string     { return "v1:job:" + sourceId }
func KeyToken(userId int64) string      { return fmt.Sprintf("v1:token:%d", userId) }
func KeySpiderTemp(siteId string) string { return "v1:spider:temp:" + siteId }

const KeyLockSyncPic = "v1:lock:sync:pic"

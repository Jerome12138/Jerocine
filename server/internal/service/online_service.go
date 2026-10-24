package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"server/internal/botdetect"
)

// OnlineService Redis 在线统计: 匿名会话心跳, 多实例共享, 查询时惰性清理。
//
// 口径(活跃 = 近 30 分钟内有心跳, 2026-10-21 由 90s 改为 30min):
//   - 客户端 30s 一次心跳; 30 分钟内有过心跳即算"活跃/在线", 超过即从实时列表消失。
//     页面切换/切后台/短暂断网不会再让一个人瞬间"掉线又上线", 数字更接近"正在访问的人"。
//   - PV = 页面访问次数(不去重): 客户端在"首次载入/路由切换"时随心跳带 pv=1 直接计一次;
//     旧客户端兜底按"首次见到该会话 / 跨天 / 页面路径变化"计。同一页面停留期间的
//     30s 常规心跳不计 PV(那只是续活, 不是访问)。实时 PV = 近 30 分钟窗口内的页面访问次数,
//     同一会话反复进同一页面也照计(不去重)
//   - UV = 去重后活跃人数: 登录用户按 uid 去重, 游客按 IP 去重
//     (同一用户开多标签/多设备只算 1; 同 IP 多游客合并为 1, 口径偏保守)
//   - 观看中 = 活跃会话中 watching=true 的会话数
//   - 访问次数(Visits) = 同一会话距上次心跳 ≥30min 记一次新访问; 跨天从 1 重新计;
//     后台"访问次数"列即该会话在当前查询窗口内的累计
//
// 今日累计(自然日滚动, 键 TTL 48h 自动过期):
//   - 今日 UV = 当日出现过的去重人数(登录 u:{uid} / 游客 i:{ip}), SET
//   - 今日 PV = 当日页面访问次数(同实时 PV 口径, 不去重), STRING
//   - 今日峰值 = 当日活跃会话数峰值, STRING (Overview 时惰性更新)
//
// 机器人过滤: 判定见 platform 层 botdetect(UA / webdriver / 缺浏览器头 / 机房 IP);
// 命中的心跳不进实时与今日统计, 只按原因计数(后台"机器人请求 PV"及其分布) ——
// 防止爬虫污染在线数据。历史遗留的机器人明细在读取时按 UA 规则兜底过滤。
//
// 存储:
//   - ZSET jc:online:act  member=sid#ip(或纯 sid) score=lastSeen(心跳续期; 查询时按窗口 ZREMRANGEBYSCORE)
//   - STRING jc:online:info:{sid#ip} 会话明细 JSON, TTL 略大于窗口(防 ZSET 清理后孤儿残留)
//   - ZSET jc:online:pvlog 页面访问流水 member=sid#ip@毫秒 score=发生时刻
//     (实时 PV = 窗口内 ZCount; 与 act 同一 cutoff 惰性清理)
type OnlineService struct {
	rdb    *redis.Client
	window time.Duration
	// gap 访问计数间隔: 同一会话距上次心跳 >= gap 记一次新访问(默认 visitGap; 测试可缩短)。
	gap time.Duration
}

// OnlineSession 一个在线会话的明细(handler 层从请求解析 IP/UID/UA)。
// 隐私口径: Paths 只记录页面路径(pathname, 不含 query 参数/影片标识),
// 不收集"正在看哪部影片"这类内容偏好; Username/IPRegion 由 Overview 时只读增强填充。
type OnlineSession struct {
	Sid      string `json:"sid"`
	IP       string `json:"ip"`
	UID      int64  `json:"uid,omitempty"`
	Watching bool   `json:"watching"`
	UA       string `json:"ua,omitempty"`
	// Path 最近一次心跳的页面路径(兼容旧前端/日志); 展示用 Paths。
	Path string `json:"path,omitempty"`
	// Paths 本次统计周期内访问过、去重后的页面路径(按首次出现顺序, 上限 pathsMax)。
	Paths []string `json:"paths,omitempty"`
	// PV 客户端标记的页面访问(首次载入/路由切换的心跳带 true): 服务端直接计一次 PV。
	// 常规 30s 续活心跳为 false, 由服务端按路径变化兜底判定。
	PV       bool   `json:"pv,omitempty"`
	Username string `json:"username,omitempty"`
	IPRegion string `json:"ipRegion,omitempty"`
	// Visits 访问次数: 同一会话距上次心跳 ≥30min 记一次, 跨天重置; 明细页展示该列。
	Visits    int   `json:"visits"`
	FirstSeen int64 `json:"firstSeen"`
	LastSeen  int64 `json:"lastSeen"`
	// Online 仅每日/近7天明细(dailySessions)返回时有效: 该会话当前是否活跃(30min 窗口内有过心跳)。
	Online bool `json:"online,omitempty"`
}

// OnlineOverview 在线概览: 实时 UV/PV/观看中 + 今日累计 + 会话明细(后台表单)。
type OnlineOverview struct {
	// UV 实时在线去重人数(登录按 uid, 游客按 IP)。
	UV int `json:"uv"`
	// PV 实时页面访问次数(近 30 分钟窗口内, 不去重)。
	PV       int             `json:"pv"`
	Watching int             `json:"watching"`
	Bots     int             `json:"bots"` // 实时机器人在线源数(30min 窗口, 单独统计)
	Today    DailyStats      `json:"today"`
	Sessions []OnlineSession `json:"sessions"`
}

// DailyStats 今日累计(自然日滚动)。
type DailyStats struct {
	UV    int `json:"uv"`    // 今日去重人数
	PV    int `json:"pv"`    // 今日页面访问次数(不去重, 同实时 PV 口径)
	Peak  int `json:"peak"`  // 今日峰值活跃(30min 窗口内的会话数最高值)
	BotPV int `json:"botPV"` // 今日机器人请求次数(被过滤掉的爬虫心跳)
	// BotReasons 机器人请求按命中原因拆分的次数(键为 botdetect.Reason: ua/webdriver/header/datacenter),
	// 用于后台核对"到底是哪条规则在起作用"; 合计等于 BotPV。
	BotReasons map[string]int `json:"botReasons,omitempty"`
}

const (
	onlineActKey   = "jc:online:act"
	onlinePVLogKey = "jc:online:pvlog" // 页面访问流水(ZSET), 实时 PV = 窗口内条数
	onlineBotsKey  = "jc:online:bots"  // 实时机器人来源(窗口内), ZSET, 与正常会话隔离
	// onlineWindow 活跃窗口: 30 分钟内有过心跳即算在线(2026-10-21 由 90s 放宽)。
	onlineWindow = 30 * time.Minute
	// onlineInfoTTL 明细 TTL 必须大于窗口, 否则窗口内还能查到 ZSET 成员却读不到明细。
	onlineInfoTTL = 35 * time.Minute

	dailyTTL = 48 * time.Hour   // 今日统计键 TTL: 覆盖当日全天 + 次日缓冲, 自然滚动
	visitGap = 30 * time.Minute // 会话"访问次数"间隔: 距上次心跳 ≥gap 记一次新访问(明细列 Visits 用)
	// 按日访问日志(明细回溯)TTL: 须支撑后台"近7天"查询 → 7 天 + 1 天缓冲;
	// 心跳每次续期, 实际留存 = 最后一次心跳 + 8 天。
	dailyLogTTL = 8 * 24 * time.Hour
	// pathsMax 单个会话保留的页面路径条数(去重后), 超出丢最早的一条 —— 后台列最多显示这么多。
	pathsMax = 10
)

// 会话唯一键: 设备 sid + 出口 IP。同浏览器换网络(IP 变)视为另一会话行,
// 便于后台看出"同一用户开了几个端/几个网络出口"。
// IP 为空(异常心跳)时退化为纯 sid, 保持旧行为。
func onlineSessionKey(sid, ip string) string {
	if ip == "" {
		return sid
	}
	return sid + "#" + ip
}

func onlineInfoKey(k string) string { return "jc:online:info:" + k }

// pvSeq pvlog member 唯一序号: 同一会话同一毫秒内多次页面访问(快速连续跳转)不互相顶掉。
// 必须在包级 —— 声明在函数体内每次心跳都会从 0 重计, 同毫秒两条 member 仍会互相顶掉。
var pvSeq atomic.Uint64

// 今日统计键。
func dailyKeys(date string) (uv, pv, peak string) {
	return "jc:online:daily:" + date + ":uv",
		"jc:online:daily:" + date + ":pv",
		"jc:online:daily:" + date + ":peak"
}

// 当日访问日志键(明细回溯): 心跳时同步写一份按日 ZSET + 按日明细, TTL 与今日统计一致。
func dailyLogKey(date string) string     { return "jc:online:daily:" + date + ":log" }
func dailyInfoKey(date, k string) string { return "jc:online:daily:" + date + ":info:" + k }

// dailyBotKey 当日机器人请求计数(HASH: 原因 → 次数), 合计即"机器人请求 PV"。
func dailyBotKey(date string) string { return "jc:online:daily:" + date + ":bot" }

// 机器人判定已收敛到 platform 层 botdetect(UA / webdriver / 缺浏览器头 / 机房 IP)。
// 注意: 埋点(telemetry)侧仍保留一套更严格的独立规则(空 UA 也算噪声), 两者不共用列表。

func NewOnlineService(rdb *redis.Client) *OnlineService {
	return &OnlineService{rdb: rdb, window: onlineWindow, gap: visitGap}
}

// Heartbeat 记录/续期一个会话。首次出现记 firstSeen(进入时间), 已存在沿用。
// ZSET score 用毫秒精度(窗口判断/排序), 明细里 FirstSeen/LastSeen 用秒(展示可读)。
//
// reason 由调用方(handler)用 botdetect.Classify 判定(能拿到请求头/IP/ISP);
// 命中即只计机器人(按原因分类), 不入实时与今日统计。
func (s *OnlineService) Heartbeat(ctx context.Context, sess OnlineSession, reason botdetect.Reason) {
	if sess.Sid == "" {
		return
	}
	if reason != "" {
		s.recordBotHit(ctx, sess, reason)
		return
	}
	now := time.Now()
	nowMs := now.UnixMilli()
	date := now.Format("20060102")
	key := onlineSessionKey(sess.Sid, sess.IP)

	// 续接旧会话: 先读实时明细, 过期(空闲 > TTL)则回落到当日明细 ——
	// 空闲超过 35 分钟后回来的人仍是"同一会话同一天", 访问次数与首次进入时间不该被重置。
	// 跨天则一律重新计(访问次数/首次进入/页面列表都是"当日"口径)。
	prev := s.prevSession(ctx, date, key)
	sess.FirstSeen = now.Unix()
	sess.LastSeen = now.Unix()
	sess.Visits = 1
	sess.Paths = mergePaths(nil, sess.Path)
	if prev != nil && time.Unix(prev.LastSeen, 0).Format("20060102") == date {
		sess.FirstSeen = prev.FirstSeen
		sess.Visits = prev.Visits
		if sess.Visits < 1 {
			sess.Visits = 1
		}
		if sess.LastSeen-prev.LastSeen >= int64(s.gap/time.Second) {
			sess.Visits++ // 距上次 ≥30min: 同一个会话的又一次访问
		}
		sess.Paths = mergePaths(prev.Paths, sess.Path)
	}
	buf, _ := json.Marshal(sess)
	// 页面访问(PV)判定: 客户端在"首次载入/路由切换"时带 pv=1, 直接计一次;
	// 旧客户端(无 pv 标记)兜底: 首次见到该会话 / 跨天 / 页面路径发生变化。
	// 同一页面停留期间的常规心跳(路径不变且无标记)不计 —— 那是续活, 不是访问。
	pvHit := sess.PV
	if !pvHit && sess.Path != "" {
		switch {
		case prev == nil:
			pvHit = true
		case time.Unix(prev.LastSeen, 0).Format("20060102") != date:
			pvHit = true // 跨天: 今天对同一页面的打开也算一次新访问
		default:
			pvHit = sess.Path != prev.Path
		}
	}
	pipe := s.rdb.Pipeline()
	pipe.ZAdd(ctx, onlineActKey, redis.Z{Score: float64(nowMs), Member: key})
	pipe.Set(ctx, onlineInfoKey(key), buf, onlineInfoTTL)
	// ---- 今日累计 ----
	uvKey, pvKey, _ := dailyKeys(date)
	uidKey := "i:" + sess.IP
	if sess.UID > 0 {
		uidKey = fmt.Sprintf("u:%d", sess.UID)
	}
	if sess.IP != "" { // 游客无 IP 时不重复计 UV(异常心跳)
		pipe.SAdd(ctx, uvKey, uidKey)
		pipe.Expire(ctx, uvKey, dailyTTL)
	}
	// 页面访问: 流水 ZSET(实时 PV 按窗口数) + 今日累计计数
	if pvHit {
		pipe.ZAdd(ctx, onlinePVLogKey, redis.Z{
			Score: float64(nowMs),
			Member: key + "@" + strconv.FormatInt(nowMs, 10) + "-" +
				strconv.FormatUint(pvSeq.Add(1), 10), // member 唯一: 同毫秒多条不互相顶掉
		})
		pipe.Incr(ctx, pvKey)
		pipe.Expire(ctx, pvKey, dailyTTL)
	}
	// ---- 当日访问日志(明细回溯): 按日 ZSET(最近活跃排序) + 按日明细, TTL 8 天(支撑近7天查询) ----
	pipe.ZAdd(ctx, dailyLogKey(date), redis.Z{Score: float64(nowMs), Member: key})
	pipe.Expire(ctx, dailyLogKey(date), dailyLogTTL)
	pipe.Set(ctx, dailyInfoKey(date, key), buf, dailyLogTTL)
	_, _ = pipe.Exec(ctx)
}

// prevSession 读同一会话的上一份明细: 实时明细优先, 缺失时回落当日明细(防空闲超 TTL 后访问次数重置)。
func (s *OnlineService) prevSession(ctx context.Context, date, key string) *OnlineSession {
	var prev OnlineSession
	if v, err := s.rdb.Get(ctx, onlineInfoKey(key)).Result(); err == nil {
		if json.Unmarshal([]byte(v), &prev) == nil && prev.LastSeen > 0 {
			return &prev
		}
	}
	if v, err := s.rdb.Get(ctx, dailyInfoKey(date, key)).Result(); err == nil {
		if json.Unmarshal([]byte(v), &prev) == nil && prev.LastSeen > 0 {
			return &prev
		}
	}
	return nil
}

// mergePaths 合并页面路径: 去重、去掉空值、上限 pathsMax(超出丢最早的一条)。
// 用首次出现顺序(读起来像浏览轨迹), 不按最近活跃重排。
func mergePaths(prev []string, cur string) []string {
	out := make([]string, 0, len(prev)+1)
	seen := make(map[string]bool, len(prev)+1)
	for _, p := range append(append([]string{}, prev...), cur) {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) > pathsMax {
		out = out[len(out)-pathsMax:]
	}
	return out
}

// recordBotHit 机器人请求单独计数: 窗口内来源去重(实时 bots) + 今日按原因累计。
// 与正常会话隔离存储, 不污染 UV/PV 与明细; 后台"机器人请求 PV"及分布即今日累计值。
func (s *OnlineService) recordBotHit(ctx context.Context, sess OnlineSession, reason botdetect.Reason) {
	now := time.Now()
	pipe := s.rdb.Pipeline()
	pipe.ZAdd(ctx, onlineBotsKey, redis.Z{Score: float64(now.UnixMilli()), Member: onlineSessionKey(sess.Sid, sess.IP)})
	pipe.Expire(ctx, onlineBotsKey, onlineInfoTTL)
	pipe.HIncrBy(ctx, dailyBotKey(now.Format("20060102")), string(reason), 1)
	pipe.Expire(ctx, dailyBotKey(now.Format("20060102")), dailyTTL)
	_, _ = pipe.Exec(ctx)
}

// Overview 返回在线 UV/PV/观看中 + 今日累计与明细(按最近活跃倒序, 由 ZRevRange 保证),
// 顺带惰性清理过期会话与页面访问流水、更新今日峰值。
func (s *OnlineService) Overview(ctx context.Context) OnlineOverview {
	cutoffMs := time.Now().UnixMilli() - int64(s.window/time.Millisecond)
	_ = s.rdb.ZRemRangeByScore(ctx, onlineActKey, "0",
		strconv.FormatInt(cutoffMs, 10)).Err() // 清理失败下轮再试
	_ = s.rdb.ZRemRangeByScore(ctx, onlinePVLogKey, "0",
		strconv.FormatInt(cutoffMs, 10)).Err()
	_ = s.rdb.ZRemRangeByScore(ctx, onlineBotsKey, "0",
		strconv.FormatInt(cutoffMs, 10)).Err()
	sessions := make([]OnlineSession, 0, 64)
	if n, err := s.rdb.ZCard(ctx, onlineActKey).Result(); err == nil && n > 0 {
		sids, _ := s.rdb.ZRevRange(ctx, onlineActKey, 0, -1).Result() // 最新活跃在前
		sessions = sids2sessions(ctx, s.rdb, sids, sessions)
	}
	// 防御: 明细再过滤一次机器人(写入端已过滤; 读取端按 UA+IP 兜底, 让规则升级立刻作用于旧数据)
	if n := len(sessions); n > 0 {
		kept := sessions[:0]
		for _, sess := range sessions {
			if !botdetect.IsKnownBot(sess.UA, sess.IP) {
				kept = append(kept, sess)
			}
		}
		sessions = kept
	}
	// 窗口内机器人来源数(单独统计)
	bots := 0
	if n, err := s.rdb.ZCount(ctx, onlineBotsKey,
		strconv.FormatInt(cutoffMs, 10), "+inf").Result(); err == nil {
		bots = int(n)
	}
	// UV 去重: 登录按 uid, 游客按 IP; 两者都空(异常)退化为按 sid
	seen := make(map[string]bool, len(sessions))
	uv, watching := 0, 0
	for i := range sessions {
		key := sessions[i].Sid
		switch {
		case sessions[i].UID > 0:
			key = fmt.Sprintf("u:%d", sessions[i].UID)
		case sessions[i].IP != "":
			key = "i:" + sessions[i].IP
		}
		if !seen[key] {
			seen[key] = true
			uv++
		}
		if sessions[i].Watching {
			watching++
		}
	}
	// 实时 PV = 窗口内页面访问次数(不去重)
	pv := int64(0)
	if n, err := s.rdb.ZCount(ctx, onlinePVLogKey,
		strconv.FormatInt(cutoffMs, 10), "+inf").Result(); err == nil {
		pv = n
	}
	today := s.todayStats(ctx, int64(len(sessions)))
	return OnlineOverview{UV: uv, PV: int(pv), Watching: watching, Bots: bots, Today: today, Sessions: sessions}
}

// todayStats 读今日 UV/PV/峰值; 当前活跃会话数超过峰值时惰性更新(峰值口径是会话数, 与 PV 无关)。
func (s *OnlineService) todayStats(ctx context.Context, curSessions int64) DailyStats {
	date := time.Now().Format("20060102")
	uvKey, pvKey, peakKey := dailyKeys(date)
	t := DailyStats{}
	if n, err := s.rdb.SCard(ctx, uvKey).Result(); err == nil {
		t.UV = int(n)
	}
	if n, err := s.rdb.Get(ctx, pvKey).Int(); err == nil {
		t.PV = n
	}
	if n, err := s.rdb.Get(ctx, peakKey).Int(); err == nil {
		t.Peak = n
	}
	// 机器人请求: HASH 按原因累计, 合计即今日 PV
	if m, err := s.rdb.HGetAll(ctx, dailyBotKey(date)).Result(); err == nil && len(m) > 0 {
		t.BotReasons = make(map[string]int, len(m))
		for k, v := range m {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				t.BotReasons[k] = n
				t.BotPV += n
			}
		}
	}
	if curSessions > int64(t.Peak) {
		t.Peak = int(curSessions)
		_ = s.rdb.Set(ctx, peakKey, curSessions, dailyTTL).Err() // 失败下轮再更新
	}
	return t
}

// sids2sessions 批量读取会话明细(缺失/解析失败的跳过)。
// 一次 MGet 取全部: 30 分钟窗口可能有上百条会话, 逐条 GET 会被后台 15s 轮询放大成上百次往返。
func sids2sessions(ctx context.Context, rdb *redis.Client, sids []string, out []OnlineSession) []OnlineSession {
	if len(sids) == 0 {
		return out
	}
	keys := make([]string, len(sids))
	for i, sid := range sids {
		keys[i] = onlineInfoKey(sid)
	}
	vals, err := rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return out
	}
	for i, v := range vals {
		if i >= len(sids) || v == nil {
			continue // 明细过期(清理竞态), 该会话本次跳过
		}
		raw, ok := v.(string)
		if !ok {
			continue
		}
		var sess OnlineSession
		if json.Unmarshal([]byte(raw), &sess) != nil {
			continue
		}
		out = append(out, sess)
	}
	return out
}

// DailySessions 返回最近 days 天(含今天)出现过的会话明细, 供后台"今日/近7天访问用户"列表。
// 排序: 在播(watching) 在前 → 当前在线(30min 窗口)其次 → 其余按最近活跃倒序。
// 同一会话跨天重复出现时按最近一天去重(取最后一次心跳的明细与时间)。
func (s *OnlineService) DailySessions(ctx context.Context, days int) []OnlineSession {
	if days < 1 {
		days = 1
	}
	if days > 30 {
		days = 30
	}
	// 当前在线会话集合(90s 窗口), 用于打 online 标记与排序
	now := time.Now()
	cutoffMs := now.UnixMilli() - int64(s.window/time.Millisecond)
	active := make(map[string]bool, 64)
	if sids, err := s.rdb.ZRevRangeByScore(ctx, onlineActKey, &redis.ZRangeBy{
		Min: strconv.FormatInt(cutoffMs, 10), Max: "+inf",
	}).Result(); err == nil {
		for _, k := range sids {
			active[k] = true
		}
	}

	// 按天收集 member → 最近一次出现(日期 + 明细 JSON) + 窗口内访问次数累计, 跨天去重
	type dailyHit struct {
		date   string
		seen   int64
		json   string
		visits int // 跨天累计: 同一会话在查询窗口内各天访问次数之和
	}
	best := make(map[string]dailyHit, 256)
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i).Format("20060102")
		members, err := s.rdb.ZRevRange(ctx, dailyLogKey(date), 0, -1).Result()
		if err != nil || len(members) == 0 {
			continue
		}
		// 批量读该日明细(MGet 一次), 顺带确定每个 member 的最近心跳时间(用明细里的 LastSeen)
		infos := make([]string, 0, len(members))
		for _, m := range members {
			infos = append(infos, dailyInfoKey(date, m))
		}
		vals, err := s.rdb.MGet(ctx, infos...).Result()
		if err != nil {
			continue
		}
		for j, m := range members {
			if j >= len(vals) || vals[j] == nil {
				continue
			}
			raw, ok := vals[j].(string)
			if !ok {
				continue
			}
			var sess OnlineSession
			if json.Unmarshal([]byte(raw), &sess) != nil {
				continue
			}
			cur, ok := best[m]
			if !ok {
				// 按天倒序遍历 ⇒ 首次命中即最近一天, 明细取它
				best[m] = dailyHit{date: date, seen: sess.LastSeen, json: raw, visits: sess.Visits}
				continue
			}
			cur.visits += sess.Visits
			if sess.LastSeen > cur.seen {
				cur.seen, cur.json, cur.date = sess.LastSeen, raw, date
			}
			best[m] = cur
		}
	}

	out := make([]OnlineSession, 0, len(best))
	for m, b := range best {
		var sess OnlineSession
		if json.Unmarshal([]byte(b.json), &sess) != nil {
			continue
		}
		// 过滤机器人/爬虫明细(写入端已过滤, 此处为规则演进/历史残留兜底)
		if botdetect.IsKnownBot(sess.UA, sess.IP) {
			continue
		}
		if b.visits > 0 {
			sess.Visits = b.visits
		}
		sess.Online = active[m]
		out = append(out, sess)
	}
	// 排序: 在播 > 在线 > 离线; 同级按最近活跃倒序
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		aw, bw := a.Watching, b.Watching
		if aw != bw {
			return aw
		}
		ao, bo := a.Online, b.Online
		if ao != bo {
			return ao
		}
		return a.LastSeen > b.LastSeen
	})
	return out
}

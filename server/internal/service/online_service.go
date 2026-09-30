package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// OnlineService Redis 在线统计: 匿名会话心跳, 多实例共享, 查询时惰性清理。
//
// 口径(在线 = 窗口 90s 内有心跳):
//   - PV = 当前活跃会话数: 每(浏览器标签/TV 设备 sid + 出口 IP)一个会话
//     同一浏览器换网络(IP 变)计 2 个会话行, 可看出同一用户多个网络出口
//   - UV = 去重后在线人数: 登录用户按 uid 去重, 游客按 IP 去重
//     (同一用户开多标签/多设备只算 1; 同 IP 多游客合并为 1, 口径偏保守)
//   - 观看中 = 活跃会话中 watching=true 的会话数
//
// 今日累计(自然日滚动, 键 TTL 48h 自动过期):
//   - 今日 UV = 当日出现过的去重人数(登录 u:{uid} / 游客 i:{ip}), SET
//   - 今日 PV = 当日访问次数: 同一 sid 距上次访问 ≥30min 计一次新访问, STRING
//   - 今日峰值 = 当日实时在线会话数峰值, STRING (Overview 时惰性更新)
//
// 机器人过滤: UA 命中爬虫关键词(applebot/googlebot 等)的心跳直接丢弃,
// 不入实时与今日统计 — 防止搜索爬虫污染在线数据。
//
// 存储:
//   - ZSET jc:online:act  member=sid#ip(或纯 sid) score=lastSeen(心跳续期; 查询时按窗口 ZREMRANGEBYSCORE)
//   - STRING jc:online:info:{sid#ip} 会话明细 JSON, TTL 略大于窗口(防 ZSET 清理后孤儿残留)
type OnlineService struct {
	rdb    *redis.Client
	window time.Duration
}

// OnlineSession 一个在线会话的明细(handler 层从请求解析 IP/UID/UA)。
// 隐私口径: Path 只记录页面路径(pathname, 不含 query 参数/影片标识),
// 不收集"正在看哪部影片"这类内容偏好; Username/IPRegion 由 Overview 时只读增强填充。
type OnlineSession struct {
	Sid       string `json:"sid"`
	IP        string `json:"ip"`
	UID       int64  `json:"uid,omitempty"`
	Watching  bool   `json:"watching"`
	UA        string `json:"ua,omitempty"`
	Path      string `json:"path,omitempty"`
	Username  string `json:"username,omitempty"`
	IPRegion  string `json:"ipRegion,omitempty"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
	// Online 仅每日/近7天明细(dailySessions)返回时有效: 该会话当前是否在线(90s 窗口内活跃)。
	Online bool `json:"online,omitempty"`
}

// OnlineOverview 在线概览: 实时 UV/PV/观看中 + 今日累计 + 会话明细(后台表单)。
type OnlineOverview struct {
	UV       int             `json:"uv"`
	PV       int             `json:"pv"`
	Watching int             `json:"watching"`
	Bots     int             `json:"bots"` // 实时机器人在线源数(90s 窗口, 单独统计)
	Today    DailyStats      `json:"today"`
	Sessions []OnlineSession `json:"sessions"`
}

// DailyStats 今日累计(自然日滚动)。
type DailyStats struct {
	UV    int `json:"uv"`    // 今日去重人数
	PV    int `json:"pv"`    // 今日访问次数(同一会话 ≥30min 间隔计新访问)
	Peak  int `json:"peak"`  // 今日峰值在线(实时会话数最高值)
	BotPV int `json:"botPV"` // 今日机器人请求PV(被过滤掉的爬虫请求次数)
}

const (
	onlineActKey  = "jc:online:act"
	onlineBotsKey = "jc:online:bots" // 实时机器人在线源(90s 窗口), ZSET, 与正常会话隔离
	onlineInfoTTL = 150 * time.Second // > 窗口 90s, 防 ZSET 清理后明细残留
	onlineWindow  = 90 * time.Second  // 心跳 30s, 90s 未上报判离线(容忍丢包/切后台)

	dailyTTL  = 48 * time.Hour    // 今日统计键 TTL: 覆盖当日全天 + 次日缓冲, 自然滚动
	visitGap  = 30 * time.Minute  // 同一 sid 距上次访问 ≥30min 计一次新访问(今日 PV)
	// 按日访问日志(明细回溯)TTL: 须支撑后台"近7天"查询 → 7 天 + 1 天缓冲;
	// 心跳每次续期, 实际留存 = 最后一次心跳 + 8 天。
	dailyLogTTL = 8 * 24 * time.Hour
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

// 今日统计键。
func dailyKeys(date string) (uv, pv, peak, lastPrefix string) {
	return "jc:online:daily:" + date + ":uv",
		"jc:online:daily:" + date + ":pv",
		"jc:online:daily:" + date + ":peak",
		"jc:online:daily:" + date + ":last:" // + sid
}

// 当日访问日志键(明细回溯): 心跳时同步写一份按日 ZSET + 按日明细, TTL 与今日统计一致。
func dailyLogKey(date string) string      { return "jc:online:daily:" + date + ":log" }
func dailyInfoKey(date, k string) string  { return "jc:online:daily:" + date + ":info:" + k }

// onlineBotUA 在线统计机器人/爬虫 UA 关键词表(小写子串匹配)。
// 与 telemetry 的 botUASubstrings 相互独立: 在线统计口径更宽松(空 UA 不判 bot,
// 可能是异常客户端而非爬虫), 且可单独追加而不互相影响。
// 注意: 子串匹配, 只加不会出现在正常浏览器/安卓壳 UA 中的关键词(如勿加 okhttp)。
// ↑ 追加识别关键词时在此新增即可。
var onlineBotUA = []string{
	// ── 通用爬虫/扫描器 ──
	"bot", "spider", "crawl", "slurp", "curl", "wget", "python", "go-http",
	"scrapy", "headless", "phantom", "masscan", "zgrab", "nmap", "censys",
	"semrush", "ahrefs", "mj12", "dotbot", "bytespider", "facebookexternalhit",
	// ── 搜索引擎/收录爬虫 ──
	"googlebot", "googleother", "adsbot-google", "mediapartners-google",
	"bingbot", "bingpreview", "baiduspider", "360spider", "yisouspider",
	"yandexbot", "duckduckbot", "petalbot", "ia_archiver", "applebot",
	"twitterbot", "linkedinbot",
	// ── AI 爬虫 ──
	"gptbot", "oai-searchbot", "claudebot", "ccbot", "amazonbot",
	// ── 监控/连通性检查 ──
	"uptimerobot", "pingdom", "statuscake", "gtmetrix", "monitis",
	// ── 即时通讯/社交平台爬虫 ──
	// 注意: 勿加 whatsapp — WhatsApp 内置浏览器(点开分享链接)UA 含该词, 是真实用户.
	"telegrambot", "slackbot", "discordbot",
	// ── 脚本 HTTP 客户端 ──
	"node", "axios", "http-client", "lwp", "mechanize", "httrack", "libwww",
	"exabot", "gigabot", "java/",
}

// isBotUAOnline 在线心跳机器人过滤(宽松版): 用 onlineBotUA 规则组,
// 但空 UA 不判 bot — 空 UA 可能是异常客户端而非爬虫, 不误伤在线统计。
func isBotUAOnline(ua string) bool {
	u := strings.ToLower(strings.TrimSpace(ua))
	if u == "" {
		return false
	}
	for _, s := range onlineBotUA {
		if strings.Contains(u, s) {
			return true
		}
	}
	return false
}

func NewOnlineService(rdb *redis.Client) *OnlineService {
	return &OnlineService{rdb: rdb, window: onlineWindow}
}

// Heartbeat 记录/续期一个会话。首次出现记 firstSeen(进入时间), 已存在沿用。
// ZSET score 用毫秒精度(窗口判断/排序), 明细里 FirstSeen/LastSeen 用秒(展示可读)。
// 机器人 UA 不入实时与今日统计, 单独计数(实时 bots + 今日 botPV), 后台展示可排除。
func (s *OnlineService) Heartbeat(ctx context.Context, sess OnlineSession) {
	if sess.Sid == "" {
		return
	}
	if isBotUAOnline(sess.UA) {
		s.recordBotHit(ctx, sess)
		return
	}
	now := time.Now()
	nowMs := now.UnixMilli()
	key := onlineSessionKey(sess.Sid, sess.IP)
	firstSeen := now.Unix()
	if v, err := s.rdb.Get(ctx, onlineInfoKey(key)).Result(); err == nil {
		var old OnlineSession
		if json.Unmarshal([]byte(v), &old) == nil && old.FirstSeen > 0 {
			firstSeen = old.FirstSeen
		}
	}
	sess.FirstSeen = firstSeen
	sess.LastSeen = now.Unix()
	buf, _ := json.Marshal(sess)
	pipe := s.rdb.Pipeline()
	pipe.ZAdd(ctx, onlineActKey, redis.Z{Score: float64(nowMs), Member: key})
	pipe.Set(ctx, onlineInfoKey(key), buf, onlineInfoTTL)
	// ---- 今日累计 ----
	date := now.Format("20060102")
	uvKey, pvKey, _, lastPrefix := dailyKeys(date)
	uidKey := "i:" + sess.IP
	if sess.UID > 0 {
		uidKey = fmt.Sprintf("u:%d", sess.UID)
	}
	if sess.IP != "" { // 游客无 IP 时不重复计 UV(异常心跳)
		pipe.SAdd(ctx, uvKey, uidKey)
		pipe.Expire(ctx, uvKey, dailyTTL)
	}
	// 今日 PV: 同一 sid 距上次访问 ≥30min 计一次新访问(首访必计)
	lastKey := lastPrefix + sess.Sid
	lastUnix, _ := s.rdb.Get(ctx, lastKey).Int64()
	if lastUnix == 0 || now.Unix()-lastUnix >= int64(visitGap/time.Second) {
		pipe.Incr(ctx, pvKey)
		pipe.Expire(ctx, pvKey, dailyTTL)
		pipe.Set(ctx, lastKey, now.Unix(), dailyTTL)
	}
	// ---- 当日访问日志(明细回溯): 按日 ZSET(最近活跃排序) + 按日明细, TTL 8 天(支撑近7天查询) ----
	pipe.ZAdd(ctx, dailyLogKey(date), redis.Z{Score: float64(nowMs), Member: key})
	pipe.Expire(ctx, dailyLogKey(date), dailyLogTTL)
	pipe.Set(ctx, dailyInfoKey(date, key), buf, dailyLogTTL)
	_, _ = pipe.Exec(ctx)
}

// recordBotHit 机器人请求单独计数: 实时 bots(90s 窗口内去重源) + 今日 botPV(请求次数)。
// 与正常会话隔离存储, 不污染 UV/PV 与明细; 后台"机器人请求PV"指标即今日累计值。
func (s *OnlineService) recordBotHit(ctx context.Context, sess OnlineSession) {
	now := time.Now()
	nowMs := now.UnixMilli()
	date := now.Format("20060102")
	pipe := s.rdb.Pipeline()
	pipe.ZAdd(ctx, onlineBotsKey, redis.Z{Score: float64(nowMs), Member: onlineSessionKey(sess.Sid, sess.IP)})
	pipe.Expire(ctx, onlineBotsKey, onlineInfoTTL)
	pipe.Incr(ctx, dailyInfoKey(date, "botpv"))
	pipe.Expire(ctx, dailyInfoKey(date, "botpv"), dailyTTL)
	_, _ = pipe.Exec(ctx)
}

// Overview 返回在线 UV/PV/观看中 + 今日累计与明细(按最近活跃倒序, 由 ZRevRange 保证),
// 顺带惰性清理过期会话、更新今日峰值。
func (s *OnlineService) Overview(ctx context.Context) OnlineOverview {
	cutoffMs := time.Now().UnixMilli() - int64(s.window/time.Millisecond)
	_ = s.rdb.ZRemRangeByScore(ctx, onlineActKey, "0",
		strconv.FormatInt(cutoffMs, 10)).Err() // 清理失败下轮再试
	_ = s.rdb.ZRemRangeByScore(ctx, onlineBotsKey, "0",
		strconv.FormatInt(cutoffMs, 10)).Err()
	sessions := make([]OnlineSession, 0, 64)
	pv, err := s.rdb.ZCard(ctx, onlineActKey).Result()
	if err == nil && pv > 0 {
		sids, _ := s.rdb.ZRevRange(ctx, onlineActKey, 0, -1).Result() // 最新活跃在前
		sessions = sids2sessions(ctx, s.rdb, sids, sessions)
	}
	// 防御: 明细再过滤一次机器人(写入端已过滤, 防规则演进/历史残留)
	if n := len(sessions); n > 0 {
		kept := sessions[:0]
		for _, sess := range sessions {
			if !isBotUAOnline(sess.UA) {
				kept = append(kept, sess)
			}
		}
		sessions = kept
	}
	// 实时机器人在线源数(90s 窗口, 单独统计)
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
	today := s.todayStats(ctx, pv)
	return OnlineOverview{UV: uv, PV: len(sessions), Watching: watching, Bots: bots, Today: today, Sessions: sessions}
}

// todayStats 读今日 UV/PV/峰值; 当前实时会话数超过峰值时惰性更新。
func (s *OnlineService) todayStats(ctx context.Context, curPV int64) DailyStats {
	date := time.Now().Format("20060102")
	uvKey, pvKey, peakKey, _ := dailyKeys(date)
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
	if n, err := s.rdb.Get(ctx, dailyInfoKey(date, "botpv")).Int(); err == nil {
		t.BotPV = n
	}
	if curPV > int64(t.Peak) {
		t.Peak = int(curPV)
		_ = s.rdb.Set(ctx, peakKey, curPV, dailyTTL).Err() // 失败下轮再更新
	}
	return t
}

// sids2sessions 批量读取会话明细(缺失/解析失败的跳过)。
func sids2sessions(ctx context.Context, rdb *redis.Client, sids []string, out []OnlineSession) []OnlineSession {
	for _, sid := range sids {
		v, err := rdb.Get(ctx, onlineInfoKey(sid)).Result()
		if err != nil {
			continue // 明细过期(清理竞态), 该会话本次跳过
		}
		var sess OnlineSession
		if json.Unmarshal([]byte(v), &sess) != nil {
			continue
		}
		out = append(out, sess)
	}
	return out
}

// DailySessions 返回最近 days 天(含今天)出现过的会话明细, 供后台"今日/近7天访问用户"列表。
// 排序: 在播(watching) 在前 → 当前在线(90s 窗口)其次 → 其余按最近活跃倒序。
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

	// 按天收集 member → 最近一次出现(日期 + 明细 JSON), 跨天去重
	type dailyHit struct {
		date string
		seen int64
		json string
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
			if cur, ok := best[m]; !ok || sess.LastSeen > cur.seen {
				best[m] = dailyHit{date: date, seen: sess.LastSeen, json: raw}
			}
		}
	}

	out := make([]OnlineSession, 0, len(best))
	for m, b := range best {
		var sess OnlineSession
		if json.Unmarshal([]byte(b.json), &sess) != nil {
			continue
		}
		// 过滤机器人/爬虫明细(写入端已过滤, 此处为规则演进/历史残留兜底)
		if isBotUAOnline(sess.UA) {
			continue
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

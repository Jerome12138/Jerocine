package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// OnlineService Redis 在线统计: 匿名会话心跳, 多实例共享, 查询时惰性清理。
//
// 口径(在线 = 窗口 90s 内有心跳):
//   - PV = 当前活跃会话数: 每个浏览器标签/TV 设备一个 sid, 打开页面即计 1
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
//   - ZSET jc:online:act  member=sid score=lastSeen(心跳续期; 查询时按窗口 ZREMRANGEBYSCORE)
//   - STRING jc:online:info:{sid} 会话明细 JSON, TTL 略大于窗口(防 ZSET 清理后孤儿残留)
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
}

// OnlineOverview 在线概览: 实时 UV/PV/观看中 + 今日累计 + 会话明细(后台表单)。
type OnlineOverview struct {
	UV       int             `json:"uv"`
	PV       int             `json:"pv"`
	Watching int             `json:"watching"`
	Today    DailyStats      `json:"today"`
	Sessions []OnlineSession `json:"sessions"`
}

// DailyStats 今日累计(自然日滚动)。
type DailyStats struct {
	UV   int `json:"uv"`   // 今日去重人数
	PV   int `json:"pv"`   // 今日访问次数(同一会话 ≥30min 间隔计新访问)
	Peak int `json:"peak"` // 今日峰值在线(实时会话数最高值)
}

const (
	onlineActKey  = "jc:online:act"
	onlineInfoTTL = 150 * time.Second // > 窗口 90s, 防 ZSET 清理后明细残留
	onlineWindow  = 90 * time.Second  // 心跳 30s, 90s 未上报判离线(容忍丢包/切后台)

	dailyTTL  = 48 * time.Hour    // 今日统计键 TTL: 覆盖当日全天 + 次日缓冲, 自然滚动
	visitGap  = 30 * time.Minute  // 同一 sid 距上次访问 ≥30min 计一次新访问(今日 PV)
)

func onlineInfoKey(sid string) string { return "jc:online:info:" + sid }

// 今日统计键。
func dailyKeys(date string) (uv, pv, peak, lastPrefix string) {
	return "jc:online:daily:" + date + ":uv",
		"jc:online:daily:" + date + ":pv",
		"jc:online:daily:" + date + ":peak",
		"jc:online:daily:" + date + ":last:" // + sid
}

// isBotUAOnline 在线心跳机器人过滤(宽松版): 复用 telemetry 的 botUASubstrings 关键词表,
// 但空 UA 不判 bot — 空 UA 可能是异常客户端而非爬虫, 不误伤在线统计。
func isBotUAOnline(ua string) bool {
	u := strings.ToLower(strings.TrimSpace(ua))
	if u == "" {
		return false
	}
	for _, s := range botUASubstrings {
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
// 机器人 UA 直接丢弃(不入实时与今日统计)。
func (s *OnlineService) Heartbeat(ctx context.Context, sess OnlineSession) {
	if sess.Sid == "" || isBotUAOnline(sess.UA) {
		return
	}
	now := time.Now()
	nowMs := now.UnixMilli()
	firstSeen := now.Unix()
	if v, err := s.rdb.Get(ctx, onlineInfoKey(sess.Sid)).Result(); err == nil {
		var old OnlineSession
		if json.Unmarshal([]byte(v), &old) == nil && old.FirstSeen > 0 {
			firstSeen = old.FirstSeen
		}
	}
	sess.FirstSeen = firstSeen
	sess.LastSeen = now.Unix()
	buf, _ := json.Marshal(sess)
	pipe := s.rdb.Pipeline()
	pipe.ZAdd(ctx, onlineActKey, redis.Z{Score: float64(nowMs), Member: sess.Sid})
	pipe.Set(ctx, onlineInfoKey(sess.Sid), buf, onlineInfoTTL)
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
	_, _ = pipe.Exec(ctx)
}

// Overview 返回在线 UV/PV/观看中 + 今日累计与明细(按最近活跃倒序, 由 ZRevRange 保证),
// 顺带惰性清理过期会话、更新今日峰值。
func (s *OnlineService) Overview(ctx context.Context) OnlineOverview {
	cutoffMs := time.Now().UnixMilli() - int64(s.window/time.Millisecond)
	_ = s.rdb.ZRemRangeByScore(ctx, onlineActKey, "0",
		strconv.FormatInt(cutoffMs, 10)).Err() // 清理失败下轮再试
	sessions := make([]OnlineSession, 0, 64)
	pv, err := s.rdb.ZCard(ctx, onlineActKey).Result()
	if err == nil && pv > 0 {
		sids, _ := s.rdb.ZRevRange(ctx, onlineActKey, 0, -1).Result() // 最新活跃在前
		sessions = sids2sessions(ctx, s.rdb, sids, sessions)
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
	return OnlineOverview{UV: uv, PV: len(sessions), Watching: watching, Today: today, Sessions: sessions}
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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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

// OnlineOverview 在线概览: UV/PV/观看中 + 会话明细(后台表单)。
type OnlineOverview struct {
	UV       int             `json:"uv"`
	PV       int             `json:"pv"`
	Watching int             `json:"watching"`
	Sessions []OnlineSession `json:"sessions"`
}

const (
	onlineActKey  = "jc:online:act"
	onlineInfoTTL = 150 * time.Second // > 窗口 90s, 防 ZSET 清理后明细残留
	onlineWindow  = 90 * time.Second  // 心跳 30s, 90s 未上报判离线(容忍丢包/切后台)
)

func onlineInfoKey(sid string) string { return "jc:online:info:" + sid }

func NewOnlineService(rdb *redis.Client) *OnlineService {
	return &OnlineService{rdb: rdb, window: onlineWindow}
}

// Heartbeat 记录/续期一个会话。首次出现记 firstSeen(进入时间), 已存在沿用。
// ZSET score 用毫秒精度(窗口判断/排序), 明细里 FirstSeen/LastSeen 用秒(展示可读)。
func (s *OnlineService) Heartbeat(ctx context.Context, sess OnlineSession) {
	if sess.Sid == "" {
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
	_, _ = pipe.Exec(ctx)
}

// Overview 返回在线 UV/PV/观看中与明细(按最近活跃倒序, 由 ZRevRange 保证),
// 顺带惰性清理过期会话。
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
	return OnlineOverview{UV: uv, PV: len(sessions), Watching: watching, Sessions: sessions}
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

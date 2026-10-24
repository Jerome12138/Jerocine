package handler

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/botdetect"
	"server/internal/domain"
	"server/internal/dto"
	"server/internal/geoip"
	"server/internal/service"
)

// ---- 在线统计: 客户端心跳上报(公开) + 后台概览/明细(管理) ----

// OnlineHeartbeat 播放器/页面心跳: {sid, watching, path, webdriver, pv}。
// sid 由客户端按设备/标签生成(localStorage 随机), watching=true 表示正在播放,
// webdriver 为 navigator.webdriver 原值(无头浏览器默认 true, 是机器人信号之一)。
// pv=true 表示本次心跳对应一次页面访问(首次载入/路由切换), 计入 PV;
// 常规 30s 续活心跳不带 pv, 由 service 按路径变化兜底判定。
// 公开接口 + 限流; 可选携带 Bearer token(登录用户): 服务端解析出 uid,
// 供后台按"同一用户"去重; 游客按 IP 去重。IP/UA 由服务端从请求取, 不信任客户端。
// 隐私: path 仅保留页面路径(pathname, 去 query/hash/影片标识), 不收集"在看什么"。
func (h *Handlers) OnlineHeartbeat(c *gin.Context) {
	var req struct {
		SID       string `json:"sid"`
		Watching  bool   `json:"watching"`
		Path      string `json:"path"`
		WebDriver bool   `json:"webdriver"`
		PV        bool   `json:"pv"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Fail(c, domain.ErrInvalidArgument)
		return
	}
	sid := strings.TrimSpace(req.SID)
	if sid == "" {
		dto.OK(c, gin.H{"ok": true}) // 客户端未生成会话标识, 不计数
		return
	}
	// 可选登录: Bearer 有效则计入 uid(跨设备/标签去重); 无效/缺失按 IP 去重, 不阻断心跳。
	var uid int64
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if claims, err := h.User.Authenticate(c.Request.Context(), strings.TrimSpace(auth[len("Bearer "):])); err == nil {
			uid = int64(claims.UserID)
		}
	}
	ip := c.ClientIP()
	sess := service.OnlineSession{
		Sid:      sid,
		IP:       ip,
		UID:      uid,
		Watching: req.Watching,
		UA:       truncate(c.Request.UserAgent(), 200),
		Path:     sanitizePath(req.Path),
		PV:       req.PV,
	}
	// 机器人判定放在 handler: 只有这里能同时拿到 UA、请求头、IP 与 IP 归属(ISP)。
	// 命中即由 service 计入机器人计数, 不进 UV/PV 与明细。
	reason := heartbeatBotReason(c, sess.UA, req.WebDriver, ip)
	h.Online.Heartbeat(c.Request.Context(), sess, reason)
	dto.OK(c, gin.H{"ok": true})
}

// heartbeatBotReason 把请求侧可见信号交给 botdetect 判定(抽成函数便于单测:
// 少了 Accept-Language/Sec-Fetch-* 的真实浏览器几乎不存在, 机房 IP 也能在这里被逮到)。
func heartbeatBotReason(c *gin.Context, ua string, webdriver bool, ip string) botdetect.Reason {
	return botdetect.Classify(botdetect.Input{
		UA:             ua,
		AcceptLanguage: c.GetHeader("Accept-Language"),
		SecFetchMode:   c.GetHeader("Sec-Fetch-Mode"),
		WebDriver:      webdriver,
		IP:             ip,
		ISP:            geoip.SearchISP(ip),
	})
}

// OnlineOverview 管理后台: 在线 UV(去重人数) / PV(窗口内页面访问次数, 不去重) / 观看中 + 会话明细表单。
// 明细只读增强: 登录用户回填昵称(uid → userName), 会话回填 IP 归属地(离线库), 查询失败静默。
func (h *Handlers) OnlineOverview(c *gin.Context) {
	ov := h.Online.Overview(c.Request.Context())
	h.enrichSessions(c.Request.Context(), ov.Sessions)
	dto.OK(c, ov)
}

// OnlineDailySessions 管理后台: 今日 / 近7天访问用户明细列表。
// query: days=1(今日, 默认) | 7(近7天); 排序: 在播 → 在线 → 最近活跃倒序。
func (h *Handlers) OnlineDailySessions(c *gin.Context) {
	days := 1
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	sessions := h.Online.DailySessions(c.Request.Context(), days)
	h.enrichSessions(c.Request.Context(), sessions)
	dto.OK(c, gin.H{"days": days, "sessions": sessions})
}

// enrichSessions 只读增强: 登录用户回填昵称 + 会话回填 IP 归属地, 查询失败静默。
func (h *Handlers) enrichSessions(ctx context.Context, sessions []service.OnlineSession) {
	ids := make([]int64, 0, len(sessions))
	for _, s := range sessions {
		if s.UID > 0 {
			ids = append(ids, s.UID)
		}
	}
	names := h.User.NamesByIDs(ctx, ids)
	for i := range sessions {
		sessions[i].Username = names[sessions[i].UID]
		sessions[i].IPRegion = geoip.Search(sessions[i].IP)
	}
}

// sanitizePath 规整页面路径: 只保留 pathname(去 query/hash 与影片标识), 限长, 必须 / 开头。
func sanitizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || !strings.HasPrefix(p, "/") {
		return ""
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	return truncate(p, 64) // 按 rune 截断: 多字节路径不会切出半个字符
}

// truncate 超长字段截断(明细表防撑爆)。
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max])
}

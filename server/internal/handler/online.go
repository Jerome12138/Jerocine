package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/domain"
	"server/internal/dto"
	"server/internal/service"
)

// ---- 在线统计: 客户端心跳上报(公开) + 后台概览/明细(管理) ----

// OnlineHeartbeat 播放器/页面心跳: {sid, watching}。
// sid 由客户端按设备/标签生成(localStorage 随机), watching=true 表示正在播放。
// 公开接口 + 限流; 可选携带 Bearer token(登录用户): 服务端解析出 uid,
// 供后台按"同一用户"去重; 游客按 IP 去重。IP/UA 由服务端从请求取, 不信任客户端。
// 隐私: 不收集页面路径/标题(与 IP/用户组合即观看行为隐私)。
func (h *Handlers) OnlineHeartbeat(c *gin.Context) {
	var req struct {
		SID      string `json:"sid"`
		Watching bool   `json:"watching"`
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
	h.Online.Heartbeat(c.Request.Context(), service.OnlineSession{
		Sid:      sid,
		IP:       c.ClientIP(),
		UID:      uid,
		Watching: req.Watching,
		UA:       truncate(c.Request.UserAgent(), 200),
	})
	dto.OK(c, gin.H{"ok": true})
}

// OnlineOverview 管理后台: 在线 UV(去重人数) / PV(会话数) / 观看中 + 会话明细表单。
func (h *Handlers) OnlineOverview(c *gin.Context) {
	dto.OK(c, h.Online.Overview(c.Request.Context()))
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

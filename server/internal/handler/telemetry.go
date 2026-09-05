package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"server/internal/dto"
)

// TelemetryOverview GET /manage/telemetry/overview?days=
func (h *Handlers) TelemetryOverview(c *gin.Context) {
	ov, err := h.Telemetry.Overview(c.Request.Context(), queryInt(c, "days", 7))
	respond(c, ov, err)
}

// TelemetryEvents GET /manage/telemetry/events?type=&days=&status=&limit=&offset=
// 看板错误列表: 前端按 type=error 拉, 含问题闭环 issueStatus, 返回 {rows,total}。
func (h *Handlers) TelemetryEvents(c *gin.Context) {
	rows, total, err := h.Telemetry.ErrorEvents(
		c.Request.Context(),
		queryInt(c, "days", 7),
		c.Query("type"),
		c.DefaultQuery("status", "active"),
		queryInt(c, "limit", 100),
		queryInt(c, "offset", 0),
	)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"rows": rows, "total": total})
}

// TelemetryPurge POST /manage/telemetry/purge-bots — 清理历史脏数据(bot/空UA/垃圾路径)。
func (h *Handlers) TelemetryPurge(c *gin.Context) {
	n, err := h.Telemetry.PurgeNoise(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"deleted": n})
}

// TelemetryPurgeApiErrors POST /manage/telemetry/purge-api-errors —
// 一次性清理误记的 API HTTP 错误历史(extra.type=error 且 category 以 api- 前缀, 如 api-4xx/api-5xx)。
func (h *Handlers) TelemetryPurgeApiErrors(c *gin.Context) {
	n, err := h.Telemetry.PurgeApiErrors(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"deleted": n})
}

// TelemetryPurgeErrorsBefore POST /manage/telemetry/purge-errors-before?days=5 —
// 按龄清理: 删除 days 天前的错误事件(extra.type=error 且 server_ts < 截止), 默认 5 天。
func (h *Handlers) TelemetryPurgeErrorsBefore(c *gin.Context) {
	n, err := h.Telemetry.PurgeErrorsBefore(c.Request.Context(), queryInt(c, "days", 5))
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"deleted": n})
}

// TelemetryPurgeChunkNoise POST /manage/telemetry/purge-chunk-noise —
// 清理 chunk 加载失败类噪声错误(部署后旧 hash 自愈现象, 非真异常)。
func (h *Handlers) TelemetryPurgeChunkNoise(c *gin.Context) {
	n, err := h.Telemetry.PurgeChunkNoise(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"deleted": n})
}

// TelemetryTopPaths GET /manage/telemetry/top-paths?hours=&limit=
func (h *Handlers) TelemetryTopPaths(c *gin.Context) {
	list, err := h.Telemetry.TopPaths(c.Request.Context(), queryInt(c, "hours", 24), queryInt(c, "limit", 20))
	respond(c, list, err)
}

// TelemetryApiPerf GET /manage/telemetry/api-perf?days=&limit=
func (h *Handlers) TelemetryApiPerf(c *gin.Context) {
	list, err := h.Telemetry.ApiPerf(c.Request.Context(), queryInt(c, "days", 7), queryInt(c, "limit", 10))
	respond(c, list, err)
}

// TelemetryHotFilms GET /manage/telemetry/hot-films?hours=&limit=  (热点视频: 按埋点 pv 访问量)
func (h *Handlers) TelemetryHotFilms(c *gin.Context) {
	list, err := h.Telemetry.HotFilms(c.Request.Context(), queryInt(c, "hours", 24*7), queryInt(c, "limit", 10))
	respond(c, list, err)
}

// TelemetryMostFavorited GET /manage/telemetry/most-favorited?limit=  (收藏最多)
func (h *Handlers) TelemetryMostFavorited(c *gin.Context) {
	list, err := h.Telemetry.MostFavorited(c.Request.Context(), queryInt(c, "limit", 10))
	respond(c, list, err)
}

type issueReq struct {
	Category string `json:"category"`
	Path     string `json:"path"`
	Label    string `json:"label"`
	CommitId string `json:"commitId"`
}

// TelemetryResolve POST /manage/telemetry/resolve
func (h *Handlers) TelemetryResolve(c *gin.Context) {
	var req issueReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Telemetry.ResolveIssue(c.Request.Context(), req.Category, req.Path, req.Label, req.CommitId); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"ok": true})
}

// TelemetryUnresolve POST /manage/telemetry/unresolve
func (h *Handlers) TelemetryUnresolve(c *gin.Context) {
	var req issueReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Telemetry.UnresolveIssue(c.Request.Context(), req.Category, req.Path, req.Label); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"ok": true})
}

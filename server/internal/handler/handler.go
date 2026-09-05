// Package handler 是 HTTP 薄层: 绑定/校验/响应 + ETag/304, 不含业务逻辑。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"server/internal/middleware"
	"server/internal/platform/blobstore"
	"server/internal/service"
)

// Handlers 持有全部 service, 提供 gin.HandlerFunc。
type Handlers struct {
	Film      *service.FilmService
	User      *service.UserService
	Config    *service.ConfigService
	M3u8      *service.M3u8Service
	Telemetry *service.TelemetryService
	Manage     *service.ManageService
	Spider     *service.SpiderService
	Blob       blobstore.BlobStore
	ResetToken string // /manage/spider/reset 二次确认 token
}

// ---- 参数助手 ----

func pathInt64(c *gin.Context, name string) (int64, bool) {
	n, err := strconv.ParseInt(c.Param(name), 10, 64)
	return n, err == nil
}

func queryInt64(c *gin.Context, name string) int64 {
	n, _ := strconv.ParseInt(c.Query(name), 10, 64)
	return n
}

func queryInt(c *gin.Context, name string, def int) int {
	if v := c.Query(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func currentUserID(c *gin.Context) int64 {
	return int64(c.GetUint(middleware.CtxUserID))
}

// Package handler 是 HTTP 薄层: 绑定/校验/响应 + ETag/304, 不含业务逻辑。
package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"server/internal/middleware"
	"server/internal/platform/blobstore"
	"server/internal/service"
)

// Handlers 持有全部 service, 提供 gin.HandlerFunc。
type Handlers struct {
	Film       *service.FilmService
	User       *service.UserService
	Config     *service.ConfigService
	Telemetry  *service.TelemetryService
	Manage     *service.ManageService
	Spider     *service.SpiderService
	Banner     *service.BannerService
	Backdrop   *service.BackdropService // TMDB 横图回填 worker(单片手动触发用)
	Hot        *service.HotService      // 榜单热度刷新(每日 04:00 自动 + 后台手动触发)
	Tasks      *service.TaskRunService
	Online     *service.OnlineService   // 内存在线/观看统计(心跳上报)
	Register   *service.RegisterService // 自注册 + 邮件通道(注册配置/发码/注册/邮件管理)
	Danmu      *service.DanmuService    // 弹幕: 豆瓣 vendors 候选 + B站弹幕 + 聚合弹幕
	Live       *service.LiveService     // 直播: 直播源公开解析 + 管理端 CRUD
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

// queryInt64s 解析逗号分隔的 int64 列表(`catIds=1,2,3`); 空串/全非法段返回 nil。
// 非法段跳过不报错 —— 筛选参数宽松处理, 解析不出值就当没传。
func queryInt64s(c *gin.Context, name string) []int64 {
	raw := c.Query(name)
	if raw == "" {
		return nil
	}
	var out []int64
	for _, part := range strings.Split(raw, ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

func currentUserID(c *gin.Context) int64 {
	return int64(c.GetUint(middleware.CtxUserID))
}

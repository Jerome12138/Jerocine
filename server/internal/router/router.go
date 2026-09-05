// Package router 装配 /api/v1 路由表。
package router

import (
	"github.com/gin-gonic/gin"

	"server/internal/config"
	"server/internal/handler"
	"server/internal/middleware"
	"server/internal/service"
)

// Register 在 engine 上挂载全部 /api/v1 业务路由(健康检查由调用方单独注册)。
func Register(r *gin.Engine, h *handler.Handlers, us *service.UserService, cfg *config.Config) {
	r.Use(middleware.Recovery(), middleware.Trace(), middleware.Cors(cfg.CORS))

	v1 := r.Group("/api/v1")

	// ---- 公开内容 (GET 可边缘缓存) ----
	v1.GET("/home", h.Home)
	v1.GET("/categories", h.Categories)
	v1.GET("/categories/:pid/filters", h.Filters)
	v1.GET("/films", h.Films)
	v1.GET("/films/classify", h.Classify)
	v1.GET("/films/:mid", h.Detail)
	v1.GET("/films/:mid/play", h.Play)
	v1.GET("/films/:mid/related", h.Related)
	v1.GET("/config/site", h.SiteConfig)
	v1.GET("/app/version/latest", h.VersionLatest)
	// m3u8 代理须为 GET: <video>/hls.js 按 src 拉取 manifest, 无法 POST。
	v1.GET("/m3u8/proxy", middleware.RateLimit("m3u8", 30, 10), h.M3u8Proxy)
	// HLS 分片/密钥由 manifest 生成的短期签名 URL 访问，避免低端设备直连 HTTPS CDN。
	v1.GET("/m3u8/stream", middleware.RateLimit("m3u8stream", 1200, 50), h.M3u8Stream)
	// m3u8 广告过滤统计: 前端据 filtered>0 弹"已过滤 N 段广告"。
	v1.GET("/m3u8/stats", middleware.RateLimit("m3u8stats", 30, 10), h.M3u8Stats)
	// m3u8 端侧混合过滤: 客户端抓流→送文本→服务器只过滤(不抓源), 用于服务器抓不到的源(bf 地域封)。
	v1.POST("/m3u8/filter", middleware.RateLimit("m3u8filter", 60, 20), h.M3u8Filter)
	v1.POST("/telemetry/events", middleware.RateLimit("telemetry", 60, 20), h.TelemetryIngest)

	// ---- 认证 ----
	v1.POST("/auth/login", middleware.RateLimit("login", 10, 0.5), h.Login)
	// 扫码登录(设备码流程): 发码 / 轮询 公开, 确认(confirm)需登录(在 me 组内)
	v1.POST("/auth/device/code", middleware.RateLimit("devcode", 10, 0.5), h.DeviceCode)
	v1.POST("/auth/device/poll", middleware.RateLimit("devpoll", 120, 5), h.DevicePoll)

	// ---- 用户(Bearer + no-store) ----
	me := v1.Group("")
	me.Use(middleware.AuthToken(us), middleware.NoStore())
	{
		me.POST("/auth/logout", h.Logout)
		me.POST("/auth/device/confirm", h.DeviceConfirm)
		me.GET("/me", h.UserInfo)
		me.PATCH("/me/password", h.ChangePassword)
		me.GET("/me/histories", h.HistoryList)
		me.POST("/me/histories", h.HistoryUpsert)
		me.DELETE("/me/histories", h.HistoryDelete)
		me.DELETE("/me/histories/clear", h.HistoryClear)
		me.GET("/me/favorites", h.FavoriteList)
		me.POST("/me/favorites", h.FavoriteAdd)
		me.DELETE("/me/favorites", h.FavoriteRemove)
		me.GET("/me/favorites/:mid", h.FavoriteCheck)
		// 账号级片头/片尾跳过设置(跨设备)
		me.GET("/me/skip-settings", h.SkipList)
		me.PUT("/me/skip-settings/:mid", h.SkipSave)
		me.DELETE("/me/skip-settings/:mid", h.SkipReset)
	}

	// ---- 管理后台(Bearer + Admin + no-store) ----
	mg := v1.Group("/manage")
	mg.Use(middleware.AuthToken(us), middleware.RequireAdmin(), middleware.NoStore())
	{
		mg.GET("/dashboard", h.Dashboard)
		mg.GET("/site-config", h.GetSiteConfig)
		mg.POST("/site-config", h.SaveSiteConfig)

		mg.GET("/collect-sources", h.ListSources)
		mg.POST("/collect-sources", h.UpsertSource)
		mg.GET("/collect-sources/health", h.ListSourceHealth) // 健康度面板(静态段, 先于 :id)
		mg.POST("/collect-sources/test", h.TestAllSources)    // 全量测速(静态段, 先于 :id 注册)
		mg.GET("/collect-sources/:id", h.GetSource)
		mg.PUT("/collect-sources/:id", h.UpsertSource)
		mg.DELETE("/collect-sources/:id", h.DeleteSource)
		mg.POST("/collect-sources/:id/test", h.TestSource)

		mg.GET("/cron-tasks", h.ListCrons)
		mg.POST("/cron-tasks", h.UpsertCron)
		mg.PUT("/cron-tasks/:id", h.UpsertCron)
		mg.DELETE("/cron-tasks/:id", h.DeleteCron)

		mg.GET("/categories", h.ListCategories)
		mg.POST("/categories", h.UpsertCategory)
		mg.PUT("/categories/:id", h.UpsertCategory)
		mg.DELETE("/categories/:id", h.DeleteCategory)

		mg.GET("/files", h.ListFiles)
		mg.POST("/files", h.UploadFile)
		mg.DELETE("/files/:id", h.DeleteFile)

		mg.GET("/app-versions", h.ListVersions)
		mg.POST("/app-versions", h.CreateVersion)
		mg.DELETE("/app-versions/:id", h.DeleteVersion)
		mg.POST("/app-versions/apk", h.UploadApk)

		mg.GET("/users", h.ListUsers)
		mg.POST("/users", h.CreateUser)

		mg.GET("/films", h.ManageFilms)
		mg.POST("/films", h.AddFilm)
		mg.GET("/films/:mid", h.ManageGetFilm)
		mg.GET("/films/:mid/detail", h.ManageFilmDetail) // 影片 + 全部源与集(实时)

		mg.GET("/spider/jobs", h.SpiderJobs)
		mg.POST("/spider/jobs", h.SpiderStart)
		mg.POST("/spider/jobs/:id/pause", h.SpiderPause)
		mg.POST("/spider/jobs/:id/resume", h.SpiderResume)
		mg.POST("/spider/jobs/:id/cancel", h.SpiderCancel)
		mg.POST("/spider/reset", h.SpiderReset)
		mg.POST("/spider/category-cover", h.CategoryCover)
		mg.POST("/spider/search", h.SpiderSearch)        // 按片名搜索各源(预览, 不落库)
		mg.POST("/spider/collect-film", h.CollectFilm)   // 按片名采集(单/多/全部源)

		mg.GET("/telemetry/overview", h.TelemetryOverview)
		mg.GET("/telemetry/events", h.TelemetryEvents)
		mg.GET("/telemetry/top-paths", h.TelemetryTopPaths)
		mg.GET("/telemetry/api-perf", h.TelemetryApiPerf)
		mg.GET("/telemetry/hot-films", h.TelemetryHotFilms)
		mg.GET("/telemetry/most-favorited", h.TelemetryMostFavorited)
		mg.POST("/telemetry/resolve", h.TelemetryResolve)
		mg.POST("/telemetry/unresolve", h.TelemetryUnresolve)
		mg.POST("/telemetry/purge-bots", h.TelemetryPurge)                  // 清理历史脏数据(bot/扫描噪声)
		mg.POST("/telemetry/purge-api-errors", h.TelemetryPurgeApiErrors)   // 清理误记的 API HTTP 错误历史(api-4xx/5xx)
		mg.POST("/telemetry/purge-errors-before", h.TelemetryPurgeErrorsBefore) // 按龄清理错误(默认删5天前)
		mg.POST("/telemetry/purge-chunk-noise", h.TelemetryPurgeChunkNoise)     // 清理 chunk 加载失败噪声(部署自愈现象)
	}
}

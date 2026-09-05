// Command server 是重构后后端入口(组合根)。
// 启动流程: Config(fail-fast) → MySQL/Redis → blob → repository → service/engine → handler → /api/v1 → cron → 监听。
// 含 -healthcheck 子命令: 供 distroless 容器 healthcheck 调用(镜像内无 curl/wget)。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"server/internal/config"
	"server/internal/handler"
	"server/internal/platform/auth"
	"server/internal/platform/blobstore"
	"server/internal/platform/db"
	repomysql "server/internal/repository/mysql"
	"server/internal/router"
	"server/internal/service"
	"server/internal/spider"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "内部健康探针: GET /healthz, 200 退出 0 否则 1")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	cfg := config.Load()
	log.Printf("starting: %s", cfg.String())

	app, err := buildApp(cfg)
	if err != nil {
		log.Fatalf("启动失败: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/livez", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/healthz", app.health)
	r.Static(cfg.Blob.BaseURL, cfg.Blob.LocalDir) // 本地图片访问(生产由 nginx 托管)

	router.Register(r, app.handlers, app.userSvc, cfg)

	// 启动 cron 调度(读 cron_task 注册已启用任务)
	app.spiderSvc.StartScheduler(context.Background())
	// 启动采集源健康检查定时任务(默认 1h, 写健康度 → 自动停采/恢复死源)
	app.handlers.Manage.StartHealthScheduler(context.Background())

	log.Printf("listening on :%s", cfg.ServerPort)
	if err := r.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}

// App 组合根容器。
type App struct {
	gdb      *gorm.DB
	cacheRdb *redis.Client
	coordRdb *redis.Client
	userSvc  *service.UserService
	spiderSvc *service.SpiderService
	handlers *handler.Handlers
}

func buildApp(cfg *config.Config) (*App, error) {
	gdb, err := db.InitMySQL(cfg.MySQLDSN)
	if err != nil {
		return nil, fmt.Errorf("init mysql: %w", err)
	}
	cacheRdb, coordRdb, err := db.InitRedis(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("init redis: %w", err)
	}
	tokenMgr, err := auth.NewTokenManager(cfg.JWT.PrivateKey, cfg.JWT.PublicKey, cfg.JWT.TTL, cfg.JWT.Issuer)
	if err != nil {
		return nil, fmt.Errorf("init jwt: %w", err)
	}
	blob := blobstore.NewLocal(cfg.Blob.LocalDir, cfg.Blob.BaseURL)
	tx := repomysql.NewTxManager(gdb)

	// repositories
	movieRepo := repomysql.NewMovieRepository(gdb)
	searchRepo := repomysql.NewSearchRepository(gdb)
	// 启动一次性回填 name_pinyin(仅空值行), 让首字母搜索覆盖存量数据; 后台执行不阻塞启动。
	go func() {
		c, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if n, err := repomysql.BackfillNamePinyin(c, gdb, 500); err != nil {
			log.Printf("[pinyin] backfill name_pinyin failed: %v", err)
		} else if n > 0 {
			log.Printf("[pinyin] backfilled name_pinyin for %d movie_search rows", n)
		}
	}()
	playRepo := repomysql.NewPlaySourceRepository(gdb)
	categoryRepo := repomysql.NewCategoryRepository(gdb)
	sourceRepo := repomysql.NewCollectSourceRepository(gdb)
	cronRepo := repomysql.NewCronTaskRepository(gdb)
	siteRepo := repomysql.NewSiteConfigRepository(gdb)
	versionRepo := repomysql.NewAppVersionRepository(gdb)
	fileRepo := repomysql.NewFileRepository(gdb)
	userRepo := repomysql.NewUserRepository(gdb)
	historyRepo := repomysql.NewHistoryRepository(gdb)
	favoriteRepo := repomysql.NewFavoriteRepository(gdb)
	skipRepo := repomysql.NewSkipSettingRepository(gdb)
	telemetryRepo := repomysql.NewTelemetryRepository(gdb)
	healthRepo := repomysql.NewSourceHealthRepository(gdb)

	// services
	filmSvc := service.NewFilmService(searchRepo, movieRepo, playRepo, categoryRepo, healthRepo, sourceRepo, cfg.PageSizes)
	userSvc := service.NewUserService(userRepo, historyRepo, favoriteRepo, skipRepo, searchRepo, tokenMgr, cfg.JWT.MaxDevices, cfg.JWT.TTL)
	configSvc := service.NewConfigService(siteRepo, versionRepo)
	m3u8Svc := service.NewM3u8Service(cfg.JWT.PrivateKey)
	telemetrySvc := service.NewTelemetryService(telemetryRepo, searchRepo, favoriteRepo, cfg.TelemetryAllowedHosts, configSvc)
	manageSvc := service.NewManageService(sourceRepo, cronRepo, siteRepo, versionRepo, fileRepo, categoryRepo, searchRepo, movieRepo, playRepo, healthRepo, tx, userSvc, blob)
	engine := spider.NewEngine(movieRepo, searchRepo, playRepo, categoryRepo, fileRepo, tx, blob, cfg.Spider.MaxGoroutine)
	spiderSvc := service.NewSpiderService(engine, sourceRepo, cronRepo, healthRepo)

	handlers := &handler.Handlers{
		Film: filmSvc, User: userSvc, Config: configSvc, M3u8: m3u8Svc,
		Telemetry: telemetrySvc, Manage: manageSvc, Spider: spiderSvc,
		Blob: blob, ResetToken: cfg.Spider.ResetToken,
	}

	return &App{
		gdb: gdb, cacheRdb: cacheRdb, coordRdb: coordRdb,
		userSvc: userSvc, spiderSvc: spiderSvc, handlers: handlers,
	}, nil
}

// health 就绪探针: ping mysql + redis(cache/coord)。
func (a *App) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if sqlDB, err := a.gdb.DB(); err != nil || sqlDB.PingContext(ctx) != nil {
		c.String(http.StatusServiceUnavailable, "mysql down")
		return
	}
	if a.cacheRdb.Ping(ctx).Err() != nil || a.coordRdb.Ping(ctx).Err() != nil {
		c.String(http.StatusServiceUnavailable, "redis down")
		return
	}
	c.String(http.StatusOK, "ok")
}

func runHealthcheck() int {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "3601"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

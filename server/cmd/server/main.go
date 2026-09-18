// Command server 是重构后后端入口(组合根)。
// 启动流程: Config(fail-fast) → MySQL/Redis → blob → repository → service/engine → handler → /api/v1 → cron → 监听。
// 含 -healthcheck 子命令: 供 distroless 容器 healthcheck 调用(镜像内无 curl/wget)。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"server/internal/config"
	"server/internal/douban"
	"server/internal/geoip"
	"server/internal/handler"
	"server/internal/platform/auth"
	"server/internal/platform/blobstore"
	"server/internal/platform/db"
	repomysql "server/internal/repository/mysql"
	"server/internal/router"
	"server/internal/service"
	"server/internal/spider"
	"server/internal/tmdb"
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

	// 优雅停机根 ctx: SIGTERM/SIGINT 取消 → 采集在跑轮次快速收尾、后台调度停止接新活。
	// docker stop / compose up -d --build 重建容器时走这条路径, 部署不再需要手动暂停采集。
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/livez", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/healthz", app.health)
	r.Static(cfg.Blob.BaseURL, cfg.Blob.LocalDir) // 本地图片访问(生产由 nginx 托管)

	router.Register(r, app.handlers, app.userSvc, cfg)

	// 后台调度统一挂 rootCtx; 采集服务额外注入根 ctx 供手动触发/定时任务派生。
	app.spiderSvc.SetBaseCtx(rootCtx)
	// 服务重启: 把上一轮遗留的 running 台账行标记为中断(避免幽灵"运行中"任务)。
	app.handlers.Tasks.MarkInterrupted(rootCtx)
	// 启动 cron 调度(读 cron_task 注册已启用任务)
	app.spiderSvc.StartScheduler(rootCtx)
	// 启动采集源健康检查定时任务(默认 1h, 写健康度 → 自动停采/恢复死源)
	app.handlers.Manage.StartHealthScheduler(rootCtx)
	// 启动 TMDB 横图回填 worker(未配置 key 时为 no-op)
	app.backdropSvc.Start(rootCtx)
	// 启动榜单热度刷新调度(每日 04:00 拉豆瓣榜单 → hot_rank/hot_score)
	app.hotSvc.Start(rootCtx)

	srv := &http.Server{Addr: ":" + cfg.ServerPort, Handler: r}
	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	log.Printf("listening on :%s", cfg.ServerPort)

	select {
	case err := <-serveErr:
		log.Fatalf("server exited: %v", err)
	case <-rootCtx.Done():
		// 停机序列: 先让 HTTP 在途请求收尾, 再等采集协程退出。总预算 30s
		// (< compose stop_grace_period 40s), 超时部分交进程退出兜底 ——
		// 单条落库是事务, 被中断的页记失败台账由补采接管, 数据不会坏。
		log.Printf("收到退出信号, 优雅停机中(HTTP 收尾 + 采集任务收尾)...")
		deadline := time.Now().Add(30 * time.Second)
		shutdownCtx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
		if app.spiderSvc.WaitJobs(shutdownCtx) {
			log.Printf("采集任务已全部收尾")
		} else {
			log.Printf("采集任务收尾超时, 进程退出兜底(中断页已记失败台账)")
		}
		log.Printf("bye")
	}
}

// App 组合根容器。
type App struct {
	gdb         *gorm.DB
	cacheRdb    *redis.Client
	coordRdb    *redis.Client
	userSvc     *service.UserService
	spiderSvc   *service.SpiderService
	backdropSvc *service.BackdropService
	hotSvc      *service.HotService
	handlers    *handler.Handlers
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
	// 离线 IP 归属地库(可选): 数据文件缺失/损坏时仅记录, 在线明细归属地列显示空, 不影响主流程。
	if err := geoip.Init("data/ip2region.db"); err != nil {
		log.Printf("[warn] geoip init: %v (在线明细 IP 归属地不可用)", err)
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
	failureRepo := repomysql.NewCollectFailureRepository(gdb)
	bannerRepo := repomysql.NewBannerRepository(gdb)

	// services
	filmSvc := service.NewFilmService(searchRepo, movieRepo, playRepo, categoryRepo, healthRepo, sourceRepo, cfg.PageSizes)
	userSvc := service.NewUserService(userRepo, historyRepo, favoriteRepo, skipRepo, searchRepo, tokenMgr, cfg.JWT.MaxDevices, cfg.JWT.TTL)
	configSvc := service.NewConfigService(siteRepo, versionRepo)
	m3u8Svc := service.NewM3u8Service(cfg.JWT.PrivateKey)
	telemetrySvc := service.NewTelemetryService(telemetryRepo, searchRepo, favoriteRepo, cfg.TelemetryAllowedHosts, configSvc)
	// TMDB 客户端: key 运行期热替换 —— 管理后台(优先, 落 site_config)保存/清除即生效; env TMDB_API_KEY 为兜底。
	tmdbClient := tmdb.New(cfg.TMDB.APIKey, cfg.TMDB.Lang, cfg.TMDB.APIBase, cfg.TMDB.ImageBase)
	manageSvc := service.NewManageService(sourceRepo, cronRepo, siteRepo, versionRepo, fileRepo, categoryRepo, searchRepo, movieRepo, playRepo, healthRepo, tx, userSvc, blob, tmdbClient)
	engine := spider.NewEngine(movieRepo, searchRepo, playRepo, categoryRepo, fileRepo, tx, blob, failureRepo, cfg.Spider.MaxGoroutine)
	taskRunRepo := repomysql.NewTaskRunRepository(gdb)
	taskSvc := service.NewTaskRunService(taskRunRepo)
	spiderSvc := service.NewSpiderService(engine, sourceRepo, cronRepo, healthRepo, failureRepo, taskSvc)
	bannerSvc := service.NewBannerService(bannerRepo, filmSvc)

	// TMDB 横图回填 worker: 每轮解析 key(后台 DB > env), 未配置时轻量探测后静默等待。
	backdropSvc := service.NewBackdropService(movieRepo, searchRepo, bannerSvc, blob, tmdbClient, siteRepo, cfg.TMDB.APIKey)
	// 事件挂钩: 采集落库/轮播变更/后台改 key → 立即触发横图重算(20min 兜底扫描之外的主路径)。
	spiderSvc.OnSettled = backdropSvc.Kick
	bannerSvc.OnChange = backdropSvc.Kick
	manageSvc.OnTMDBKeyChange = backdropSvc.Kick

	// 榜单热度刷新: 每日 04:00 拉豆瓣 L1 榜单(8 集合 / 约 675 条 / 18 次请求),
	// 匹配本地影片后写 hot_rank / hot_rank_at / hot_score, 顺带回填 db_id 与 db_score。
	hotSvc := service.NewHotService(movieRepo, douban.New())

	handlers := &handler.Handlers{
		Film: filmSvc, User: userSvc, Config: configSvc, M3u8: m3u8Svc,
		Telemetry: telemetrySvc, Manage: manageSvc, Spider: spiderSvc, Banner: bannerSvc,
		Hot: hotSvc, Tasks: taskSvc, Online: service.NewOnlineService(cacheRdb),
		Blob: blob, ResetToken: cfg.Spider.ResetToken,
	}

	return &App{
		gdb: gdb, cacheRdb: cacheRdb, coordRdb: coordRdb,
		userSvc: userSvc, spiderSvc: spiderSvc, backdropSvc: backdropSvc, hotSvc: hotSvc,
		handlers: handlers,
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

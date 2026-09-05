# Jerocine — 项目协作说明 (CLAUDE.md)

在线观影站。前端 `web/`（Vue3 + Vite + TS + Pinia + UnoCSS，一套代码服务 desktop/mobile/tv，经 Capacitor 8 打成 Android APK），后端 `server/`（Go/Gin + GORM + go-redis），`deploy/` 为 Docker Compose 部署。

---

## 开发约定

- **API 契约真相**：`server/openapi/openapi.yaml`。前后端改动以契约为准。
- **较大改动必写单测**（后端 `go test ./...`，前端 vitest），合并前本地测绿。
- **前端门禁必须 `pnpm run build`**（= `vue-tsc -p tsconfig.app.json --noEmit && vite build`），不能只跑裸 `vue-tsc --noEmit`：`tsconfig.app.json` 开了 `noUncheckedIndexedAccess` 等严格项，裸 tsc 用宽松配置会漏报（`map[k]` 实为 `T|undefined`）。
- **方案/架构设计存档到 `docs/`**，不要只留对话里。
- 任何重启线上服务的操作（`docker compose build/up` 等）前先征维护者同意。重部署前若有采集（spider）任务在跑，先 `POST /api/v1/manage/spider/jobs/:id/pause` 记进度再动（重启 film_api 会杀采集 goroutine、丢在途进度）。

## 部署（Docker Compose）

- Docker 栈定义在 `deploy/docker-compose.yml`（service 名 `nginx`/`film`，不是 api）：
  - `film_nginx`：多阶段 `web/Dockerfile`（node:20-alpine 构建前端含 vue-tsc 门禁 → nginx:1.27-alpine 托管 + 反代 /api）。部署机无需装 node。
  - `film_api`：`deploy/Dockerfile`（golang:1.21-alpine 编译 → distroless **nonroot UID 65532**，无 shell）。
  - `film_mysql`、`film_redis`。
- **部署命令**：
  - 前端改动：`cd deploy && sudo docker compose build nginx && sudo docker compose up -d --no-deps nginx`
    （`--no-deps` 只重建 nginx、**不连带重启 film_api** → 不打断采集）
  - 后端改动：`... build film && ... up -d film`（**会重启 film_api → 必打断采集**；先 pause 采集）
  - ⚠️ nginx `depends_on: film`，**不加 `--no-deps`** 会连带重启 film_api；故纯前端改动务必带 `--no-deps`。
- **DB 迁移（golang-migrate）**：由 compose 独立一次性服务 `migrate`（只 `up`，`restart:no`）跑；`film` `depends_on: migrate(service_completed_successfully)` → `up -d film` 会**先跑完待应用迁移再起 film_api**。只单跑迁移不重启 api：`sudo docker compose run --rm migrate`。迁移文件 `server/migrations/000NNN_*.{up,down}.sql`。
- **后端 Go 编译/测试**（可用容器跑）：
  `docker run --rm -v "$PWD/server":/src -w /src golang:1.21-alpine sh -c "go build ./... && go test ./internal/..."`
- **致命坑 — 权限**：`deploy/secrets/*.pem`(JWT key) 和 `deploy/apk/` 必须属 **65532:65532**（distroless nonroot UID），否则 film_api 读不到 key / 写不了 APK 而崩。

## ⚠️ 部署后必清缓存（踩过坑）

后端 API 改动重建 film_api 后，接口**仍返回旧数据**——因为两层缓存：
1. **nginx proxy_cache**：`deploy/data/nginx/nginx.conf` 对 `/api` 开了 `proxy_cache api_cache; proxy_cache_valid 200 7d`。
   清：`sudo docker exec film_nginx sh -c "rm -rf /var/cache/nginx/api/* && nginx -s reload"`
2. **Redis 应用层缓存**：`v1:movie:detail:<mid>` 等。清需带密码：
   `sudo docker exec film_redis redis-cli -a "$REDIS_PASSWORD" --no-auth-warning --scan --pattern "v1:movie:detail:*" | xargs -r ...del`

## Android 工程版本控制

- `web/android/` 源码纳入 git。精细忽略：`build/`、`.gradle/`、`local.properties`、`assets/public`(cap 产物)、**`*.keystore`/`*.jks`(发布密钥严禁 commit)**。
- **APK 只能本地 `gradlew` 打**（构建机需 JDK17 + Android SDK）：`cd web && pnpm build:no-check && npx cap sync android && cd android && ./gradlew assembleDebug`。
- 原生播放器控件 = 自定义 Media3 布局 `res/layout/exo_player_control_view.xml`（进度条下方一排[图标+2字]按钮），由 `PlayerActivity.bindControlButtons()` 绑定。

## Android APK（Capacitor 壳）

- APK 以**远程加载**站点 URL 运行（不是本地 capacitor:// 资源），见 `web/android` 主 Activity 的 `loadUrl(...)`。因此：
  - **前端改动部署线上即对 APK 生效**（APK 清缓存重启即可），无需重打 APK。
  - **`window.Capacitor` 不会注入**（远程页）→ 壳在 pause/resume `eval("window.Capacitor.triggerEvent(...)")` 会报 undefined。已用 `src/utils/capacitorShim.ts` 垫片兜底；生产(release)包 `onConsoleMessage` 不再弹 Toast。
- 断网兜底在**原生层** `MainActivity`（`showOfflineOverlay`），Web 层兜底覆盖不到纯断网。
- 视频：APK 内有**原生 ExoPlayer 全屏播放器**（`PlayerActivity.java` + `JerocineBridge` + `jerocineNative.ts`），`isNative()` 时 `PlayView` 派发给原生、不渲染 video.js。
- **⚠️ 原生播放派发有 3 个入口，配置必须同步**（踩过：只 PlayView 带 `proxyBase`，另两条漏，致原生端侧过滤"代理地址未传"不触发）：
  1. `router/index.ts` beforeEach 守卫——**APK 上进 `/play` 会被它拦截**（拉 detail → 直接 `jerocine.playPlaylist` → 重定向 `/filmDetail`），**PlayView 在原生上根本不挂载**（其内的 playPlaylist 派发=原生死代码）。收藏/外链/历史进入走这里。
  2. `FilmDetailView.gotoPlay`——详情页"立即播放/继续观看"直跳原生，**不走 `/play` 路由**。
  3. `PlayView.applyCurrentEpisodeToPlayer`——仅 **web** 端有效（原生被 #1 拦截）。
  改 `jerocine.playPlaylist` 入参（`proxyBase`/`skipIntroSec`/`skipOutroSec`/历史 `record`）**必须 #1#2#3 同步**，否则不同入口行为不一致。`proxyBase` 统一用 `jerocineNative.ts` 的 `absApiBase()`。注：原生 **不读 `cfg.adFilter`**（用自身 `PREF_AD_FILTER`，默认开）。
- **端侧广告过滤（方案B，部分片源有服务器地域封锁）**：设备抓 m3u8 → POST `/api/v1/m3u8/filter`（服务端只过滤文本、不联网抓源）→ 设备直连分片播。原生 `FilterPlaylistParser` 对 **master 和子表都会触发**（Media3 每张表都解析）；web（`PlayView.clientSideFilter`）需**自己跟随 master→子表逐层过滤**（否则子表被播放器直连绕过）。后端 `/m3u8/filter` 有排查日志：`docker logs film_api | grep "m3u8 filter"`。
- 本地联调：APK「连按4次返回→改服务器地址」可填 `http://<电脑IP>:3600` 连本地 dev（`JEROCINE_DEV_PROXY=<后端站点> pnpm dev --host`）。

## TV / WebView 已知约束

- TV 焦点环是 `box-shadow`/`outline`，会被祖先 `overflow:hidden/auto/clip` 上下裁切；横滚行/tab 条需留纵向 padding 或用 outline。
- TV 下 `overflow-x:hidden` 会被 CSS 规范强制把另一轴 `overflow-y` 变 `auto` → 造成嵌套滚动容器、Router `scrollBehavior{top:0}` 滚错对象。统一用 `overflow-x:clip`。
- TV 基准字号用 `clamp(vw)` 跟随分辨率（真机 WebView CSS 视口因 dpr 常被压到 ~960）。
- **`overflow:hidden` 会把元素变成滚动容器**（即便无滚动条）→ 遥控器聚焦其内元素时浏览器 `scrollIntoView` 会**滚动该容器**，导致这块（如详情页 hero 海报+文字，因 `inset:-40px` 模糊背景使可滚区大于可视框）整体偏移。要裁溢出又不想被聚焦滚动，用 **`overflow:clip`**。
- **空间导航**（`useSpatialNavigation.ts` `findNearest`）：方向键策略=**「最近一行/列优先（主轴 band 内归一排），排内再按副轴对齐」**。曾用"主轴+0.5×副轴"打分，致"上一行只有偏侧按钮时被更远的对齐行抢走、上一行被跳过"。

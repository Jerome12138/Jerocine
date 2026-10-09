# Jerocine — 项目协作说明 (CLAUDE.md)

在线观影站。前端 `web/`（Vue3 + Vite + TS + Pinia + UnoCSS，一套代码服务 desktop/mobile/tv，经 Capacitor 8 打成 Android APK），后端 `server/`（Go/Gin + GORM + go-redis），`deploy/` 为 Docker Compose 部署。

## ⚠️ 安卓播放器架构事实（agent 先读，勿混淆）

- **web 壳 APK 与 TV 版使用同一个原生安卓播放器**：`tv/player-core` 的 `PlayerActivity`（ExoPlayer/Media3 系）。
  **不是网页播放器**。前端 `web/src/views/public/PlayView.vue`（video.js）**只在纯 web 端渲染**。
- 壳（`web/android`）内 `/play` 路由被 router 守卫拦截 → 直接调 `jerocine.playPlaylist` 派发**原生播放器**；
  `PlayView` 在原生上**根本不挂载**。因此前端样式/行为改动对原生播放器**无效**。
- **安卓端的一切播放器 UI（控制栏/弹窗/抽屉/过滤提示/下载面板）都是原生实现**（player-core），
  桌面 web 才用 SPA 里的网页播放器组件。做播放器相关功能时**先在 `tv/player-core` 里找**，别在 web 前端里找。
- 详细机制（派发入口/广告过滤阶梯/预取）见下文「Android APK（Capacitor 壳）」小节。

## 仓库边界（先判断再落笔）

本仓库是**公开代码仓**，只放两类内容：

1. **代码**：`server/` `web/` `tv/` `deploy/`；
2. **可公开的通用文档**：部署指南、架构蓝图、README、OpenAPI（放 `docs/`）。

**不要放进来**：个人笔记、工作日志、服务器运维档案/快照、内网地址或端口拓扑、账号与环境细节、任务跟踪——这些属于维护者的**私有开发仓库**（Harness），与本仓分离维护。若发现本仓出现此类内容，提醒维护者移走。拿不准一个文件是否可公开时：默认不放本仓。

---

## 开发约定

- **API 契约真相**：`server/openapi/openapi.yaml`。前后端改动以契约为准。
- **本地 web dev 连后端**：默认 `/api` 走 Vite 代理 → `127.0.0.1:3601`（本地 Go 后端）。要连**线上后端**必须用 `JEROCINE_DEV_PROXY=https://jerocine.art pnpm dev`（vite.config.ts 内置的代理开关），**不要**用 `.env.local` + `VITE_API_BASE`（dev 下不生效，请求仍走代理，本地后端没起时页面接口全是 500）。
- **较大改动必写单测**（后端 `go test ./...`，前端 vitest），合并前本地测绿。
- **前端门禁必须 `pnpm run build`**（= `vue-tsc -p tsconfig.app.json --noEmit && vite build`），不能只跑裸 `vue-tsc --noEmit`：`tsconfig.app.json` 开了 `noUncheckedIndexedAccess` 等严格项，裸 tsc 用宽松配置会漏报（`map[k]` 实为 `T|undefined`）。
- **方案/架构设计存档到 `docs/`**，不要只留对话里。
- 任何重启线上服务的操作（`docker compose build/up` 等）前先征维护者同意。**采集无需手动暂停**：server 有 SIGTERM 优雅停机（HTTP 收尾 → 等采集协程退出，中断页记失败台账由补采/增量窗口自愈）。

## 构建脚本清单（新增或改动务必登记到这里）

> **约定**：任何新增/修改的构建、打包、发布脚本，都要在本节留一条（位置、用法、前置条件、产物去哪）。
> 目的是让后来的人（和 AI）不用读脚本源码就知道有什么、怎么用。

| 脚本 | 作用 | 前置 | 产物 |
|---|---|---|---|
| `scripts/build-android.sh` | 打安卓包，目标 `web`(Capacitor 壳) / `native`(原生 TV) / `all` | JDK 17+、Android SDK、**正式签名密钥**（仓库外，见下） | 各工程 `app/build/outputs/apk/<variant>/`，并复制一份到系统下载目录 |

`scripts/build-android.sh` 用法：

```bash
scripts/build-android.sh all                                   # 两个包都打
scripts/build-android.sh tv --api-base=https://你的域名/        # 指定 TV 的后端地址(编译期常量)；tv 与 native 等价
scripts/build-android.sh web --debug                           # debug 试用包，跳过密钥校验
scripts/build-android.sh web --keep-version                    # 故意重打同一个构建号（会给重复发版警告）
scripts/build-android.sh all -- -PwebVersionCode=1042          # `--` 之后原样透传给 gradle
```

- 产物命名：`Jerocine-TV-{web,native}-v<版本名>(<构建号>).apk`，debug 追加 `-debug`。
  例：`Jerocine-TV-web-v1.0.9(1041).apk`。构建号取 APK 产物目录 `output-metadata.json` 里的
  `versionCode`（gradle 生成的真实值），读不到才退回版本源文件。
- **打包后自动装 pad**：交付成功后 `maybe_adb_install` 检测 `adb devices`，有设备就对最新 web/native
  包各执行一次 `install -r`（成功打日志，无设备静默跳过）。注意 adb.exe 不认 `/d/...` 风格路径，
  脚本内已做 `cygpath -m` 转换 —— 手工装包时同理。
- **产物复制到哪 —— 系统真实「下载」目录**，解析顺序：① `JEROCINE_DOWNLOAD_DIR`（显式指定，最优先）；
  ② Windows 读注册表 `Shell Folders\{374DE290-123F-4565-9164-39C4925E467B}`（支持用户把"下载"位置
  改到别的盘，如 `D:\Downloads`）；③ macOS/Linux 走 `xdg-user-dir DOWNLOAD` / `~/Downloads`。
  **⚠️ `reg.exe` 可能被「命令安全 → 程序黑名单」拦住**（进程起不来，报 `PROGRAM BLOCKED BY SECURITY POLICY`；
  该拦截**不可批准也不可绕过**，只能在安全中心里移除，且**与"沙箱隔离/完全权限"无关** —— 别混为一谈）。
  而脚本里这步带 `2>/dev/null ... || true`，**静默无输出** → 会悄悄回退到 `C:\Users\<user>\Downloads`，
  包就落到了"错"的目录（用户会以为没出包）。
  这种情况下显式传 `JEROCINE_DOWNLOAD_DIR=/d/Downloads`（Harness 的 `build-android-local.sh` 已按本机注入）。
- **版本号唯一来源：`scripts/android-versions.properties`**（构建号统一千位编号 = 1000 + 迭代号）。
  `web/android/app/build.gradle`、`tv/app/build.gradle.kts`、`scripts/build-android.sh` 都读它，
  **发新版只改这一个文件**，别在 `build.gradle` 里写死数字（否则产物名会和包内元数据对不上）。
  单次覆盖：`-PwebVersionCode=1042 -PwebVersionName=1.1.0` / `-PnativeVersionCode=1002`。
- **⚠️ 每次正式打包 versionCode 必须 +1**（versionName 可以不动；Android 只接受更大的
  versionCode，同号包装不上已装旧包的设备，也可能被分发渠道判为重复版本）。
  **这一步不用靠人记——脚本有闸门**：正式构建时脚本把 `web./native.versionCode` 与文件里的
  `served.*.versionCode` 水位线（= 该 kind 已出过正式包的最大构建号）比较：
  - 当前号 **≤ 水位线** → 这一版已经发过包了 → **自动 +1、回写文件**，然后才构建；
  - 当前号 **> 水位线** → 手动改大了 → 尊重你的值，脚本不动（想提前占号直接改大即可）。
  交付成功后水位线才推进到本次实际构建号，**构建失败不推进**，所以重试仍是同一个号、不白烧号。
  `--keep-version` 关掉自动递增（重发同一版用）；`--` 之后用 `-P<kind>VersionCode=` 显式指定时
  也跳过递增。脚本自动改过版本源文件时会在结尾提醒提交它。
- **web 包的构建顺序是强制的**：`vue-tsc → vite build → cap sync android → gradlew assemble`。脚本已内建
  两道保险，别为"顺手简化"改掉：
  ① `cap sync` 必须重定向 stdin（`< /dev/null`）：**stdin 不是 TTY 时（后台/脚本/CI/agent 里都是）
  它会在拷贝完成后去读 stdin 并一直阻塞**，全程无输出、看着像卡死（实测 25s 仍在挂；加了重定向 18.5s 正常结束）；
  ② 出包前断言 `web/dist` 的每个文件都存在于 `web/android/app/src/main/assets/public`
  —— `cap sync` 会先把该目录（约 181 文件）删空再重灌，中途被打断就留下残缺前端，
  那种包**编译照过、装上却是白屏**，日志里没有任何异常。手工兜底时同样要加 `< /dev/null`。
  脚本对 `cap sync` 这步另加超时兜底（默认 300s，`JEROCINE_CAP_SYNC_TIMEOUT` 可调）：Linux 用 `timeout`，
  macOS 无 `timeout` 时自动改用 `gtimeout`，两者都没有就只告警并照常执行（少一层兜底，不影响正确性）。
- **release 必须用正式密钥**，脚本找不到就报错退出（不静默降级成 debug 签名）。
  密钥不入库，也**刻意不从工程目录里读**——必须显式指定：目录里放好 `keystore.properties`
  （键名 `storePassword` / `keyAlias` / `keyPassword`）后用 `JEROCINE_KEY_DIR=<目录>` 指过去，
  或用 `JEROCINE_KEYSTORE` / `JEROCINE_STORE_PASSWORD` / `JEROCINE_KEY_ALIAS` / `JEROCINE_KEY_PASSWORD`
  逐项给。放仓库里靠 `.gitignore` 挡是单点防线（漏一条规则就泄漏），显式指定还能保证
  「找不到密钥」时永远报错，不会悄悄采用了别处遗留的一份。
- 两个包**共用同一密钥**（证书 SHA-256 `0754fe8d…295b`），靠 applicationId 区分：
  `art.jerocine.app`(web 壳) / `art.jerocine.tv`(原生 TV)，可并存安装、互不覆盖。
- 跨平台 macOS / Linux / **Windows 需 Git Bash**（PowerShell/cmd 跑不了 `.sh`）。

## 部署（Docker Compose）

- **从零部署 / 新机器上线**：先读 [`docs/部署指南.md`](./docs/部署指南.md)（前置条件、`.env` 配置清单、部署步骤、验证与排障）；本节只讲日常运维。
- Docker 栈定义在 `deploy/docker-compose.yml`（service 名 `nginx`/`server`）：
  - `jerocine_nginx`：多阶段 `web/Dockerfile`（node:20-alpine 构建前端含 vue-tsc 门禁 → nginx:1.27-alpine 托管 + 反代 /api）。部署机无需装 node。
  - `jerocine_server`：`deploy/Dockerfile`（golang:1.27-alpine 编译 → distroless **nonroot UID 65532**，无 shell）。
  - `jerocine_mysql`、`jerocine_redis`。
- **部署命令**：日常用一键脚本（pull → build+up → 等健康 → 自动清两层缓存）：
  - `cd deploy && ./deploy.sh`            # 常规：server + nginx
  - `./deploy.sh nginx`                   # 纯前端：只重建 nginx，不碰 server/采集
  - `./deploy.sh server`                  # 只更新后端
  - 脚本对 docker 无权限时自动回退 `sudo -n docker`；采集**无需手动暂停**（优雅停机自愈）。
  - 等价手工形式（排障用）：`git pull && cd deploy && sudo docker compose --env-file .env up -d --build server nginx`，
    部署后手动清缓存见下节。
  - 手工纯前端（绕过脚本）用 `up -d --build --no-deps nginx`：nginx `depends_on: server`，不带 `--no-deps` 时 `--build` 会顺带重建 server 镜像并跑一次 migrate（实测 compose v5.5.1 **不会** recreate 已运行的 server 容器、采集不中断，但多花一次后端构建）；带 `--no-deps` 只动 nginx。
- **DB 迁移（golang-migrate）**：由 compose 独立一次性服务 `migrate`（只 `up`，`restart:no`）跑；`server` `depends_on: migrate(service_completed_successfully)` → `up -d server` 会**先跑完待应用迁移再起 jerocine_server**。只单跑迁移不重启 api：`sudo docker compose run --rm migrate`。迁移文件 `server/migrations/000NNN_*.{up,down}.sql`。
- **后端 Go 编译/测试**（可用容器跑）：
  `docker run --rm -v "$PWD/server":/src -w /src golang:1.27-alpine sh -c "go build ./... && go test ./internal/..."`
- **致命坑 — 权限**：`deploy/secrets/*.pem`(JWT key) 和 `deploy/apk/` 必须属 **65532:65532**（distroless nonroot UID），否则 jerocine_server 读不到 key / 写不了 APK 而崩。

## ⚠️ 部署后必清缓存（踩过坑；`deploy.sh` 已内置自动化）

后端 API 改动重建 jerocine_server 后，接口**仍返回旧数据**——因为两层缓存（`./deploy.sh` 每次部署后自动清，无需手工）：
1. **nginx proxy_cache**：`deploy/data/nginx/nginx.conf` 对 `/api` 开了 `proxy_cache api_cache; proxy_cache_valid 200 7d`。
   手工清：`sudo docker exec jerocine_nginx sh -c "rm -rf /var/cache/nginx/api/* && nginx -s reload"`
2. **Redis 应用层缓存**：`v1:movie:detail:<mid>` 等。手工清（`-a` 参数在 exec 链路易 NOAUTH，用 REDISCLI_AUTH）：
   `sudo docker exec -e REDISCLI_AUTH="$REDIS_PASSWORD" jerocine_redis redis-cli --no-auth-warning -n 0 FLUSHDB`

## Android 工程版本控制

- `web/android/` 源码纳入 git。精细忽略：`build/`、`.gradle/`、`local.properties`、`assets/public`(cap 产物)、**`*.keystore`/`*.jks`(发布密钥严禁 commit)**。
- **构建水位线回写（`scripts/android-versions.properties`）不单独提交**：跟近期功能改动一起提交即可（amend 进当前功能提交，或随下一个提交）—— 单独的水位线提交只会刷屏提交历史。
- **APK 构建**：见上文「构建脚本清单」的 `scripts/build-android.sh`（已封装 `pnpm build` + `cap sync android` + `gradlew`，并强制校验正式签名）。手工兜底：`cd web && pnpm build:no-check && npx cap sync android < /dev/null && cd android && ./gradlew assembleDebug`（debug 无需密钥；`< /dev/null` 不能省，见上文 stdin 坑）。
- **原生播放器只有一个实现：`tv/player-core`**（Java 库，被 `tv` 与 `web/android` 两个工程各自 include，两壳不再自带播放器副本）。
  - 自定义 Media3 控件布局 `tv/player-core/src/main/res/layout/exo_player_control_view.xml`（进度条下方一排[图标+2字]按钮），按钮绑定在 `PlayerDialogHelper.bindControlButtons()`，**不在** `PlayerActivity`。
  - 该模块**没有 AndroidManifest.xml**：`com.jerocine.player.PlayerActivity` 必须由各壳清单声明（两端都已声明），传参统一走 `PlayerActivity.EXTRA_*` 常量，别写字面量字符串。
  - **播放器资源只在 player-core 定义一份**：壳里出现同名 drawable/color/style 会**覆盖库资源**（改了 core 不生效），要改样式改 core，别在壳里复制副本。
  - **播放器内的可见文案以"前端 web 播放器"为唯一基准**（`web/src/views/public/PlayView.vue`，video.js 那套；**不是** `web/android` 壳）。广告过滤角标即五态：`过滤未开启 / 该源无需过滤 / 服务端过滤中 / 未检出广告 / 已过滤 N 段广告`，文案与色调由纯逻辑 `AdFilterStatus` 给出（+2 个 Android 特有态 `过滤未生效 / 过滤失败`），壳层只做 `Tone → drawable` 映射（`jc_badge_dot_{ok,idle,busy}`）。**过滤关掉时不隐藏**（显示"过滤未开启"），只有尚无片源才隐藏 —— 历史上的"开了才显示"会让用户以为功能不存在。改文案两端同步，另有单测锁定优先级。
  - **遥控器按键约定**：`MENU/INFO` = **唤出/收起上下操作栏**（不再弹"播放控制"弹窗 —— 原弹窗里的倍速/选集/换源/过滤/跳过/退出底栏都有按钮，弹窗已删除）；`BACK` = 面板开着先收面板，否则 2s 内双击退出；面板隐藏时 ←→=快进退、↑↓=上/下一集（需再按一次确认）、OK=播/暂。
  - **底栏按钮即全部播放控制**：上集 / 下集 / 选集 / 倍速 / 换源 / **中转** / **过滤** / 跳过 / 退出。加按钮改 `exo_player_control_view.xml` + `PlayerDialogHelper.bindControlButtons`，新增状态往 `PlayerSession.Host` 加渲染出口（别让 helper 直接摸 View）。
  - **两个开关类按钮（中转 / 过滤）的显示约定**：文案**恒定不变**（就叫「中转」「过滤」，不写"切到X"、不写"过滤 开/关"），文字**不随开关变色**（`jc_ctl_btn_text.xml` 只保留 focused/pressed 分支）；开/关状态一律由**左上角状态点**表达（绿=开 `jc_status_dot_on`、灰=关 `jc_status_dot_off`）。实现上这两个按钮各套一层 `FrameLayout`、点作为兄弟 View 叠在 `top|start` —— 不能用 compound drawable 拼点（`JcPlayerCtlBtn` 的 `drawableTint` 会把整块染成单色、绿点一起被染掉）。**改这两个按钮的文案/配色前先看这条**。
  - **线路（直连 / 中转）**：默认 **关** = 不包代理 —— 清单由设备自己抓 + 端侧混合过滤（见下条"广告过滤"）；开关态持久化在 `PlayerNetworkModeHelper`（prefs `network_relay_enabled`）。开 = 清单包 `/m3u8/proxy` 且 `proxyMedia=1`，分片也经服务器转发，可绕开直连受限的源、代价是更耗带宽（服务端抓不到的源无法中转，`canSwitchNetworkMode` 直接拦下）。**自愈与开关分属两套状态，别合并**：直连失败（CDN 地域封锁/证书链异常）→ 自动把**那一集**标进 `forceRelayIdx`（`PlayerSession` 里的单集自愈，不落盘、不动开关、不改变状态点），中转再失败则标 `forceRawIdx` 回退原始地址（保能播、牺牲过滤）；`isRelay(idx)` = 用户开关 `relayOn` **或** 本集自愈标记，取反仍优先 `forceRawIdx`。顶部曾有常驻 `network_mode_badge`（直连/中转），**已删**（状态统一由底栏按钮的状态点表达）。

## Android APK（Capacitor 壳）

- APK 以**远程加载**站点 URL 运行（不是本地 capacitor:// 资源），见 `web/android` 主 Activity 的 `loadUrl(...)`。因此：
  - **前端改动部署线上即对 APK 生效**（APK 清缓存重启即可），无需重打 APK。
  - **`window.Capacitor` 不会注入**（远程页）→ 壳在 pause/resume `eval("window.Capacitor.triggerEvent(...)")` 会报 undefined。已用 `src/utils/capacitorShim.ts` 垫片兜底；生产(release)包 `onConsoleMessage` 不再弹 Toast。
- 断网兜底在**原生层** `MainActivity`（`showOfflineOverlay`），Web 层兜底覆盖不到纯断网。
- 视频：APK 内有**原生 ExoPlayer 全屏播放器**（`tv/player-core` 的 `PlayerActivity` + 壳侧 `JerocineBridge` + 前端 `jerocineNative.ts`），`isNative()` 时 `PlayView` 派发给原生、不渲染 video.js。
- **⚠️ 原生播放派发有 3 个入口，配置必须同步**（踩过：只 PlayView 带 `proxyBase`，另两条漏，致原生端侧过滤"代理地址未传"不触发）：
  1. `router/index.ts` beforeEach 守卫——**APK 上进 `/play` 会被它拦截**（拉 detail → 直接 `jerocine.playPlaylist` → 重定向 `/filmDetail`），**PlayView 在原生上根本不挂载**（其内的 playPlaylist 派发=原生死代码）。收藏/外链/历史进入走这里。
  2. `FilmDetailView.gotoPlay`——详情页"立即播放/继续观看"直跳原生，**不走 `/play` 路由**。
  3. `PlayView.applyCurrentEpisodeToPlayer`——仅 **web** 端有效（原生被 #1 拦截）。
  改 `jerocine.playPlaylist` 入参（`proxyBase`/`skipIntroSec`/`skipOutroSec`/历史 `record`）**必须 #1#2#3 同步**，否则不同入口行为不一致。`proxyBase` 统一用 `jerocineNative.ts` 的 `absApiBase()`。注：原生 **不读 `cfg.adFilter`**（用自身 `PREF_AD_FILTER`，默认开）。
- **广告过滤的两条链路 + 选路阶梯（2026-09-24 重定为「端侧优先」，与 web 播放页一致）**：
  ① **端侧混合过滤（默认 / 主路径）**—— 设备自己抓 m3u8 → POST `/api/v1/m3u8/filter`（服务端只过滤文本、不联网抓源）→ 设备直连分片播；
  ② **服务端清单代理（降级）**—— 清单包成 `/api/v1/m3u8/proxy?src=…`，服务器替设备抓源并过滤。
  **选路阶梯**（`PlayerUrls.buildPlayableUrl`，过滤开 + m3u8）：默认**不包代理**（走 ①，与 `PlayView.resolvePlaySrc` 同策，即"端侧混合过滤优先"）；本集端侧过滤失败 → `forceProxyIdx` 升级为 ②（`PlayerAdFilterHelper.escalateToProxy`）；用户开「中转」或本集直连失败自愈 → ② 且 `proxyMedia=1`；② 也失败 → `forceRawIdx` 回退原始地址（保能放、牺牲过滤）。优先级 `forceRawIdx` > `forceProxyIdx`/中转。
  **`proxyUsable`（来自 play 接口的 `adFilterOk`）是硬闸**：显式 `false` ⇒ ①②都不可用（服务端抓不到，包了必 500），只能原始地址 + 端侧过滤，**同时禁用中转**（`canSwitchNetworkMode` 拦下并提示"服务端抓不到该片源"）；`undefined`（未测 / 老壳没传）按"可用"处理。透传链：web `utils/nativePlay.ts` 的 `mapped`（值为 `undefined` 时 JSON 自动省略该键）→ `sources_json` → `PlayerSourceHelper.parseSources`（必须用 `isNull` 区分"未测"与"false"）；TV 壳走 `Models.kt` 的 `PlaySource.adFilterOk` + `NativePlayerLauncher.kt` 的 `NativeSource`。
  **为什么必须端侧优先**：源站按 IP 拒机房 —— 实测同一 URL 服务端 500（bf=404、U酷=403、**360=连接超时 11~15s**）而设备侧 200；**能回源的源也不快**（0.6~9s，速播 8.1s / 新浪 9.0s）。旧"服务端代理优先"等于每次起播白等一跳，且服务端抓不到的源**永远过滤不了**（先撞必挂的代理再回退直连），用户看到的就是"清单代理失败, 已切换直连"+ 起播奇慢。
  `sourcePreferProxy`（本片源端侧过滤"坏过"的粘性偏好）：某源的端侧 POST 失败过一次后，本片**后续集直接走代理**，避免每集都白等一轮 12s 超时；换片/换源（`startFromIntent`）时复位。
  **web 层一律把原始地址交给原生**（`PlayView` 不再预包装代理）：包不包代理由原生按上面的阶梯 + `adFilterOk` 自己决定；预包装会让原生看不到 `adFilterOk` 而白撞一次代理。
  原生 `FilterPlaylistParser` 对 **master 和子表都会触发**（Media3 每张表都解析）；web（`PlayView.clientSideFilter`）需**自己跟随 master→子表逐层过滤**（否则子表被播放器直连绕过）。`adStatsClient` 超时 12s：媒体表可达 900+ 段（实测单张 25KB 上行 / 77KB 下行、服务端纯过滤就要 3s+），8s 会把"其实能过滤"误判成"过滤失败"。后端 `/m3u8/filter` 有排查日志：`docker logs jerocine_server | grep "m3u8 filter"`。
  **端侧预取 `PlayerPrefetchHelper`（2026-09-24）**—— 端侧优先的隐性成本是**每集要过滤两次**（master + 子表：服务端 `FilterText` 以 `proxyChild=false` 调用，只绝对化、**不改写**子表地址 ⇒ ExoPlayer 仍去抓原始 CDN 子表再送一次 filter），单次 1.4s（小清单）~5.1s（979 段）⇒ 切集白等 3~10s。故**进入片尾窗口时**在后台单线程（`jc-prefetch`）**预取下一集**：GET 原始清单（UA 与 `buildCacheFactory` 一致，源站可能按 UA 给不同表）→ POST `/v1/m3u8/filter` → 写 `PlayerSession.prefetched`（缓存放 Session 是因为 `PlayerPrefetchHelper` 写、`PlayerAdFilterHelper` 读，两 helper 不能互相引用），并**下钻一层把子表一并预热**（`PlayerUrls.childPlaylistUrls`，≤3 个；认 `#EXT-X-STREAM-INF` 后继行 + `#EXT-X-MEDIA` 的 `URI=`，只收 m3u8；子表与 master 同批重做，保证整棵树同一时间基准）。`FilterPlaylistParser` 每张表先 `takePrefetched(uri)`，命中直接用（跳过 POST，但**仍会更新 `filterAttempted`/`pendingFilteredCount`**）。
  只预取"下一集确实走端侧过滤"的情形（`canPrefetch`：过滤开 + 非 bypass 模式 + 有 proxyBase + `mediaUriFor(next,raw)==raw`）—— 走代理/中转的由服务端自己缓存。**触发只有一处**：`progressTick`（5s）里 `remaining <= skipOutroMs + 60s` → `schedule()`，与 web 的 `remaining <= outro + PREFETCH_LEAD_S(60)` 同条件；`schedule()` 内用"结果年龄 < 60s 就返回"节流，所以调用方可以 5s 无脑调一次。**刻意不做"起播就预取下一集"**（用户 2026-09-25 拍板）：预取 TTL 只有 10min，而一集 30~45min ⇒ 起播那份到切集时必然过期、对自动连播是白做，唯一能覆盖的"10min 内手动按下一集"窗口太窄，不值当每集多烧一次服务端过滤（3~5s CPU + 25KB 上行）；宁可只保留"切集前那一刻一定新鲜"的这一路。失效：`loadPlaylistIntoPlayer` 里 `invalidatePrefetch()`（换片/换源/开关切换；**切集不走装载路径**，所以缓存能活过切集）+ `escalateToProxy`（本片源改走代理后端侧预取全无意义）。另有 TTL 10min、上限 8 条、`prefetchGen` 代际（在途任务换片后写回被丢弃）、`onDestroy` `shutdown()`。预取失败静默 —— 切集时按原路径再付一次全价，不改变行为。**web 播放页那套预取（`PlayView.prefetchNextEpisode`）预热的是服务端 proxy 缓存，端侧优先路径下没人查 ⇒ 对浏览器/APK 均零收益**（APK 内 video.js 从不渲染，更用不上），要受益得同样改成"预取过滤后文本"。
- 本地联调：APK「连按4次返回→改服务器地址」可填 `http://<电脑IP>:3600` 连本地 dev（`JEROCINE_DEV_PROXY=<后端站点> pnpm dev --host`）。

## 播放器弹窗 / 控制面板铁律（player-core，2026-10-09）

设计文档：`docs/播放器通用弹窗组件设计-JcDialog-2026-10-09.md`（在 Harness 私有仓 docs/）。

- **新增弹窗一律走 `com.jerocine.player.ui.JcDialog`**（`list` / `message` / `custom` 三入口同一个 builder），**禁止再手写 `AlertDialog.Builder`**。`checkable(bool)` 控制左侧圆圈有无；条目 `JcDialogItem.tag` 有无控制右侧灰标显隐（合并了单选/带tag单选/普通列表三种形态）；按钮可选、文案可传参（默认「确定/取消」）；80% 屏高上限组件自管。
- **版式 token 唯一定义处** = `jc_dialog_frame.xml` / `jc_dialog_row.xml` / `JcDialogBtn`（标题 17sp bold + padTop/padBottom 16dp、主文字与按钮 15sp、tag 13sp、行高 36dp、圆圈 16dp、无按钮时内容区补 16dp 底距）。改观感只动这三处，**业务代码禁止散写字号/间距**。
- 自定义 View 内容（如跳过设置面板）想与标题左对齐，用 `JcDialog.contentPaddingX(ctx)`（解析 dialogPreferredPadding），别硬编码 24dp。
- 控制面板自动收起策略 = `PlayerAutoHidePolicy`（**暂停/缓冲中不主动消失**，播放中 3s 自动收），由 `PlayerActivity.updateControllerAutoHide()` 落地。别绕开它直接 `setControllerShowTimeoutMs`。
- ⚠️ **主题 style 禁止挂无前缀 appcompat attr**（`listChoiceIndicatorSingle` / `listPreferredItemHeightSmall` 等）：库单独编译不报错，壳层 `:app` 资源链接时才炸 `attr not found`（实测 1081 打包失败）。圆圈/行高覆盖一律用 `android:` 前缀属性。

## TV / WebView 已知约束

- TV 焦点环是 `box-shadow`/`outline`，会被祖先 `overflow:hidden/auto/clip` 上下裁切；横滚行/tab 条需留纵向 padding 或用 outline。
- TV 下 `overflow-x:hidden` 会被 CSS 规范强制把另一轴 `overflow-y` 变 `auto` → 造成嵌套滚动容器、Router `scrollBehavior{top:0}` 滚错对象。统一用 `overflow-x:clip`。
- **TV 字号阶梯的唯一定义处 = `web/src/assets/styles/theme.css` 里的 `[data-mode="tv"]` 块**（语义约定 `hero/3xl/2xl/xl/lg/md/base/sm/xs/badge` 就写在块内注释里；新增样式对号入座，别在组件里硬编码字号 —— 海报角标统一用 `--jc-fs-badge`）。真机 WebView CSS 视口因 dpr 常被压到 ~960 ⇒ **CSS 值 ×2 ＝ 物理像素**，10-foot 可读下限约 24 物理 px。2026-09-23 按用户反馈把整条阶梯**降了一档**（原值偏大：区块标题 28px 级、角标 14px 级）。
- **`overflow:hidden` 会把元素变成滚动容器**（即便无滚动条）→ 遥控器聚焦其内元素时浏览器 `scrollIntoView` 会**滚动该容器**，导致这块（如详情页 hero 海报+文字，因 `inset:-40px` 模糊背景使可滚区大于可视框）整体偏移。要裁溢出又不想被聚焦滚动，用 **`overflow:clip`**。
- **空间导航**（`useSpatialNavigation.ts` `findNearest`）：方向键策略=**「最近一行/列优先（主轴 band 内归一排），排内再按副轴对齐」**。曾用"主轴+0.5×副轴"打分，致"上一行只有偏侧按钮时被更远的对齐行抢走、上一行被跳过"。

# Jerocine影视 — 原生 Android TV 客户端

> ⚠️ **2026-10-15 结构统一**：本模块已从 `tv/app` 上移为顶层 `android/` 工程里的 **`android/tvshell`**
> （旧 `tv/` 独立工程已删除）。下文提到的 `tv/app` 路径一律对应现在的 `android/tvshell`。
>
> ⚠️ **2026-10-11 单包合并**：本模块不再是独立 APK，而是宿主 `android/app` 里的 **`:tvshell` library**，
> 与 Capacitor 壳（`art.jerocine.app`）打进**同一个包**，由 `EntryRouterActivity` 按设备能力路由
> （低配/WebView 内核过旧 → 进本模块的 `NativeShellActivity`）。因此：
> - **不再有 `build-android.sh tv` 目标**（传它会明确报错）；打单一 APK 用 `build-android.sh all`（或 `web`，等价）。
> - 本模块**无自己的 applicationId / versionCode / versionName**，版本号跟宿主走 `scripts/android-versions.properties` 的 `web.*`。
> - 下文「技术栈 / 功能 / 后端」描述对本模块内部仍成立；**构建与签名一节已按单包口径重写**。
>
> 本文其余部分保留合并前语境（历史说明），如需权威现状以仓库根 `CLAUDE.md` 为准。

为覆盖**坚果 Nano（Android 6.0 / API 23 / Chromium 48）这类老投影**而做的原生壳。
Capacitor(WebView) 方案在该设备上跑不动（WebView 内核 48 远低于前端要求），故采用原生实现；
合并后它作为宿主内的 `:tvshell` library 被 `EntryRouterActivity` 按设备能力路由进来。

> ✅ 单包 minSdk 已降到 **21**（2026-10-12；对 Capacitor/cordova 依赖用 `tools:overrideLibrary` 放行），
> 所以 API 21–23 的老设备**装得上**这个包，启动后由 `DeviceProbe` 判为低配 → 直接进本原生壳
> （那条路径上 Capacitor 的类从不加载）。这正是"同一个包、低配也能安装并自动走原生"的设计要点。

## 技术栈

- Kotlin + **Android View/XML + RecyclerView**（针对低配电视减少渲染开销）
- **Media3 (ExoPlayer)** + SurfaceView（解码帧直贴屏，弱机最流畅）
- Retrofit + kotlinx-serialization（无反射，适合弱机）
- **minSdk = 21**（与宿主同值；合并后的 APK 只有一条 `uses-sdk minSdkVersion=21`）；
  `targetSdk` 对库无意义，已删。
- 复用现有 Go 后端 API，仅新增设备码登录 3 接口

## 功能

- 首页（分类海报墙）→ 详情 → 播放（断点/剧集/多播放源）
- 搜索、分类浏览
- 设备码登录（TV 出码，手机端确认）、观看历史、收藏
- 播放失败自动切换下一个播放源

## 构建

本模块**不再单独出包**。打单一 APK（含本壳）在仓库根执行：

```bash
scripts/build-android.sh all      # 单包 art.jerocine.app（Capacitor 壳 + 本原生壳）
scripts/build-android.sh web      # 等价于 all（web 作兼容别名保留）
```

Windows 用 Git Bash 执行 `bash scripts/build-android.sh all ...`（PowerShell / cmd 跑不了 .sh）。

需要 JDK 17+（AGP 8.x 要求，实测 17 可用）与 Android SDK；脚本会自动探测 `ANDROID_HOME`、
`~/Library/Android/sdk`、`~/Android/Sdk` 并补写 `local.properties`。

**签名**：release 包必须用正式密钥（不接受 debug 签名兜底，也不静默降级）。密钥不入库、
也**不从工程目录里读**：把 `keystore.properties`（键名 `storePassword` / `keyAlias` /
`keyPassword`）和密钥文件放在仓库外的任意目录，构建时用 `JEROCINE_KEY_DIR=<目录>` 指过去
（或用 `JEROCINE_KEYSTORE` 等环境变量逐项给）。缺密钥时脚本直接报错退出，
并打印当前解析到的路径、哪一项缺失、以及怎么放。

不改脚本、手工构建**本库**也可以（只编库，不产 APK）：

```bash
export ANDROID_HOME=~/Android/Sdk
export JAVA_HOME=/path/to/jdk-17
cd tv
echo "sdk.dir=$ANDROID_HOME" > local.properties   # 或用 Android Studio 打开
./gradlew :app:assembleDebug                      # 编出 :tvshell 的 aar（不产 APK）
```

> 域名参数（2026-10-29 起）：**公开仓不存真实域名**。安卓自身接口的根由 :app 的
> `BuildConfig.JEROCINE_API_BASE` 构建期注入（形状同前端 `VITE_API_BASE`：含 `/api`、不含 `/v1`，
> gradle property `-PjerocineApiBase` 或环境变量 `JEROCINE_API_BASE`；未注入时接口回退
> `serverBase + /api/v1/` 旧行为）；出厂站点地址同理走 `JEROCINE_WEB_BASE`。运行时「设置 →
> 站点地址」只改页面加载地址（落共享 prefs `jerocine_prefs/server_url`），不影响接口域。

## 自测

```bash
# JVM 单测（解析 + MockWebServer 端到端网络栈，无需设备）
./gradlew :tvshell:testDebugUnitTest
```

已覆盖：响应解析、首页/详情/播放/搜索、设备码流程、历史记录、焦点导航及播放器配置。

> 旧的真机冒烟脚本 `tv/scripts/smoke-nano.sh` 属合并前产物，已在开头加硬拦截（装错包/起错组件直接拒跑）；
> 需要真机冒烟时按单包口径（`art.jerocine.app` / `.EntryRouterActivity` / 宿主 APK 路径）重写。当前日常不做真机测试。

## 后端改动

`server/controller/TvAuthController.go` + `server/router/router.go`：
新增 `POST /tv/code`、`GET /tv/token`、`POST /tv/approve`（设备码登录，Redis 短时效配对）。

## 已完成

- [x] 手机端「设备授权」确认页：`web` 的 `/tv-auth`（调用 `POST /tv/approve`）
- [x] 播放健壮性：ExoPlayer 失败自动换源（IJK 兜底已于 2026-10-11 移除——代码中从未接线，仅剩死依赖）
- [x] 登录二维码（zxing，手机扫码直达授权页并预填配对码）
- [x] token 加密存储（EncryptedSharedPreferences，API<23 自动回退明文）

## 待办（设备环节）

- [ ] **坚果 Nano 真机功能 + 性能实测**（单包 minSdk=21 已可安装；待验证：装包成功 → 启动进原生壳 → Capacitor 类未加载 → 界面/滚动正常）
- [ ] **新版 View/XML UI 真机视觉与滚动复核**

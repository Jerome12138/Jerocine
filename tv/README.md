# Jerocine影视 — 原生 Android TV 客户端

> ⚠️ **2026-10-11 单包合并**：`tv/app` 已不再是独立 APK，而是宿主 `web/android` 里的 **`:tvshell` library**，
> 与 Capacitor 壳（`art.jerocine.app`）打进**同一个包**，由 `EntryRouterActivity` 按设备能力路由
> （低配/WebView 内核过旧 → 进本模块的 `NativeShellActivity`）。因此：
> - **不再有 `build-android.sh tv` 目标**（传它会明确报错）；打单一 APK 用 `build-android.sh web`（或 `all`，等价）。
> - 本模块**无自己的 applicationId / versionCode / versionName**，版本号跟宿主走 `scripts/android-versions.properties` 的 `web.*`。
> - 下文「技术栈 / 功能 / 后端」描述对本模块内部仍成立；**构建与签名一节已按单包口径重写**。
>
> 本文其余部分保留合并前语境（历史说明），如需权威现状以仓库根 `CLAUDE.md` 为准。

为覆盖**坚果 Nano（Android 6.0 / API 23 / Chromium 48）这类老投影**而做的原生壳。
Capacitor(WebView) 方案在该设备上无法安装/运行，故采用原生实现；合并后它作为宿主内的
`:tvshell` library 被 `EntryRouterActivity` 按设备能力路由进来。

> ⚠️ 但合并后宿主 **minSdk 被抬到 24**（Capacitor 8 内置 Cordova AAR 的硬要求），
> 因此**原生壳实际服务的是"能装上单包、但 WebView 内核过旧/内存过小"的记忆体设备**；
> API 23 的坚果 Nano 已装不上单包（属既有结论，需产品层决策），不再是本壳的目标设备。

## 技术栈

- Kotlin + **Android View/XML + RecyclerView**（针对低配电视减少渲染开销）
- **Media3 (ExoPlayer)** + SurfaceView（解码帧直贴屏，弱机最流畅）
- Retrofit + kotlinx-serialization（无反射，适合弱机）
- **library `minSdk = 21`，但合并后宿主 `minSdk = 24` 生效**（manifest 合并取二者较大值）；
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
scripts/build-android.sh web      # 单包 art.jerocine.app（Capacitor 壳 + 本原生壳）
scripts/build-android.sh all      # 等价于 web（保留 all 仅为兼容旧脚本）
```

Windows 用 Git Bash 执行 `bash scripts/build-android.sh web ...`（PowerShell / cmd 跑不了 .sh）。

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

> 后端地址**不再走编译期常量**：`-PapiBase=`/`--api-base=` 只是遗留开关，写入的
> `BuildConfig.API_BASE_URL` 当前无人引用；运行时后端地址由应用内「设置 → 服务器地址」
> 填写并落在共享 prefs（`jerocine_prefs/server_url`）里。

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

- [ ] **坚果 Nano 真机功能 + 性能实测**（⚠️ 合并后 minSdk=24，API 23 的 Nano 装不上已发的 APK —— 该设备实测项已失效，待产品层决策）
- [ ] **新版 View/XML UI 真机视觉与滚动复核**

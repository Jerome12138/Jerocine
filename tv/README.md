# Jerocine — 原生 Android TV 客户端

为覆盖**坚果 Nano（Android 6.0 / API 23 / Chromium 48）这类老投影**而做的原生客户端。
Capacitor(WebView) 方案在该设备上无法安装/运行，故采用原生实现。

## 技术栈

- Kotlin + **Android View/XML + RecyclerView**（针对低配电视减少渲染开销）
- **Media3 (ExoPlayer)** + SurfaceView（解码帧直贴屏，弱机最流畅）
- Retrofit + kotlinx-serialization（无反射，适合弱机）
- **minSdk 21 / targetSdk 30**（覆盖 Android 6.0；不上 36 避免老 ROM 解析问题）
- 复用现有 Go 后端 API，仅新增设备码登录 3 接口

## 功能

- 首页（分类海报墙）→ 详情 → 播放（断点/剧集/多播放源）
- 搜索、分类浏览
- 设备码登录（TV 出码，手机端确认）、观看历史、收藏
- 播放失败自动切换下一个播放源

## 构建

```bash
export ANDROID_HOME=~/Android/Sdk
export JAVA_HOME=/path/to/jdk-21
cd tv
echo "sdk.dir=$ANDROID_HOME" > local.properties   # 或用 Android Studio 打开
./gradlew :app:assembleDebug
# 产物：app/build/outputs/apk/debug/app-debug.apk（minSdk 21，可装 Android 6.0）
```

配置后端地址：`app/build.gradle.kts` 的 `API_BASE_URL`（默认 `http://localhost:9000/`，构建时用 `-PapiBase=...` 指向实际后端）。

## 自测

```bash
# JVM 单测（解析 + MockWebServer 端到端网络栈，无需设备）
./gradlew :app:testDebugUnitTest
```

已覆盖：响应解析、首页/详情/播放/搜索、设备码流程、历史记录、焦点导航及播放器配置。

## 真机冒烟（坚果 Nano，需设备在线）

```bash
./scripts/smoke-nano.sh <投影仪IP>
```

脚本会：连接 adb → 校验 Android 版本 → 安装 APK → 启动 → 抓 logcat 崩溃。

## 后端改动

`server/controller/TvAuthController.go` + `server/router/router.go`：
新增 `POST /tv/code`、`GET /tv/token`、`POST /tv/approve`（设备码登录，Redis 短时效配对）。

## 已完成

- [x] 手机端「设备授权」确认页：`web` 的 `/tv-auth`（调用 `POST /tv/approve`）
- [x] 播放健壮性：ExoPlayer 失败自动换源；**IJKPlayer 兜底**（arm64+armv7 .so）智能降级——仅真·解码失败才切 IJK，垃圾数据不喂避免 ffmpeg 原生崩溃
- [x] 登录二维码（zxing，手机扫码直达授权页并预填配对码）
- [x] token 加密存储（EncryptedSharedPreferences，API<23 自动回退明文）

## 待办（设备环节）

- [ ] **坚果 Nano 真机功能 + 性能实测**（设备上线后跑 `scripts/smoke-nano.sh`；当前已设自动重试）
- [ ] **新版 View/XML UI 真机视觉与滚动复核**

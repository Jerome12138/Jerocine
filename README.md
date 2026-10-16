# Jerocine影视

一个自托管的在线观影平台：**多采集源自由配置、服务端广告过滤、Android TV / 老投影原生支持、播放器触屏手势、扫码登录**。
一套代码覆盖桌面 / 移动 / TV（UA 自适应），Web 经 Capacitor 打包安卓 **单包 APK**（内含 Web 壳 + 原生壳，启动按设备能力自动路由）。

> **⚠️ 免责声明**
>
> 本项目**仅供学习交流使用，禁止用于任何商业用途**。
> 项目本身不提供、不聚合任何受版权保护的影视内容，采集源由使用者自行配置；使用者部署、采集与公开运营产生的内容版权、合规及法律风险，一律由使用者自行承担。
> 请遵守所在地法律法规，尊重版权。继续使用本项目即视为同意上述声明。

## ✨ 特色功能

- **多采集源自由配置**：管理后台可自由增删采集源（XML / JSON 解析器），按源测速、自动停采、健康度面板；支持手动采集与定时更新，采集数据先入暂存表、再影子表原子 reindex
- **服务端广告过滤**：m3u8 流在服务端过滤广告分片，播放页展示过滤角标，Web / TV / 原生播放器全端生效
- **Android TV 全家桶（单包双壳）**：
  - 单一 APK（`art.jerocine.app`）：`EntryRouterActivity` 探测设备能力，能力强走 `web/` 的 Capacitor Web 壳，
    能力弱（WebView 内核过旧 / 内存不足）自动落到原生 TV 壳，D-pad 焦点管理
  - 两壳共用同一原生播放器核心 `android/player-core`（ExoPlayer/Media3），前端产物由 nginx 独立发布、APK 远程加载
- **平板 / 触屏手势**：播放页双击全屏、长按倍速、横滑快进快退，带可视化手势提示（全屏触屏）
- **扫码登录（设备码）**：Web 端扫码授权；TV / 老设备输入设备码轮询登录，登录后观看历史 / 收藏跨端同步
- **多线路播放**：同一影片聚合多采集源线路，按实测播放延迟自动排序；自动续播、自动下一集、跳过片头片尾
- **热度榜单体系**：hot_score 热度分 + 榜单（首页分类行 / 分类页排行榜 / 相关推荐），管理端可触发刷新
- **全文检索 + 拼音**：物化宽表 FULLTEXT 检索 + 首字母拼音索引
- **APP 自升级**：版本检查 + APK 下载，支持灰度白名单
- **设备登录管理**：单账号多设备登录数与踢出管理
- **部署自动化**：一键部署脚本（pull → 构建 → 健康检查 → 自动清双层缓存），优雅停机不打断采集

## 近期更新（2026-10）

- **老 Android 兼容**：minSdk 21（Android 5.0 起可安装），Android 6 老投影真机全链路修复；公共层 / TV 壳出包强制 lint `NewApi` 守卫，API 误用出包即拦
- **老 WebView 兼容**：web 端 plugin-legacy 双产物 + Service Worker 运行时缓存，WebView 61+ 即可跑 Web 壳（现代设备体验零变化）
- **安卓工程整合**：全部安卓代码收敛到顶层 `android/` 单 Gradle 工程（`app` + `tvshell` + `player-core`），构建链去 cap sync——生成物入库，clone 后直接打 APK

## 组成

| 模块 | 说明 | 技术栈 |
|---|---|---|
| `web/` | Web 单页应用：用户端 + 管理后台 + TV 模式（UA 自适应） | Vue 3.5 · Vite · TypeScript · Pinia · UnoCSS · Capacitor |
| `server/` | 后端 API 与采集引擎 | Go · Gin · GORM · go-redis · golang-migrate |
| `android/` | 安卓单包工程：`app`(Capacitor 宿主壳) + `tvshell`(原生 TV 壳 library) + `player-core`(播放器/共享层，两壳共用) | Kotlin/Java · Capacitor · ExoPlayer |
| `deploy/` | 部署编排：Docker Compose、Nginx、golang-migrate、APK 版本 | Docker Compose · Nginx |

## 架构要点

- 分层：`handler → service → repository(+cache) → db`，领域层（`internal/domain`）不依赖具体基础设施
- MySQL 唯一权威真相；Redis 纯缓存（可淘汰、可回源），读路径物化宽表 + FULLTEXT 检索
- API 统一 `/api/v1` 语义化 REST，契约唯一依据 `server/openapi/openapi.yaml`
- 数据库结构由 `server/migrations/` 版本化 SQL 管理（golang-migrate），禁止运行时改表
- 采集引擎内置 XML / JSON 采集源解析、per-source 暂存与影子表 reindex

## 快速开始

```bash
# 后端（需本地 MySQL/Redis 或 docker compose 起依赖）
cd server && go run ./cmd/server

# 前端
cd web && pnpm i && pnpm dev        # http://localhost:5173，API 走 /api 反代

# 单测
cd server && go test ./...
cd web && pnpm test
```

生产部署：从零部署手册见 [`docs/部署指南.md`](./docs/部署指南.md)；日常运维命令速查见 [`deploy/README.md`](./deploy/README.md)。

## 目录结构

```text
Jerocine/
├─ web/        # Vue3 SPA（前端产物由 nginx 独立发布；APK 远程加载线上站点）
├─ android/    # 安卓单包工程（Capacitor 壳 app + 原生 TV 壳 tvshell + 共享层 player-core）
├─ server/     # Go 后端（cmd/server 入口 + internal/ 业务 + migrations/ + openapi/）
└─ deploy/     # 部署编排（docker-compose / Dockerfile / nginx / apk/）
```

> ℹ️ `web/android` 在 git 里**不存在**——它只是本地 junction（→ `../android`，Capacitor 平台目录硬约定）。
> **clone 后无需处理**：打 APK（`scripts/build-android.sh`）不经过它；手动跑 `scripts/cap-sync.sh` 时脚本会自动补建。

各模块细节见各自 README：[server](./server/README.md) · [web](./web/README.md) · [deploy](./deploy/README.md)。

## 致谢（Credits）

本项目受开源项目 [GoFilm](https://github.com/ProudMuBai/GoFilm)（MIT © 2023 ProudMuBai）启发，特此致谢。许可与声明见 [LICENSE](./LICENSE)。

## License

[MIT](./LICENSE) © 2026 jerome12138 (Jerocine影视)

# Jerocine

一个在线观影平台：Vue 3 单页应用（桌面 / 移动 / TV 自适应）+ Go 后端 + Android TV 原生壳 + Capacitor 安卓包。

## 组成

| 模块 | 说明 | 技术栈 |
|---|---|---|
| `web/` | Web 单页应用：用户端 + 管理后台 + TV 模式（UA 自适应），并可经 Capacitor 打包安卓 APK | Vue 3.5 · Vite · TypeScript · Pinia · UnoCSS |
| `server/` | 后端 API 与采集引擎 | Go · Gin · GORM · go-redis · golang-migrate |
| `tv/` | Android TV 原生壳（Leanback，播放器/账号/历史/收藏） | Kotlin · ExoPlayer · View 系 |
| `deploy/` | 部署编排：Docker Compose、Nginx、golang-migrate、APK 版本检查配置 | Docker Compose · Nginx |

## 功能

**用户端**

- 首页轮播 + 多分类 Row + 热点推荐，分类导航 / 筛选 / 首字母 + 拼音搜索
- 影片详情（多采集源、多选集、相关推荐），播放页（多源切换、自动续播、自动下一集、跳过片头片尾、广告过滤角标）
- 观看历史 / 我的收藏（未登录本地保存，登录后无缝迁移云端、跨设备同步）
- 扫码登录（设备码轮询），TV 端 D-pad 导航与焦点管理
- APP 自升级（版本检查 + APK 下载，支持灰度白名单）

**管理后台（`/manage`）**

- 仪表盘、影片 / 分类 / 文件管理、站点配置
- 采集源管理与健康度面板（测速、自动停采）、采集任务与定时更新
- APP 版本管理（版本发布 + APK 上传）

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

生产部署：完整从零部署手册见 [`docs/部署指南.md`](./docs/部署指南.md)（前置条件、环境变量配置清单、部署步骤、验证与排障）；快速命令参考 [`deploy/README.md`](./deploy/README.md)（`docker compose up -d --build`，nginx 对外 8080，后端 3601 仅内网）。

## 目录结构

```text
Jerocine/
├─ web/        # Vue3 SPA + Capacitor 安卓壳（android/）
├─ tv/         # Android TV 原生壳（Kotlin, 包名 com.jerocine.tv）
├─ server/           # Go 后端（cmd/server 入口 + internal/ 业务 + migrations/ + openapi/）
└─ deploy/           # 部署编排（docker-compose / Dockerfile / nginx / apk/）
```

各模块细节见各自 README：[server](./server/README.md) · [web](./web/README.md) · [deploy](./deploy/README.md)。

## License

[MIT](./LICENSE) © 2026 jerome12138 (Jerocine)

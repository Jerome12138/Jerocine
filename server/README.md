# Film Server

Jerocine 后端（重构后架构，2026-08 起为唯一线上版本）。

- 提供前端（web / tv）所需的 `/api/v1` REST API、采集（spider）与管理端接口
- Go 1.21 + gin + gorm + go-redis，golang-migrate 管理版本化 DDL
- 设计原则：MySQL 唯一权威真相；Redis 纯缓存（可回源）；读路径物化 + cache-aside；语义化 HTTP 契约
- 契约唯一依据：`openapi/openapi.yaml`（OpenAPI 3.1，`/api/v1` 前缀）

## 目录结构

```text
server/
├─ cmd/server/        入口（组合根）: Config(fail-fast) → MySQL/Redis → blob → repository
│                     → service/engine → handler → /api/v1 → cron → 监听;
│                     含 -healthcheck 子命令（distroless 镜像无 curl，compose 探针用）
├─ internal/          全部业务代码（禁止被外部项目 import）
│  ├─ config/         单一配置加载，env 驱动，fail-fast
│  ├─ domain/         领域层: entity + repository 端口（纯领域，不 import db/redis）
│  ├─ dto/            请求/响应结构与错误格式（RFC7807 风格）
│  ├─ handler/        HTTP 处理器（参数绑定 → 调 service）
│  ├─ service/        业务逻辑
│  ├─ repository/     mysql 仓储实现（含 FULLTEXT 检索、影子表 reindex、事务）
│  ├─ cache/          cache-aside 封装: keys / invalidate / lock / ratelimit / jobprogress
│  ├─ middleware/     auth(JWT) / 限流 / CORS 等
│  ├─ platform/       基础设施: db / redis / auth / blobstore
│  ├─ router/         /api/v1 路由注册（85 个端点）
│  └─ spider/         采集引擎: fetch / parse / engine（XML+JSON 采集源、分页抓取、入库）
├─ migrations/        golang-migrate 版本化 SQL（替代旧 AutoMigrate，勿手工改库）
├─ openapi/           API 契约（openapi.yaml）
├─ go.mod / go.sum
└─ README.md
```

依赖流向：`handler → service → repository(+cache decorator) → db`，领域层不依赖具体基础设施。

## 本地开发

```bash
cd server
go build ./...            # 编译
go test ./...             # 单测
go vet ./...              # 静态检查
go run ./cmd/server       # 本地运行（需配置 env，见 docker-compose.yml environment 段）
```

## 部署

镜像由 `deploy/Dockerfile` 构建（golang:1.21-alpine 编译 → distroless nonroot 运行，监听 3601），
构建上下文为仓库根：`docker compose -p jerocine --env-file .env up -d --build mysql redis migrate film`。
`migrate` 服务先跑 `migrations/`，`film` 依赖其完成。日常只更新后端：`docker compose build film && docker compose up -d --no-deps film`。

> 历史：2026-05 底按分层蓝图从旧结构（controller/logic/model/plugin/router）全量重构至当前架构，2026-08-03 上线。

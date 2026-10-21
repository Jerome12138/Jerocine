# Jerocine Server

Jerocine 后端。

- 提供各端（Web SPA / TV 原生壳 / 安卓壳）所需的 `/api/v1` REST API、采集（spider）与管理端接口
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
├─ migrations/        golang-migrate 版本化 SQL（勿手工改库）
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
go run ./cmd/server       # 本地运行（需配置 env，逐项见 docs/环境变量参考.md）
```

## 部署

常规发布 = **本机构建 → SSH 上送 → 服务器解包**：`bash scripts/deploy-server.sh`（详见根
[`docs/部署指南.md`](../docs/部署指南.md) 与 [`deploy/README.md`](../deploy/README.md)）。

容器：`deploy/Dockerfile.runtime`（常规，薄运行镜像：COPY 本机交叉编译的二进制，distroless nonroot，监听 3601）；
`deploy/Dockerfile`（备用，容器内 golang 编译）。compose 服务：mysql / redis / migrate / server / nginx。

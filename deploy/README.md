# Deploy — Docker Compose 部署

```text
deploy/
├─ docker-compose.yml   # 服务编排: mysql / redis / migrate / server / nginx (build.context = 仓库根)
├─ Dockerfile           # 后端镜像: golang:1.21-alpine 编译 → distroless nonroot (UID 65532), 监听 3601
├─ .env.example         # 环境变量模板 (cp .env.example .env 后填生产值; .env 不入库)
├─ data/nginx/nginx.conf # nginx 配置: SPA 静态托管 + /api 反代 + proxy_cache
├─ secrets/             # JWT RS256 密钥对 (不入库, 见 .gitignore)
└─ apk/                 # APK 下载目录 (容器内只读挂载)
```

## 首次部署

1. 准备密钥（RS256，挂载给 distroless 容器）：

   ```bash
   openssl genrsa -out secrets/jwt_private.pem 2048
   openssl rsa -in secrets/jwt_private.pem -pubout -RSAPublicKey_out -out secrets/jwt_public.pem
   chmod 600 secrets/*.pem
   ```

   ⚠️ `secrets/` 与 `apk/` 目录及其内容必须属 **65532:65532**（distroless nonroot UID），否则 jerocine_server 读不到密钥而启动失败。

2. 配置环境：`cp .env.example .env`，修改所有 `change_me_*` 占位值（MySQL/Redis 密码、`SPIDER_RESET_TOKEN` 设长随机串、按需配 `CORS_ALLOWED_ORIGINS`）。

3. 启动（在 `deploy/` 目录）：

   ```bash
   sudo docker compose --env-file .env up -d --build
   ```

   启动顺序：mysql/redis → `migrate` 一次性服务跑完 `server/migrations/` → jerocine_server → nginx。
   默认对外端口 `8080`（`NGINX_PORT` 可改），后端 3601 仅容器网络内可达。

4. 首次登录：默认管理员 `admin / change_me_admin`（`000005_seed_admin` 迁移创建），**公网部署后立即改密**。

## 日常更新

```bash
# 只更新前端（--no-deps 不会连带重启 jerocine_server，不打断采集任务）
sudo docker compose build nginx && sudo docker compose up -d --no-deps nginx

# 更新后端（会重启 jerocine_server；若有采集任务先 POST /api/v1/manage/spider/jobs/:id/pause）
sudo docker compose build server && sudo docker compose up -d server   # 会先自动跑待应用迁移

# 只跑迁移不重启
sudo docker compose run --rm migrate
```

## 部署后必清缓存

后端 API 改动后接口可能仍返回旧数据（两层缓存）：

```bash
# 1. nginx proxy_cache（/api 缓存 7 天）
sudo docker exec jerocine_nginx sh -c "rm -rf /var/cache/nginx/api/* && nginx -s reload"

# 2. Redis 应用层缓存（v1:movie:detail:* 等，带 TTL）
sudo docker exec jerocine_redis redis-cli -a "$REDIS_PASSWORD" --no-auth-warning \
  --scan --pattern 'v1:movie:detail:*' | xargs -r docker exec -i jerocine_redis redis-cli -a "$REDIS_PASSWORD" --no-auth-warning del
```

## 健康检查与排障

- 容器健康：`sudo docker compose ps`（全部应为 healthy；jerocine_server 内置 `-healthcheck` 子命令）
- 冒烟：`curl localhost:8080/`（前端 200）、`curl localhost:8080/api/v1/films?keyword=test`（API 200）
- 日志：`sudo docker logs jerocine_server`、`sudo docker logs jerocine_nginx`

## APP 版本管理

APK 版本检查/灰度在管理后台 `/manage` 的"APP 版本管理"维护（存 DB），APK 文件放入 `deploy/apk/` 供下载。

# Deploy — Docker Compose 部署

```text
deploy/
├─ docker-compose.yml   # 服务编排: mysql / redis / migrate / server / nginx (build.context = 仓库根)
├─ deploy.sh            # 一键部署: pull → build+up → 等健康 → 清两层缓存
├─ Dockerfile           # 后端镜像: golang:1.27-alpine 编译 → distroless nonroot (UID 65532), 监听 3601
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
   nginx 容器直接监听 **443** 并终结 TLS（端口在 compose 里硬编码，无 `NGINX_PORT` 变量），
   证书由 `deploy/certs/{fullchain,privkey}.pem` 挂载；后端 3601 仅容器网络内可达。

4. 首次登录：默认管理员 `admin / change_me_admin`（`000005_seed_admin` 迁移创建），**公网部署后立即改密**。

## 日常更新

```bash
./deploy.sh          # 常规部署: server + nginx, 自动清两层缓存
./deploy.sh nginx    # 纯前端改动: 只重建 nginx(不打断采集), 脚本内已带等健康与清缓存
./deploy.sh server   # 只更新后端
```

**采集不需要手动暂停**：server 收到 SIGTERM（compose 重建容器时自动发送）会优雅停机——
HTTP 在途请求收尾 → 采集在跑轮次取消收尾（被中断的页记入失败台账，由补采/滚动增量窗口自愈）
→ 新容器起来后 cron 调度器自动恢复下一轮增量（20 分钟周期）。停机宽限 40s（`stop_grace_period`）。

手动操作等价形式（排查问题时用）：

```bash
# 只跑迁移不重启
sudo docker compose run --rm migrate

# 手动清缓存（deploy.sh 已内置）
sudo docker exec jerocine_nginx sh -c "rm -rf /var/cache/nginx/api/* && nginx -s reload"
RP=$(grep -E '^REDIS_PASSWORD=' .env | cut -d= -f2-)
sudo docker exec -e REDISCLI_AUTH="$RP" jerocine_redis redis-cli --no-auth-warning -n 0 FLUSHDB
```

## 健康检查与排障

- 容器健康：`sudo docker compose ps`（全部应为 healthy；jerocine_server 内置 `-healthcheck` 子命令）
- 冒烟：`curl -sk https://localhost/`（前端 200）、`curl -sk https://localhost/api/v1/films?page=1`（API 200）；
  公网域名：`curl -s https://jerocine.art/`
- 日志：`sudo docker logs jerocine_server`、`sudo docker logs jerocine_nginx`

## APP 版本管理

APK 版本检查/灰度在管理后台 `/manage` 的"APP 版本管理"维护（存 DB），APK 文件放入 `deploy/apk/` 供下载。

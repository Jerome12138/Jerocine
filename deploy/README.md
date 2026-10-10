# Deploy — Docker Compose 部署

```text
deploy/
├─ docker-compose.yml    # 服务编排: mysql / redis / migrate / server / nginx (build.context = 仓库根)
├─ deploy.sh             # 一键部署: pull(→必要时自我重启) → build+up → 等健康 → 清两层缓存 (web/web-init/web-rollback 见下)
├─ lib/web-release.sh    # 前端产物 发布/回滚 纯逻辑 (版本目录 / 排除 map / 保留 3 版 / 整版快照 / 陈旧根文件清理)
├─ tests/web-release.test.sh   # ↑ 的单测 (本机可跑, 不需要 docker)
├─ tests/deploy-reexec.test.sh # deploy.sh 自我重启机制的单测 (假仓库 + 假 git)
├─ Dockerfile            # 后端镜像: golang:1.27-alpine 编译 → distroless nonroot (UID 65532), 监听 3601
├─ .env.example          # 环境变量模板 (cp .env.example .env 后填生产值; .env 不入库)
├─ data/nginx/nginx.conf # nginx 配置: SPA 静态托管 + /api 反代 + proxy_cache + SW no-cache
├─ data/html/            # 前端产物 (挂载给 nginx; 不入库, 由 ./deploy.sh web 生成)
├─ data/releases/<TS>/   # 每个发布版本的整版快照 {index.html, sw.js, workbox-*.js} (回滚用)
├─ data/debugmap/<TS>/   # 归档的 sourcemap (挂载目录之外 ⇒ 公网不可达)
├─ data/root-manifest.txt # 上一版写入 html 根的条目名清单 (用于清理陈旧根文件; 不入库)
├─ secrets/              # JWT RS256 密钥对 (不入库, 见 .gitignore)
└─ apk/                  # APK 下载目录 (容器内只读挂载)
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
./deploy.sh nginx    # 纯前端改动(旧模式): 只重建 nginx 镜像(不打断采集), 脚本内已带等健康与清缓存
./deploy.sh server   # 只更新后端
```

> `git pull` 之后脚本会用**新版本重新 exec 自己一次**（防重入标记 `JEROCINE_DEPLOY_REEXEC=1`）——
> pull 会覆写脚本自身，而 bash 是"边执行边读"的。因此日志里会看到两轮 pull/前置输出，这是预期行为；
> 它保证后续流程一定跑在刚拉到的版本上（例如某个修复只在 `deploy.sh` 里，第一次执行就能生效）。

## 前端发布（Service Worker + 版本目录）

> 2026-10-09 起前端静态目录改为**宿主机挂载**（`./data/html`），不再 COPY 进 nginx 镜像 ——
> 这是"保留 3 版 / 整版回滚"的前提（镜像重建不再清空线上目录）。源码改动后按下面顺序操作一次。

```bash
# 【仅"迁移"时】旧容器还活着、且它还在服务镜像内自带的产物 ⇒ 先把产物搬到挂载目录
# (不搬就切挂载, 线上立刻 404)。全新安装没有可弄坏的站点, 跳过这步。
./deploy.sh web-init

# 之后每次前端发布: 构建(注入 JC_BUILD_TS) → 同步 data/html → reload nginx(含 sw.js no-cache)
./deploy.sh web

# 出问题时整版回滚(快照三件套一起换; TS 见 data/releases/)
./deploy.sh web-rollback 20261009-190000
```

- **`./deploy.sh nginx` 语义已变**：静态目录改挂载后它只重建 nginx 容器、**不产出前端文件**
  （改 `nginx.conf` 时才用它）。前端发布一律走 `./deploy.sh web`。
- **空挂载守卫**：`web` 在"线上旧容器仍在服务镜像内产物"时拒绝执行（提示先 `web-init`）；
  `nginx` / 全量部署在 `data/html` 为空时拒绝执行（提示先 `web`）。两者都不会把站点打空。
- 产物结构：`index.html`（引用 `/assets/<TS>/…`）+ `sw.js` + `workbox-<hash>.js` + 根静态 + `LICENSE`；
  版本目录 = `data/html/assets/<TS>/`，只保留最近 **3** 版。
- `*.map` **不进挂载目录**，归档到 `data/debugmap/<TS>/`（排障用；nginx 另有 `.map → 404` 双保险）。
- **陈旧根文件清理**：每次发布把"本版放到 html 根的条目名"记进 `data/root-manifest.txt`（挂载目录**之外**，
  不公开）；下次发布据此删掉"上一版有、本版没有"的根文件/目录（改名后的 icons/、被移除的 robots.txt 等）。
  只认这份清单 ⇒ 运维手工放进挂载目录的东西不会被误删；`assets/` 恒被跳过（归保留窗口管）。
- 顺序约束：**先写目录、最后才 reload**（避免切换期间的 404 窗口）；陈旧清理也在写目录阶段完成。
- 整版回滚 = 快照三件套（`index.html` + `sw.js` + `workbox-*.js`）；根静态不随回滚变化。
- 本机纯逻辑单测：
  - `bash tests/web-release.test.sh`（8 组 55 条断言：布局 / map 排除归档 / 保留 3 版 / 陈旧根文件清理 / 回滚）
  - `bash tests/deploy-reexec.test.sh`（4 条：`git pull` 后自我重启 + 防重入 + 参数传递）
- **迁移注意**：`web-init` 从旧容器拷出的 `assets/*`（旧版无版本目录的产物）不归新流程管，也不会被自动清理；
  `web-init` 会列出来提示，确认新版上线正常后可手动删除。`docker cp` 产物属主为 **root**（daemon 写盘），
  `web-init` 会 `chown` 回当前用户再继续 —— 否则随后的 `web` 发布以普通用户覆盖写 `index.html` 会 Permission denied。

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

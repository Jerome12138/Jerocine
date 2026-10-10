# Deploy — Docker Compose 部署

```text
deploy/
├─ docker-compose.yml    # 服务编排: mysql / redis / migrate / server / nginx (build.context = 仓库根)
├─ deploy.sh             # 一键部署: pull(→必要时自我重启) → build+up → 等健康 → 清两层缓存
│                        #   常规发布子命令 web-deploy / server-deploy(解包本地上传的产物包)
├─ Dockerfile            # 【备用】后端镜像: golang:1.27-alpine 编译 → distroless nonroot, 监听 3601
├─ Dockerfile.runtime    # 【常规】后端运行镜像(薄): 只 COPY 本机编译好的二进制, 容器内不编译
├─ lib/web-release.sh    # 前端产物 发布/回滚 纯逻辑 (版本目录 / 排除 map / 保留 3 版 / 整版快照 /
│                        #   陈旧根文件清理 / 上传包解包与留档)
├─ tests/web-release.test.sh   # ↑ 的单测 (本机可跑, 不需要 docker)
├─ tests/deploy-reexec.test.sh # deploy.sh 自我重启机制的单测 (假仓库 + 假 git)
├─ .env.example          # 环境变量模板 (cp .env.example .env 后填生产值; .env 不入库)
├─ data/nginx/nginx.conf # nginx 配置: SPA 静态托管 + /api 反代 + proxy_cache + SW no-cache
├─ incoming/             # 本机上传的产物包落地处(SSH 上送到此, 由 *-deploy 解包; 不入库)
├─ server/               # 后端产物包解包处(= Dockerfile.runtime 的 build context; 不入库)
├─ data/html/            # 前端产物 (挂载给 nginx; 不入库, 由前端发布写入)
├─ data/releases/<TS>/   # 每个发布版本的整版快照 {index.html, sw.js, workbox-*.js} (回滚用)
├─ data/debugmap/<TS>/   # 归档的 sourcemap (挂载目录之外 ⇒ 公网不可达)
├─ data/packages/           # 上传的产物包留档(常规发布手段, 事后可重发/比对; 各保留 5 份):
│   │                        #   web: <TS>.tar.gz / server: jerocine-server-<TS>-<sha>-linux-<arch>.tar.gz
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
   sudo docker compose --env-file .env up -d --build   # 拉起全栈（前端挂载目录此时为空）
   ./deploy.sh web                                     # 构建并发布前端产物到挂载目录（否则站点 404 / nginx 不健康）
   ```

   启动顺序：mysql/redis → `migrate` 一次性服务跑完 `server/migrations/` → jerocine_server → nginx。
   nginx 容器直接监听 **443** 并终结 TLS（端口在 compose 里硬编码，无 `NGINX_PORT` 变量），
   证书由 `deploy/certs/{fullchain,privkey}.pem` 挂载；后端 3601 仅容器网络内可达。
   **全新安装不需要 `web-init`**（那是"从旧容器搬运镜像内产物"的迁移专用步骤，见下节）。

4. 首次登录：默认管理员 `admin / change_me_admin`（`000005_seed_admin` 迁移创建），**公网部署后立即改密**。

## 日常更新

**常规做法：本机构建 → 压缩包上传 → 服务器只负责"解包 + 落盘 + reload"**（2026-10-10 起）。
服务器因此不需要 Node/pnpm/Go，也不留 docker 构建缓存（历史上那 13.87GB build cache 主要就是
前端 node 阶段与后端 golang 阶段攒出来的）。

```bash
# 前端：本机构建(注入 JC_BUILD_TS + 布局断言) → tar.gz → SSH 上送 → 远端 ./deploy.sh web-deploy
cd ../ && bash scripts/build-web.sh          # 只构建打包（产物在系统下载目录）
bash scripts/deploy-web.sh                   # 构建 + 上传 + 发布（一步到位）
bash scripts/deploy-web.sh --pkg <tgz>       # 复用已构建好的包（跳过构建）

# 后端：本机交叉编译(CGO_ENABLED=0 GOOS=linux) → tar.gz → SSH 上送 → 远端 ./deploy.sh server-deploy
bash scripts/build-server.sh                 # 只编译打包
bash scripts/deploy-server.sh                # 编译 + 上传 + 发布（一步到位）
```

服务器侧对应的子命令（一般由上面的脚本调用，排障时可手动跑）：

```bash
./deploy.sh web-deploy <pkg.tar.gz>    # 解包前端产物包 → 同一套发布语义 → 留档 data/packages/ → reload
./deploy.sh server-deploy <pkg.tar.gz> # 解包后端产物包 → 构建薄运行镜像 → 重建容器 → 等健康 → 清缓存 → 留档 data/packages/
```

**备用路径**（没有本机工具链、或想在服务器上从某个 commit 重跑时用；这才会在服务器上编译）：

```bash
./deploy.sh            # 常规部署: server + nginx 容器, 自动清两层缓存(不含前端产物)
./deploy.sh web        # 前端发布(服务器上构建): 需服务器有 docker + node 构建阶段网络
./deploy.sh server     # 只更新后端(服务器上 golang 编译)
./deploy.sh nginx      # 只重建 nginx 容器(改 nginx.conf 时用), **不产出前端产物**
```

> 两条路径**共用**同一套发布语义（版本目录 / 排除 map / 保留 3 版 / 整版快照 / 陈旧根文件清理 /
> 空挂载守卫），所以行为一致；区别只在"产物的二进制从哪来"。
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
  - `bash tests/web-release.test.sh`（10 组 81 条断言：布局 / map 排除归档 / 保留 3 版 / 陈旧根文件清理 / web-init 收尾 / 回滚 / 上传包解包与留档）
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

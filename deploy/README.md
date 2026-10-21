# Deploy — Docker Compose 部署运维

首次部署的完整步骤见 [`docs/部署指南.md`](../docs/部署指南.md)；本文是日常运维参考。

> 备选方案：[`bare/`](./bare/README.md) —— 不依赖 Docker 的裸机部署（同一套产物包与发布语义，服务器侧 systemctl + 原生 nginx/MySQL/Redis）。

## 目录

```text
deploy/
├─ docker-compose.yml    # mysql / redis / migrate / server / nginx（build.context = 仓库根）
├─ deploy.sh             # 一键部署入口（子命令见下表）
├─ Dockerfile            # 【备用】后端镜像：golang alpine 编译 → distroless nonroot，监听 3601
├─ Dockerfile.runtime    # 【常规】后端运行薄镜像：只 COPY 本机编译好的二进制
├─ lib/web-release.sh    # 前端发布/回滚纯逻辑（版本目录 / 保留 3 版 / 整版快照）
├─ tests/                # web-release + deploy 自我重启的单测（本机可跑，不需要 docker）
├─ .env.example          # 环境变量模板（cp 为 .env 后填生产值；.env 不入库）
│                        # 每个键的用途/默认值/必填项见 ../docs/环境变量参考.md
├─ data/nginx/nginx.conf # nginx：SPA 静态托管 + /api 反代 + proxy_cache + SW no-cache
├─ secrets/              # JWT RS256 密钥对（不入库，须属 65532:65532）
├─ apk/                  # APK 下载目录（须属 65532:65532）
├─ incoming/ · server/   # 上传产物包解包处（不入库）
└─ data/
   ├─ html/              # 前端产物，挂载给 nginx（不入库，由发布写入）
   ├─ releases/<TS>/     # 每版整版快照（回滚用）
   ├─ debugmap/<TS>/     # 归档 sourcemap（挂载目录之外 ⇒ 公网不可达）
   ├─ packages/          # 上传包留档（可重发/比对，各保留 5 份）
   └─ root-manifest.txt  # 上一版根文件清单（陈旧清理依据）
```

## deploy.sh 子命令

| 子命令 | 作用 |
|---|---|
| （无参数） | server + nginx 容器重建 → 等健康 → 自动清两层缓存（不含前端产物） |
| `web` | 服务器上构建前端 → 发布到挂载目录 `data/html` → reload |
| `server` | 服务器上编译后端 → 重建容器 |
| `nginx` | 只重建 nginx 容器（改 nginx.conf 时用），**不产出前端文件** |
| `web-deploy <pkg>` | 解包本地上传的前端包 → 发布 → 留档 → reload |
| `server-deploy <pkg>` | 解包后端包 → 薄运行镜像 → 重建容器 → 等健康 → 清缓存 → 留档 |
| `web-rollback <TS>` | 整版回滚（`index.html` + `sw.js` + `workbox-*.js` 三件套），TS 见 `data/releases/` |

> `git pull` 后脚本会用新版本重新 exec 自己一次（防脚本执行中被覆写），日志看到两轮 pull 属预期。

## 日常更新

常规做法 = **本机构建 → 压缩包上传 → 服务器只解包**（服务器不需要 Node/pnpm/Go，也不攒 docker 构建缓存）：

```bash
# 前端：本机构建（注入 JC_BUILD_TS + 布局断言）→ tar.gz → SSH 上送 → 远端 web-deploy
bash scripts/build-web.sh          # 只构建打包
bash scripts/deploy-web.sh         # 构建 + 上传 + 发布（一步到位）
bash scripts/deploy-web.sh --pkg <tgz>   # 复用已构建的包

# 后端：本机 CGO_ENABLED=0 交叉编译 linux/amd64 → tar.gz → 上送 → 远端 server-deploy
bash scripts/build-server.sh       # 只编译打包
bash scripts/deploy-server.sh      # 编译 + 上传 + 发布（一步到位）
```

需要 SSH 别名（`~/.ssh/config` 配 `Host jerocine`）。覆盖默认值：`JEROCINE_SSH_HOST` / `JEROCINE_DEPLOY_DIR`（默认 `/home/ubuntu/jerocine/deploy`）/ `JEROCINE_DOWNLOAD_DIR`（本机产物落地目录）。

本机没有工具链时用 `./deploy.sh web` / `./deploy.sh server`（在服务器上构建，见上表）。

## 前端发布语义

两条发布路径（本机上传包 / 服务器构建）共用同一套语义，差别只在产物从哪来：

- 静态目录是**宿主机挂载** `data/html`（不在镜像内），这是"保留 3 版 / 整版回滚"的前提
- 版本目录 `assets/<TS>/` 只保留最近 **3** 版；`*.map` 归档到 `data/debugmap/`（不上线）
- `assets/<TS>/` 含 **legacy 兼容产物**（plugin-legacy 给低版本 WebView 的双产物），体积大属预期；sw.js precache 已排除，发布链无需特殊处理
- 每次发布按 `data/root-manifest.txt` 清理"上一版有、本版没有"的根文件（运维手工放进挂载目录的东西不会被误删）
- 顺序约束：**先写目录、最后 reload**（无 404 窗口）
- 空挂载守卫：`nginx` / 全量部署在挂载目录 `data/html` 为空时拒绝执行（防空站点 404）
- 纯逻辑单测：`bash tests/web-release.test.sh`（81 断言）· `bash tests/deploy-reexec.test.sh`（4 断言）

## 缓存清理（手动）

`deploy.sh` 与 `server-deploy` 已内置；排障手动清：

```bash
# ① nginx proxy_cache（/api 缓存 7 天）
sudo docker exec jerocine_nginx sh -c "rm -rf /var/cache/nginx/api/* && nginx -s reload"
# ② Redis 应用层缓存（DB0；DB1 禁动）
RP=$(grep -E '^REDIS_PASSWORD=' .env | cut -d= -f2-)
sudo docker exec -e REDISCLI_AUTH="$RP" jerocine_redis redis-cli --no-auth-warning -n 0 FLUSHDB
```

## 健康检查

```bash
sudo docker compose ps                 # 全部应为 healthy（server 内置 -healthcheck 子命令）
curl -sk https://localhost/            # 前端 200
curl -sk https://localhost/api/v1/films?page=1   # API 200
sudo docker logs jerocine_server       # 日志
```

## APP 版本管理

APK 版本检查/灰度在管理后台 `/manage` 的「APP 版本管理」维护（存 DB），APK 文件放入 `deploy/apk/` 供下载。

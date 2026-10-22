# 裸机（无 Docker）部署

同一套产物包、同一套发布语义，服务器侧不依赖 Docker。适用：目标机器禁装/不想装 Docker、想省掉 Docker daemon 常驻开销（约 100-300MB）。

## 与 Docker 模式的关系

| | Docker 模式（deploy/） | 裸机模式（本目录） |
|---|---|---|
| 产物包 | `scripts/build-{web,server}.sh` 产出 | **完全相同**，无需重打 |
| 后端 | 解包 → docker build 薄镜像 → 容器 | 解包 → 目录轮换 → `systemctl restart` |
| 前端 | `data/html` 挂载 → nginx 容器 | `$PREFIX/webdata/html` → 原生 nginx（**复用 `lib/web-release.sh` 全套语义**：排除 map / 保留 3 版 / 整版回滚 / 包留档） |
| MySQL/Redis/nginx | 官方镜像 | apt 安装 + 配置模板落位 |
| 数据库迁移 | migrate 容器 | `migrate.sh`（同一 migrations/、同一 schema_migrations 表，两种模式迁移历史连续） |
| 部署脚本来源 | 随产物包 `tooling/` 自包含（模式1，2026-10-22 起；不再需要服务器 git 树） | **服务器 git 树**（`deploy/bare/`）：日常发布前先 `git pull --ff-only`，与 Docker 模式2 同 |
| 优雅停机 40s | compose `stop_grace_period` | systemd `TimeoutStopSec=40s` |
| 健康探针 | 容器 healthcheck（`main -healthcheck`） | 同一端点：`GET /healthz` |

## 目录布局（`PREFIX` 默认 `/opt/jerocine`）

```
/opt/jerocine/
├─ server.env          由 bare-lib.sh 从 bare.env 组装（systemd EnvironmentFile）
├─ server/             当前运行的后端（main + data/ + static/upload + LICENSE）
├─ backup/server-<TS>/ 上一版整目录备份（保留 3 份，手动回退）
├─ packages/           后端包留档（保留 5 份）
├─ webdata/            前端发布数据（html/releases/debugmap/packages，与 docker 模式 data/ 同构）
├─ apk/                APK 自升级包（nginx alias /apk/）
└─ secrets/            JWT 密钥对（install.sh 自动从 deploy/secrets 复制 → 两种模式 token 互通）
```

> `bare.env` 不在这个树里 —— 它在**仓库的 `deploy/bare/` 目录**（脚本从自身目录读；
> 也可用 `BARE_ENV=/path/to/bare.env` 指定任意位置）。机器敏感，chmod 600。

## 首次安装（Ubuntu 22.04/24.04）

```bash
cd deploy/bare
cp bare.env.example bare.env && vi bare.env      # 填生产值（每个键的用途/必填项见 docs/环境变量参考.md）
sudo bash install.sh                              # apt 三件套 + 配置落位 + 建库建用户
# 放 TLS 证书 → /etc/nginx/jerocine-certs/{fullchain,privkey}.pem
# 上传产物包（本机 build + 上传链路与 docker 模式相同）：
sudo bash bare-deploy.sh jerocine-server-*.tar.gz # 解包+轮换+重启+健康检查
bash migrate.sh                                   # 数据库迁移
sudo systemctl start jerocine-server
sudo bash bare-web-deploy.sh <TS>.tar.gz          # 前端发布（末尾自动 reload nginx）
```

## 日常发布 / 回滚

```bash
sudo bash bare-deploy.sh <server-pkg.tar.gz>       # 后端（失败时上一版留在 backup/，日志有回退命令）
sudo bash bare-web-deploy.sh <TS>.tar.gz           # 前端
sudo bash bare-web-deploy.sh rollback <TS>         # 前端整版回滚
sudo systemctl restart jerocine-server             # 手动重启
journalctl -u jerocine-server -n 100               # 后端日志（journald）
```

## 从 Docker 模式迁移数据（一次性）

```bash
# 1. 导出（Docker 服务器上）
docker exec jerocine_mysql sh -c 'exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction --routines jerocine' > dump.sql
docker run --rm -v jerocine_uploads:/data -v "$PWD:/out" alpine tar czf /out/uploads.tgz -C /data .
# 2. 导入（裸机服务器上）—— 先 systemctl stop jerocine-server（若已启动）
mysql jerocine < dump.sql
tar xzf uploads.tgz -C /opt/jerocine/server/static/upload
chown -R jerocine:jerocine /opt/jerocine/server/static/upload
# 3. Redis 不用迁 —— 可重建缓存（读路径回源 MySQL），空库起步即可
# 4. schema_migrations 已随 dump 带过来，migrate.sh 从断点继续，不会重跑
```

## 注意事项

- **MySQL 配置必须先于首次初始化落位**（`lower_case_table_names` 只允许初始化时设置）——`install.sh` 已保证顺序，手工操作时别颠倒。
- bare.env 里 DSN 密码若含 `@ ( ) /` 等字符需按 go-sql-driver 规则转义（与 compose 模式同一限制）。
- nginx 的 `listen 80` 保留给本机探活/ACME；宿主机 80 另有用途就删掉那行。
- 连接地址固定 `127.0.0.1`（MySQL/Redis 都只听回环），不像 compose 有容器网络隔离——**不要**把 3306/6379 改绑公网。

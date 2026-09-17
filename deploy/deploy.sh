#!/usr/bin/env bash
# Jerocine 一键部署
#
# 用法(在服务器或本机都可, 脚本自动定位 deploy/ 目录):
#   ./deploy.sh              # 默认部署 server + nginx(常规全栈更新)
#   ./deploy.sh nginx        # 纯前端改动: 只重建 nginx, 不碰 server/采集
#   ./deploy.sh server       # 只更新后端
#
# 自动完成: git pull → compose build+up → 等待健康 → 清两层缓存(nginx proxy_cache + Redis DB0)。
# 采集无需手动暂停: server 收到 SIGTERM 会优雅停机(在跑页记失败台账, 增量窗口滚动自愈),
# 新容器起来后调度器自动恢复下一轮增量采集。
set -euo pipefail

cd "$(dirname "$0")"   # 总在 deploy/ 下执行, compose 相对路径才正确

# docker 权限: 可直连用 docker, 否则回退 sudo -n (服务器约定)
if docker ps >/dev/null 2>&1; then DOCKER=(docker); else DOCKER=(sudo -n docker); fi
COMPOSE=("${DOCKER[@]}" compose --env-file .env)

services=("$@")
if [ ${#services[@]} -eq 0 ]; then services=(server nginx); fi

echo "==> git pull --ff-only"
git -C .. pull --ff-only

echo "==> compose up -d --build: ${services[*]}"
# 纯前端必须 --no-deps: nginx depends_on server, 不带会连带重启后端打断采集
if [ "${services[*]}" = "nginx" ]; then
  "${COMPOSE[@]}" up -d --build --no-deps nginx
else
  "${COMPOSE[@]}" up -d --build "${services[@]}"
fi

# 等待 server 健康(纯 nginx 部署时容器本来就该 healthy, 快速通过)
echo "==> 等待 jerocine_server healthy..."
ok=""
for _ in $(seq 1 60); do
  st=$("${DOCKER[@]}" inspect --format '{{.State.Health.Status}}' jerocine_server 2>/dev/null || echo missing)
  if [ "$st" = "healthy" ]; then ok=1; break; fi
  sleep 5
done
if [ -z "$ok" ]; then
  echo "!! jerocine_server 5 分钟未达 healthy, 请查日志: ${DOCKER[*]} logs jerocine_server" >&2
  exit 1
fi

echo "==> 清 nginx proxy_cache"
"${DOCKER[@]}" exec jerocine_nginx sh -c 'rm -rf /var/cache/nginx/api/* 2>/dev/null; nginx -s reload 2>/dev/null || true'

echo "==> 清 Redis 应用缓存(DB0)"
RP=$(grep -E '^REDIS_PASSWORD=' .env | head -1 | cut -d= -f2-)
"${DOCKER[@]}" exec -e REDISCLI_AUTH="$RP" jerocine_redis redis-cli --no-auth-warning -n 0 FLUSHDB >/dev/null

echo "==> 完成: ${services[*]} 已部署, 健康检查通过, 两层缓存已清"
"${DOCKER[@]}" ps --format '{{.Names}}\t{{.Status}}' | grep jerocine

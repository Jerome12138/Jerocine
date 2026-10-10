#!/usr/bin/env bash
# Jerocine 一键部署
#
# 用法(在服务器或本机都可, 脚本自动定位 deploy/ 目录):
#   ./deploy.sh                     # 默认部署 server + nginx(常规全栈更新)
#   ./deploy.sh server              # 只更新后端
#   ./deploy.sh nginx               # 只重建 nginx 容器(改 nginx.conf 时用), **不发前端产物**
#   ./deploy.sh web-init            # 【一次性】切挂载前, 把现有 /usr/share/nginx/html 复制到 data/html
#   ./deploy.sh web                 # 【推荐】前端发布: 构建 → 同步 data/html → 挂载 → reload(含 SW)
#   ./deploy.sh web-rollback <TS>   # 整版回滚(TS 见 data/releases/)
#
# `web` 与 `nginx` 的区别(**重要, 语义已变更**): 静态目录从"镜像内 COPY dist"改成宿主机挂载
# `./data/html`, 所以 `nginx` 只重建容器、不产出前端文件; 前端发布必须走 `web`。
# `web` 另具备"保留 3 版 / 整版回滚 / 先写目录最后 reload"能力。
# 两者都会在 data/html 未初始化时拒绝执行(否则挂载点为空的 nginx 直接把站点打成 404)。
#
# 常规全栈部署自动完成: git pull(→ 必要时自我重启, 见 pull_and_reexec) → compose build+up →
# 等待健康 → 清两层缓存 (nginx proxy_cache + Redis DB0)。
# 采集无需手动暂停: server 收到 SIGTERM 会优雅停机(在跑页记失败台账, 增量窗口滚动自愈),
# 新容器起来后调度器自动恢复下一轮增量采集。
set -euo pipefail

# Git Bash(MSYS) 会把以 / 开头的参数当 Windows 路径改写(如 docker cp 的容器内路径) —— 关掉。
# Linux 上该变量无意义, 无副作用。
export MSYS_NO_PATHCONV=1

# 自身绝对路径 + 原始参数 —— 供 pull 之后"重新 exec 自己"用(见 pull_and_reexec)。
# 必须在 cd 之前取: $0 可能是相对**调用方** cwd 的路径(如 bash ../deploy/deploy.sh),
# cd 进 deploy/ 之后它就不再指向本文件了。
SELF="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"
SCRIPT_ARGS=("$@")

cd "$(dirname "$0")"   # 总在 deploy/ 下执行, compose 相对路径才正确

# Web 发布/回滚的纯文件系统逻辑(含本机单测: tests/web-release.test.sh)
# shellcheck source=lib/web-release.sh
source ./lib/web-release.sh

# docker 权限: 可直连用 docker, 否则回退 sudo -n (服务器约定)
if docker ps >/dev/null 2>&1; then DOCKER=(docker); else DOCKER=(sudo -n docker); fi
COMPOSE=("${DOCKER[@]}" compose --env-file .env)

# ============================ git pull 与"自我重启" ============================

# 拉取最新代码, 然后**重新 exec 本脚本一次**。
#
# 为什么必须重启: `git -C .. pull` 会覆写本文件, 而 bash 是"边执行边按偏移读取脚本"的 ——
# 拉到新版本后, 后面的语句可能来自"半新半旧"的文件(轻则行为不一致, 重则语法错误直接崩)。
# 重新 exec 一次(带防重入标记)才能保证后续流程一定跑在刚拉到的版本上。
# 代价: 一次进程启动 + 一轮重复日志输出; 换来"部署脚本自身也能随 push 热更新"。
pull_and_reexec() {
  echo "==> git pull --ff-only"
  git -C .. pull --ff-only

  if [ "${JEROCINE_DEPLOY_REEXEC:-0}" = "1" ]; then
    return 0   # 已是重启后的那一轮: 不再重入(pull 本身幂等, 重复调一次无副作用)
  fi
  export JEROCINE_DEPLOY_REEXEC=1
  echo "==> 代码可能已更新 ⇒ 以新版本重新执行自身(JEROCINE_DEPLOY_REEXEC=1)"
  if [ "${#SCRIPT_ARGS[@]}" -gt 0 ]; then
    exec bash "$SELF" "${SCRIPT_ARGS[@]}"
  fi
  exec bash "$SELF"
}

# ============================ Web 发布(v2: 挂载 + 版本目录) ============================

# 把 docker cp 拷出来的目录属主归一到当前用户。
# docker cp 由 docker daemon(root) 写盘 ⇒ 拷出来的文件属主是 root; 不归一化的话, 后续
# 「以当前用户删除/覆盖写」会 Permission denied(删文件要求其父目录可写; cp 覆盖是 truncate 而非 unlink,
#  root:644 对普通用户只读)。需要 root ⇒ 服务器约定 sudo -n 可用(docker 本身已走它); 以 root 运行则跳过。
chown_to_me() {
  local target="$1"
  [ "$(id -u)" = "0" ] && return 0
  if ! sudo -n chown -R "$(id -u):$(id -g)" "$target" 2>/dev/null; then
    echo "!! 无法把 $target 属主改回当前用户(docker cp 产物属主为 root)。请手动执行:" >&2
    echo "   sudo chown -R $(id -u):$(id -g) \"$target\"   然后重试" >&2
    exit 1
  fi
}

# 挂载目录是否已有可服务的产物
web_mount_ready() { [ -f data/html/index.html ]; }

# 旧容器是否仍在服务"镜像内自带"的产物(镜像里 COPY dist 的时代)。
# 切挂载之后这个探测自然为假(容器内就是挂载点, 挂载为空则文件不存在) ⇒
# 用它区分「迁移(有线上站点, 不能弄坏)」和「首次安装 / 空目录修复(没有可弄坏的东西)」。
web_legacy_html_live() {
  "${DOCKER[@]}" inspect jerocine_nginx >/dev/null 2>&1 || return 1
  "${DOCKER[@]}" exec jerocine_nginx test -f /usr/share/nginx/html/index.html >/dev/null 2>&1
}

# 构建前端产物(复用 web/Dockerfile 的 build 阶段, 同一层缓存) → 取出 dist → 写入挂载目录 → reload
deploy_web() {
  # 前置守卫: 只有"线上仍在服务、而挂载目录是空的"才会被本次发布打成 404。
  #   - 迁移场景(旧容器还活着且镜像里有产物) ⇒ 必须先 web-init 把现有产物搬到挂载目录;
  #   - 首次安装(没有容器 / 挂载本来就是空) ⇒ 放行, 由本次发布完成初始化。
  #     (否则会与 web-init 形成死锁: web 让你先跑 web-init, web-init 说没有容器就跑 web。)
  if ! web_mount_ready && web_legacy_html_live; then
    echo "!! data/html 尚未初始化, 而线上旧容器仍在服务镜像内的产物。" >&2
    echo "   现在发布会让线上 404。请先执行(必须在旧容器还活着时): ./deploy.sh web-init" >&2
    exit 1
  fi

  pull_and_reexec

  local ts builder cid tmp
  ts="$(date +%Y%m%d-%H%M%S)"
  # 固定 tag(不是 :$ts) —— 每次发版换 tag 会在 docker images 里累积一堆含 node_modules
  # + dist 的大镜像; 固定 tag 只替换引用, 层仍被新镜像复用(buildkit 缓存不丢)。
  builder="jerocine-web-builder:latest"
  echo "==> 构建前端产物 (JC_BUILD_TS=$ts)"

  # 1) 只构建到 build 阶段(产物在 /app/dist)。上下文 = 仓库根, 故 subshell 里 cd ..
  (
    cd .. && "${DOCKER[@]}" build \
      -f web/Dockerfile --target build \
      --build-arg "JC_BUILD_TS=$ts" \
      -t "$builder" .
  )

  # 2) 从构建产物镜像里取出 dist(docker create + cp + rm, 比 docker run 干净)
  tmp="$(mktemp -d)"
  cid="$("${DOCKER[@]}" create "$builder")"
  "${DOCKER[@]}" cp "$cid:/app/dist/." "$tmp/"
  "${DOCKER[@]}" rm -f "$cid" >/dev/null
  # docker cp 产物属主是 root ⇒ 必须归一化, 否则 ① 收尾 rm -rf "$tmp" 会 Permission denied,
  # 触发 set -e 直接中止, 后面的「重建 nginx + reload + 清缓存」全部被跳过;
  # ② 拷进挂载目录的文件属主也不干净(影响下次覆盖写)。
  chown_to_me "$tmp"

  # 3) 同步进挂载目录(先写目录; 排除 *.map 并归档; 保留 3 版; 生成整版快照)
  wr_sync_dist "$tmp" "data" "$ts" "../LICENSE"
  rm -rf "$tmp"

  # 4) 应用挂载(配置首次变更时重建容器) + 校验配置 + reload —— 最后一步才让新版本可见
  echo "==> 应用挂载并 reload nginx"
  "${COMPOSE[@]}" up -d --build --no-deps nginx
  "${DOCKER[@]}" exec jerocine_nginx nginx -t
  "${DOCKER[@]}" exec jerocine_nginx nginx -s reload

  # 5) 清缓存(index.html 已 no-cache, 这里清的是 API 缓存; 与既有部署行为一致)
  clear_caches

  echo "==> 完成: 前端已发布 $ts"
  "${DOCKER[@]}" ps --format '{{.Names}}\t{{.Status}}' | grep jerocine || true
  echo "   查看版本: ls -1 data/releases ; 回滚: ./deploy.sh web-rollback <TS>"
}

# 一次性: 把旧容器镜像里的产物搬到宿主机挂载目录(必须在切挂载之前执行)
web_init() {
  if [ -f data/html/index.html ]; then
    echo "==> data/html 已初始化, 跳过"
    return 0
  fi
  echo "==> 从运行中的 jerocine_nginx 容器复制现有产物到 data/html"
  if ! "${DOCKER[@]}" inspect jerocine_nginx >/dev/null 2>&1; then
    wr_die "找不到 jerocine_nginx 容器; 若线上还没跑过, 直接 ./deploy.sh web 即可"
  fi
  mkdir -p data/html
  "${DOCKER[@]}" cp jerocine_nginx:/usr/share/nginx/html/. data/html/
  # docker cp 产物属主是 root(见 chown_to_me 注释): 不归一化则下面删 apk 副本会 Permission denied,
  # 紧接着的 ./deploy.sh web 覆盖写 html/index.html 也会被拒。
  chown_to_me data/html
  if [ ! -f data/html/index.html ]; then
    echo "!! 容器内没有前端产物(可能已是新版镜像 / 已切挂载)。" >&2
    echo "   直接跑 ./deploy.sh web 用构建产物初始化挂载目录即可。" >&2
    exit 1
  fi
  # 删掉 docker cp 带出的 apk 副本, 并写出初始根清单(详见 lib/web-release.sh 的 wr_init_after_copy)
  wr_init_after_copy data
  echo "   已复制(不含 apk 副本):"
  ls -1 data/html | sed 's/^/     /'
  # 旧版(没有版本目录的年代)的 assets/* 不归新发布管: 清单恒不含 assets/, 也不会被自动清理。
  # 不自动删 —— 这些文件不是新流程产出的, 删了无法回滚; 只提示, 由运维确认后处置。
  if [ -d data/html/assets ]; then
    local legacy
    legacy="$(ls -1 data/html/assets 2>/dev/null | grep -vE "$WR_TS_RE" | head -5 || true)"
    if [ -n "$legacy" ]; then
      echo "   注意: assets/ 下有旧版(非版本目录)产物, 新方案不会引用它们:"
      echo "$legacy" | sed 's/^/     /'
      echo "         确认新版上线正常后可手动删除(它们不在自动清理范围内)"
    fi
  fi
  echo "==> 初始化完成。接着跑 ./deploy.sh web 发布首个带 Service Worker 的版本"
}

# 整版回滚: 快照三件套(index.html + sw.js + workbox-*.js) 一起覆盖回根, 再 reload
web_rollback_cmd() {
  local ts="${1:-}"
  if [ -z "$ts" ]; then
    echo "可用版本(新 → 旧):"
    ls -1 data/releases 2>/dev/null | sort -r | sed 's/^/  /' || true
    echo "用法: ./deploy.sh web-rollback <TS>" >&2
    exit 1
  fi
  wr_rollback data "$ts"
  "${DOCKER[@]}" exec jerocine_nginx nginx -t
  "${DOCKER[@]}" exec jerocine_nginx nginx -s reload
  clear_caches
  echo "==> 回滚完成: $ts（客户端 sw.js 字节变化后会重装该版 SW, 与旧 index.html 自洽）"
}

# ============================ 原有服务部署 ============================

deploy_services() {
  local services=("$@")
  if [ ${#services[@]} -eq 0 ]; then services=(server nginx); fi

  # 静态目录已改宿主机挂载(镜像内不再 COPY dist): 挂载点为空的 nginx 会把站点直接打成 404。
  # 本子命令**不具备**发布前端的能力 ⇒ 挂载目录为空时直接拒绝, 并指路 `web`(只有它会写挂载目录)。
  # (只更新 server 时不重建 nginx, 不拦。)
  local s touches_nginx=""
  for s in "${services[@]}"; do
    if [ "$s" = "nginx" ]; then touches_nginx=1; break; fi
  done
  if [ -n "$touches_nginx" ] && ! web_mount_ready; then
    echo "!! data/html 尚未初始化 —— 镜像已不再内置前端产物, 重建 nginx 会让线上 404。" >&2
    echo "   首次安装 / 修复: ./deploy.sh web（构建产物并写入挂载目录）" >&2
    if web_legacy_html_live; then
      echo "   线上旧容器还在服务镜像内产物, 想先保住现状再切挂载: ./deploy.sh web-init" >&2
    fi
    exit 1
  fi

  pull_and_reexec

  echo "==> compose up -d --build: ${services[*]}"
  # 纯前端必须 --no-deps: nginx depends_on server, 不带会连带重启后端打断采集
  if [ "${services[*]}" = "nginx" ]; then
    "${COMPOSE[@]}" up -d --build --no-deps nginx
  else
    "${COMPOSE[@]}" up -d --build "${services[@]}"
  fi

  # 等待 server 健康(纯 nginx 部署时容器本来就该 healthy, 快速通过)
  echo "==> 等待 jerocine_server healthy..."
  local ok=""
  for _ in $(seq 1 60); do
    local st
    st="$("${DOCKER[@]}" inspect --format '{{.State.Health.Status}}' jerocine_server 2>/dev/null || echo missing)"
    if [ "$st" = "healthy" ]; then ok=1; break; fi
    sleep 5
  done
  if [ -z "$ok" ]; then
    echo "!! jerocine_server 5 分钟未达 healthy, 请查日志: ${DOCKER[*]} logs jerocine_server" >&2
    exit 1
  fi

  clear_caches

  echo "==> 完成: ${services[*]} 已部署, 健康检查通过, 两层缓存已清"
  "${DOCKER[@]}" ps --format '{{.Names}}\t{{.Status}}' | grep jerocine
}

# 清两层缓存: nginx API 代理缓存 + Redis 应用缓存(DB0; DB1 是协调态, 禁动)
clear_caches() {
  echo "==> 清 nginx proxy_cache"
  "${DOCKER[@]}" exec jerocine_nginx sh -c 'rm -rf /var/cache/nginx/api/* 2>/dev/null; nginx -s reload 2>/dev/null || true'

  echo "==> 清 Redis 应用缓存(DB0)"
  local rp
  rp="$(grep -E '^REDIS_PASSWORD=' .env | head -1 | cut -d= -f2-)"
  "${DOCKER[@]}" exec -e REDISCLI_AUTH="$rp" jerocine_redis redis-cli --no-auth-warning -n 0 FLUSHDB >/dev/null
}

# ============================ 子命令分发 ============================
# 必须放在所有函数定义之后(bash 顺序执行, 提前调用会 command not found)

cmd="${1:-}"
case "$cmd" in
  web)          shift; deploy_web "$@" ;;
  web-init)     shift; web_init "$@" ;;
  web-rollback) shift; web_rollback_cmd "$@" ;;
  *)            deploy_services "$@" ;;
esac

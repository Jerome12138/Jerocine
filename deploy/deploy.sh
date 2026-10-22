#!/usr/bin/env bash
# Jerocine 一键部署
#
# 用法(在服务器或本机都可, 脚本自动定位 deploy/ 目录):
#   ./deploy.sh server-deploy <pkg> # 【常规后端发布】解包本地上传的产物包 → 薄运行镜像 → 重建容器
#   ./deploy.sh web-deploy <pkg>    # 【常规前端发布】解包本地上传的产物包 → 同步 data/html → reload
#   ./deploy.sh web-rollback <TS>   # 整版回滚(TS 见 data/releases/)
#   ./deploy.sh nginx               # 只重建 nginx 容器(树上改 nginx.conf 时用), **不发前端产物**
#   ./deploy.sh web-init            # 【一次性】切挂载前, 把现有 /usr/share/nginx/html 复制到 data/html
#
# 只有一条发布路径: 产物包。产物在本机(scripts/build-web.sh / scripts/build-server.sh)构建并
# 打包成 tar.gz, 上传到 deploy/incoming/ 后由本脚本解包发布 —— 服务器因此**不需要**
# node/pnpm/golang 与构建缓存(以前那 13GB docker build cache 主要就是树上构建攒出来的),
# 且上传包会留档到 data/packages/ 供事后重发/回滚(秒级, 与 git 无关)。
# 应急(服务器自行构建, 不建议 —— 编译抢占线上内存/CPU): 用容器跑 scripts/build-*.sh 产出
# 同格式包, 再走 *-deploy, 见 docs/部署指南.md「应急」一节。
#
# 部署脚本自身怎么到服务器上(2026-10-22 起, 模式1 自包含):
#   *-deploy 的产物包内带 tooling/(deploy.sh + lib + compose + Dockerfile + nginx.conf,
#   由 scripts/build-{server,web}.sh 打包)。deploy.sh 检测到它, 就同步到固定目录
#   deploy/runtime/(gitignored) 并**以包内版本重新执行自身** —— 施工工具与二进制同包同版,
#   于是模式1(本地构建 → 上传包 → 发布)不再需要服务器 git 树保持最新:
#     · 包内有 tooling/           ⇒ 自引导到 runtime/ 执行, 全程 0 次 git pull;
#     · 包内没有 tooling/(旧格式包, 回滚场景) ⇒ legacy: git pull + 树内执行(与从前一致)。
#
# `nginx`(树上运维)与发布路径的关系: 静态目录是宿主机挂载 `./data/html`, nginx 容器重建
# 不产出前端文件 —— 前端发布必须走 `web-deploy`(具备"保留 3 版 / 整版回滚 / 先写目录最后 reload")。
# 两者都会在 data/html 未初始化时拒绝执行(否则挂载点为空的 nginx 直接把站点打成 404)。
#
# 发布收尾自动完成: 等待健康 → 清两层缓存 (nginx proxy_cache + Redis DB0)。
# 采集无需手动暂停: server 收到 SIGTERM 会优雅停机(在跑页记失败台账, 增量窗口滚动自愈),
# 新容器起来后调度器自动恢复下一轮增量采集。
set -euo pipefail

# Git Bash(MSYS) 会把以 / 开头的参数当 Windows 路径改写(如 docker cp 的容器内路径) —— 关掉。
# Linux 上该变量无意义, 无副作用。
export MSYS_NO_PATHCONV=1

# 自身所在目录 + 绝对路径 + 原始参数。目录必须在 cd 之前取: $0 可能是相对**调用方** cwd 的
# 路径(如 bash ../deploy/deploy.sh), cd 进 deploy/ 之后它就不再指向本文件了。
# SELF_DIR 还用来定位 lib/: 包内工具链(deploy/runtime/)必须 source 自带的 lib, 与 deploy.sh
# 同包同版 —— 用 cwd 定位会在树内 lib 落后时静默拿到旧版本。
SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
SELF="$SELF_DIR/$(basename "$0")"
SCRIPT_ARGS=("$@")

# 防误用: 手动执行 deploy/runtime/ 里的副本、却没走自引导(没有 JEROCINE_DEPLOY_ROOT) ⇒
# 后面所有相对路径(data/ .env incoming/)都会指错地方, 不如在这里就拒绝并指路。
if [ -z "${JEROCINE_DEPLOY_ROOT:-}" ] && [ "$(basename "$SELF_DIR")" = "runtime" ]; then
  echo "!! 这是随产物包上送的工具链副本(deploy/runtime/), 不能直接运行 —— 它缺少" >&2
  echo "   JEROCINE_DEPLOY_ROOT 上下文(相对路径会指错)。模式1 发布请在 deploy/ 下运行:" >&2
  echo "   ./deploy.sh server-deploy|web-deploy <包>  (它会自动用包内 tooling 自引导)" >&2
  exit 1
fi

# 运行时根目录: 树内执行 = 本脚本所在目录(deploy/); 包内工具链执行 = JEROCINE_DEPLOY_ROOT
# (由 jc_bootstrap_tooling 导出, 见下)。两者都 cd 过去, 于是 data/ .env incoming/ 等
# 相对路径的语义在两种模式下完全一致。
if [ -n "${JEROCINE_DEPLOY_ROOT:-}" ]; then
  cd "$JEROCINE_DEPLOY_ROOT"
else
  cd "$SELF_DIR"   # 总在 deploy/ 下执行, compose 相对路径才正确
fi

# Web 发布/回滚的纯文件系统逻辑(含本机单测: tests/web-release.test.sh)
# shellcheck source=lib/web-release.sh
source "$SELF_DIR/lib/web-release.sh"
# shellcheck source=lib/db-migrate.sh
source "$SELF_DIR/lib/db-migrate.sh"

# docker 权限: 可直连用 docker, 否则回退 sudo -n (服务器约定)
if docker ps >/dev/null 2>&1; then DOCKER=(docker); else DOCKER=(sudo -n docker); fi
# 包内工具链模式(由 jc_bootstrap_tooling 置位): compose 文件用包内副本
# (deploy/runtime/docker-compose.yml) —— 它可能比树里版本新, 这正是随包上送的意义;
# compose 里的相对挂载已写成 ${JEROCINE_DEPLOY_ROOT:-.}/..., 这里 export 成绝对根即可正确解析。
# nginx 镜像构建同步切到随包上送的薄配方(deploy/Dockerfile.nginx, 上下文 = deploy/runtime/)。
if [ -n "${JEROCINE_DEPLOY_ROOT:-}" ]; then
  RUNTIME_DF="runtime/Dockerfile.runtime"
  export JEROCINE_NGINX_CONTEXT="$JEROCINE_DEPLOY_ROOT/runtime"
  export JEROCINE_NGINX_DOCKERFILE="Dockerfile.nginx"
  # ⚠️ sudo 剥环境变量: 服务器 docker 走 `sudo -n docker`(见上), sudo 默认 env_reset,
  # export 的 JEROCINE_* 到不了 compose 进程 ⇒ YAML 里 ${JEROCINE_DEPLOY_ROOT:-.} 回退成
  # `.`、按 compose 文件所在目录解析成 runtime/data/migrations(空目录) —— 迁移必挂,
  # nginx 构建上下文同理(2026-10-22 首次模式1 发布实测)。解法: 把三个值落盘成 compose
  # 专用 env 文件 —— sudo 剥不掉磁盘文件; 排在 --env-file .env 之后(后读覆盖前者)。
  # shell 环境的 export 仍保留: 不经 sudo 的环境(本机/用户在 docker 组)行为不变。
  printf 'JEROCINE_DEPLOY_ROOT=%s\nJEROCINE_NGINX_CONTEXT=%s\nJEROCINE_NGINX_DOCKERFILE=%s\n' \
    "$JEROCINE_DEPLOY_ROOT" "$JEROCINE_NGINX_CONTEXT" "$JEROCINE_NGINX_DOCKERFILE" \
    > "$JEROCINE_DEPLOY_ROOT/runtime/deploy.env"
  COMPOSE=("${DOCKER[@]}" compose --env-file .env --env-file "$JEROCINE_DEPLOY_ROOT/runtime/deploy.env" -f runtime/docker-compose.yml)
else
  RUNTIME_DF="Dockerfile.runtime"
  COMPOSE=("${DOCKER[@]}" compose --env-file .env)
fi

# ============================ git pull 与"自我重启" ============================

# 拉取最新代码, 然后**重新 exec 本脚本一次**。
#
# 为什么必须重启: `git -C .. pull` 会覆写本文件, 而 bash 是"边执行边按偏移读取脚本"的 ——
# 拉到新版本后, 后面的语句可能来自"半新半旧"的文件(轻则行为不一致, 重则语法错误直接崩)。
# 重新 exec 一次(带防重入标记)才能保证后续流程一定跑在刚拉到的版本上。
# 代价: 一次进程启动 + 一轮重复日志输出; 换来"部署脚本自身也能随 push 热更新"。
pull_and_reexec() {
  if [ -n "${JEROCINE_DEPLOY_ROOT:-}" ]; then
    # 正运行随包上送的工具链(deploy/runtime/): 它与二进制同包同版本, 已是最新 ——
    # 模式1 发布到此断开 git 货币(服务器树落后/超前都不影响发版)。
    echo "==> 工具链来自产物包(deploy/runtime/) —— 跳过 git pull"
    return 0
  fi
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

# ============================ 模式1 自包含: 工具链随包自引导 ============================
#
# 背景: 产物(二进制 / 迁移 / dist)与"施工工具"(deploy.sh / lib / compose / Dockerfile)曾经是
# 两条腿 —— 后者靠每次发布前 git pull 更新, 于是"改过部署脚本的发版必须先 push 再发布",
# 且服务器 git 树必须保持最新。现在 scripts/build-{server,web}.sh 把施工工具打进产物包的
# tooling/; 本函数检测到它, 就同步到固定目录 deploy/runtime/(gitignored) 并**以包内版本
# 重新执行自身**。
#
# 为什么是重新 exec 而不是继续跑: 与 pull_and_reexec 同样的理由 —— 保证后续流程一定运行
# 在刚同步的工具链版本上, 而不是"半新半旧的树内副本"。
#
# 包内没有 tooling/(旧格式产物包, 例如回滚到自动迁移上线前打的包) ⇒ 直接返回, 调用方走
# legacy: git pull + 树内执行, 行为与从前完全一致。
jc_bootstrap_tooling() {
  if [ -n "${JEROCINE_DEPLOY_ROOT:-}" ]; then return 0; fi   # 已在包内工具链里跑, 不重复自引导
  local pkg="$1" cmd="$2"; shift 2   # 其余参数原样透传(如 web-deploy 的可选 [TS]), 否则重 exec 后静默丢失
  local list ctx
  # 包内有没有 tooling/。刻意不用 `tar -tzf | grep -q`: grep 提前退出会让 tar 收 SIGPIPE,
  # 叠加 pipefail 后"明明有"反而判没有, 且结果随条目在包里的位置漂移(踩过, 见
  # build-server.sh 的 pkg_has_entry 注释)。这里用 shell 通配, 零子进程风险。
  list="$(tar -tzf "$pkg" 2>/dev/null || true)"
  case "$list" in
    *./tooling/*) ;;          # 新模式1 包: 继续走自引导
    *) return 0 ;;            # 旧格式包(回滚场景): 无 tooling/, 走 legacy
  esac

  ctx="$(cd "$(mktemp -d)" && pwd)"
  # 只解 tooling/ 成员(GNU tar 对目录成员递归解包), 不整体解包 —— server 包 30MB+, 没必要
  tar xzf "$pkg" -C "$ctx" ./tooling || { rm -rf "$ctx"; wr_die "解包 tooling/ 失败: $pkg"; }
  [ -f "$ctx/tooling/deploy.sh" ] || { rm -rf "$ctx"; wr_die "tooling/ 里没有 deploy.sh —— 产物包不完整?"; }

  # 整目录替换: 旧 tooling 的文件不残留(例如某个 lib 文件在新版里被删掉)。
  # sudo 兜底: docker(root) 可能往 runtime/ 里自动创建过挂载点目录(踩过: 迁移
  # 挂载解析错时建出 runtime/data/migrations), root 属主普通用户删不掉;
  # 服务器 docker 全程走 sudo -n, 这里同样用免密 sudo 清理, 没有就报错给手动命令。
  rm -rf runtime 2>/dev/null || true
  if [ -d runtime ]; then
    sudo -n rm -rf runtime || wr_die \
      "旧 runtime/ 含 root 属主残留且免密 sudo 不可用 —— 手动清理后重跑: sudo rm -rf '$(pwd)/runtime'"
  fi
  cp -R "$ctx/tooling" runtime
  chmod +x runtime/deploy.sh 2>/dev/null || true
  rm -rf "$ctx"

  echo "==> 产物包自带部署工具链(tooling/) ⇒ 同步到 $(pwd)/runtime, 并以该版本执行(本发布起不依赖 git)"
  export JEROCINE_DEPLOY_ROOT="$(pwd)"
  # 参数原样透传: pkg 路径的解析语义与 legacy 一致(两边都 cd 到同一个 deploy/)。
  exec bash "runtime/deploy.sh" "$cmd" "$pkg" "$@"
}

# ============================ Web 发布(v2: 挂载 + 版本目录) ============================
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

# 空挂载守卫(web / web-deploy 共用): 只有"线上旧容器仍在服务镜像内产物、而挂载目录是空的"
# 才会被本次发布打成 404。
#   - 迁移场景(旧容器还活着且镜像里有产物) ⇒ 必须先 web-init 把现有产物搬到挂载目录;
#   - 首次安装(没有容器 / 挂载本来就是空) ⇒ 放行, 由本次发布完成初始化。
#     (否则会与 web-init 形成死锁: web 让你先跑 web-init, web-init 说没有容器就跑 web。)
require_web_mount_safe() {
  if ! web_mount_ready && web_legacy_html_live; then
    echo "!! data/html 尚未初始化, 而线上旧容器仍在服务镜像内的产物。" >&2
    echo "   现在发布会让线上 404。请先执行(必须在旧容器还活着时): ./deploy.sh web-init" >&2
    exit 1
  fi
}

# 一次性: 把旧容器镜像里的产物搬到宿主机挂载目录(必须在切挂载之前执行)
web_init() {
  if [ -f data/html/index.html ]; then
    echo "==> data/html 已初始化, 跳过"
    return 0
  fi
  echo "==> 从运行中的 jerocine_nginx 容器复制现有产物到 data/html"
  if ! "${DOCKER[@]}" inspect jerocine_nginx >/dev/null 2>&1; then
    wr_die "找不到 jerocine_nginx 容器; 若线上还没跑过, 直接发布一个前端产物包即可(./deploy.sh web-deploy <pkg>)"
  fi
  mkdir -p data/html
  "${DOCKER[@]}" cp jerocine_nginx:/usr/share/nginx/html/. data/html/
  # docker cp 产物属主是 root(见 chown_to_me 注释): 不归一化则下面删 apk 副本会 Permission denied,
  # 紧接着的 web-deploy 覆盖写 html/index.html 也会被拒。
  chown_to_me data/html
  if [ ! -f data/html/index.html ]; then
    echo "!! 容器内没有前端产物(可能已是新版镜像 / 已切挂载)。" >&2
    echo "   直接发布一个前端产物包初始化挂载目录即可: ./deploy.sh web-deploy <pkg>" >&2
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
  echo "==> 初始化完成。接着发布前端产物包: ./deploy.sh web-deploy <pkg>"
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

# 常规前端发布: 解包本地上传的产物包 → 走同一套发布语义 → 留档上传包 → reload
#
# 用法: ./deploy.sh web-deploy <pkg.tar.gz> [TS]
#   包由本机 scripts/build-web.sh 生成(jerocine-web-<TS>.tar.gz), 由 scripts/deploy-web.sh
#   上传到 deploy/incoming/ 后调本命令。TS 缺省从包内 assets/<TS>/ 推(权威来源), 也可显式给。
deploy_web_from_pkg() {
  local pkg="${1:-}" ts_arg="${2:-}"
  if [ -z "$pkg" ]; then
    echo "用法: ./deploy.sh web-deploy <jerocine-web-<TS>.tar.gz> [TS]" >&2
    echo "  (产物包在本机生成: scripts/build-web.sh; 上传+远程调用: scripts/deploy-web.sh)" >&2
    exit 1
  fi
  [ -f "$pkg" ] || wr_die "找不到产物包: $pkg"

  # 模式1 自包含: 包内带 tooling/ ⇒ 同步到 deploy/runtime/ 并以包内版本重新执行本命令
  # (不再返回); 旧格式包(无 tooling/, 回滚场景) ⇒ 返回, 继续走下面的 legacy 路径。
  # "${@:2}": web-deploy 的可选 [TS] 等附加参数必须透传, 否则自引导后静默丢失。
  jc_bootstrap_tooling "$pkg" web-deploy "${@:2}"

  require_web_mount_safe

  # legacy 路径才需要 pull: 服务器侧的 deploy.sh / lib / nginx.conf 也要跟着走
  # (上传包放在未被跟踪的 incoming/ 里, git pull 不会动它)。
  # 包内 tooling 已被上面自引导接管, pull_and_reexec 会自行跳过。
  pull_and_reexec

  local ts tmp pkg_ts
  tmp="$(cd "$(mktemp -d)" && pwd)"
  wr_extract_pkg "$pkg" "$tmp"
  ts="$ts_arg"
  [ -n "$ts" ] || ts="$(wr_ts_from_dist "$tmp")"
  # 包名与发布版本不一致时说一声(多半是传错了文件; 显式传了 [TS] 时以它为准)
  if pkg_ts="$(wr_pkg_ts "$pkg")" && [ "$pkg_ts" != "$ts" ]; then
    echo "!! 包名版本($pkg_ts) 与发布版本($ts) 不一致, 以发布版本为准" >&2
  fi

  echo "==> 发布上传产物包 $pkg (版本 $ts)"
  wr_sync_dist "$tmp" "data" "$ts" "../LICENSE"
  rm -rf "$tmp"
  wr_archive_pkg "$pkg" "data" "$ts"

  # 应用挂载(配置首次变更时重建容器) + 校验配置 + reload —— 最后一步才让新版本可见
  echo "==> 应用挂载并 reload nginx"
  "${COMPOSE[@]}" up -d --build --no-deps nginx
  "${DOCKER[@]}" exec jerocine_nginx nginx -t
  "${DOCKER[@]}" exec jerocine_nginx nginx -s reload

  clear_caches

  echo "==> 完成: 前端已发布 $ts"
  "${DOCKER[@]}" ps --format '{{.Names}}\t{{.Status}}' | grep jerocine || true
  echo "   产物包留档: data/packages/$ts.tar.gz ; 回滚: ./deploy.sh web-rollback <TS>"
}

# 常规后端发布: 解包本地上传的产物包 → 构建薄运行镜像 → 重建容器 → 等健康 → 清缓存
#
# 用法: ./deploy.sh server-deploy <pkg.tar.gz>
#   包由本机 scripts/build-server.sh 交叉编译生成
#   (jerocine-server-<TS>-<sha7>-linux-<arch>.tar.gz), 由 scripts/deploy-server.sh 上传后调本命令。
#   镜像用 deploy/Dockerfile.runtime(只 COPY 二进制, 容器内不再编译)。
# 后端上传包留档(与 web-deploy 的 wr_archive_pkg 对等): 复制进 data/packages/,
# 保留 WR_KEEP_PACKAGES 份(只按 server 包名裁剪, 不碰 web 包)。
# 与 web 包的差别: 后端包名自带 TS+sha+arch, 直接用原文件名, 不改写。
archive_server_pkg() {
  local pkg="$1" dest old srcre
  mkdir -p data/packages
  dest="data/packages/$(basename "$pkg")"
  if [ "$(readlink -f "$pkg")" != "$(readlink -f "$dest")" ]; then
    cp -f "$pkg" "$dest"
  fi
  srcre='^jerocine-server-[0-9]{8}-[0-9]{6}-[0-9a-f]{7}-linux-(amd64|arm64)\.tar\.gz$'
  old="$(ls -1t data/packages/ 2>/dev/null | grep -E "$srcre" | tail -n +$((WR_KEEP_PACKAGES + 1)) || true)"
  if [ -n "$old" ]; then (cd data/packages && rm -f $old); fi
}

deploy_server_from_pkg() {
  local pkg="${1:-}"
  if [ -z "$pkg" ]; then
    echo "用法: ./deploy.sh server-deploy <jerocine-server-*.tar.gz>" >&2
    echo "  (产物包在本机生成: scripts/build-server.sh; 上传+远程调用: scripts/deploy-server.sh)" >&2
    exit 1
  fi
  [ -f "$pkg" ] || wr_die "找不到产物包: $pkg"

  # 模式1 自包含(同 web-deploy): 包内 tooling/ ⇒ 自引导到 deploy/runtime/ 并以包内版本
  # 重新执行本命令; 旧格式包 ⇒ 返回, 走 legacy(git pull + 树内执行)。
  jc_bootstrap_tooling "$pkg" server-deploy "${@:2}"

  pull_and_reexec

  local ctx
  ctx="$(cd "$(mktemp -d)" && pwd)"
  wr_extract_tar "$pkg" "$ctx"
  [ -f "$ctx/main" ] || wr_die "包内没有 main —— 用 scripts/build-server.sh 打包"
  [ -d "$ctx/data" ] || wr_die "包内没有 data/（IP 归属地离线库）"
  [ -d "$ctx/static/upload" ] || wr_die "包内没有 static/upload/（upload 卷挂载点）"
  [ -f "$ctx/LICENSE" ] || wr_die "包内没有 LICENSE"
  # migrations/ 缺失 = 旧版产物包(自动迁移上线前打的包)。旧包与旧二进制同版, 对当前库
  # (迁移只加带默认值的列, 只多不少)是兼容的 ⇒ 响亮跳过而非阻断 —— 否则"重新部署旧包"
  # 这条唯一的 server 回滚路径会被堵死(线上 data/packages/ 就躺着 3 个旧格式包)。

  echo "==> 构建后端运行镜像(内置本机编译的二进制, 容器内不再编译)"
  # 上下文 = 解包目录(布局见 deploy/Dockerfile.runtime); -f 相对 deploy/(cwd)。
  # 包内工具链模式下 Dockerfile 也用包内副本(runtime/Dockerfile.runtime, 见顶部 RUNTIME_DF)。
  "${DOCKER[@]}" build -f "$RUNTIME_DF" -t jerocine-server:latest "$ctx"
  # 迁移先于新二进制运行(顺序不能反): 失败则中止发布, 线上保持旧版本(镜像已 build, 无副作用)。
  # 无新迁移时 golang-migrate up 幂等秒退, 不会动库。迁移文件与二进制同包同版,
  # 同步到 data/migrations(compose migrate 服务挂载它, 不再依赖服务器 git 工作树)。
  if [ -d "$ctx/migrations" ]; then
    if ! run_db_migrations "$ctx/migrations" "data/migrations"; then
      echo "!! 发布已中止 —— 线上仍是旧版本; 排查: ${COMPOSE[*]} logs migrate" >&2
      rm -rf "$ctx"
      exit 1
    fi
  else
    echo "!! 包内没有 migrations/（旧版产物包）—— 跳过自动迁移" >&2
    echo "   仅当确认该包与当前数据库结构配套(例如回滚到旧版)时才应继续发布" >&2
  fi
  rm -rf "$ctx"

  # --no-build: compose 的 server 服务没有 build 段 —— 镜像只由本命令按产物包构建
  echo "==> 重建 jerocine_server(--no-build)"
  "${COMPOSE[@]}" up -d --no-build server

  if ! wait_for_server_healthy; then
    echo "   修复迁移问题后重跑本命令(迁移幂等); 历史包回滚: server-deploy data/packages/<旧包>" >&2
    exit 1
  fi

  clear_caches

  archive_server_pkg "$pkg"
  echo "==> 完成: 后端已发布(镜像 jerocine-server:latest 由 $pkg 构建)"
  echo "   产物包留档: data/packages/$(basename "$pkg") （保留 $WR_KEEP_PACKAGES 份）"
  "${DOCKER[@]}" ps --format '{{.Names}}\t{{.Status}}' | grep jerocine || true
}

# ============================ nginx 重建(树上运维) ============================

# 等 jerocine_server 达到 healthy（超时打印日志提示并返回 1）。deploy_nginx 与 server-deploy 共用。
wait_for_server_healthy() {
  echo "==> 等待 jerocine_server healthy..."
  local st i
  for i in $(seq 1 60); do
    st="$("${DOCKER[@]}" inspect --format '{{.State.Health.Status}}' jerocine_server 2>/dev/null || echo missing)"
    if [ "$st" = "healthy" ]; then
      echo "==> jerocine_server healthy(第 $i 次探测)"
      return 0
    fi
    printf '  [%03ds] health=%s\n' $((i * 5)) "$st"
    sleep 5
  done
  echo "!! jerocine_server 5 分钟未达 healthy, 请查日志: ${DOCKER[*]} logs jerocine_server" >&2
  return 1
}

# 手工改过树内 nginx.conf 后的运维小工具: 按树内 web/Dockerfile(运行配方)重建 nginx 容器。
# ⚠️ 只重建容器, **不发布前端产物**(html 走挂载); 日常也不需要它 —— nginx.conf 的正式变更
# 随产物包 tooling/ 走(web-deploy 会用包内配方重建 nginx)。树上手工改配置属于临时绕过,
# 记得改回公开仓并提交, 否则下次随包发布会把它冲掉。
deploy_nginx() {
  # 挂载点为空的 nginx 会把站点直接打成 404: 首次安装先发一个前端产物包, 或用 web-init 修复。
  if ! web_mount_ready; then
    echo "!! data/html 尚未初始化 —— 重建 nginx 会让线上 404。" >&2
    echo "   首次安装: ./deploy.sh web-deploy <pkg>（写挂载目录）; 修复挂载: ./deploy.sh web-init" >&2
    if web_legacy_html_live; then
      echo "   线上旧容器还在服务镜像内产物, 想先保住现状再切挂载: ./deploy.sh web-init" >&2
    fi
    exit 1
  fi

  pull_and_reexec

  echo "==> compose up -d --build nginx(按树内 web/Dockerfile 运行配方)"
  # --no-deps: nginx depends_on server, 不带会连带重启后端打断采集
  "${COMPOSE[@]}" up -d --build --no-deps nginx
  "${DOCKER[@]}" exec jerocine_nginx nginx -t

  # server 容器未被触碰, 本来就该 healthy(快速通过; 顺便确认没被误伤)
  wait_for_server_healthy || exit 1

  clear_caches

  echo "==> 完成: nginx 已重建, 健康检查通过, 两层缓存已清"
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

# 包内工具链(deploy/runtime/)只承担产物包发布与挂载目录相关操作 —— 这些不碰 git 树;
# nginx 重建需要树上构建上下文, 必须在树内执行(指路回树内脚本)。
if [ -n "${JEROCINE_DEPLOY_ROOT:-}" ]; then
  case "$cmd" in
    server-deploy|web-deploy|web-rollback|web-init) ;;
    *)
      echo "!! '$cmd' 需要服务器 git 树(源码 / 树上构建) —— 请运行树内的 ./deploy.sh" >&2
      echo "   deploy/runtime/ 是随产物包上送的施工工具, 只用于产物包发布(*-deploy 等)" >&2
      exit 1 ;;
  esac
fi

case "$cmd" in
  web-deploy)    shift; deploy_web_from_pkg "$@" ;;
  web-init)      shift; web_init "$@" ;;
  web-rollback)  shift; web_rollback_cmd "$@" ;;
  server-deploy) shift; deploy_server_from_pkg "$@" ;;
  nginx)         deploy_nginx ;;
  *)
    echo "用法: ./deploy.sh <子命令> [参数]" >&2
    echo "  日常发布(产物包, 推荐): server-deploy|web-deploy <jerocine-*.tar.gz>" >&2
    echo "  运维: web-rollback <TS> · web-init · nginx(树上改 nginx.conf 后重建容器)" >&2
    echo "  应急(服务器自行构建, 不建议): 用容器跑 scripts/build-*.sh 产包后走 *-deploy" >&2
    echo "                                —— 见 docs/部署指南.md「应急」一节" >&2
    exit 1 ;;
esac

#!/usr/bin/env bash
# 本机上传后端产物包并触发服务器发布 —— 常规后端发布的第二步(与 scripts/build-server.sh 配套)。
#
# 链路: 本机交叉编译打包(build-server.sh) → scp 上传到 <deploy>/incoming/ → ssh 执行
#       ./deploy.sh server-deploy <远端包路径>(解包 → 构建薄运行镜像 → 重建容器 → 等健康 → 清缓存)
#
# 用法:
#   bash scripts/deploy-server.sh                        # 构建 + 上传 + 发布
#   bash scripts/deploy-server.sh --pkg <tgz>            # 复用已构建好的产物包(跳过编译)
#   bash scripts/deploy-server.sh --arch arm64           # 指定目标架构(透传给 build-server.sh)
#
# 环境变量(也可用同名参数覆盖):
#   JEROCINE_SSH_HOST    ssh 主机/别名(必填, 例如 jerocine)
#   JEROCINE_DEPLOY_DIR  远端 deploy/ 目录(默认 /home/ubuntu/jerocine/deploy)
#   JEROCINE_INCOMING_DIR 远端上传落点(默认 <deploy>/incoming)
set -euo pipefail

export MSYS_NO_PATHCONV=1

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"

info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[31m[错误]\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
上传后端产物包并在服务器上发布(常规后端发布手段)

用法:
  bash scripts/deploy-server.sh                    # 构建 + 上传 + 发布
  bash scripts/deploy-server.sh --pkg <tgz>        # 复用已构建好的产物包(跳过编译)
  bash scripts/deploy-server.sh --arch arm64       # 指定目标架构

参数:
  --host <ssh>       ssh 主机/别名(默认取 JEROCINE_SSH_HOST)
  --deploy-dir <d>   远端 deploy/ 目录(默认取 JEROCINE_DEPLOY_DIR)
  --pkg <tgz>        直接给产物包(跳过编译)
  --arch <a>         目标架构 amd64|arm64(透传给 build-server.sh)

环境变量:
  JEROCINE_SSH_HOST      ssh 主机/别名(必填)
  JEROCINE_DEPLOY_DIR    远端 deploy/ 目录(默认 /home/ubuntu/jerocine/deploy)
  JEROCINE_INCOMING_DIR  远端上传落点(默认 <deploy>/incoming)
  JEROCINE_DOWNLOAD_DIR  本机构建产物落地目录(透传给 build-server.sh)
EOF
}

HOST="${JEROCINE_SSH_HOST:-}"
DEPLOY_DIR="${JEROCINE_DEPLOY_DIR:-/home/ubuntu/jerocine/deploy}"
INCOMING="${JEROCINE_INCOMING_DIR:-}"
PKG=""
ARCH=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --host)       HOST="${2:-}"; shift 2 ;;
    --host=*)     HOST="${1#*=}"; shift ;;
    --deploy-dir) DEPLOY_DIR="${2:-}"; shift 2 ;;
    --deploy-dir=*) DEPLOY_DIR="${1#*=}"; shift ;;
    --incoming-dir) INCOMING="${2:-}"; shift 2 ;;
    --incoming-dir=*) INCOMING="${1#*=}"; shift ;;
    --pkg)        PKG="${2:-}"; shift 2 ;;
    --pkg=*)      PKG="${1#*=}"; shift ;;
    --arch)       ARCH="${2:-}"; shift 2 ;;
    --arch=*)     ARCH="${1#*=}"; shift ;;
    -h|--help)    usage; exit 0 ;;
    *) die "未知参数: $1（--help 看用法）" ;;
  esac
done

[ -n "$HOST" ] || die "缺少 ssh 主机：用 --host <ssh> 或设 JEROCINE_SSH_HOST(例如 jerocine)"
[ -n "$INCOMING" ] || INCOMING="$DEPLOY_DIR/incoming"

# ---------------------------------------------------------------- 1. 编译打包(可选)

if [ -z "$PKG" ]; then
  info "本机交叉编译并打包后端"
  if [ -n "$ARCH" ]; then
    PKG="$(bash "$HERE/build-server.sh" --arch "$ARCH" | sed -n 's/^产物包: \([^ ]*\).*/\1/p' | tail -1)"
  else
    PKG="$(bash "$HERE/build-server.sh" | sed -n 's/^产物包: \([^ ]*\).*/\1/p' | tail -1)"
  fi
  [ -n "$PKG" ] || die "没能从编译输出里解析出产物包路径"
fi

[ -f "$PKG" ] || die "找不到产物包: $PKG"
PKG_ABS="$(cd "$(dirname "$PKG")" && pwd)/$(basename "$PKG")"
PKG_NAME="$(basename "$PKG_ABS")"

# ---------------------------------------------------------------- 2. 上传

info "准备远端目录 $HOST:$INCOMING"
ssh "$HOST" "mkdir -p '$INCOMING'"

info "上传 $PKG_NAME"
scp -q "$PKG_ABS" "$HOST:$INCOMING/$PKG_NAME"

# ---------------------------------------------------------------- 3. 远端发布

REMOTE_PKG="$INCOMING/$PKG_NAME"
info "远端发布: $DEPLOY_DIR → ./deploy.sh server-deploy $REMOTE_PKG"
ssh "$HOST" "cd '$DEPLOY_DIR' && ./deploy.sh server-deploy '$REMOTE_PKG'"

info "完成: $PKG_NAME 已发布"

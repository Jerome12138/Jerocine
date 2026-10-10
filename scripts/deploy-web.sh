#!/usr/bin/env bash
# 本机上传前端产物包并触发服务器发布 —— 常规前端发布的第二步(与 scripts/build-web.sh 配套)。
#
# 链路: 本机构建打包(build-web.sh) → SSH 上送到 <deploy>/incoming/ → ssh 执行
#       ./deploy.sh web-deploy <远端包路径>(解包 → 同一套发布语义 → 留档 data/packages/ → reload)
#
# 用法:
#   bash scripts/deploy-web.sh                       # 构建 + 上传 + 发布
#   bash scripts/deploy-web.sh --pkg <tgz>           # 复用已构建好的产物包(跳过构建)
#   bash scripts/deploy-web.sh --ts 20261010-1830    # 指定版本号(透传给 build-web.sh)
#
# 环境变量(也可用同名参数覆盖):
#   JEROCINE_SSH_HOST    ssh 主机/别名(必填, 例如 jerocine)
#   JEROCINE_DEPLOY_DIR  远端 deploy/ 目录(默认 /home/ubuntu/jerocine/deploy)
#   JEROCINE_INCOMING_DIR 远端上传落点(默认 <deploy>/incoming)
set -euo pipefail

# Git Bash(MSYS) 会改写以 / 开头的参数(远端路径) —— 关掉。Linux 上该变量无意义, 无副作用。
export MSYS_NO_PATHCONV=1

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPO="$(cd -- "$HERE/.." && pwd -P)"

info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[31m[错误]\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
上传前端产物包并在服务器上发布(常规前端发布手段)

用法:
  bash scripts/deploy-web.sh                     # 构建 + 上传 + 发布
  bash scripts/deploy-web.sh --pkg <tgz>         # 复用已构建好的产物包(跳过构建)
  bash scripts/deploy-web.sh --ts 20261010-1830  # 指定版本号

参数:
  --host <ssh>       ssh 主机/别名(默认取 JEROCINE_SSH_HOST)
  --deploy-dir <d>   远端 deploy/ 目录(默认取 JEROCINE_DEPLOY_DIR)
  --pkg <tgz>        直接给产物包(跳过构建)
  --ts <TS>          版本号(透传给 build-web.sh)

环境变量:
  JEROCINE_SSH_HOST      ssh 主机/别名(必填)
  JEROCINE_DEPLOY_DIR    远端 deploy/ 目录(默认 /home/ubuntu/jerocine/deploy)
  JEROCINE_INCOMING_DIR  远端上传落点(默认 <deploy>/incoming)
  JEROCINE_DOWNLOAD_DIR  本机构建产物落地目录(透传给 build-web.sh)
EOF
}

HOST="${JEROCINE_SSH_HOST:-}"
DEPLOY_DIR="${JEROCINE_DEPLOY_DIR:-/home/ubuntu/jerocine/deploy}"
INCOMING="${JEROCINE_INCOMING_DIR:-}"
PKG=""
TS=""

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
    --ts)         TS="${2:-}"; shift 2 ;;
    --ts=*)       TS="${1#*=}"; shift ;;
    -h|--help)    usage; exit 0 ;;
    *) die "未知参数: $1（--help 看用法）" ;;
  esac
done

[ -n "$HOST" ] || die "缺少 ssh 主机：用 --host <ssh> 或设 JEROCINE_SSH_HOST(例如 jerocine)"
[ -n "$INCOMING" ] || INCOMING="$DEPLOY_DIR/incoming"

# ---------------------------------------------------------------- 1. 构建(可选)

if [ -z "$PKG" ]; then
  info "本机构建产物包"
  if [ -n "$TS" ]; then
    PKG="$(bash "$HERE/build-web.sh" --ts "$TS" | sed -n 's/^产物包: \([^ ]*\).*/\1/p' | tail -1)"
  else
    PKG="$(bash "$HERE/build-web.sh" | sed -n 's/^产物包: \([^ ]*\).*/\1/p' | tail -1)"
  fi
  [ -n "$PKG" ] || die "没能从构建输出里解析出产物包路径"
fi

[ -f "$PKG" ] || die "找不到产物包: $PKG"
PKG_ABS="$(cd "$(dirname "$PKG")" && pwd)/$(basename "$PKG")"
PKG_NAME="$(basename "$PKG_ABS")"

# ---------------------------------------------------------------- 2. 上传
#
# 用 `cat <本地文件> | ssh host 'cat > 目标'` —— **不用 scp**：本仓库运维约定里 scp 是禁用的，
# 单向传文件的既有姿态就是"走 ssh stdin"(见 Harness `.agents/skills/jerocine-ops` §3 通道 B)。
# 为什么用 `cat |` 而不是更简洁的 `ssh ... < 本地文件`：后者让 ssh 直接把大文件当 stdin，
# 在带命令沙箱的环境里对 >1MB 的文件会被 SIGTERM；经一个 cat 中间进程则正常。
# 代价是没有 scp 自带的校验，所以下面显式比对字节数。

info "准备远端目录 $HOST:$INCOMING"
ssh "$HOST" "mkdir -p '$INCOMING'"

REMOTE_PKG="$INCOMING/$PKG_NAME"
info "上传 $PKG_NAME"
local_size="$(wc -c < "$PKG_ABS" | tr -d ' ')"
cat "$PKG_ABS" | ssh "$HOST" "cat > '$REMOTE_PKG'" || die "上传失败"
remote_size="$(ssh "$HOST" "wc -c < '$REMOTE_PKG'" | tr -d ' ')"
[ "$local_size" = "$remote_size" ] || die \
  "上传后大小不一致(本地 $local_size / 远端 $remote_size) —— 传输被截断，请重试"

# ---------------------------------------------------------------- 3. 远端发布

info "远端发布: $DEPLOY_DIR → ./deploy.sh web-deploy $REMOTE_PKG"
ssh "$HOST" "cd '$DEPLOY_DIR' && ./deploy.sh web-deploy '$REMOTE_PKG'"

info "完成: $PKG_NAME 已发布"

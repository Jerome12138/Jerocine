#!/usr/bin/env bash
# 本机上传前端产物包并触发服务器发布 —— 常规前端发布的第二步(与 scripts/build-web.sh 配套)。
#
# 链路: 本机构建打包(build-web.sh) → SSH 上送到 <deploy>/incoming/ → ssh 执行
#       ./deploy.sh web-deploy <远端包路径>(包内 tooling/ 自引导到 deploy/runtime/ → 同一套发布
#       语义 → 留档 data/packages/ → reload; 模式1 全程不需要服务器 git pull)
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
# 第 2 层: 各小节(构建/上传/发布)内部的明细, 与 upload-over-ssh.sh 的层级约定一致。
sub() { printf '  \033[36m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[31m[错误]\033[0m %s\n' "$*" >&2; exit 1; }

now_s()   { date +%s; }
fmt_dur() { local s="${1:-0}"; if [ "$s" -ge 60 ]; then printf '%dm%02ds' $((s / 60)) $((s % 60)); else printf '%ss' "$s"; fi; }

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
  JEROCINE_DOWNLOAD_DIR  本机构建产物落地目录(透传给 build-web.sh, 默认 <repo>/deploy/pkg)
  JEROCINE_UPLOAD_CMD    自定义上送命令注入(参数: <host> <本地包> <远端路径>)
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

T0="$(now_s)"      # 全程计时起点(构建+上传+发布)
T_BUILD=0; DID_BUILD=0
T_UPLOAD=0
T_PUBLISH=0

# ---------------------------------------------------------------- 1. 构建(可选)

if [ -z "$PKG" ]; then
  DID_BUILD=1
  _tb="$(now_s)"
  info "本机构建产物包"
  # 构建脚本的 stderr(vite 等工具输出)缩进 2 空格作为本节第 2 层; stdout 仍走
  # 下面的 sed 提取产物包路径。
  if [ -n "$TS" ]; then
    PKG="$(bash "$HERE/build-web.sh" --ts "$TS" 2> >(sed 's/^/  /' >&2) | sed -n 's/^产物包: \([^ ]*\).*/\1/p' | tail -1)"
  else
    PKG="$(bash "$HERE/build-web.sh" 2> >(sed 's/^/  /' >&2) | sed -n 's/^产物包: \([^ ]*\).*/\1/p' | tail -1)"
  fi
  [ -n "$PKG" ] || die "没能从构建输出里解析出产物包路径"
  T_BUILD=$(( $(now_s) - _tb ))
  sub "构建耗时 $(fmt_dur "$T_BUILD")"
fi

[ -f "$PKG" ] || die "找不到产物包: $PKG"
PKG_ABS="$(cd "$(dirname "$PKG")" && pwd)/$(basename "$PKG")"
PKG_NAME="$(basename "$PKG_ABS")"

# ---------------------------------------------------------------- 2. 上传
#
# **不用 scp**：本仓库运维约定里 scp 是禁用的，单向传文件的既有姿态就是
# "走 ssh stdin"（见 Harness `.agents/skills/jerocine-ops` §3 通道 B）。
# 通用实现 = 整包直传 + sha256 双端校验（比字节数更严）。
#
# JEROCINE_UPLOAD_CMD 注入钩子（可选）：设成自定义上送命令（参数:
# <host> <本地包> <远端路径>），成功返回 0。用于特殊环境，例如带命令沙箱
# 的 agent 终端里整包长流会被 SIGTERM 掐死 —— Harness 私有仓用 1MB 分片 +
# 断点续传的实现注入（scripts/lib/upload-over-ssh.sh），公开仓不背这份复杂度。

info "上传 $PKG_NAME"
sub "准备远端目录 $HOST:$INCOMING"
ssh "$HOST" "mkdir -p '$INCOMING'"

REMOTE_PKG="$INCOMING/$PKG_NAME"
_tu="$(now_s)"
if [ -n "${JEROCINE_UPLOAD_CMD:-}" ]; then
  # shellcheck disable=SC2086  # 刻意允许 "bash <脚本>" 这种带空格的注入形式
  $JEROCINE_UPLOAD_CMD "$HOST" "$PKG_ABS" "$REMOTE_PKG" || die "上传失败(见上方日志)"
else
  local_sha="$(sha256sum "$PKG_ABS" 2>/dev/null | awk '{print $1}')"
  # 秒传: 远端已有相同文件(上次传完但发布被打断) → sha256 一致即跳过重传
  remote_sha="$(ssh "$HOST" "sha256sum '$REMOTE_PKG'" 2>/dev/null | awk '{print $1}')"
  if [ -n "$local_sha" ] && [ "$local_sha" = "$remote_sha" ]; then
    sub "远端已存在相同文件(sha256 ${local_sha:0:12}… 一致) → 跳过上传, 直接复用"
  else
    cat "$PKG_ABS" | ssh "$HOST" "cat > '$REMOTE_PKG'" || die "上传失败"
    remote_sha="$(ssh "$HOST" "sha256sum '$REMOTE_PKG'" 2>/dev/null | awk '{print $1}')"
    [ -n "$local_sha" ] && [ "$local_sha" = "$remote_sha" ] || die \
      "sha256 校验不一致(本地 ${local_sha:-?} / 远端 ${remote_sha:-?}) —— 传输被截断或截短，请重试"
    sub "sha256 校验一致: ${local_sha:0:12}…"
  fi
fi
T_UPLOAD=$(( $(now_s) - _tu ))

# ---------------------------------------------------------------- 3. 远端发布

info "远端发布: $DEPLOY_DIR → ./deploy.sh web-deploy $REMOTE_PKG"
_tp="$(now_s)"
# 远端输出缩进 2 空格, 作为本节(第 1 层「==> 远端发布」)下的第 2 层明细;
# 2>&1 把 docker build(stderr)一并缩进。pipefail 保住 ssh 退出码(set -e 语义不变)。
ssh "$HOST" "cd '$DEPLOY_DIR' && ./deploy.sh web-deploy '$REMOTE_PKG'" 2>&1 | sed 's/^/  /'
T_PUBLISH=$(( $(now_s) - _tp ))

summary="上传 $(fmt_dur "$T_UPLOAD") + 发布 $(fmt_dur "$T_PUBLISH")"
if [ "$DID_BUILD" = "1" ]; then summary="构建 $(fmt_dur "$T_BUILD") + $summary"; fi
info "完成: $PKG_NAME 已发布 —— 总耗时 $(fmt_dur "$(( $(now_s) - T0 ))")（$summary）"

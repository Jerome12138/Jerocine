#!/usr/bin/env bash
# 本机上传 danmu-service 产物包并触发服务器发布 —— danmu 发布的第二步(与 scripts/build-danmu.sh 配套)。
#
# 链路: 本机构建打包(build-danmu.sh) → SSH 上送到 <deploy>/incoming/ → ssh 执行
#       ./deploy.sh danmu-deploy <远端包路径>(包内 tooling/ 自引导到 deploy/runtime/ → 以本机
#       npm 产物为上下文构建薄运行镜像 → 重建 jerocine_danmu 容器(--no-build); 全程不需要服务器
#       git pull / 不需要服务器装 node 依赖)。
#
# 与 server-deploy 的差别: 无迁移、无健康检查等待 —— danmu-service 不可用期间 Go 网关静默降级
# "暂无弹幕"(不阻塞播放), 故发布顺序无所谓先后; 但仍建议先 danmu 后 server(避免 server 端
# 负缓存 2h 窗口, 见 docs/环境变量参考.md DANMU_SERVICE_URL 条目)。
#
# 用法:
#   bash scripts/deploy-danmu.sh                       # 构建 + 上传 + 发布
#   bash scripts/deploy-danmu.sh --pkg <tgz>           # 复用已构建好的产物包(跳过构建)
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
上传 danmu-service 产物包并在服务器上发布(danmu 发布手段)

用法:
  bash scripts/deploy-danmu.sh                     # 构建 + 上传 + 发布
  bash scripts/deploy-danmu.sh --pkg <tgz>         # 复用已构建好的产物包(跳过构建)

参数:
  --host <ssh>       ssh 主机/别名(默认取 JEROCINE_SSH_HOST)
  --deploy-dir <d>   远端 deploy/ 目录(默认取 JEROCINE_DEPLOY_DIR)
  --incoming-dir <d> 远端上传落点(默认取 JEROCINE_INCOMING_DIR 或 <deploy>/incoming)
  --pkg <tgz>        直接给产物包(跳过构建)

环境变量:
  JEROCINE_SSH_HOST      ssh 主机/别名(必填)
  JEROCINE_DEPLOY_DIR    远端 deploy/ 目录(默认 /home/ubuntu/jerocine/deploy)
  JEROCINE_INCOMING_DIR  远端上传落点(默认 <deploy>/incoming)
  JEROCINE_DOWNLOAD_DIR  本机构建产物落地目录(透传给 build-danmu.sh, 默认 <repo>/deploy/pkg)
  JEROCINE_UPLOAD_CMD    自定义上送命令注入(参数: <host> <本地包> <远端路径>)
EOF
}

HOST="${JEROCINE_SSH_HOST:-}"
DEPLOY_DIR="${JEROCINE_DEPLOY_DIR:-/home/ubuntu/jerocine/deploy}"
INCOMING="${JEROCINE_INCOMING_DIR:-}"
PKG=""

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
  # 构建脚本的 stderr(npm 输出)缩进 2 空格作为本节第 2 层; stdout 仍走下面的 sed 提取产物包路径。
  PKG="$(bash "$HERE/build-danmu.sh" 2> >(sed 's/^/  /' >&2) | sed -n 's/^产物包: \([^ ]*\).*/\1/p' | tail -1)"
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
# "走 ssh stdin"（见 Harness .agents/skills/jerocine-ops §3 通道 B）。
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

info "远端发布: $DEPLOY_DIR → 包内工具链(runtime/) danmu-deploy $REMOTE_PKG"
_tp="$(now_s)"
# 远端输出缩进 2 空格, 作为本节(第 1 层「==> 远端发布」)下的第 2 层明细;
# 2>&1 把 docker build(stderr)一并缩进。pipefail 保住 ssh 退出码(set -e 语义不变)。
#
# **刻意不走树里的 ./deploy.sh**: danmu-deploy 是 2026-10-25 新增的子命令, 而服务器
# git 树可能落后于本仓(未 push 时树不会更新) —— 旧树 deploy.sh 不认识该子命令会直接
# 打用法退出(首次 danmu 发布实测踩到)。包内 tooling/ 与产物包同版本, 先刷新 runtime/
# 再以包内版本执行, 全程不依赖服务器树状态(与 deploy.sh 内 jc_bootstrap_tooling 同
# 逻辑; server/web 发布仍走树 ./deploy.sh —— 那两个子命令 10-22 起树里就有)。
# sudo 兜底: docker(root) 往 runtime/ 建过的挂载点目录普通用户删不掉(见 bootstrap 注释)。
ssh "$HOST" "cd '$DEPLOY_DIR' && ctx=\$(mktemp -d) && tar xzf '$REMOTE_PKG' -C \$ctx ./tooling && { rm -rf runtime 2>/dev/null; if [ -d runtime ]; then sudo -n rm -rf runtime || exit 1; fi; } && cp -R \$ctx/tooling runtime && chmod +x runtime/deploy.sh && rm -rf \$ctx && JEROCINE_DEPLOY_ROOT='$DEPLOY_DIR' bash runtime/deploy.sh danmu-deploy '$REMOTE_PKG'" 2>&1 | sed 's/^/  /'
T_PUBLISH=$(( $(now_s) - _tp ))

summary="上传 $(fmt_dur "$T_UPLOAD") + 发布 $(fmt_dur "$T_PUBLISH")"
if [ "$DID_BUILD" = "1" ]; then summary="构建 $(fmt_dur "$T_BUILD") + $summary"; fi
info "完成: $PKG_NAME 已发布 —— 总耗时 $(fmt_dur "$(( $(now_s) - T0 ))")（$summary）"

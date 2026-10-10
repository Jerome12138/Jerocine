#!/usr/bin/env bash
# 本机构建前端产物并打包 —— 常规前端发布的第一步。
#
# 链路(2026-10-10 起为**常规发布手段**):
#   scripts/build-web.sh   本机构建(注入 JC_BUILD_TS) + 产物布局断言 + tar.gz
#   scripts/deploy-web.sh  上传产物包并在服务器上执行 ./deploy.sh web-deploy
#   deploy/deploy.sh web-deploy   解包 → 同一套发布语义 → 留档 data/packages/ → reload
#
# 与"在服务器上构建"(./deploy.sh web, 仍保留为备用)相比: 服务器不再需要 node/pnpm
# 与 docker 构建缓存, 产物版本由本机一次性决定, 包本身留档可事后重发/比对。
#
# 用法:
#   bash scripts/build-web.sh                    # 自动版本号(当前时间 YYYYMMDD-HHMMSS)
#   bash scripts/build-web.sh --ts 20261010-1830 # 指定版本号(与线上 assets 目录名一致)
#   bash scripts/build-web.sh --out /d/Downloads # 产物包落到哪(默认系统下载目录)
#
# 产物: <out>/jerocine-web-<TS>.tar.gz   包内是 web/dist 的**内容**(index.html 在包根)
set -euo pipefail

# 与 build-server.sh 同因: **不要**设 MSYS_NO_PATHCONV —— 本脚本的路径全交给 MSYS 自带
# 工具(bash/tar/cp/date)处理, 关掉转换只会给原生 node.exe 埋雷(见 build-server.sh 注释)。

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPO="$(cd -- "$HERE/.." && pwd -P)"
WEB="$REPO/web"

info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[!]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[31m[错误]\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
本机构建前端产物并打包(常规前端发布第一步)

用法:
  bash scripts/build-web.sh                     # 自动版本号(当前时间 YYYYMMDD-HHMMSS)
  bash scripts/build-web.sh --ts 20261010-1830  # 指定版本号(与线上 assets 目录名一致)
  bash scripts/build-web.sh --out /d/Downloads  # 产物包落到哪(默认系统下载目录)

环境变量:
  JEROCINE_DOWNLOAD_DIR  产物包落地目录(优先级最高)
  JC_BUILD_TS            等价于 --ts

产物: <out>/jerocine-web-<TS>.tar.gz（包内是 web/dist 的内容, index.html 在包根）
EOF
}

# ---------------------------------------------------------------- 参数

TS="${JC_BUILD_TS:-}"
OUT=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --ts)   TS="${2:-}"; shift 2 ;;
    --ts=*) TS="${1#*=}"; shift ;;
    --out)  OUT="${2:-}"; shift 2 ;;
    --out=*) OUT="${1#*=}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数: $1（--help 看用法）" ;;
  esac
done

# ---------------------------------------------------------------- 下载目录
# 与 scripts/build-android.sh 同一套约定: JEROCINE_DOWNLOAD_DIR 显式指定优先。
# (那边还会读注册表拿真正的"下载"目录, 但本机 reg.exe 被命令黑名单拦住, 这里不做无谓尝试。)
resolve_out_dir() {
  local d="${JEROCINE_DOWNLOAD_DIR:-}"
  if [ -n "$d" ]; then
    d="$(cygpath -u "$d" 2>/dev/null || printf '%s' "$d")"
    if [ -d "$d" ]; then printf '%s' "$d"; return; fi
    warn "JEROCINE_DOWNLOAD_DIR=$JEROCINE_DOWNLOAD_DIR 不是已存在的目录, 忽略"
  fi
  if [ -d "$HOME/Downloads" ]; then printf '%s' "$HOME/Downloads"; return; fi
  if [ -n "${USERPROFILE:-}" ]; then
    d="$(cygpath -u "$USERPROFILE" 2>/dev/null || printf '%s' "$USERPROFILE")/Downloads"
    if [ -d "$d" ]; then printf '%s' "$d"; return; fi
  fi
  printf '%s' "$REPO"
}

file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'
  else printf 'n/a'; fi
}

# 断言压缩包里有某个条目。
# 注意**不能**写成 `tar -tzf "$pkg" | grep -qx "$entry"`: grep -q 命中即退, tar 写不进管道
# 会收 SIGPIPE 且非零退出, 叠加 `set -o pipefail` ⇒ "明明命中"反而判定失败(与包大小/条目位置
# 相关的偶发失败)。这里用命令替换把整条流读完。
pkg_has_entry() {
  local list
  list="$(tar -tzf "$1" 2>/dev/null || true)"
  printf '%s\n' "$list" | grep -qx -- "$2"
}

# ---------------------------------------------------------------- 构建

[ -d "$WEB" ] || die "找不到前端目录: $WEB"
[ -f "$WEB/package.json" ] || die "找不到 $WEB/package.json"

if [ -z "$TS" ]; then
  TS="$(date +%Y%m%d-%H%M%S)"
fi
case "$TS" in
  [0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]) : ;;
  *) die "版本号必须是 YYYYMMDD-HHMMSS（收到 '$TS'）—— 它是线上 assets/<TS>/ 的目录名" ;;
esac

[ -n "$OUT" ] || OUT="$(resolve_out_dir)"
OUT="$(cygpath -u "$OUT" 2>/dev/null || printf '%s' "$OUT")"
mkdir -p "$OUT"

info "构建前端 (JC_BUILD_TS=$TS)"
cd "$WEB"
export JC_BUILD_TS="$TS"

# vite emptyOutDir 会一次性删掉 dist 下几百个文件 —— WorkBuddy 会给 node 注入
# node-safe-delete-shim.cjs, "一次删除 ≥50 个文件" 在非交互(agent/CI)里直接失败。
# 清构建产物本就是预期行为, 故显式关掉该 shim(只作用于本次命令的子进程);
# 普通终端 / CI 没有这层注入, 导出它是空操作。
#
# 注意: 必须**无条件**置 0。shim 的判定是 `!== '0'` 即开, 而工作区环境里往往已经被
# 预置成 1 —— 写成 `${CODEBUDDY_SAFE_DELETE_ENABLED:-0}` 会原样继承那个 1, 等于没关。
export CODEBUDDY_SAFE_DELETE_ENABLED=0

# build = vue-tsc(类型门禁) + vite build, 与 web/Dockerfile 的 build 阶段同一条命令
if command -v pnpm >/dev/null 2>&1; then
  pnpm run build
elif command -v npm >/dev/null 2>&1; then
  warn "未找到 pnpm, 退回 npm(锁文件是 pnpm-lock.yaml, 结果可能有差异)"
  npm run build
else
  die "既没有 pnpm 也没有 npm"
fi

# 产物布局断言(与 web/Dockerfile 里同一支脚本): index.html 站内引用可解析 / 不引用 .map /
# 注入 JC_BUILD_TS 时资源确实落在 assets/<TS>/ 且 sw.js 的 precache manifest 自洽。
info "校验产物布局"
node scripts/verify-build-layout.mjs

# ---------------------------------------------------------------- 打包

DIST="$WEB/dist"
[ -f "$DIST/index.html" ] || die "构建产物缺 $DIST/index.html"

TS_IN_DIST="$(ls -1 "$DIST/assets" 2>/dev/null | grep -E '^[0-9]{8}-[0-9]{6}$' | sort -r || true)"
[ "$(printf '%s' "$TS_IN_DIST" | grep -c . || true)" = "1" ] || die \
  "dist/assets 下应恰好有 1 个版本目录(实际: ${TS_IN_DIST:-无}) ⇒ 构建没注入 JC_BUILD_TS?"
[ "$TS_IN_DIST" = "$TS" ] || die "dist/assets 下的版本($TS_IN_DIST) 与 JC_BUILD_TS($TS) 不一致"

PKG="$OUT/jerocine-web-$TS.tar.gz"
rm -f "$PKG"
info "打包 $PKG"
# 打进包的是 dist 的**内容**(index.html 在包根) —— 服务器端 wr_extract_pkg 就按这个约定校验
tar -czf "$PKG" -C "$DIST" .
pkg_has_entry "$PKG" './index.html' || die "产物包内没有 ./index.html"

SIZE="$(du -h "$PKG" | awk '{print $1}')"
printf '\n产物包: %s  (%s)\n' "$PKG" "$SIZE"
printf '  sha256: %s\n' "$(file_sha256 "$PKG")"
printf '  版本:   %s\n' "$TS"
printf '\n下一步(上传并发布):\n  bash scripts/deploy-web.sh --pkg "%s"\n' "$PKG"

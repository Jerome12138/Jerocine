#!/usr/bin/env bash
# 本机构建 danmu-service 运行产物包 —— 常规 danmu 发布第一步
# (与 scripts/build-server.sh 同哲学: 依赖裁剪/快照在本机完成, 服务器只 build 薄镜像)。
#
# 链路:
#   scripts/build-danmu.sh            npm ci --production + 打包
#   scripts/deploy-danmu.sh           (可选)上传并在服务器执行 ./deploy.sh danmu-deploy
#   deploy/deploy.sh danmu-deploy     解包 → 构建薄运行镜像(deploy/Dockerfile.danmu) → 重建容器
#
# 产物包布局(= deploy/Dockerfile.danmu 的 COPY 目标):
#   package.json  server.js  README.md  壳代码
#   lib/ scripts/ smoke/ upstream/      壳逻辑 + 上游快照(pin 0dc573e)
#   node_modules/                       --omit=dev 裁剪(含 postinstall 链接的 @rexnow/* 快照)
#   tooling/                           部署工具链(deploy.sh + lib + compose + Dockerfile* + nginx.conf)
#
# 用法:
#   bash scripts/build-danmu.sh                     # 产物落 <repo>/deploy/pkg
#   bash scripts/build-danmu.sh --out /d/Downloads  # 换落地目录
#
# 产物: <out>/jerocine-danmu-<TS>-<sha7>.tar.gz
set -euo pipefail

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPO="$(cd -- "$HERE/.." && pwd -P)"
DANMU="$REPO/deploy/danmu"

info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[31m[错误]\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
本机构建 danmu-service 运行产物包(常规 danmu 发布第一步)

用法:
  bash scripts/build-danmu.sh                     # 默认产物落 <repo>/deploy/pkg
  bash scripts/build-danmu.sh --out /d/Downloads  # 换落地目录

环境变量:
  JEROCINE_DOWNLOAD_DIR  产物包落地目录(优先级最高)

产物: <out>/jerocine-danmu-<TS>-<sha7>.tar.gz
EOF
}

OUT=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --out)    OUT="${2:-}"; shift 2 ;;
    --out=*)  OUT="${1#*=}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数: $1（--help 看用法）" ;;
  esac
done

command -v node >/dev/null 2>&1 || die "本机没有 node（需要 ≥22，与 deploy/danmu/package.json 的 engines 一致）"
command -v npm >/dev/null 2>&1 || die "本机没有 npm"

[ -d "$DANMU" ] || die "找不到 danmu-service 目录: $DANMU"
[ -f "$DANMU/package.json" ] || die "缺少 $DANMU/package.json（壳服务骨架不完整）"
[ -f "$DANMU/server.js" ] || die "缺少 $DANMU/server.js"
[ -d "$DANMU/upstream/packages/scraper-kit" ] || die "缺少 upstream/packages/scraper-kit（上游快照，见 deploy/danmu/README.md）"

OUT="${JEROCINE_DOWNLOAD_DIR:-$OUT}"
if [ -z "$OUT" ]; then OUT="$REPO/deploy/pkg"; fi
mkdir -p "$OUT"

TS="$(date +%Y%m%d-%H%M%S)"
SHA="$(git -C "$REPO" rev-parse --short=7 HEAD 2>/dev/null || printf 'nogit')"
STAGE="$(cd "$(mktemp -d)" && pwd)"
trap 'rm -rf "$STAGE"' EXIT

info "npm ci --omit=dev（生产依赖 + postinstall 链接 @rexnow 快照）…"
( cd "$DANMU" && npm ci --omit=dev --no-audit --no-fund )

info "组装产物目录…"
# 壳代码 + 快照
cp "$DANMU/package.json" "$DANMU/server.js" "$DANMU/README.md" "$STAGE/"
cp -r "$DANMU/lib" "$DANMU/scripts" "$DANMU/smoke" "$DANMU/upstream" "$STAGE/"
# node_modules(整目录, 含 @rexnow/* 链接产物)
cp -r "$DANMU/node_modules" "$STAGE/"

# tooling/ 随包上送(与 build-server.sh 同一套施工工具; 新增 Dockerfile.danmu)
TOOLING="$REPO/deploy"
mkdir -p "$STAGE/tooling/lib"
cp "$TOOLING/deploy.sh" "$STAGE/tooling/"
cp "$TOOLING"/lib/*.sh "$STAGE/tooling/lib/"
cp "$TOOLING/docker-compose.yml" "$TOOLING/Dockerfile.runtime" "$TOOLING/Dockerfile.nginx" \
   "$TOOLING/Dockerfile.danmu" "$TOOLING/data/nginx/nginx.conf" "$STAGE/tooling/"

PKG="$OUT/jerocine-danmu-$TS-$SHA.tar.gz"
info "打包 $PKG"
tar -czf "$PKG" -C "$STAGE" .

# 断言压缩包里有某个条目。
# 注意**不能**用 `grep -q`: 命中即退, printf 写管道收 SIGPIPE, 叠加 set -o pipefail
# 会把"明明命中"判成失败(大包条目多时必踩)。grep 完整读完拿 rc 才稳。
pkg_has_entry() {
  local list
  list="$(tar -tzf "$1" 2>/dev/null || true)"
  printf '%s\n' "$list" | grep -F -x -- "$2" >/dev/null
}
for f in ./server.js ./package.json ./lib/scraper.js ./upstream/packages/scraper-kit/dist/index.js \
         ./node_modules/ ./tooling/deploy.sh ./tooling/docker-compose.yml ./tooling/Dockerfile.danmu; do
  pkg_has_entry "$PKG" "$f" || die "包内缺少 $f —— 部署会失败"
done

file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'
  else printf 'n/a'; fi
}

printf '\n产物包: %s  (%s)\n' "$PKG" "$(du -h "$PKG" | awk '{print $1}')"
printf '  sha256: %s\n' "$(file_sha256 "$PKG")"
printf '  提交:   %s\n' "$SHA"
printf '\n下一步(上传并发布):\n  将产物包上传到服务器后执行 ./deploy.sh danmu-deploy %s\n' "$(basename "$PKG")"

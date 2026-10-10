#!/usr/bin/env bash
# 本机交叉编译后端并打包 —— 常规后端发布的第一步(与 deploy/deploy.sh server-deploy 配套)。
#
# 链路:
#   scripts/build-server.sh    交叉编译 linux/<arch> 静态二进制 + 打包
#   scripts/deploy-server.sh   上传产物包并在服务器上执行 ./deploy.sh server-deploy
#   deploy/deploy.sh server-deploy  解包 → 构建薄运行镜像(deploy/Dockerfile.runtime) → 重建容器
#
# 为什么在服务器上编译不好: 服务器要装 golang、要连 go module 代理、还要留一份 docker
# 构建缓存(本机实测曾涨到 13.87GB, 可回收 3.17GB)。本机交叉编译(CGO_ENABLED=0 纯静态)
# 产物与容器内编译等价, 且包本身可留档比对。
#
# 用法:
#   bash scripts/build-server.sh                    # 默认 linux/amd64
#   bash scripts/build-server.sh --arch arm64       # 换架构(需与服务器 uname -m 一致)
#   bash scripts/build-server.sh --out /d/Downloads # 产物包落到哪(默认系统下载目录)
#
# 产物包布局(= deploy/Dockerfile.runtime 的 COPY 目标):
#   main                 linux/<arch> 静态二进制
#   static/upload/       upload 卷挂载点(空目录)
#   data/ip2region.db    IP 归属地离线库
#   LICENSE
set -euo pipefail

# 注意: 这里**不能**像 deploy.sh 那样 `export MSYS_NO_PATHCONV=1`。
# 该开关关掉的是"把 POSIX 路径转成 Windows 路径"的兜底, 而本脚本要把路径交给原生
# go.exe / git.exe: 关掉之后 `go build -o /tmp/x/main` 会被 go 当成**当前盘根**下的
# `D:\tmp\x\main`(而且 go 仍然返回 0!), `git -C /d/Git/Jerocine` 直接找不到仓库。
# 正确做法是显式 cygpath 转换(见 to_native), 那样无论开关如何都稳。

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPO="$(cd -- "$HERE/.." && pwd -P)"
SRV="$REPO/server"

info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[!]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[31m[错误]\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
本机交叉编译后端并打包(常规后端发布第一步)

用法:
  bash scripts/build-server.sh                     # 默认 linux/amd64
  bash scripts/build-server.sh --arch arm64        # 换架构(须与服务器 uname -m 一致)
  bash scripts/build-server.sh --out /d/Downloads  # 产物包落到哪(默认系统下载目录)

环境变量:
  JEROCINE_DOWNLOAD_DIR  产物包落地目录(优先级最高)
  JEROCINE_GO_ARCH       等价于 --arch
  GOPROXY                go module 代理(默认 https://goproxy.cn,direct)

产物: <out>/jerocine-server-<TS>-<sha7>-linux-<arch>.tar.gz
EOF
}

ARCH="${JEROCINE_GO_ARCH:-amd64}"
OUT=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --arch)   ARCH="${2:-}"; shift 2 ;;
    --arch=*) ARCH="${1#*=}"; shift ;;
    --out)    OUT="${2:-}"; shift 2 ;;
    --out=*)  OUT="${1#*=}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数: $1（--help 看用法）" ;;
  esac
done

case "$ARCH" in
  amd64|arm64) : ;;
  *) die "--arch 只支持 amd64 / arm64（收到 '$ARCH'）" ;;
esac

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

# POSIX 路径 → 原生 Windows 路径(交给 go.exe / git.exe 这类原生程序时必须转换)。
# 没有 cygpath(Linux/macOS)则原样返回。
to_native() {
  if command -v cygpath >/dev/null 2>&1; then cygpath -w "$1"; else printf '%s' "$1"; fi
}

# 断言压缩包里有某个条目。
# 注意**不能**写成 `tar -tzf "$pkg" | grep -qx "$entry"`: grep -q 命中即退, tar 写不进管道
# 会收 SIGPIPE 且非零退出, 叠加 `set -o pipefail` ⇒ "明明命中"反而判定失败。包越大越容易踩
# (踩过一次: 30MB 二进制的包必失败, 而 index.html 在包首的 web 包侥幸通过)。
pkg_has_entry() {
  local list
  list="$(tar -tzf "$1" 2>/dev/null || true)"
  printf '%s\n' "$list" | grep -qx -- "$2"
}

# ELF 头自检: 避免把 Windows/其它架构的产物发上线(容器里只会 exit 126, 排查成本高)
check_elf() {
  local f="$1" arch="$2" magic machine
  # 显式兜住"文件不存在/读不出"——`set -e` 下裸的 command substitution 失败会**静默**中止脚本,
  # 只剩一个退出码, 排查成本极高(踩过一次: go 把产物写去了别的盘)。
  [ -f "$f" ] || die "找不到产物文件: $f —— 交叉编译实际没有写出产物"
  magic="$(od -An -tx1 -N4 "$f" 2>/dev/null | tr -d ' \n' || true)"
  [ -n "$magic" ] || die "读不出 $f 的文件头(od 无输出)"
  [ "$magic" = "7f454c46" ] || die "产物不是 ELF 文件(魔数=$magic) —— 交叉编译没生效?"
  # e_machine 在偏移 18(0x12), 小端 2 字节
  machine="$(od -An -tx1 -j18 -N2 "$f" 2>/dev/null | tr -d ' \n' || true)"
  case "$arch:$machine" in
    amd64:3e00) : ;;
    arm64:b700) : ;;
    *) die "ELF e_machine=$machine 与目标架构 $arch 不符" ;;
  esac
}

# ---------------------------------------------------------------- 编译

[ -d "$SRV" ] || die "找不到后端目录: $SRV"
command -v go >/dev/null 2>&1 || die "本机没有 go（需要 1.27+，与 deploy/Dockerfile 的 golang 版本一致）"

[ -n "$OUT" ] || OUT="$(resolve_out_dir)"
OUT="$(cygpath -u "$OUT" 2>/dev/null || printf '%s' "$OUT")"
mkdir -p "$OUT"

TS="$(date +%Y%m%d-%H%M%S)"
SHA="$(git -C "$(to_native "$REPO")" rev-parse --short=7 HEAD 2>/dev/null || printf 'nogit')"
STAGE="$(cd "$(mktemp -d)" && pwd)"
trap 'rm -rf "$STAGE"' EXIT

export CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

info "交叉编译后端 linux/$ARCH（$(go version | awk '{print $3}')）"
( cd "$SRV" && go build -trimpath -ldflags="-s -w" -o "$(to_native "$STAGE/main")" ./cmd/server )

check_elf "$STAGE/main" "$ARCH"
# 补 exec 位(Windows 文件系统上 go 产物默认 644; Git Bash 的 chmod 能落到 tar 的 mode 里)。
# 双保险 —— 运行镜像侧还有 COPY --chmod=0755 兜底, 但裸用包内容(如手工解包替换)时靠这层。
chmod 0755 "$STAGE/main"

# ---------------------------------------------------------------- 打包

mkdir -p "$STAGE/static/upload" "$STAGE/data"
[ -f "$SRV/data/ip2region.db" ] || die "缺少 $SRV/data/ip2region.db（IP 归属地离线库）"
cp "$SRV/data/ip2region.db" "$STAGE/data/"
[ -f "$REPO/LICENSE" ] || die "缺少 $REPO/LICENSE"
cp "$REPO/LICENSE" "$STAGE/LICENSE"

PKG="$OUT/jerocine-server-$TS-$SHA-linux-$ARCH.tar.gz"
rm -f "$PKG"
info "打包 $PKG"
tar -czf "$PKG" -C "$STAGE" .
pkg_has_entry "$PKG" './main' || die "产物包内没有 ./main"
# 可执行位断言(踩过一次: Windows 上 tar 进包的 main 是 644, distroless nonroot 起容器
# 直接 permission denied)。tar -tvzf 第 1 列是 mode。
main_mode="$(tar -tvzf "$PKG" './main' | awk '{print $1}')"
case "$main_mode" in
  *x*) : ;;
  *) die "包内 ./main 没有可执行位(mode=$main_mode) —— 服务器上容器会起不来" ;;
esac

printf '\n产物包: %s  (%s)\n' "$PKG" "$(du -h "$PKG" | awk '{print $1}')"
printf '  sha256: %s\n' "$(file_sha256 "$PKG")"
printf '  目标:   linux/%s  (提交 %s)\n' "$ARCH" "$SHA"
printf '\n下一步(上传并发布):\n  bash scripts/deploy-server.sh --pkg "%s"\n' "$PKG"

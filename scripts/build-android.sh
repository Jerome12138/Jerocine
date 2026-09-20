#!/usr/bin/env bash
#
# Jerocine Android 一键构建
#
# 用法:
#   scripts/build-android.sh web                    # Capacitor 壳应用 art.jerocine.app
#   scripts/build-android.sh tv                     # 原生 TV 客户端 art.jerocine.tv
#   scripts/build-android.sh all                    # 两个都打(默认)
#   scripts/build-android.sh tv --api-base=https://jerocine.art/
#   scripts/build-android.sh web --debug            # 出 debug 包(不需要正式密钥, 仅供本机试装)
#   scripts/build-android.sh all -- -PwebVersionCode=1042   # -- 之后原样透传给 gradle
#
# 产物: 各工程 app/build/outputs/apk/<variant>/ 下, 并按下面规则复制一份到系统下载目录:
#   Jerocine-TV-web-v<版本名>(<构建号>).apk        Capacitor 壳应用
#   Jerocine-TV-native-v<版本名>(<构建号>).apk     原生 TV 客户端
#   debug 包追加 -debug 后缀。例: Jerocine-TV-web-v1.0.9(1041).apk
#
# 版本号唯一来源是 scripts/android-versions.properties（构建号千位编号, 1000+迭代号），
# 本脚本不猜版本, 以 APK 产物目录里的 output-metadata.json 为准。
#
# 跨平台: macOS / Linux 直接跑; Windows 请用 Git Bash(随 Git for Windows 安装):
#   bash scripts/build-android.sh all
# PowerShell / cmd 无法执行 .sh, 这是 bash 方案的固有限制。
#
set -euo pipefail

if [ -z "${BASH_VERSION:-}" ]; then
  printf '错误: 本脚本需要 bash。请用 `bash %s [web|tv|all]` 运行。\n' "$0" >&2
  exit 1
fi

SRC="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd -- "$(dirname -- "$SRC")" && pwd -P)"
ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd -P)"
VERSIONS_FILE="$ROOT/scripts/android-versions.properties"

TARGET="all"
VARIANT="Release"
API_BASE=""
WEB_BASE=""
COPIED_COUNT=0
EXTRA_GRADLE_ARGS=()

usage() {
  cat <<'EOF'
Jerocine Android 一键构建

用法:
  scripts/build-android.sh [web|tv|all] [选项] [-- <gradle 参数>...]

目标:
  web          Capacitor 壳应用 (art.jerocine.app)，会先编译前端再灌进壳工程
  tv           原生 TV 客户端 (art.jerocine.tv)
  all          两个都打 (默认)

选项:
  --debug              出 debug 包 (不校验正式密钥，仅供本机试装)
  --api-base=<URL>     TV 包的后端地址 (编译期常量，默认 http://localhost:9000/)
  --web-base=<URL>     TV 包里二维码指向的 Web 地址
  -- <参数>...          之后的参数原样传给 gradle，如 -PwebVersionCode=1042
  -h, --help           显示本帮助

版本号: 唯一来源是 scripts/android-versions.properties（构建号 = 1000 + 迭代号），
        发新版改那一个文件即可，本脚本和两个 Gradle 工程都读它。

产物: 各工程 app/build/outputs/apk/<variant>/ 下，并按下面规则复制一份到系统下载目录:
  Jerocine-TV-web-v<版本名>(<构建号>).apk      Capacitor 壳应用
  Jerocine-TV-native-v<版本名>(<构建号>).apk   原生 TV 客户端
  debug 包追加 -debug 后缀。

跨平台: macOS / Linux 直接跑；Windows 请用 Git Bash:
  bash scripts/build-android.sh all
EOF
}

# ---------------------------------------------------------------- 参数

SEEN_EXTRA=0
for arg in "$@"; do
  if [ "$SEEN_EXTRA" = 1 ]; then
    EXTRA_GRADLE_ARGS+=("$arg")
    continue
  fi
  case "$arg" in
    all) TARGET="all" ;;
    web) TARGET="web" ;;
    tv|native) TARGET="tv" ;;   # native = 原生 TV 客户端, 与产物名 Jerocine-TV-native-... 对齐
    --debug) VARIANT="Debug" ;;
    --api-base=*) API_BASE="${arg#*=}" ;;
    --web-base=*) WEB_BASE="${arg#*=}" ;;
    --) SEEN_EXTRA=1 ;;
    -h|--help) usage; exit 0 ;;
    *) printf '未知参数: %s\n\n' "$arg" >&2; usage >&2; exit 1 ;;
  esac
done

VARIANT_LC="$(printf '%s' "$VARIANT" | tr '[:upper:]' '[:lower:]')"
IS_RELEASE=1
[ "$VARIANT_LC" = "release" ] || IS_RELEASE=0

# ---------------------------------------------------------------- 平台探测

case "$(uname -s)" in
  Darwin) OS=mac ;;
  Linux)  OS=linux ;;
  MINGW*|MSYS*|CYGWIN*) OS=windows ;;
  *) OS=unknown ;;
esac

# Windows 下关掉 MSYS 的自动路径转换，否则 -PapiBase=https://x/ 会被改写成盘符路径
if [ "$OS" = windows ]; then
  export MSYS_NO_PATHCONV=1
fi

win2unix() {
  if [ "$OS" = windows ] && command -v cygpath >/dev/null 2>&1; then
    cygpath -u "$1"
  else
    printf '%s' "$1"
  fi
}

info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[!]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[31m[错误]\033[0m %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------- 下载目录

resolve_downloads() {
  local d
  if [ "$OS" = linux ] && command -v xdg-user-dir >/dev/null 2>&1; then
    d="$(xdg-user-dir DOWNLOAD 2>/dev/null || true)"
    if [ -n "$d" ] && [ -d "$d" ]; then printf '%s' "$d"; return; fi
  fi
  if [ -d "$HOME/Downloads" ]; then printf '%s' "$HOME/Downloads"; return; fi
  # Git Bash 的 HOME 有时不是 /c/Users/<name>，退回 USERPROFILE
  if [ "$OS" = windows ] && [ -n "${USERPROFILE:-}" ]; then
    d="$(win2unix "$USERPROFILE")/Downloads"
    if [ -d "$d" ]; then printf '%s' "$d"; return; fi
  fi
  printf '%s' "$HOME"
}

# ---------------------------------------------------------------- JDK / SDK

java_major() {
  local bin="java"
  [ -n "${JAVA_HOME:-}" ] && [ -x "${JAVA_HOME}/bin/java" ] && bin="${JAVA_HOME}/bin/java"
  command -v "$bin" >/dev/null 2>&1 || return 1
  "$bin" -version 2>&1 | sed -n 's/.*version "\([0-9][0-9]*\).*/\1/p' | head -1 || true
}

ensure_java() {
  if [ -z "${JAVA_HOME:-}" ] && [ "$OS" = mac ] && [ -x /usr/libexec/java_home ]; then
    local j
    j="$(/usr/libexec/java_home -v 17 2>/dev/null || true)"
    [ -n "$j" ] && export JAVA_HOME="$j"
  fi
  local major
  major="$(java_major || true)"
  [ -n "$major" ] || die "找不到 java。请把 JAVA_HOME 指向 JDK 17+（AGP 8.x 要求 JDK 17 起）。"
  [ "$major" -ge 17 ] || die "JDK 版本过低（$major），需要 17+。请把 JAVA_HOME 指向 JDK 17 或更高。"
  info "JDK $major  JAVA_HOME=${JAVA_HOME:-<来自 PATH>}"
}

resolve_sdk() {
  local cand
  for cand in "${ANDROID_HOME:-}" "${ANDROID_SDK_ROOT:-}" \
              "$HOME/Library/Android/sdk" "$HOME/Android/Sdk" \
              "${LOCALAPPDATA:+$(win2unix "$LOCALAPPDATA")/Android/Sdk}"; do
    if [ -n "$cand" ] && [ -d "$cand" ]; then printf '%s' "$cand"; return; fi
  done
}

ensure_sdk() {
  local sdk
  sdk="$(resolve_sdk)"
  if [ -z "$sdk" ]; then
    warn "没探测到 Android SDK。若 Gradle 报 SDK location not found，请设置 ANDROID_HOME，"
    warn "或用 Android Studio 打开工程让它自己生成 local.properties。"
    return 0
  fi
  export ANDROID_HOME="$sdk"
  export ANDROID_SDK_ROOT="$sdk"
  # local.properties 是机器相关文件(gitignored)。缺失或指向不存在的目录时补写，省掉手工步骤。
  local lp cur
  for lp in "$ROOT/web/android/local.properties" "$ROOT/tv/local.properties"; do
    [ -d "$(dirname -- "$lp")" ] || continue
    cur=""
    [ -f "$lp" ] && cur="$(sed -n 's/^sdk\.dir=//p' "$lp" | head -1 | tr -d '\r' || true)"
    if [ -z "$cur" ] || [ ! -d "$cur" ]; then
      printf 'sdk.dir=%s\n' "$sdk" > "$lp"
    fi
  done
  info "Android SDK $sdk"
}

# ---------------------------------------------------------------- 密钥

# 读取 properties 文件里的键值（顺手吃掉 Windows 的 CR）
read_prop() {
  local file="$1" key="$2"
  [ -f "$file" ] || return 0
  sed -n "s/^[[:space:]]*${key}[[:space:]]*=[[:space:]]*//p" "$file" | head -1 | tr -d '\r' || true
}

# 解析正式签名凭据。密钥属于使用者私有，不进本仓，只认显式指定的位置:
#   1. 环境变量 JEROCINE_KEYSTORE / JEROCINE_STORE_PASSWORD / JEROCINE_KEY_ALIAS / JEROCINE_KEY_PASSWORD
#   2. $JEROCINE_KEY_DIR 目录下的 keystore.properties（密钥放在同一目录）
# 刻意不在工程里找密钥: 密钥放仓库里靠 .gitignore 挡是单点防线, 漏一条规则就泄漏;
# 显式指定也让"找不到密钥"永远报错, 不会悄悄用了别处遗留的一份。
resolve_signing() {
  KS_STORE=""
  KS_PASS=""
  KS_ALIAS=""
  KS_KEYPASS=""
  KS_PROPS_FILE=""

  local dir="${JEROCINE_KEY_DIR:-}"
  if [ -n "$dir" ] && [ -f "$dir/keystore.properties" ]; then
    KS_PROPS_FILE="$dir/keystore.properties"
  fi

  if [ -n "$KS_PROPS_FILE" ]; then
    KS_PASS="$(read_prop "$KS_PROPS_FILE" storePassword)"
    KS_ALIAS="$(read_prop "$KS_PROPS_FILE" keyAlias)"
    KS_KEYPASS="$(read_prop "$KS_PROPS_FILE" keyPassword)"
    KS_STORE="$(dirname -- "$KS_PROPS_FILE")/jerocine.keystore"
  fi

  # 环境变量优先级最高，允许逐项覆盖文件里的值
  [ -n "${JEROCINE_STORE_PASSWORD:-}" ] && KS_PASS="$JEROCINE_STORE_PASSWORD"
  [ -n "${JEROCINE_KEY_ALIAS:-}" ]      && KS_ALIAS="$JEROCINE_KEY_ALIAS"
  [ -n "${JEROCINE_KEY_PASSWORD:-}" ]   && KS_KEYPASS="$JEROCINE_KEY_PASSWORD"
  [ -n "${JEROCINE_KEYSTORE:-}" ]       && KS_STORE="$JEROCINE_KEYSTORE"

  [ -n "$KS_ALIAS" ] || KS_ALIAS="jerocine"
}

require_signing() {
  resolve_signing

  local have_pass="缺失" have_keypass="缺失"
  [ -n "$KS_PASS" ] && have_pass="已提供"
  [ -n "$KS_KEYPASS" ] && have_keypass="已提供"

  if [ -z "$KS_STORE" ] || [ ! -f "$KS_STORE" ] || [ -z "$KS_PASS" ] || [ -z "$KS_KEYPASS" ]; then
    cat >&2 <<EOF
[错误] release 构建需要正式签名密钥，但没找全凭据。

  放哪儿（任一即可）:
    JEROCINE_KEY_DIR=<目录>                                  # 目录里放 keystore.properties + 密钥
    或直接用环境变量 JEROCINE_KEYSTORE + JEROCINE_STORE_PASSWORD + JEROCINE_KEY_ALIAS + JEROCINE_KEY_PASSWORD

  keystore.properties 内容（键名固定）:
    storePassword=...
    keyAlias=...
    keyPassword=...

  当前解析结果:
    keystore        : ${KS_STORE:-<未找到>}
    storePassword   : $have_pass
    keyAlias        : ${KS_ALIAS:-<缺失>}
    keyPassword     : $have_keypass

  密钥不该提交进仓库：自签包没有吊销/轮换机制，签名密钥泄露 = 别人能签出被系统认可
  的"合法升级"静默覆盖已装用户，只能换密钥 + 老用户重装。请让它保持在 git 之外。

  只想本机试装: 加 --debug（debug 签名，不能覆盖已装的正式包）。
EOF
    exit 1
  fi

  # Git Bash on Windows: gradlew is Windows process, convert keystore path to Windows format
  # (/c/Users/... exists()=false on Windows File(), would fall back to unsigned)
  if command -v cygpath >/dev/null 2>&1 && [ -n "$KS_STORE" ]; then
    KS_STORE="$(cygpath -w "$KS_STORE")"
  fi
  export JEROCINE_KEYSTORE="$KS_STORE"
  export JEROCINE_STORE_PASSWORD="$KS_PASS"
  export JEROCINE_KEY_ALIAS="$KS_ALIAS"
  export JEROCINE_KEY_PASSWORD="$KS_KEYPASS"
  info "签名密钥 $KS_STORE（alias=$KS_ALIAS）"
}

# ---------------------------------------------------------------- 构建

run_gradlew() {
  local dir="$1"; shift
  # 从 zip / Windows 上 clone 下来可能丢执行位
  [ -x "$dir/gradlew" ] || chmod +x "$dir/gradlew" 2>/dev/null || true
  ( cd "$dir" && ./gradlew "$@" )
}

# 取该 variant 输出目录里最新的 APK（目录不存在或无产物时返回空）
newest_apk() {
  local dir="$1"
  [ -d "$dir" ] || return 0
  ls -t "$dir"/*.apk 2>/dev/null | head -1 || true
}

# 读版本源文件里的键（键名带点，转义掉正则里的通配）
versions_get() {
  [ -f "$VERSIONS_FILE" ] || return 0
  local key
  key="$(printf '%s' "$1" | sed 's/\./\\./g')"
  sed -n "s/^[[:space:]]*${key}[[:space:]]*=[[:space:]]*//p" "$VERSIONS_FILE" | head -1 | tr -d '\r' || true
}

# 从 APK 产物目录的 output-metadata.json 取 versionCode / versionName：
# 这是"包里到底写了什么"的权威来源（gradle 自己生成的），比任何人工维护的副本可信。
meta_field() {
  [ -f "$1" ] || return 0
  sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\{0,1\}\([^,\"[:space:]]*\)\"\{0,1\}.*/\1/p" "$1" | head -1 || true
}

build_web() {
  local w="$ROOT/web" a="$ROOT/web/android"
  [ -f "$w/package.json" ] || die "找不到 $w/package.json"

  if [ ! -d "$w/node_modules" ]; then
    info "安装前端依赖（首次需要）..."
    if command -v pnpm >/dev/null 2>&1; then
      ( cd "$w" && pnpm install )
    elif command -v npm >/dev/null 2>&1; then
      warn "未装 pnpm，退回 npm。本项目锁文件是 pnpm-lock.yaml，npm 可能报 ERESOLVE。"
      ( cd "$w" && npm install )
    else
      die "需要 Node.js（含 pnpm 或 npm）来编译前端资源。"
    fi
  fi

  command -v node >/dev/null 2>&1 || die "找不到 node。web/android 是 Capacitor 壳，必须先编译前端。"
  [ -x "$w/node_modules/.bin/vite" ] || die "缺少 $w/node_modules/.bin/vite，依赖装得不完整。"
  [ -x "$w/node_modules/.bin/cap" ] || die "缺少 $w/node_modules/.bin/cap，请安装 @capacitor/cli。"

  info "[web] 编译前端（vue-tsc 类型检查 + vite build）"
  ( cd "$w" && ./node_modules/.bin/vue-tsc -p tsconfig.app.json --noEmit && ./node_modules/.bin/vite build )

  info "[web] cap sync android（把 dist 灌进壳工程）"
  ( cd "$w" && ./node_modules/.bin/cap sync android )

  local args=("assemble$VARIANT")
  if [ "${#EXTRA_GRADLE_ARGS[@]}" -gt 0 ]; then
    args+=("${EXTRA_GRADLE_ARGS[@]}")
  fi
  info "[web] gradle ${args[*]}"
  run_gradlew "$a" "${args[@]}"

  WEB_APK="$(newest_apk "$a/app/build/outputs/apk/$VARIANT_LC" || true)"
  [ -n "$WEB_APK" ] || die "[web] 构建结束但没找到 APK。"
}

build_tv() {
  local t="$ROOT/tv"
  [ -f "$t/settings.gradle.kts" ] || die "找不到 $t/settings.gradle.kts"

  local args=(":app:assemble$VARIANT")
  # TV 的后端地址是编译期常量，不指定就只能连 localhost
  [ -n "$API_BASE" ] && args+=("-PapiBase=$API_BASE")
  [ -n "$WEB_BASE" ] && args+=("-PwebBase=$WEB_BASE")
  if [ "${#EXTRA_GRADLE_ARGS[@]}" -gt 0 ]; then
    args+=("${EXTRA_GRADLE_ARGS[@]}")
  fi

  if [ "$IS_RELEASE" = 1 ] && [ -z "$API_BASE" ]; then
    warn "[tv] 未指定 --api-base，该包会指向默认的 http://localhost:9000/（装到电视上连不通）。"
  fi

  info "[tv] gradle ${args[*]}"
  run_gradlew "$t" "${args[@]}"

  TV_APK="$(newest_apk "$t/app/build/outputs/apk/$VARIANT_LC" || true)"
  [ -n "$TV_APK" ] || die "[tv] 构建结束但没找到 APK。"
}

# ---------------------------------------------------------------- 交付

DOWNLOADS="$(resolve_downloads)"

deliver() {
  local apk="$1" kind="$2" prefix="$3"

  if [ "$IS_RELEASE" = 1 ] && printf '%s' "${apk##*/}" | grep -q "unsigned"; then
    die "${apk##*/} 是未签名包 —— 说明签名配置没生效，装不上。请检查 keystore.properties 内容。"
  fi

  # 版本以 APK 产物目录的 output-metadata.json 为准（gradle 写进去的真实值）；
  # 读不到才退回 scripts/android-versions.properties。
  local metadir ver build
  metadir="$(dirname -- "$apk")"
  ver="$(meta_field "$metadir/output-metadata.json" versionName || true)"
  build="$(meta_field "$metadir/output-metadata.json" versionCode || true)"
  if [ -z "$ver" ] || [ -z "$build" ]; then
    warn "读不到 $(basename -- "$metadir")/output-metadata.json 的版本，退回 $VERSIONS_FILE"
    [ -n "$ver" ]   || ver="$(versions_get "$prefix.versionName" || true)"
    [ -n "$build" ] || build="$(versions_get "$prefix.versionCode" || true)"
  fi
  [ -n "$ver" ] || ver="0.0.0"
  # 构建号是千位编号(1000+迭代号)，补足 4 位只是兜底，正常已是 4 位
  build="$(printf '%04d' "${build:-0}" 2>/dev/null || printf '%s' "${build:-0}")"

  # 形如 Jerocine-TV-web-v1.0.9(1041).apk
  local suffix="" name
  [ "$IS_RELEASE" = 1 ] || suffix="-debug"
  name="Jerocine-TV-${kind}-v${ver}(${build})${suffix}.apk"
  cp -f -- "$apk" "$DOWNLOADS/$name"
  COPIED_COUNT=$((COPIED_COUNT + 1))
  printf '  %s\n    ← %s\n' "$DOWNLOADS/$name" "$apk"
}

# ---------------------------------------------------------------- main

info "仓库根 $ROOT（$OS，$VARIANT）"

[ -f "$VERSIONS_FILE" ] || die "找不到版本源文件 $VERSIONS_FILE —— 构建号与版本名都从这里读。"
info "版本源 $VERSIONS_FILE"
grep -v '^[[:space:]]*#' "$VERSIONS_FILE" | grep '=' | sed 's/^/  /' || true

if [ "$IS_RELEASE" = 1 ]; then
  require_signing
else
  warn "--debug 模式：不校验正式密钥，产物只能本机试装。"
fi

ensure_java
ensure_sdk

WEB_APK=""
TV_APK=""

case "$TARGET" in
  web) build_web ;;
  tv)  build_tv ;;
  all) build_web; build_tv ;;
esac

printf '\n构建完成，产物:\n'
if [ -n "$WEB_APK" ]; then deliver "$WEB_APK" web web; fi
if [ -n "$TV_APK" ]; then deliver "$TV_APK" native native; fi

printf '\n已复制 %s 个 APK 到 %s\n' "$COPIED_COUNT" "$DOWNLOADS"

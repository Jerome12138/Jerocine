#!/usr/bin/env bash
#
# cap sync 手动维护脚本（2026-10-15 起构建链不再自动跑 cap sync）
#
# 什么时候需要跑（跑完检查 git diff 并提交, 生成物已全部入库）:
#   1. 升级 @capacitor/* 依赖（pnpm update 后）
#   2. 增删 Capacitor 插件
#   3. 改 web/capacitor.config.ts
#   4. 改 android/web-stub/（占位页）
#
# 它做四件事:
#   ① 确保 web/android -> ../android 桥接存在（Capacitor 硬约定平台目录必须是 <cap 根>/android;
#      工程物理在顶层 android/, junction 让 cap CLI 照常工作 —— junction 不进 git, 缺了自动补建）
#   ② 跑 cap sync android（stdin 重定向 /dev/null: 非 TTY 下它会读 stdin 阻塞, 踩过）
#   ③ 修正 capacitor.settings.gradle 的相对路径（cap sync 按"工程在 web/ 下"假设生成
#      ../node_modules, 物理落 android/ 后 gradle 解析断链 -> 改写成 ../web/node_modules）
#   ④ 提醒提交 diff
#
# 用法: bash scripts/cap-sync.sh
#
set -euo pipefail

SRC="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd -- "$(dirname -- "$SRC")" && pwd -P)"
ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd -P)"
W="$ROOT/web"
A="$ROOT/android"

info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[31m[错误]\033[0m %s\n' "$*" >&2; exit 1; }

[ -d "$A" ] || die "找不到 $A —— 安卓工程应在仓库根 android/。"
[ -f "$W/package.json" ] || die "找不到 $W/package.json"
[ -x "$W/node_modules/.bin/cap" ] || die "缺少 $W/node_modules/.bin/cap —— 先 cd web && pnpm install。"
command -v node >/dev/null 2>&1 || die "找不到 node。cap sync 需要 node。"

# ① 桥接
if [ ! -e "$W/android" ]; then
  info "补建 web/android -> ../android 桥接"
  case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*)
      jlink="$(cygpath -w "$W/android")"
      jtarget="$(cygpath -w "$A")"
      powershell -NoProfile -Command "New-Item -ItemType Junction -Path '$jlink' -Target '$jtarget' | Out-Null" \
        || die "建 junction 失败。请在 web/ 下手动: New-Item -ItemType Junction -Path web/android -Target ../android"
      ;;
    *)
      ln -s ../android "$W/android" || die "建 symlink 失败。请手动: ln -s ../android web/android"
      ;;
  esac
fi
[ -f "$W/android/settings.gradle" ] || die "web/android 桥接不可用(指向的不是安卓工程?)。请检查 junction/symlink。"

# ② sync（stdin 重定向 + 超时兜底）
cap_timeout="${JEROCINE_CAP_SYNC_TIMEOUT:-300}"
timeout_cmd=""
if command -v timeout >/dev/null 2>&1; then
  timeout_cmd="timeout"
elif command -v gtimeout >/dev/null 2>&1; then
  timeout_cmd="gtimeout"
fi

info "cap sync android（超时 ${cap_timeout}s）"
cap_rc=0
if [ -n "$timeout_cmd" ]; then
  ( cd "$W" && "$timeout_cmd" "$cap_timeout" ./node_modules/.bin/cap sync android < /dev/null ) || cap_rc=$?
else
  warn() { printf '\033[33m[!]\033[0m %s\n' "$*" >&2; }
  warn "未找到 timeout/gtimeout，cap sync 没有超时兜底。"
  ( cd "$W" && ./node_modules/.bin/cap sync android < /dev/null ) || cap_rc=$?
fi
[ "$cap_rc" -eq 0 ] || die "cap sync 失败或超时。可调大预算重跑: JEROCINE_CAP_SYNC_TIMEOUT=1800 bash scripts/cap-sync.sh"

# ③ 路径修正
if grep -q "new File('../node_modules" "$A/capacitor.settings.gradle" 2>/dev/null; then
  sed -i "s|new File('../node_modules|new File('../web/node_modules|g" "$A/capacitor.settings.gradle"
  info "已修正 capacitor.settings.gradle 的 node_modules 相对路径(junction 副作用)"
fi

[ -f "$A/app/src/main/assets/public/index.html" ] || die "sync 后缺 assets/public/index.html —— sync 被打断? 重跑一次。"

# ④ 提醒
info "完成。生成物已入库 —— 请检查并提交 diff:"
if git -C "$ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git -C "$ROOT" status --short | grep -E "capacitor|assets/public|config.xml" || printf '  (无生成物变化)\n'
fi

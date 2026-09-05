#!/usr/bin/env bash
# 坚果 Nano 真机功能 + 性能实测（task #11）
# 用法：./scripts/smoke-nano.sh <投影仪IP>   用法: ./scripts/smoke-nano.sh <投影仪IP>
#
# 覆盖：安装 → 冷启动耗时 → 首页(从后端拉数据) → D-pad 导航详情 → ▶播放
#       → View/XML 滚动帧率(gfxinfo jank) → 崩溃检查。
set -uo pipefail

IP="${1:?指定投影仪IP}"
DEV="$IP:5555"
ADB="${ANDROID_HOME:-$HOME/Android/Sdk}/platform-tools/adb"
PKG="art.jerocine.tv"
APK="$(cd "$(dirname "$0")/.." && pwd)/app/build/outputs/apk/debug/app-debug.apk"

say() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
key() { "$ADB" -s "$DEV" shell input keyevent "$1"; sleep "${2:-1.2}"; }

say "连接 $DEV"
"$ADB" connect "$DEV" || { echo "连接失败——设备离线？"; exit 1; }
"$ADB" -s "$DEV" wait-for-device

say "设备信息（验证是 Android 6.0 老机）"
REL=$("$ADB" -s "$DEV" shell getprop ro.build.version.release | tr -d '\r')
SDK=$("$ADB" -s "$DEV" shell getprop ro.build.version.sdk | tr -d '\r')
MODEL=$("$ADB" -s "$DEV" shell getprop ro.product.model | tr -d '\r')
echo "  $MODEL  Android $REL (API $SDK)"

say "安装 APK（minSdk 21，应可装入 API ${SDK}）"
"$ADB" -s "$DEV" install -r "$APK" || { echo "安装失败"; exit 1; }

say "冷启动耗时（am start -W，TotalTime 即冷启动毫秒）"
"$ADB" -s "$DEV" shell am force-stop "$PKG"
"$ADB" -s "$DEV" logcat -c
"$ADB" -s "$DEV" shell am start -W -n "$PKG/.MainActivity" 2>&1 | grep -E "TotalTime|WaitTime" || true
sleep 4

say "重置图形统计（准备测 View/XML 滚动帧率）"
"$ADB" -s "$DEV" shell dumpsys gfxinfo "$PKG" reset >/dev/null 2>&1

say "首页横向滚动 x8（压测 View/XML 在弱 GPU 上的帧率）"
for i in 1 2 3 4 5 6 7 8; do key 22 0.5; done   # DPAD_RIGHT 连续滚动海报墙
key 20 0.6; for i in 1 2 3 4; do key 22 0.5; done

say "View/XML 滚动帧率 / jank 统计"
"$ADB" -s "$DEV" shell dumpsys gfxinfo "$PKG" 2>&1 | grep -iE "Total frames|Janky frames|50th|90th|95th|99th|Number Missed Vsync|frames rendered" | head -12

say "D-pad 进详情 → ▶播放（验证全链路 + 真实 m3u8 播放）"
key 20; key 23; sleep 2   # 进第一个海报 → 详情
key 23; sleep 10          # 详情 ▶播放 自动聚焦 → 播放页, 等待缓冲/首帧

say "播放引擎 / 首帧 / 网络请求"
"$ADB" -s "$DEV" logcat -d 2>&1 | grep -iE "filmPlayInfo|ExoPlayerImpl: Init|IjkMediaPlayer|dropped|MediaCodec.*(configure|createByCodecName)|onFirstFrame" | tail -12

say "崩溃检查（应无 FATAL）"
if "$ADB" -s "$DEV" logcat -d 2>&1 | grep -iE "FATAL|AndroidRuntime.*$PKG|SIGSEGV|SIGBUS.*$PKG" | grep -q .; then
  echo "  ⚠️ 检测到崩溃日志："
  "$ADB" -s "$DEV" logcat -d 2>&1 | grep -iE "FATAL|AndroidRuntime|SIGSEGV|SIGBUS" | tail -10
else
  echo "  ✅ 无崩溃"
fi

say "进程存活确认"
"$ADB" -s "$DEV" shell pidof "$PKG" >/dev/null && echo "  ✅ APP 存活" || echo "  ❌ APP 不在运行"

say "完成。人工复核项：① 遥控滚动/焦点是否跟手 ② 视频是否流畅无卡顿"
echo "性能判据：关注 Janky frames 比例及 90th-99th 帧耗时，并与改造前数据对比。"

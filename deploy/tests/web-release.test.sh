#!/usr/bin/env bash
# deploy/lib/web-release.sh 的单测 —— 纯文件系统语义, 不需要 docker/网络/服务器。
#   cd deploy && bash tests/web-release.test.sh
#
# 覆盖真正会出事的地方: 版本目录布局 / 排除 *.map 并归档 / 整版快照 / 保留 3 版 /
# workbox 换 hash 后旧运行时清理 / 回滚(含"目标已出窗口"必须失败)。
set -uo pipefail

cd "$(dirname "$0")/.."          # → deploy/
# shellcheck source=../lib/web-release.sh
source ./lib/web-release.sh

pass=0
fail=0
ok()   { pass=$((pass + 1)); echo "  ok   $*"; }
bad()  { fail=$((fail + 1)); echo "  FAIL $*"; }
assert_eq()   { [ "$2" = "$3" ] && ok "$1" || bad "$1 (期望 [$3] 实得 [$2])"; }
assert_has()  { [ -e "$2" ] && ok "$1" || bad "$1 (缺少 $2)"; }
assert_gone() { [ ! -e "$2" ] && ok "$1" || bad "$1 (不该存在 $2)"; }
assert_die()  { if ( "$@" ) >/dev/null 2>&1; then bad "应失败但成功了: $*"; else ok "按预期失败: $*"; fi; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
DATA="$WORK/data"
mkdir -p "$DATA"

# ---------- 构造一个假的 dist(含 map / 根静态 / 无 hash 的杂项) ----------
# make_dist <ts> <chunk> <wbhash>
make_dist() {
  local ts="$1" chunk="$2" wb="$3"
  local d="$WORK/dist-$ts"
  rm -rf "$d"
  mkdir -p "$d/assets/$ts" "$d/assets/$ts/sub"
  printf 'console.log("hi")\n'      > "$d/assets/$ts/index-$chunk.js"
  printf '{"version":3}\n'          > "$d/assets/$ts/index-$chunk.js.map"
  printf 'body{}\n'                 > "$d/assets/$ts/style-$chunk.css"
  printf '{"version":3}\n'          > "$d/assets/$ts/sub/lazy-$chunk.js.map"
  printf 'console.log("lazy")\n'    > "$d/assets/$ts/sub/lazy-$chunk.js"
  printf '<html>%s</html>\n'        > "$d/index.html"
  printf 'self.addEventListener("install",()=>{})\n' > "$d/sw.js"
  printf 'sw.js.map\n'              > "$d/sw.js.map"
  printf 'workbox %s\n' "$wb"       > "$d/workbox-$wb.js"
  printf 'ICO\n'                    > "$d/favicon.ico"
  printf 'UA\n'                     > "$d/robots.txt"
  printf 'SVG\n'                    > "$d/default-avatar.svg"
  echo "$d"
}

echo "== 1. 单次同步: 布局 / 排除 map / 归档 / 快照 / LICENSE =="
D1="$(make_dist 20260101-000001 aaa111 deadbeef)"
printf 'MIT\n' > "$WORK/LICENSE"
wr_sync_dist "$D1" "$DATA" 20260101-000001 "$WORK/LICENSE" >/dev/null

assert_has  "assets/<TS>/ 版本资源"        "$DATA/html/assets/20260101-000001/index-aaa111.js"
assert_has  "assets/<TS>/ 子目录资源"      "$DATA/html/assets/20260101-000001/sub/lazy-aaa111.js"
assert_gone "assets/<TS>/ 内不落 *.map"    "$DATA/html/assets/20260101-000001/index-aaa111.js.map"
assert_gone "assets/<TS>/ 子目录也不落 map" "$DATA/html/assets/20260101-000001/sub/lazy-aaa111.js.map"
assert_has  "根 index.html"                "$DATA/html/index.html"
assert_has  "根 sw.js"                     "$DATA/html/sw.js"
assert_has  "根 workbox-*.js"              "$DATA/html/workbox-deadbeef.js"
assert_gone "根 sw.js.map 被排除"          "$DATA/html/sw.js.map"
assert_has  "根静态 favicon.ico"           "$DATA/html/favicon.ico"
assert_has  "根静态 robots.txt"            "$DATA/html/robots.txt"
assert_has  "LICENSE 随产物分发"           "$DATA/html/LICENSE"
assert_has  "map 归档到 debugmap(挂载外)"  "$DATA/debugmap/20260101-000001/index-aaa111.js.map"
assert_has  "子目录 map 也归档"            "$DATA/debugmap/20260101-000001/lazy-aaa111.js.map"
assert_has  "debugmap 不在 html 内(不公开)" "$WORK/data/debugmap/20260101-000001"
assert_gone "html 内没有 debugmap"         "$DATA/html/debugmap"
assert_has  "整版快照 index.html"          "$DATA/releases/20260101-000001/index.html"
assert_has  "整版快照 sw.js"               "$DATA/releases/20260101-000001/sw.js"
assert_has  "整版快照 workbox"             "$DATA/releases/20260101-000001/workbox-deadbeef.js"

echo
echo "== 2. 缺 JC_BUILD_TS(没有 assets/<TS>) 必须拒绝 =="
make_dist 20260101-000002 bbb222 cafe0001 >/dev/null
D_NO_TS="$WORK/dist-20260101-000002"
mkdir -p "$WORK/bad/assets"; printf '<html>x</html>\n' > "$WORK/bad/index.html"
assert_die wr_sync_dist "$WORK/bad" "$DATA" 20260101-000009 ""

echo
echo "== 3. 连续发布 4 版 → 只保留最近 3 版(assets/releases/debugmap 同步裁剪) =="
for i in 2 3 4; do
  ts="20260101-00000$i"
  D="$(make_dist "$ts" "c$i" "wb00000$i")"
  wr_sync_dist "$D" "$DATA" "$ts" "" >/dev/null
done
assert_has  "第 4 版(v4)保留"        "$DATA/html/assets/20260101-000004"
assert_has  "第 3 版(v3)保留"        "$DATA/html/assets/20260101-000003"
assert_has  "第 2 版(v2)保留"        "$DATA/html/assets/20260101-000002"
assert_gone "第 1 版(v1)已裁剪"      "$DATA/html/assets/20260101-000001"
assert_gone "v1 的 releases 已裁剪"  "$DATA/releases/20260101-000001"
assert_gone "v1 的 debugmap 已裁剪"  "$DATA/debugmap/20260101-000001"
assert_eq   "assets 版本目录数 = 3"   "$(ls -1 "$DATA/html/assets" | grep -cE "$WR_TS_RE")" "3"
assert_eq   "releases 版本目录数 = 3" "$(ls -1 "$DATA/releases" | grep -cE "$WR_TS_RE")" "3"

echo
echo "== 4. workbox 运行时换 hash: 根目录不残留旧运行时 =="
assert_has  "根只有当前版 workbox"   "$DATA/html/workbox-wb000004.js"
assert_gone "旧 workbox 已被清理"    "$DATA/html/workbox-deadbeef.js"
assert_eq   "根 workbox 文件数 = 1"  "$(ls -1 "$DATA/html"/workbox-*.js | wc -l | tr -d ' ')" "1"

echo
echo "== 5. 同 TS 重发(重跑同版本) 幂等且覆盖 =="
D5="$(make_dist 20260101-000004 c4new wb000099)"
wr_sync_dist "$D5" "$DATA" 20260101-000004 "" >/dev/null
assert_has  "重发后新 chunk 存在"     "$DATA/html/assets/20260101-000004/index-c4new.js"
assert_gone "重发后旧 chunk 已清"     "$DATA/html/assets/20260101-000004/index-c4.js"
assert_has  "重发后 workbox 更新"     "$DATA/html/workbox-wb000099.js"
assert_gone "重发后旧 workbox 清理"   "$DATA/html/workbox-wb000004.js"
assert_has  "快照随重发更新"          "$DATA/releases/20260101-000004/workbox-wb000099.js"

echo
echo "== 6. 整版回滚 =="
# 先把当前根改成"新版"内容, 再回滚, 验证三件套整体被覆盖回去
cp "$DATA/releases/20260101-000003/index.html" "$DATA/html/index.html"
wr_rollback "$DATA" 20260101-000003 >/dev/null
if diff -q "$DATA/html/index.html" "$DATA/releases/20260101-000003/index.html" >/dev/null; then
  ok "回滚后 index.html 与快照一致"
else
  bad "回滚后 index.html 与快照一致"
fi
if diff -q "$DATA/html/sw.js" "$DATA/releases/20260101-000003/sw.js" >/dev/null; then
  ok "回滚后 sw.js 与快照一致(整版)"
else
  bad "回滚后 sw.js 与快照一致(整版)"
fi
assert_has  "回滚后 workbox 为快照版"  "$DATA/html/workbox-wb000003.js"
assert_gone "回滚后非快照 workbox 清理" "$DATA/html/workbox-wb000099.js"
assert_eq   "回滚后根 workbox 数 = 1"   "$(ls -1 "$DATA/html"/workbox-*.js | wc -l | tr -d ' ')" "1"

echo
echo "== 7. 回滚边界: 目标已出保留窗口 / 版本不存在 ⇒ 必须失败 =="
assert_die wr_rollback "$DATA" 20260101-000001   # 已被裁剪
assert_die wr_rollback "$DATA" 20990101-000000   # 从不存在
assert_die wr_rollback "$DATA" ""                # 缺参数

echo
echo "===================================="
echo "  通过 $pass / 失败 $fail"
echo "===================================="
[ "$fail" -eq 0 ]

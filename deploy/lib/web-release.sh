#!/usr/bin/env bash
# Web 前端产物 发布 / 回滚 库（方案 §3.1-§3.4 的可执行定义）
#
# 被 deploy.sh source（线上路径），也被 deploy/tests/web-release.test.sh source（本机单测）。
# 设计原则：这里**只做纯文件系统语义**，不碰 docker / 网络 / nginx —— 于是可以在本机用假
# 目录完整测试（含"排除 map / 保留 3 版 / 整版快照 / 回滚"这些真正会出事的地方）。
#
# 目录约定（deploy/ 下）:
#   data/html/            挂载给 nginx（compose: ./data/html:/usr/share/nginx/html）= 公开
#   ├── index.html        引用 /assets/<TS>/...（no-cache）
#   ├── sw.js             构建产物根（no-cache）
#   ├── workbox-<hash>.js 同上
#   ├── LICENSE           MIT 合规
#   ├── <根静态>          favicon / android-chrome / default-avatar.svg ...
#   └── assets/<TS>/      本版资源（版本窗口 = 保留 3 版）
#   data/releases/<TS>/   整版快照 {index.html, sw.js, workbox-*.js} —— 回滚最小单位
#   data/debugmap/<TS>/   归档的 *.map（**挂载目录之外** ⇒ 公网不可达）
#
# 为什么 snapshots/debugmap 放在 html 之外：html 是公开根，sourcemap 里就是源码；
# 方案 §3.3 允许的两种做法里，我们取"放非 nginx root 下"，不依赖 location 规则兜底。

# 保留版本数（assets / releases / debugmap 三处同步裁剪）
WR_KEEP_VERSIONS="${WR_KEEP_VERSIONS:-3}"

# 版本目录名格式（与 build 期 JC_BUILD_TS 一致）：YYYYMMDD-HHMMSS
WR_TS_RE='^[0-9]{8}-[0-9]{6}$'

wr_die() { echo "!! $*" >&2; exit 1; }

# 拷贝目录树但排除 *.map（GNU tar，Windows Git Bash 与 Ubuntu 都有）
wr_copy_excluding_maps() {
  local src="$1" dst="$2"
  ( cd "$src" && tar cf - --exclude='*.map' . ) | ( cd "$dst" && tar xf - )
}

# 把 src 下的 *.map 归档到 dst（保持文件名；无 map 时不创建目录）
wr_archive_maps() {
  local src="$1" dst="$2" f found=0
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    if [ "$found" = "0" ]; then mkdir -p "$dst"; found=1; fi
    cp "$f" "$dst/$(basename "$f")"
  done < <(find "$src" -type f -name '*.map' 2>/dev/null || true)
}

# 保留最近 $3 版，删除更旧的；以 assets/ 下的版本目录为权威列表（名字排序 = 时间序）
wr_prune_versions() {
  local data="$1" keep="${2:-$WR_KEEP_VERSIONS}" d i=0
  local versions
  versions="$(ls -1 "$data/html/assets" 2>/dev/null | grep -E "$WR_TS_RE" | sort -r || true)"
  for d in $versions; do
    i=$((i + 1))
    if [ "$i" -le "$keep" ]; then continue; fi
    rm -rf "$data/html/assets/$d" "$data/releases/$d" "$data/debugmap/$d"
    echo "==> 清理旧版本 $d（超出保留 $keep 版）"
  done
}

# wr_sync_dist <dist_dir> <data_dir> <ts> [license_file]
# 顺序 = 先写目录；调用方负责最后才 reload nginx（R9：避免覆盖期间的 404 窗口）
wr_sync_dist() {
  local dist="$1" data="$2" ts="$3" license="${4:-}"
  local html="$data/html" f base wb

  [ -d "$dist" ] || wr_die "dist 目录不存在: $dist"
  [ -f "$dist/index.html" ] || wr_die "dist 缺 index.html（构建失败？）"
  [ -n "$ts" ] || wr_die "缺少版本号 TS"
  [ -d "$dist/assets/$ts" ] || wr_die "dist/assets/$ts 不存在 ⇒ 构建未注入 JC_BUILD_TS"

  mkdir -p "$html/assets" "$data/releases/$ts"

  # 1) 版本资源目录：先清同 TS 残留（重发同版场景）再拷，排除 *.map
  rm -rf "$html/assets/$ts"
  mkdir -p "$html/assets/$ts"
  wr_copy_excluding_maps "$dist/assets/$ts" "$html/assets/$ts"

  # 2) html 根：除 assets/ 与 *.map 外全部（index.html / sw.js / workbox-*.js / 根静态）
  #    先清上一版的 workbox 运行时（内容哈希文件名，不清会永久堆积 && 干扰整版回滚）
  rm -f "$html"/workbox-*.js
  for f in "$dist"/*; do
    [ -e "$f" ] || continue
    base="$(basename "$f")"
    [ "$base" = "assets" ] && continue
    case "$base" in *.map) continue ;; esac
    cp -R "$f" "$html/$base"
  done
  [ -f "$html/index.html" ] || wr_die "index.html 未落到 $html"

  # 3) LICENSE（随产物分发，站点 /LICENSE 可访问）
  if [ -n "$license" ] && [ -f "$license" ]; then cp "$license" "$html/LICENSE"; fi

  # 4) sourcemap 归档（挂载目录之外 ⇒ 公网不可达）
  wr_archive_maps "$dist/assets/$ts" "$data/debugmap/$ts"

  # 5) 整版快照（回滚只认这一套三件：index.html + sw.js + workbox-*.js）
  cp "$html/index.html" "$data/releases/$ts/index.html"
  if [ -f "$html/sw.js" ]; then cp "$html/sw.js" "$data/releases/$ts/sw.js"; fi
  for wb in "$html"/workbox-*.js; do
    if [ ! -e "$wb" ]; then continue; fi
    cp "$wb" "$data/releases/$ts/$(basename "$wb")"
  done

  # 6) 保留最近 N 版
  wr_prune_versions "$data" "$WR_KEEP_VERSIONS"

  echo "==> 已同步版本 $ts → $html"
}

# wr_rollback <data_dir> <ts>
# 必须整版回滚：只回 index.html 而 sw.js 不变 ⇒ SW 不更新 ⇒ precache 仍是新版 manifest，
# 与旧 index.html 引用的 chunk 对不上（R8）。sw.js 是 no-cache，字节变化后浏览器会重装旧 SW。
wr_rollback() {
  local data="$1" ts="$2" f
  local rel="$data/releases/$ts" html="$data/html"

  [ -n "$ts" ] || wr_die "用法: web-rollback <TS>"
  [ -f "$rel/index.html" ] || wr_die "没有 $ts 的整版快照（$rel/index.html 不存在）"
  [ -d "$html/assets/$ts" ] || wr_die "assets/$ts 已不在保留窗口内，无法回滚（请改用仍被保留的版本）"

  cp "$rel/index.html" "$html/index.html"
  if [ -f "$rel/sw.js" ]; then
    cp "$rel/sw.js" "$html/sw.js"
  else
    rm -f "$html/sw.js"
  fi
  rm -f "$html"/workbox-*.js
  for f in "$rel"/workbox-*.js; do
    [ -e "$f" ] || continue
    cp "$f" "$html/"
  done

  echo "==> 已回滚静态产物到 $ts（接着 reload nginx 生效）"
}

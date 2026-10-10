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
#   data/packages/<TS>.tar.gz  本地上传的产物包留档（常规发布手段；保留 WR_KEEP_PACKAGES 份）
#   data/root-manifest.txt 上一版写入 html 根的条目名清单（用于清理陈旧根静态）
#
# 为什么 snapshots/debugmap 放在 html 之外：html 是公开根，sourcemap 里就是源码；
# 方案 §3.3 允许的两种做法里，我们取"放非 nginx root 下"，不依赖 location 规则兜底。

# 保留版本数（assets / releases / debugmap 三处同步裁剪）
WR_KEEP_VERSIONS="${WR_KEEP_VERSIONS:-3}"

# 上传产物包（data/packages/<TS>.tar.gz）的保留份数 —— 常规发布走
# 「本地构建 → 压缩包上传 → 服务器解包发布」，上传包本身也留档，方便事后重发/比对。
WR_KEEP_PACKAGES="${WR_KEEP_PACKAGES:-5}"

# 版本目录名格式（与 build 期 JC_BUILD_TS 一致）：YYYYMMDD-HHMMSS
WR_TS_RE='^[0-9]{8}-[0-9]{6}$'

# 上传产物包的归档文件名格式
WR_PKG_RE='^[0-9]{8}-[0-9]{6}\.tar\.gz$'

# html 根"由发布写入"的条目清单（每次发布覆盖写）。
# 用途：下一次发布时删掉"上一版有、本版没有"的根文件/目录 —— 否则被改名/移除的产物
# （如换名后的 icons 目录、旧版遗留的 robots.txt）会永久留在公开目录里。
# 放在挂载目录**之外**（与 releases/ debugmap/ 同级）⇒ 不是公开文件。
WR_ROOT_MANIFEST='root-manifest.txt'

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

# 写根条目清单：wr_write_root_manifest <data_dir> <条目名...>
# 无条目时删除清单（下次发布不清理任何东西，比留一份旧清单安全）。
wr_write_root_manifest() {
  local data="$1" mf name
  mf="$data/$WR_ROOT_MANIFEST"
  if [ "$#" -le 1 ]; then
    rm -f "$mf"
    return 0
  fi
  shift
  for name in "$@"; do echo "$name"; done > "$mf"
  return 0
}

# 删除"上一版清单里有、本版清单里没有"的 html 根条目（目录也删）。
# 只认自己写的清单 ⇒ 不会碰运维手工放进挂载目录的东西（如 ACME 的 .well-known/）。
# assets/ 恒被跳过：它由 wr_prune_versions 按保留窗口管理，绝不能被这里整目录删掉。
wr_prune_root_stale() {
  local data="$1"; shift
  local mf="$data/$WR_ROOT_MANIFEST" f old base hit
  [ -f "$mf" ] || return 0
  while IFS= read -r old; do
    [ -n "$old" ] || continue
    # 清单是我们自己写的，但仍挡一下脏值：绝不因清单内容越出 html 目录或删到 assets
    case "$old" in
      */*|.|..|assets) continue ;;
    esac
    hit=0
    for base in "$@"; do
      if [ "$base" = "$old" ]; then hit=1; break; fi
    done
    [ "$hit" = "1" ] && continue
    f="$data/html/$old"
    if [ -e "$f" ]; then
      rm -rf "$f"
      echo "==> 清理本版已不存在的根文件 $old"
    fi
  done < "$mf"
  return 0
}

# wr_init_after_copy <data_dir>
# web-init 专用: docker cp 把旧容器产物落到 html/ 之后, 做两件纯文件的事 ——
#   ① 删掉 docker cp 连带带出的 apk 副本(容器内 /usr/share/nginx/html/apk 是 ./apk 的只读挂载点,
#      不属于镜像自带产物; 留在挂载目录里会被 ./apk 挂载遮蔽, 还会干扰根清单清理);
#   ② 写出初始根清单 —— 有了它, 紧随其后的 ./deploy.sh web 才能清掉"旧版有、本版没有"的根文件。
# 抽成纯函数(不碰 docker/权限)便于本机单测: tests/web-release.test.sh 第 9 组。
wr_init_after_copy() {
  local data="$1" html="$1/html" entries=() e
  [ -d "$html" ] || return 0
  if [ -d "$html/apk" ]; then
    rm -rf "$html/apk"
    echo "    (已移除 docker cp 带出的 apk 副本 —— 它由 ./apk 单独挂载)"
  fi
  while IFS= read -r e; do
    [ -n "$e" ] || continue
    [ "$e" = "assets" ] && continue
    entries+=("$e")
  done < <(ls -1 "$html" 2>/dev/null || true)
  wr_write_root_manifest "$data" "${entries[@]}"
}

# wr_sync_dist <dist_dir> <data_dir> <ts> [license_file]
# 顺序 = 先写目录；调用方负责最后才 reload nginx（R9：避免覆盖期间的 404 窗口）
wr_sync_dist() {
  local dist="$1" data="$2" ts="$3" license="${4:-}"
  local html="$data/html" f base wb
  local root_entries=()

  [ -d "$dist" ] || wr_die "dist 目录不存在: $dist"
  [ -f "$dist/index.html" ] || wr_die "dist 缺 index.html（构建失败？）"
  [ -n "$ts" ] || wr_die "缺少版本号 TS"
  [ -d "$dist/assets/$ts" ] || wr_die "dist/assets/$ts 不存在 ⇒ 构建未注入 JC_BUILD_TS"

  mkdir -p "$html/assets" "$data/releases/$ts"

  # 1) 版本资源目录：先清同 TS 残留（重发同版场景）再拷，排除 *.map
  rm -rf "$html/assets/$ts"
  mkdir -p "$html/assets/$ts"
  wr_copy_excluding_maps "$dist/assets/$ts" "$html/assets/$ts"

  # 2) 本版要放到 html 根的条目（除 assets/ 与 *.map）—— 先算清单，既用于拷贝，
  #    也用于清掉"上一版有、本版没有"的陈旧根文件（step 2.1）。
  for f in "$dist"/*; do
    [ -e "$f" ] || continue
    base="$(basename "$f")"
    [ "$base" = "assets" ] && continue
    case "$base" in *.map) continue ;; esac
    root_entries+=("$base")
  done
  [ "${#root_entries[@]}" -gt 0 ] || wr_die "dist 根目录为空（构建异常？）"

  # 2.0) 先清上一版的 workbox 运行时（内容哈希文件名，不清会永久堆积 && 干扰整版回滚）
  rm -f "$html"/workbox-*.js
  for base in "${root_entries[@]}"; do
    cp -R "$dist/$base" "$html/$base"
  done
  [ -f "$html/index.html" ] || wr_die "index.html 未落到 $html"

  # 2.1) 清掉上一版有、本版没有的根条目（只认自己写的清单），再记下本版清单供下一版清理。
  #      注意顺序：必须在版本资源目录写完**之后**（清单恒不含 assets/，但要防手工清单脏值）。
  wr_prune_root_stale "$data" "${root_entries[@]}"
  wr_write_root_manifest "$data" "${root_entries[@]}"

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

# ==================== 上传产物包（本地构建 → 压缩包 → 服务器解包发布） ====================
#
# 常规发布链路（2026-10-10 起）：
#   本机  scripts/build-web.sh    构建(注入 JC_BUILD_TS) + 布局校验 + tar czf dist → jerocine-web-<TS>.tar.gz
#   本机  scripts/deploy-web.sh   scp 上传到 <deploy>/incoming/ 并调远端 ./deploy.sh web-deploy <包>
#   服务器 deploy.sh web-deploy   解包 → wr_sync_dist(同一套发布语义) → 留档 data/packages/ → reload
# 下面这几个函数只做纯文件系统语义，便于在本机完整测（见 tests/web-release.test.sh 第 10 组）。

# wr_pkg_ts <pkg>  —— 从产物包文件名解出版本号（jerocine-web-<TS>.tar.gz）；不匹配返回非 0
wr_pkg_ts() {
  local base
  base="$(basename "${1:-}")"
  case "$base" in
    jerocine-web-*.tar.gz) base="${base#jerocine-web-}"; base="${base%.tar.gz}" ;;
    *) return 1 ;;
  esac
  printf '%s' "$base" | grep -Eq "$WR_TS_RE" || return 1
  printf '%s' "$base"
}

# wr_extract_tar <pkg> <dest>  —— 解包 tar.gz(通用; 前端/后端产物包都用它)
wr_extract_tar() {
  local pkg="${1:-}" dest="${2:-}"
  [ -n "$pkg" ] || wr_die "用法: wr_extract_tar <pkg.tar.gz> <dest>"
  [ -n "$dest" ] || wr_die "用法: wr_extract_tar <pkg.tar.gz> <dest>"
  [ -f "$pkg" ] || wr_die "找不到产物包: $pkg"
  mkdir -p "$dest"
  tar xzf "$pkg" -C "$dest" || wr_die "解包失败（不是合法的 tar.gz？）: $pkg"
  return 0
}

# wr_extract_pkg <pkg> <dest>  —— 解包前端产物包 + 最低限度校验（包内必须是 dist 的**内容**）
wr_extract_pkg() {
  local pkg="${1:-}" dest="${2:-}"
  wr_extract_tar "$pkg" "$dest"
  [ -f "$dest/index.html" ] || wr_die \
    "包内没有 index.html —— 打包时应打进 web/dist 的**内容**(tar -C web/dist .)，不是 dist 目录本身"
  return 0
}

# wr_ts_from_dist <dist>  —— dist/assets 下唯一的版本目录名就是本次发布的 TS（构建期注入）
wr_ts_from_dist() {
  local dist="${1:-}" vers n
  vers="$(ls -1 "$dist/assets" 2>/dev/null | grep -E "$WR_TS_RE" | sort -r || true)"
  n="$(printf '%s' "$vers" | grep -c . || true)"
  if [ "$n" != "1" ]; then
    wr_die "dist/assets 下应恰好有 1 个版本目录(实际 $n 个) ⇒ 构建时没注入 JC_BUILD_TS？"
  fi
  printf '%s' "$vers"
}

# wr_archive_pkg <pkg> <data> <ts>  —— 把上传的产物包留档到 data/packages/<TS>.tar.gz，只保留最近 N 份
wr_archive_pkg() {
  local pkg="${1:-}" data="${2:-}" ts="${3:-}" dst f i=0
  [ -n "$pkg" ] && [ -n "$data" ] && [ -n "$ts" ] || wr_die "用法: wr_archive_pkg <pkg> <data_dir> <ts>"
  mkdir -p "$data/packages"
  dst="$data/packages/$ts.tar.gz"
  # 已经在归档目录里(比如直接对 data/packages 下的包再发一次)就不搬
  if [ "$(cd "$(dirname "$pkg")" && pwd)/$(basename "$pkg")" != "$(cd "$data/packages" && pwd)/$ts.tar.gz" ]; then
    mv -f "$pkg" "$dst"
  fi
  for f in $(ls -1 "$data/packages" 2>/dev/null | grep -E "$WR_PKG_RE" | sort -r || true); do
    i=$((i + 1))
    [ "$i" -le "$WR_KEEP_PACKAGES" ] && continue
    rm -f "$data/packages/$f"
    echo "==> 清理旧产物包 $f（超出保留 $WR_KEEP_PACKAGES 份）"
  done
  echo "==> 产物包已留档: $dst"
}

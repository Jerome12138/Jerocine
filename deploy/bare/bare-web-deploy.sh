#!/usr/bin/env bash
# Jerocine 前端发布（裸机部署）—— 复用 docker 模式同一套发布语义(lib/web-release.sh)。
#
# 用法(服务器上, install.sh 已跑过):
#   sudo bash bare-web-deploy.sh /path/to/<TS>.tar.gz     # 发布
#   sudo bash bare-web-deploy.sh rollback <TS>            # 整版回滚
#
# 产物包与本机 scripts/build-web.sh 的 docker 模式包**完全同一个**(web/dist 内容)。
# 数据目录 $PREFIX/webdata 与 docker 模式 data/ 同构:
#   html/       nginx root(公开) — assets/<TS>/ 版本窗口, 保留 3 版
#   releases/   整版快照(回滚最小单位)   debugmap/ 归档 *.map(公开根之外)
#   packages/   上传包留档(保留 5 份)    root-manifest.txt  陈旧根清理清单
# ⚠ 与 docker 模式的差异: docker 里 /apk/ 是独立挂载, 这里 nginx alias 到 /opt/jerocine/apk;
#   wr_init_after_copy 会主动删掉 html/apk 残留 —— 若有本地 apk 文件, 放 /opt/jerocine/apk/。
set -euo pipefail

cd "$(dirname "$0")"
# shellcheck source=../lib/web-release.sh
source ../lib/web-release.sh
# shellcheck source=bare-lib.sh
source ./bare-lib.sh
bare_load_env

if [ -z "${BARE_SKIP_ROOT_CHECK:-}" ]; then
  [ "$(id -u)" = 0 ] || bare_die "请用 root 运行(sudo bash bare-web-deploy.sh)"
  [ -f /etc/nginx/nginx.conf ] || bare_die "nginx 未配置 —— 先跑 install.sh"
fi

DATA="$PREFIX/webdata"

cmd="${1:-}"
case "$cmd" in
  rollback)
    ts="${2:-}"
    [ -n "$ts" ] || bare_die "用法: bash bare-web-deploy.sh rollback <TS>  (TS 见 $DATA/releases/)"
    echo "==> 整版回滚 → $ts"
    wr_rollback "$DATA" "$ts"
    bare_reload_nginx
    echo "==> 完成: 已回滚并 reload nginx"
    ;;

  *)
    pkg="$cmd"
    [ -n "$pkg" ] || bare_die "用法: bash bare-web-deploy.sh <TS>.tar.gz | rollback <TS>"
    [ -f "$pkg" ] || bare_die "找不到产物包: $pkg"

    ctx="$(cd "$(mktemp -d)" && pwd)"
    trap 'rm -rf "$ctx"' EXIT
    wr_extract_pkg "$pkg" "$ctx"
    ts="$(wr_ts_from_dist "$ctx")"

    # 首次发布前初始化 root-manifest(语义与 docker 模式 web-deploy 一致)
    if [ ! -f "$DATA/root-manifest.txt" ]; then
      mkdir -p "$DATA/html"
      wr_init_after_copy "$DATA"
    fi

    echo "==> 发布前端 $ts → $DATA/html (排除 *.map / 保留 $WR_KEEP_VERSIONS 版)"
    license="$ctx/LICENSE"
    [ -f "$license" ] || license=""
    # 顺序 = 先写目录, 最后才 reload(R9: 避免覆盖期间的 404 窗口)
    wr_sync_dist "$ctx" "$DATA" "$ts" "$license"
    wr_archive_pkg "$pkg" "$DATA" "$ts"

    bare_reload_nginx
    bare_flush_caches
    echo "==> 完成: 前端 $ts 已发布, nginx 已 reload, 两层缓存已清"
    ;;
esac

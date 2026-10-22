#!/usr/bin/env bash
# Jerocine 后端发布（裸机部署）—— 解包产物包 → 轮换 $PREFIX/server → 重启 → 健康检查。
#
# 用法(服务器上, install.sh 已跑过):
#   sudo bash bare-deploy.sh /path/to/jerocine-server-<TS>-<sha7>-linux-<arch>.tar.gz
#
# 产物包与本机 scripts/build-server.sh 的 docker 模式包**完全同一个**(main + data/ +
# static/upload + LICENSE) —— 本机构建/上传链路不变, 只是服务器侧从"解包→docker build"
# 换成"解包→目录轮换→systemctl restart"。
#
# 失败语义: 健康检查不过 → 退出 1, 上一版保留在 $PREFIX/backup/server-<TS>(手动回退:
#   mv backup/server-<TS> server && systemctl restart jerocine-server), 不自动回滚
#   (与 docker 模式 server-deploy 失败不自动回退的口径一致)。
set -euo pipefail

cd "$(dirname "$0")"
# shellcheck source=bare-lib.sh
source ./bare-lib.sh
bare_load_env

pkg="${1:-}"
[ -n "$pkg" ] || bare_die "用法: bash bare-deploy.sh <jerocine-server-*.tar.gz>"
[ -f "$pkg" ] || bare_die "找不到产物包: $pkg"

# ---- 解包 + 校验(与 docker 模式 deploy_server_from_pkg 同一套断言) ----
ctx="$(cd "$(mktemp -d)" && pwd)"
trap 'rm -rf "$ctx"' EXIT
tar xzf "$pkg" -C "$ctx"
[ -f "$ctx/main" ] || bare_die "包内没有 main —— 用 scripts/build-server.sh 打包"
[ -d "$ctx/data" ] || bare_die "包内没有 data/（IP 归属地离线库）"
[ -d "$ctx/static/upload" ] || bare_die "包内没有 static/upload/（上传文件目录）"
[ -f "$ctx/LICENSE" ] || bare_die "包内没有 LICENSE"

# root / unit 检查(BARE_SKIP_ROOT_CHECK 仅供单测注入假环境)
if [ -z "${BARE_SKIP_ROOT_CHECK:-}" ]; then
  [ "$(id -u)" = 0 ] || bare_die "请用 root 运行(sudo bash bare-deploy.sh)"
  [ -f /etc/systemd/system/jerocine-server.service ] || bare_die "未安装 systemd unit —— 先跑 install.sh"
fi

# ---- 备份旧版 → 安装新版(目录整体轮换) ----
ts="$(date +%Y%m%d-%H%M%S)"
backup_name=""
if [ -d "$PREFIX/server" ]; then
  mkdir -p "$PREFIX/backup"
  bts="$ts"
  bump=0
  # 同一秒内重复发布时追加 -1/-2… 后缀; bump 显式初始化, 不依赖外部残留变量
  while [ -e "$PREFIX/backup/server-$bts" ]; do
    bump=$((bump + 1))
    bts="$ts-$bump"
  done
  backup_name="server-$bts"
  mv "$PREFIX/server" "$PREFIX/backup/$backup_name"
fi
mkdir -p "$PREFIX/server/static/upload"
cp -r "$ctx/main" "$ctx/data" "$ctx/static" "$ctx/LICENSE" "$PREFIX/server/"
chmod 755 "$PREFIX/server/main"   # Windows 打包产物无 exec 位(踩过: distroless permission denied)
if id jerocine >/dev/null 2>&1; then
  chown -R jerocine:jerocine "$PREFIX/server"
fi

# ---- 数据库迁移(先于新版启动; 失败即中止并恢复上一版) ----
# 上面已把新文件装进 $PREFIX/server(运行中的仍是旧 inode): 迁移失败若只 exit,
# 磁盘留的是"新二进制 + 旧结构", 将来任何一次重启/开机都会跑没迁移的代码 ——
# 必须把备份 mv 回来, 让磁盘与运行中的进程保持一致(都是旧版)。
if ! bare_run_migrations "$ctx/migrations"; then
  echo "!! 迁移失败, 发布中止" >&2
  if [ -n "$backup_name" ]; then
    rm -rf "$PREFIX/server"
    mv "$PREFIX/backup/$backup_name" "$PREFIX/server"
    echo "   已恢复上一版($PREFIX/server ← $backup_name); 服务未重启, 旧版仍在运行" >&2
  else
    echo "   首次安装无备份可恢复; 服务未启动, 排查后重跑本脚本即可(迁移幂等)" >&2
  fi
  exit 1
fi

# ---- 重启 + 健康检查 ----
echo "==> systemctl restart jerocine-server"
systemctl restart jerocine-server
if ! bare_wait_healthy; then
  if [ -n "$backup_name" ]; then
    echo "!! 发布失败。上一版在 $PREFIX/backup/$backup_name, 回退:" >&2
    echo "   mv $PREFIX/backup/$backup_name $PREFIX/server && systemctl restart jerocine-server" >&2
  else
    echo "!! 发布失败(首次安装, 无旧版可回退 —— 排查后重跑本脚本即可)。" >&2
  fi
  echo "   日志: journalctl -u jerocine-server -n 100" >&2
  exit 1
fi

# ---- 收尾: 两层缓存 + 留档 ----
bare_flush_caches

mkdir -p "$PREFIX/packages"
dest="$PREFIX/packages/$(basename "$pkg")"
if [ "$(readlink -f "$pkg")" != "$(readlink -f "$dest")" ]; then
  cp -f "$pkg" "$dest"
fi
bare_prune_by_name "$PREFIX/packages" \
  '^jerocine-server-[0-9]{8}-[0-9]{6}-[0-9a-f]{7}-linux-(amd64|arm64)\.tar\.gz$' 5
# 版本备份是目录, 单独裁剪(保留 3 份)
for d in $(ls -1dt "$PREFIX/backup"/server-* 2>/dev/null | tail -n +4 || true); do
  rm -rf "$d"
  echo "==> 清理旧备份 $(basename "$d")"
done

echo "==> 完成: 后端已发布并健康(systemd: jerocine-server)"
echo "   包留档: $dest(保留 5 份) / 版本备份保留 3 份"

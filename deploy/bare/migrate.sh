#!/usr/bin/env bash
# Jerocine 数据库迁移（裸机部署）—— golang-migrate up。
#
# 用法(服务器上, bare.env 就位后):
#   bash migrate.sh                    # 应用全部未执行的迁移
#   MIGRATE_BIN=... bash migrate.sh    # 指定二进制
#   MIGRATIONS_DIR=... bash migrate.sh # 指定迁移目录(缺省解析同仓 server/migrations)
#
# 与 docker 模式的 migrate 服务完全同源: 同一 migrations/、同一 golang-migrate v4.17.x、
# 同一 schema_migrations 表 —— 两种部署模式可互换, 迁移历史连续。
set -euo pipefail

cd "$(dirname "$0")"
# shellcheck source=bare-lib.sh
source ./bare-lib.sh
bare_load_env

MIGRATE_BIN="${MIGRATE_BIN:-/usr/local/bin/migrate}"
[ -x "$MIGRATE_BIN" ] || bare_die "找不到 golang-migrate: $MIGRATE_BIN —— 见 install.sh 第 7 步, 或设 MIGRATE_BIN"

# 本脚本在 deploy/bare/ 下, 仓库 server/migrations 在 deploy/ 的上两级
# (deploy/bare/../../server/migrations)。此前写成 ../server/migrations 会
# 解析到 deploy/server/migrations(不存在) —— 首次部署必失败且报错为空路径。
MIGRATIONS_DIR="${MIGRATIONS_DIR:-$(cd ../../server/migrations && pwd)}"
[ -d "$MIGRATIONS_DIR" ] || bare_die "迁移目录不存在: $MIGRATIONS_DIR"

echo "==> golang-migrate up ($MIGRATIONS_DIR)"
"$MIGRATE_BIN" -path "$MIGRATIONS_DIR" -database "$(bare_migrate_dsn)" up
echo "==> 迁移完成"

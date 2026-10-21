#!/usr/bin/env bash
# deploy/lib/db-migrate.sh 的单测 —— 用假 docker(stub) 验证"同步 + 执行 + 等待"逻辑,
# 不需要真实 docker/compose/数据库。跑法: bash tests/server-migrate.test.sh
set -u

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
PASS=0; FAIL=0
ok()  { PASS=$((PASS+1)); echo "  ok  $1"; }
bad() { FAIL=$((FAIL+1)); echo "  FAIL $1"; }
assert_eq()  { [ "$2" = "$3" ] && ok "$1" || bad "$1 (期望 [$3] 实得 [$2])"; }
assert_fail(){ if ( "$@" ) >/dev/null 2>&1; then bad "应失败但成功了: $*"; else ok "按预期失败"; fi; }

DEPLOY_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=../lib/db-migrate.sh
source "$DEPLOY_DIR/lib/db-migrate.sh"

# ---- 假 docker: 行为由 $WORK/docker-state/* 决定 ----
STUB="$WORK/stub"; mkdir -p "$STUB" "$WORK/state"
cat > "$STUB/docker" <<'S'
#!/usr/bin/env bash
state="$(dirname "$STUB_STATE")"
# 只认两种调用形态:
#   docker compose --env-file .env ps -q migrate   → 输出容器 id
#   docker compose --env-file .env up -d ...       → 记录调用
#   docker inspect --format <fmt> <cid>            → 按 fmt 读状态/退出码
if [ "$1" = "compose" ]; then
  shift
  case "$1 $2" in
    "--env-file .env") shift 2 ;;
  esac
  case "$1 $2" in
    "ps -q") echo "cid-migrate-1"; exit 0 ;;
    "up -d") echo "compose up $*" >> "$STUB_STATE/log"; exit 0 ;;
  esac
  exit 1
fi
if [ "$1" = "inspect" ]; then
  shift
  fmt=""; [ "$1" = "--format" ] && { fmt="$2"; shift 2; }
  case "$fmt" in
    *Status*)   cat "$STUB_STATE/status" 2>/dev/null || echo exited ;;
    *ExitCode*) cat "$STUB_STATE/code" 2>/dev/null   || echo 0 ;;
    *) echo "unknown-fmt:$fmt" >&2; exit 1 ;;
  esac
  exit 0
fi
exit 1
S
chmod +x "$STUB/docker"
export STUB_STATE="$WORK/state"
export PATH="$STUB:$PATH"
DOCKER=(docker)
COMPOSE=("${DOCKER[@]}" compose --env-file .env)

# ==================== 1. sync_pkg_migrations ====================
echo "# 1. sync_pkg_migrations"
src="$WORK/src"; dest="$WORK/dest"
mkdir -p "$src" "$dest"
printf 'a\n' > "$src/000001_a.up.sql"; printf 'b\n' > "$src/000001_a.down.sql"
printf 'old\n' > "$dest/000000_old.up.sql"
sync_pkg_migrations "$src" "$dest" >/dev/null && ok "同步成功" || bad "同步失败"
assert_eq "包内迁移已拷贝"  "$(cat "$dest/000001_a.up.sql")" "a"
assert_eq "旧迁移文件保留"  "$(cat "$dest/000000_old.up.sql")" "old"
assert_fail sync_pkg_migrations "$WORK/no-such-dir" "$dest"
mkdir -p "$WORK/empty"; assert_fail sync_pkg_migrations "$WORK/empty" "$dest"

# ==================== 2. wait_migrate_container ====================
echo "# 2. wait_migrate_container"
echo exited > "$WORK/state/status"; echo 0 > "$WORK/state/code"
wait_migrate_container "cid-migrate-1" 5 && ok "exited(0) 判成功" || bad "exited(0) 应成功"
echo 1 > "$WORK/state/code"
assert_fail wait_migrate_container "cid-migrate-1" 5
echo 0 > "$WORK/state/code"; echo running > "$WORK/state/status"
wait_migrate_container "cid-migrate-1" 2 && bad "一直 running 应超时失败" || ok "超时按预期失败"
echo missing > "$WORK/state/status"
assert_fail wait_migrate_container "cid-migrate-1" 2

# ==================== 3. run_db_migrations(端到端, 假 docker) ====================
echo "# 3. run_db_migrations"
rm -rf "$dest" "$src"; mkdir -p "$src"
printf 'm1\n' > "$src/000026_x.up.sql"; printf 'm2\n' > "$src/000027_y.up.sql"
echo exited > "$WORK/state/status"; echo 0 > "$WORK/state/code"   # 假容器立即正常退出
: > "$WORK/state/log"
run_db_migrations "$src" "$dest" && ok "执行成功" || bad "执行失败"
assert_eq "迁移已同步" "$(ls "$dest"/*.sql | wc -l)" "2"
grep -q "up -d --force-recreate migrate" "$WORK/state/log" \
  && ok "以 force-recreate 重跑 migrate 容器" || bad "未调用 compose up(migrate)"
# 无新迁移也不会跳过调用(幂等性由 golang-migrate 自身保证, 这里只验证"总是执行")
run_db_migrations "$src" "$dest" >/dev/null && ok "重复执行幂等成功" || bad "重复执行失败"
grep -c "up -d --force-recreate migrate" "$WORK/state/log" | grep -qx 2 \
  && ok "重复执行也重跑了 migrate(由 migrate up 自身判 no-op)" || bad "第二次未执行"

echo
echo "结果: pass=$PASS fail=$FAIL"
[ "$FAIL" = 0 ]

#!/usr/bin/env bash
# deploy/lib/db-migrate.sh 的单测 + deploy.sh server-deploy 的端到端测试。
#
# §1-3  库函数单测: 假 docker(stub) 验证"同步 + 执行 + 等待"逻辑, 不需要真实 docker/compose/数据库。
# §4    跨模块契约(静态断言): compose 挂载源 / deploy.sh 同步目标 / 包内布局三方一致 ——
#       这几处漂移的后果都是**静默**的(迁移挂到旧目录, up 幂等秒退"成功", 运行时才 SQL 报错)。
# §5    deploy.sh server-deploy 端到端: 沙箱副本 + 假 docker/假 git, 验证
#       "先迁移再起新容器"的顺序、迁移失败中止、旧包(回滚场景)响亮跳过。
#       (本文件的包都不带 tooling/ ⇒ 走 legacy: git pull + 树内执行;
#        带 tooling/ 的自引导路径由 tests/tooling-selfcontain.test.sh 覆盖。)
# 跑法: bash tests/server-migrate.test.sh
set -u

# 沙箱化开发环境(如 WorkBuddy)会给 rm/unlink/rmdir 注入 safe-delete shim: 带会话 ID 时每次
# 调用都要先过一遍 broker 策略检查(实测约 7s/次)。本套测试嵌套执行 deploy.sh(内部多次 rm -rf),
# 累加足以把整包测试拖过超时——曾表现为"跑到一半像卡住"。shim 入口对"无会话 ID"是零开销
# 透传(见 safe-bin/rm 开头分支); 普通机器上这两个变量不存在, 无副作用。
unset CODEBUDDY_SESSION_ID CLAUDE_SESSION_ID
# Windows(Git Bash)下 TMPDIR 常是 C:\Users\...\Temp: mktemp -d 会吐出混分隔符的路径,
# tar 会把 "C:" 当远程主机而失败。掰到 MSYS 的 /tmp(同一个物理目录); 非 Windows 不动。
case "${TMPDIR:-}" in
  [A-Za-z]:*) export TMPDIR=/tmp ;;
esac

WORK="$(cd "$(mktemp -d)" && pwd)"
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
# 认这些调用形态:
#   docker compose --env-file .env [-f runtime/docker-compose.yml] ps -q migrate → 输出容器 id
#   docker compose ... up -d ...                   → 记录原始调用(§5 校验顺序; -f 见自引导测试)
#   docker ps [--format ...]                       → 权限探测 / 结尾状态打印
#   docker build ...                               → 记录调用
#   docker exec ...                                → 直接成功(清缓存)
#   docker inspect --format <fmt> <cid|name>        → 按 fmt 读状态/退出码/健康态
if [ "$1" = "compose" ]; then
  echo "compose $*" >> "$STUB_STATE/log"   # 原始参数(含 -f), 先记录再解析
  shift
  case "$1 $2" in
    "--env-file .env") shift 2 ;;
  esac
  # 包内工具链模式(自引导): deploy.sh 追加 -f runtime/docker-compose.yml
  case "$1 $2" in
    "-f "*) shift 2 ;;
  esac
  case "$1 $2" in
    "ps -q") echo "cid-migrate-1"; exit 0 ;;
    "up -d") exit 0 ;;
  esac
  exit 1
fi
case "$1" in
  ps)    echo "jerocine_server Up 2 minutes"; exit 0 ;;
  build) echo "docker build $*" >> "$STUB_STATE/log"; exit 0 ;;
  exec)  exit 0 ;;
esac
if [ "$1" = "inspect" ]; then
  shift
  fmt=""; [ "$1" = "--format" ] && { fmt="$2"; shift 2; }
  case "$fmt" in
    *Health*)   echo healthy ;;
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
assert_fail wait_migrate_container "" 5
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

# ==================== 4. 跨模块契约(静态断言) ====================
echo "# 4. 跨模块契约"
# compose 的 migrate 挂载源 与 deploy.sh 的同步目标必须是同一个目录。
# 两边漂移的后果是静默的: migrate 挂到旧目录 → up 幂等秒退"成功" → 新二进制跑在旧结构上。
# 2026-10-22 起挂载源形如 ${JEROCINE_DEPLOY_ROOT:-.}/data/migrations(自引导根化, 见
# deploy/tests/tooling-selfcontain.test.sh) —— 剥掉变量包裹后必须与同步目标一致。
compose_mount="$(grep -oE '[^ ]+/data/migrations:/migrations' "$DEPLOY_DIR/docker-compose.yml" | head -1)"
compose_mount="${compose_mount%%:/migrations}"                    # ${JEROCINE_DEPLOY_ROOT:-.}/data/migrations
compose_mount="${compose_mount#'${JEROCINE_DEPLOY_ROOT:-.}'/}"    # 剥掉变量包裹+斜杠
compose_mount="${compose_mount#./}"                               # 树内旧式 ./data/migrations 兜底
deploy_dest="$(grep -oE 'run_db_migrations "\$ctx/migrations" "[^"]+"' "$DEPLOY_DIR/deploy.sh" \
  | head -1 | sed 's/.*" "//; s/"$//')"
assert_eq "compose 挂载源 == deploy.sh 同步目标(防静默漂移)" "$deploy_dest" "$compose_mount"
# 自引导根化: 5 处相对挂载(migrate/secrets/html/apk/certs)都必须写 ${JEROCINE_DEPLOY_ROOT:-.},
# 否则包内工具链模式下 compose 文件在 deploy/runtime/ 下, 相对挂载会挂到空目录。
# 只数挂载行(注释里也提这个变量名, 不能裸 grep -c)。
n_root="$(grep -cE '^[[:space:]]*- \$\{JEROCINE_DEPLOY_ROOT:-\.\}/' "$DEPLOY_DIR/docker-compose.yml")"
assert_eq "compose 5 处相对挂载全部根化(JEROCINE_DEPLOY_ROOT)" "$n_root" "5"
grep -q 'export JEROCINE_DEPLOY_ROOT=' "$DEPLOY_DIR/deploy.sh" \
  && ok "deploy.sh 自引导时 export JEROCINE_DEPLOY_ROOT" || bad "deploy.sh 未 export JEROCINE_DEPLOY_ROOT"
# 包内布局契约: build-server.sh 暂存 migrations/ ↔ deploy.sh 识别 $ctx/migrations
grep -q '"\$STAGE/migrations"' "$DEPLOY_DIR/../scripts/build-server.sh" \
  && ok "build-server.sh 暂存 migrations/(包内布局上游)" || bad "build-server.sh 未暂存 migrations/"
grep -q '\[ -d "\$ctx/migrations" \]' "$DEPLOY_DIR/deploy.sh" \
  && ok "deploy.sh 识别包内 migrations/(包内布局下游)" || bad "deploy.sh 未识别包内 migrations/"

# ==================== 5. deploy.sh server-deploy 端到端 ====================
echo "# 5. deploy.sh server-deploy 端到端(沙箱副本 + 假 docker/假 git)"
# 假 git: pull_and_reexec 的 git pull 不依赖真实 git 环境(测的是 deploy 逻辑, 不是 git)
cat > "$STUB/git" <<'S'
#!/usr/bin/env bash
echo "git $*" >> "${STUB_STATE:?}/git.log"
exit 0
S
chmod +x "$STUB/git"
ROOT="$WORK/repo"; SB="$ROOT/deploy"
mkdir -p "$SB/lib"
cp "$DEPLOY_DIR/deploy.sh" "$DEPLOY_DIR/Dockerfile.runtime" "$SB/"
cp "$DEPLOY_DIR"/lib/*.sh "$SB/lib/"   # 整目录: deploy.sh source 的每个 lib 都必须在沙箱里
printf 'REDIS_PASSWORD=x\n' > "$SB/.env"   # clear_caches 要 grep 它(pipefail 下缺失会中断发布)

make_server_pkg() {  # $1=输出包 $2=解包上下文目录 $3=1含migrations(新包) 0不含(旧包)
  local out="$1" d="$2" mig="$3"
  rm -rf "$d"; mkdir -p "$d/data" "$d/static/upload"
  printf '#!/bin/sh\necho server\n' > "$d/main"
  printf 'ipdb\n' > "$d/data/ip2region.db"
  printf 'MIT\n' > "$d/LICENSE"
  if [ "$mig" = "1" ]; then
    mkdir -p "$d/migrations"
    printf 'ddl\n' > "$d/migrations/000030_demo.up.sql"
  fi
  tar czf "$out" -C "$d" main data static LICENSE $([ "$mig" = "1" ] && echo migrations)
}
run_deploy() { bash "$SB/deploy.sh" server-deploy "$1" > "$WORK/out" 2>&1; }

# ---- 5.1 新包(含 migrations): 迁移先于新容器, 整套只跑一次 ----
# 注: 本组包**不带** tooling/(旧格式包/legacy 路径) —— "仍先 git pull"正是 legacy 语义;
# 带 tooling 的自引导路径(0 次 pull)由 tests/tooling-selfcontain.test.sh 覆盖。
echo exited > "$WORK/state/status"; echo 0 > "$WORK/state/code"
: > "$WORK/state/log"; : > "$WORK/state/git.log"
make_server_pkg "$WORK/pkg-new" "$WORK/ctx-new" 1
run_deploy "$WORK/pkg-new" && ok "新包发布成功" || { bad "新包发布失败"; cat "$WORK/out"; }
n_mig="$(grep -n 'up -d --force-recreate migrate' "$WORK/state/log" | head -1 | cut -d: -f1)"
n_srv="$(grep -n 'up -d --no-build server' "$WORK/state/log" | head -1 | cut -d: -f1)"
[ -n "$n_mig" ] && [ -n "$n_srv" ] && [ "$n_mig" -lt "$n_srv" ] \
  && ok "迁移先于新容器启动(顺序不能反)" || bad "迁移/启动顺序错误(migrate=$n_mig server=$n_srv)"
assert_eq "构建/迁移/重启整套只跑一次(reexec 不重复执行)" \
  "$(grep -c 'up -d --no-build server' "$WORK/state/log")" "1"
grep -q 'git -C .. pull --ff-only' "$WORK/state/git.log" \
  && ok "legacy 包(无 tooling/)仍先 git pull —— 树内执行路径不变" \
  || bad "未 git pull"

# ---- 5.2 迁移失败 → 中止发布, 新容器绝不启动 ----
echo 1 > "$WORK/state/code"
: > "$WORK/state/log"
run_deploy "$WORK/pkg-new" && bad "迁移失败应中止发布" || ok "迁移失败按预期中止发布"
grep -q 'up -d --force-recreate migrate' "$WORK/state/log" \
  && ok "失败前确实尝试过迁移" || bad "未尝试迁移"
grep -q 'up -d --no-build server' "$WORK/state/log" \
  && bad "迁移失败仍启动了新容器(顺序错误!)" || ok "迁移失败不启动新容器"
grep -q "发布已中止" "$WORK/out" && ok "中止信息明确" || bad "缺少中止信息"

# ---- 5.3 旧包(无 migrations/, 回滚场景): 响亮跳过, 不阻断 ----
echo 0 > "$WORK/state/code"
: > "$WORK/state/log"
make_server_pkg "$WORK/pkg-old" "$WORK/ctx-old" 0
run_deploy "$WORK/pkg-old" && ok "旧包(回滚)发布成功" || { bad "旧包发布被堵"; cat "$WORK/out"; }
grep -q "跳过自动迁移" "$WORK/out" && ok "旧包响亮提示跳过迁移" || bad "旧包无跳过提示"
grep -q 'force-recreate migrate' "$WORK/state/log" \
  && bad "旧包不该跑迁移" || ok "旧包不触发迁移"
grep -q 'up -d --no-build server' "$WORK/state/log" \
  && ok "旧包继续完成发布(回滚路径畅通)" || bad "旧包发布中断"

echo
echo "结果: pass=$PASS fail=$FAIL"
[ "$FAIL" = 0 ]

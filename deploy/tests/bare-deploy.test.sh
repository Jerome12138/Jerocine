#!/usr/bin/env bash
# deploy/bare 的单测 —— 纯文件系统/字符串语义, 不需要 apt/systemctl/网络。
#   cd deploy && env -u BASH_ENV -u 'BASH_FUNC_rm%%' -u 'BASH_FUNC_unlink%%' -u 'BASH_FUNC_rmdir%%' \
#     TMPDIR=/tmp bash tests/bare-deploy.test.sh
#
# 覆盖真正会出事的地方: server.env 组装(与 compose environment 逐项对应) /
# bare-deploy 解包校验→备份轮换→留档裁剪→健康失败语义 / bare-web-deploy 发布与回滚接线 /
# migrate DSN 格式。
set -uo pipefail

cd "$(dirname "$0")/.."          # → deploy/
# shellcheck source=../bare/bare-lib.sh
source ./bare/bare-lib.sh

pass=0
fail=0
ok()   { pass=$((pass + 1)); echo "  ok   $*"; }
bad()  { fail=$((fail + 1)); echo "  FAIL $*"; }
assert_eq()   { [ "$2" = "$3" ] && ok "$1" || bad "$1 (期望 [$3] 实得 [$2])"; }
assert_has()  { [ -e "$2" ] && ok "$1" || bad "$1 (缺少 $2)"; }
assert_gone() { [ ! -e "$2" ] && ok "$1" || bad "$1 (不该存在 $2)"; }
assert_grep() { printf '%s' "$2" | grep -qF -- "$3" && ok "$1" || bad "$1 (输出缺 [$3])"; }
assert_fail() { if ( "$@" ) >/dev/null 2>&1; then bad "应失败但成功了: $*"; else ok "按预期失败: $*"; fi; }

WORK="$(cd "$(mktemp -d)" && pwd)"
trap 'rm -rf "$WORK"' EXIT

# ---------- 通用测试环境: 假 PREFIX + bare.env + stub 命令目录 ----------
STUB="$WORK/bin"
mkdir -p "$STUB"
cat > "$STUB/curl" <<'S'
#!/usr/bin/env bash
exit "${STUB_CURL_EXIT:-0}"
S
cat > "$STUB/systemctl" <<'S'
#!/usr/bin/env bash
echo "systemctl $*" >> "${STUB_LOG:?}/systemctl.log"
exit 0
S
cat > "$STUB/redis-cli" <<'S'
#!/usr/bin/env bash
echo "redis-cli $*" >> "${STUB_LOG:?}/redis.log"
exit 0
S
chmod +x "$STUB"/*

P="$WORK/prefix"
mkdir -p "$P"
cat > "$WORK/bare.env" <<'ENV'
PREFIX=__P__
MYSQL_ROOT_PASSWORD=root_pw
MYSQL_DATABASE=jerocine
MYSQL_USER=jerocine
MYSQL_PASSWORD=app_pw
REDIS_PASSWORD=redis_pw
REDIS_MAXMEMORY=512mb
SPIDER_RESET_TOKEN=token_x
ENV
sed -i "s|__P__|$P|" "$WORK/bare.env"
export BARE_ENV="$WORK/bare.env"
export STUB_LOG="$WORK/log" PATH="$STUB:$PATH" BARE_SKIP_ROOT_CHECK=1 \
       BARE_HEALTH_TRIES=2 BARE_HEALTH_SLEEP=0 NGINX_RELOAD_CMD="echo nginx-reloaded"
mkdir -p "$STUB_LOG"

bare_load_env

# 回归(鸡生蛋): 全新机器 PREFIX 不存在时, no_prefix_check 放行 / 默认检查拒绝
# (用不含 PREFIX 的临时 env —— 否则 env 文件里的 PREFIX= 行会覆盖命令行注入值)
printf 'DUMMY_NO_PREFIX_CHECK=1\n' > "$WORK/bare-noprefix.env"
BARE_ENV="$WORK/bare-noprefix.env" PREFIX="$WORK/nonexistent-prefix" bare_load_env no_prefix_check \
  && ok "no_prefix_check 放行(全新机器)" || bad "no_prefix_check 应放行"
( BARE_ENV="$WORK/bare-noprefix.env" PREFIX="$WORK/nonexistent-prefix" bare_load_env ) >/dev/null 2>&1 \
  && bad "PREFIX 缺失时默认检查应拒绝" || ok "PREFIX 缺失时默认检查按预期拒绝"

# ==================== 1. server.env 组装(与 compose environment 对齐) ====================
echo "# 1. bare_derive_server_env"

env_out="$(bare_derive_server_env "$P")"
assert_grep "DSN 指向 127.0.0.1"        "$env_out" 'MYSQL_DSN=jerocine:app_pw@(127.0.0.1:3306)/jerocine?charset=utf8mb4&parseTime=True&loc=Local'
assert_grep "Redis 指向 127.0.0.1"       "$env_out" 'REDIS_ADDR=127.0.0.1:6379'
assert_grep "Redis 双库分离"             "$env_out" 'REDIS_DB_CACHE=0'
assert_grep "Redis 协调库"               "$env_out" 'REDIS_DB_COORD=1'
assert_grep "JWT 私钥随 PREFIX"          "$env_out" "JWT_PRIVATE_KEY_FILE=$P/secrets/jwt_private.pem"
assert_grep "BLOB 落 PREFIX 内"          "$env_out" "BLOB_LOCAL_DIR=$P/server/static/upload"
assert_grep "BLOB 基址不变"              "$env_out" 'BLOB_BASE_URL=/api/upload/'
assert_grep "GIN release"                "$env_out" 'GIN_MODE=release'
assert_grep "默认 TTL 168h"              "$env_out" 'AUTH_TOKEN_TTL_HOURS=168'
assert_grep "默认设备数 5"               "$env_out" 'MAX_DEVICE_LOGIN=5'

bare_write_server_env "$P"
# MSYS(Git Bash) 的 chmod 是模拟实现, stat 读回 644 属环境噪声 —— 真实 Linux 上 600 生效
case "$(uname -s)" in
  *MSYS*|*MINGW*) ok "server.env 权限(跳过: MSYS 模拟 chmod 不可断言)" ;;
  *) assert_eq "server.env 权限收紧到 600" "$(stat -c %a "$P/server.env")" "600" ;;
esac

# ==================== 2. bare_prune_by_name ====================
echo "# 2. bare_prune_by_name"
mkdir -p "$WORK/arch"
for i in 1 2 3 4 5; do : > "$WORK/arch/jerocine-server-2026010$i-120000-deadbee-linux-amd64.tar.gz"; done
bare_prune_by_name "$WORK/arch" '^jerocine-server-.*\.tar\.gz$' 3 >/dev/null
assert_eq  "保留最新 3 份" "$(ls -1 "$WORK/arch" | wc -l)" "3"
assert_has "保留最新一份" "$WORK/arch/jerocine-server-20260105-120000-deadbee-linux-amd64.tar.gz"
assert_gone "裁掉最旧一份" "$WORK/arch/jerocine-server-20260101-120000-deadbee-linux-amd64.tar.gz"

# ==================== 3. bare_migrate_dsn ====================
echo "# 3. bare_migrate_dsn"
assert_eq "migrate DSN 格式(与 compose migrate 同构)" "$(bare_migrate_dsn)" \
  'mysql://jerocine:app_pw@tcp(127.0.0.1:3306)/jerocine?multiStatements=true'

# ==================== 4. bare-deploy.sh 端到端(假包 + stub systemctl/curl) ====================
echo "# 4. bare-deploy.sh"

make_pkg() {
  local pkg="$1" mark="$2"
  local ctx="$WORK/pkgctx"
  rm -rf "$ctx"
  mkdir -p "$ctx/data" "$ctx/static/upload"
  printf '#!/bin/sh\necho server-%s\n' "$mark" > "$ctx/main"
  printf 'ipdb\n'            > "$ctx/data/ip2region.db"
  printf '%s\n' "$mark"     > "$ctx/static/VERSION"   # 标记文件, 随 static 一起被安装
  printf 'MIT\n'             > "$ctx/LICENSE"
  tar czf "$pkg" -C "$ctx" main data static LICENSE
}

pkg1="$WORK/jerocine-server-20260101-120000-deadbee-linux-amd64.tar.gz"
make_pkg "$pkg1" v1
BARE_HEALTH_TRIES=1 bash bare/bare-deploy.sh "$pkg1" >"$WORK/out1" 2>&1 \
  && ok "v1 发布成功" || { bad "v1 发布失败"; cat "$WORK/out1"; }
assert_has "server/main 落位"        "$P/server/main"
assert_eq   "main 恢复 exec 位"      "$(stat -c %a "$P/server/main")" "755"
assert_has "static/upload 目录在位"  "$P/server/static/upload"
assert_has "包已留档"                "$P/packages/jerocine-server-20260101-120000-deadbee-linux-amd64.tar.gz"
grep -q "restart jerocine-server" "$STUB_LOG/systemctl.log" \
  && ok "systemctl restart 已调用" || bad "未调用 systemctl restart"

# v2 → 备份轮换; 连发 5 版 → 备份保留 3
for v in 2 3 4 5; do
  pkgv="$WORK/jerocine-server-2026010$v-120000-deadbee-linux-amd64.tar.gz"
  make_pkg "$pkgv" "v$v"
  BARE_HEALTH_TRIES=1 bash bare/bare-deploy.sh "$pkgv" >"$WORK/out$v" 2>&1 \
    || { bad "v$v 发布失败"; cat "$WORK/out$v"; }
done
assert_eq  "备份保留 3 份" "$(ls -1d "$P/backup"/server-* | wc -l)" "3"
assert_eq  "当前是最新版 v5" "$(cat "$P/server/static/VERSION")" "v5"
assert_eq  "包留档保留 5 份" "$(ls -1 "$P/packages" | wc -l)" "5"

# 健康检查失败 → 退出 1 + 给出回退指引, 当前目录保持新包(不静默回滚, 与 docker 口径一致)
pkg6="$WORK/jerocine-server-20260106-120000-deadbee-linux-amd64.tar.gz"
make_pkg "$pkg6" v6
STUB_CURL_EXIT=1 BARE_HEALTH_TRIES=1 bash bare/bare-deploy.sh "$pkg6" >"$WORK/out6" 2>&1 \
  && bad "健康失败应退出非 0" || ok "健康失败按预期退出非 0"
assert_grep "失败输出回退指引" "$(cat "$WORK/out6")" "backup"
assert_eq   "新包已安装(等人工处置)" "$(cat "$P/server/static/VERSION")" "v6"

# 校验断言: 缺 LICENSE 的包被拒
ctx="$WORK/pkgctx-bad"; rm -rf "$ctx"; mkdir -p "$ctx/static/upload"
: > "$ctx/main"; : > "$ctx/LICENSE"
pkgbad="$WORK/jerocine-server-20260107-000000-deadbee-linux-amd64.tar.gz"
tar czf "$pkgbad" -C "$ctx" . 2>/dev/null
bash bare/bare-deploy.sh "$pkgbad" >/dev/null 2>&1 && bad "缺 data/ 的包应被拒" || ok "缺 data/ 的包按预期被拒"

# ==================== 5. bare-web-deploy.sh 发布与回滚 ====================
echo "# 5. bare-web-deploy.sh"

make_web_pkg() {
  local pkg="$1" ts="$2"
  local d="$WORK/webdist-$ts"
  rm -rf "$d"
  mkdir -p "$d/assets/$ts"
  printf '<html>%s</html>\n' "$ts"                    > "$d/index.html"
  printf 'console.log("%s")\n' "$ts"                  > "$d/assets/$ts/app.js"
  printf '{"version":3}\n'                            > "$d/assets/$ts/app.js.map"
  printf 'sw\n'                                       > "$d/sw.js"
  printf 'MIT\n'                                      > "$d/LICENSE"
  tar czf "$pkg" -C "$d" .
}

w1="$WORK/20260101-120000.tar.gz"
make_web_pkg "$w1" 20260101-120000
bash bare/bare-web-deploy.sh "$w1" >"$WORK/outw1" 2>&1 \
  && ok "web v1 发布成功" || { bad "web v1 发布失败"; cat "$WORK/outw1"; }
assert_has "html/index.html 落位"   "$P/webdata/html/index.html"
assert_has "assets 版本目录"        "$P/webdata/html/assets/20260101-120000/app.js"
assert_gone "map 被排除在公开根外"  "$P/webdata/html/assets/20260101-120000/app.js.map"
assert_has "map 归档到 debugmap"    "$P/webdata/debugmap/20260101-120000"
assert_has "包留档"                 "$P/webdata/packages/20260101-120000.tar.gz"
grep -q "nginx-reloaded" "$WORK/outw1" && ok "nginx reload 已调用" || bad "未 reload nginx"

# 第二版发布 → root-manifest 更新(v2 的根条目: index.html / sw.js / LICENSE)
w2="$WORK/20260102-120000.tar.gz"
make_web_pkg "$w2" 20260102-120000
bash bare/bare-web-deploy.sh "$w2" >/dev/null 2>&1 || bad "web v2 发布失败"
assert_eq "root-manifest 条目数(v2 根文件)" "$(grep -c . "$P/webdata/root-manifest.txt")" "3"

# 整版回滚
bash bare/bare-web-deploy.sh rollback 20260101-120000 >"$WORK/outrb" 2>&1 \
  && ok "回滚成功" || { bad "回滚失败"; cat "$WORK/outrb"; }
assert_grep "index.html 已回到 v1" "$(cat "$P/webdata/html/index.html")" "20260101-120000"

# 回滚到窗口外的版本必须失败
assert_fail bash bare/bare-web-deploy.sh rollback 20990101-000000

# ==================== 6. migrate.sh 接线(假 migrate 二进制) ====================
echo "# 6. migrate.sh"
cat > "$STUB/migrate" <<'S'
#!/usr/bin/env bash
echo "migrate-bin $*" >> "${STUB_LOG:?}/migrate.log"
S
chmod +x "$STUB/migrate"
mkdir -p "$WORK/migs"
: > "$WORK/migs/0001_init.up.sql"
MIGRATE_BIN="$STUB/migrate" MIGRATIONS_DIR="$WORK/migs" bash bare/migrate.sh >"$WORK/outm" 2>&1 \
  && ok "migrate.sh 调用成功" || { bad "migrate.sh 失败"; cat "$WORK/outm"; }
grep -q -- "-path $WORK/migs" "$STUB_LOG/migrate.log" && ok "迁移目录传入" || bad "迁移目录未传入"
grep -q "tcp(127.0.0.1:3306)/jerocine?multiStatements=true" "$STUB_LOG/migrate.log" \
  && ok "DSN 指向本机 MySQL" || bad "DSN 不对"

# 默认路径分支(不带 MIGRATIONS_DIR): 应解析到仓库 server/migrations
# (migrate.sh 在 deploy/bare/ 下, ../../server/migrations 才是正确指向 —— 回归旧 bug)
MIGRATE_BIN="$STUB/migrate" bash bare/migrate.sh >"$WORK/outm2" 2>&1 \
  && ok "migrate.sh 默认路径解析成功" || { bad "migrate.sh 默认路径解析失败"; cat "$WORK/outm2"; }
grep -Eq -- "-path .*/server/migrations -database" "$STUB_LOG/migrate.log" \
  && ok "默认迁移目录解析到仓库 server/migrations" || bad "默认迁移目录解析错误(见 migrate.log)"

# ==================== 7. 密码字符集校验(渲染前 fail-fast) ====================
echo "# 7. bare_assert_secret_charset"
bare_assert_secret_charset MYSQL_PASSWORD REDIS_PASSWORD SPIDER_RESET_TOKEN \
  && ok "合法密码通过校验" || bad "合法密码应通过校验"
( MYSQL_PASSWORD="bad|pw" bare_assert_secret_charset MYSQL_PASSWORD ) >/dev/null 2>&1 \
  && bad "含 | 的密码应被拒" || ok "含 | 的密码按预期被拒"
( MYSQL_PASSWORD="bad'pw" bare_assert_secret_charset MYSQL_PASSWORD ) >/dev/null 2>&1 \
  && bad "含 ' 的密码应被拒" || ok "含 ' 的密码按预期被拒"
( REDIS_PASSWORD='bad&pw' bare_assert_secret_charset REDIS_PASSWORD ) >/dev/null 2>&1 \
  && bad "含 & 的密码应被拒" || ok "含 & 的密码按预期被拒"
( SPIDER_RESET_TOKEN="bad pw" bare_assert_secret_charset SPIDER_RESET_TOKEN ) >/dev/null 2>&1 \
  && bad "含空白的 token 应被拒" || ok "含空白的 token 按预期被拒"

# ==================== 8. install.sh 静态断言(不真跑 install —— apt/systemctl 不可注入) ====================
echo "# 8. install.sh 静态断言"
for f in bare/install.sh bare/bare-deploy.sh bare/bare-web-deploy.sh bare/migrate.sh bare/bare-lib.sh; do
  bash -n "$f" && ok "语法: $f" || bad "语法错误: $f"
done
# 历史坑①: 编辑吃掉行尾换行 → 两条命令合并成一行(bash -n 查不出, 运行时 set -e 才炸)
grep -q '^chown root:redis "\$REDIS_CONF"$' bare/install.sh \
  && ok "REDIS_CONF chown 独立成行" || bad "REDIS_CONF 行被合并(历史坑①复发)"
# 历史坑②: 容忍 migrate 缺失却用 `[ -x ] && echo` —— set -e 会在收尾杀掉 install
grep -qF '[ -x "$MIGRATE_BIN" ] && echo' bare/install.sh \
  && bad "migrate 检查又变回 [ -x ] && echo(历史坑②复发)" || ok "migrate 检查用 if/else 兜底"
# 本次审查修复: 渲染前密码字符校验已接入 install.sh(fail-fast, 防 sed/SQL 静默破坏)
grep -qF 'bare_assert_secret_charset MYSQL_PASSWORD REDIS_PASSWORD SPIDER_RESET_TOKEN' bare/install.sh \
  && ok "install.sh 已接入密码字符校验" || bad "install.sh 未接入密码字符校验"

echo
echo "结果: pass=$pass fail=$fail"
[ "$fail" = 0 ]

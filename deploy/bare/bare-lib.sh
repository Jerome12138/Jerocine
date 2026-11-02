#!/usr/bin/env bash
# Jerocine 裸机部署共享库 —— 被 install.sh / bare-deploy.sh / bare-web-deploy.sh / migrate.sh source。
# 与 deploy/lib/web-release.sh 同一设计原则: 纯文件系统/字符串语义, 不做 apt/systemctl 等副作用,
# 可在本机用假目录完整单测(deploy/tests/bare-deploy.test.sh)。
set -euo pipefail

bare_die() { echo "!! $*" >&2; exit 1; }

# bare_assert_secret_charset <VAR...> —— 渲染前校验密码/密钥类变量(fail-fast)。
# 这些值会进入三处敏感上下文:
#   ① install.sh 建用户 SQL 内插 → 禁 ' 与 `(单引号/反引号会破坏语句或注入);
#   ② sed 替换串(redis 密码 / nginx @PREFIX@ 渲染, 分隔符 |) → 禁 | 与 &;
#   ③ server.env 被 shell source(bare.env 本身) 与 systemd EnvironmentFile 解析
#      → 禁空白(DSN 同样不允许)。
#   @ : / 是 go-sql-driver DSN 保留字(compose 模式同样有此限制)。
# 与其让安装进行到一半被静默破坏的配置炸掉, 不如开头就拒绝。
bare_assert_secret_charset() {
  local name val
  for name in "$@"; do
    val="${!name:-}"
    [ -n "$val" ] || bare_die "$name 未设置(bare.env) —— 见 bare.env.example"
    case "$val" in
      *"'"*) bare_die "$name 含禁止字符 ' —— 见 bare.env.example 密码约束" ;;
      *'`'*) bare_die "$name 含禁止字符 \` —— 见 bare.env.example 密码约束" ;;
      *'|'*) bare_die "$name 含禁止字符 | —— 见 bare.env.example 密码约束" ;;
      *'&'*) bare_die "$name 含禁止字符 & —— 见 bare.env.example 密码约束" ;;
      *'@'*) bare_die "$name 含禁止字符 @ —— 见 bare.env.example 密码约束" ;;
      *':'*) bare_die "$name 含禁止字符 : —— 见 bare.env.example 密码约束" ;;
      *'/'*) bare_die "$name 含禁止字符 / —— 见 bare.env.example 密码约束" ;;
      *[[:space:]]*) bare_die "$name 含空白字符 —— 见 bare.env.example 密码约束" ;;
    esac
  done
}

# bare_script_dir —— 调用方脚本所在目录(bare/), 在 source 前由脚本自身定义亦可, 此处兜底解析。
bare_script_dir() {
  local src="${BASH_SOURCE[1]:-${BASH_SOURCE[0]}}"
  cd "$(dirname "$src")" && pwd
}

# bare_load_env —— 读 $PREFIX 同目录的 bare.env(或 BARE_ENV 显式指定)。
# 所有键 set -a 导出, 供后续 systemctl / mysql / redis-cli 子进程使用。
# 第一参数传 "no_prefix_check" 可跳过 PREFIX 存在性检查(install.sh 专用:
# 全新机器上 PREFIX 还没建, 检查会让安装脚本第一步就死 —— 鸡生蛋)。
bare_load_env() {
  local env="${BARE_ENV:-$(bare_script_dir)/bare.env}"
  [ -f "$env" ] || bare_die "找不到 $env —— 先 cp bare.env.example bare.env 并填生产值"
  # shellcheck disable=SC1090
  set -a; . "$env"; set +a
  PREFIX="${PREFIX:-/opt/jerocine}"
  if [ "${1:-}" != "no_prefix_check" ] && [ ! -d "$PREFIX" ]; then
    bare_die "$PREFIX 不存在 —— 先跑 install.sh 完成初始化"
  fi
}

# bare_derive_server_env —— 把 bare.env 的"零件"组装成 server 进程真正读的一组环境变量。
# 输出到 stdout(key=value 行, 可直接作为 systemd EnvironmentFile)。
# 与 docker-compose.yml server.environment 逐项对应, 差异只有地址类:
#   mysql: mysql:3306 → 127.0.0.1:3306;  redis: redis:6379 → 127.0.0.1:6379;
#   JWT:   /etc/jerocine/secrets → $PREFIX/secrets;  BLOB_LOCAL_DIR 随 PREFIX。
# ⚠ DSN 密码含 @ ( ) / 等字符需按 go-sql-driver 规则转义(compose 模式同样有此限制)。
bare_derive_server_env() {
  local prefix="${1:?用法: bare_derive_server_env <prefix>}"
  cat <<EOF
TZ=Asia/Shanghai
SERVER_PORT=3601
MYSQL_DSN=${MYSQL_USER:-jerocine}:${MYSQL_PASSWORD}@(127.0.0.1:3306)/${MYSQL_DATABASE:-jerocine}?charset=utf8mb4&parseTime=True&loc=Local
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=${REDIS_PASSWORD}
REDIS_DB_CACHE=0
REDIS_DB_COORD=1
JWT_PRIVATE_KEY_FILE=${prefix}/secrets/jwt_private.pem
JWT_PUBLIC_KEY_FILE=${prefix}/secrets/jwt_public.pem
JWT_ISSUER=Jerocine
CORS_ALLOWED_ORIGINS=${CORS_ALLOWED_ORIGINS:-}
SPIDER_RESET_TOKEN=${SPIDER_RESET_TOKEN}
BLOB_DRIVER=local
BLOB_LOCAL_DIR=${prefix}/server/static/upload
BLOB_BASE_URL=/api/upload/
TMDB_LANG=${TMDB_LANG:-zh-CN}
TMDB_API_BASE=${TMDB_API_BASE:-}
TMDB_IMAGE_BASE=${TMDB_IMAGE_BASE:-}
GIN_MODE=release
EOF
}

# bare_write_server_env —— 生成 $PREFIX/server.env 并收紧权限(server 进程读它, 属 jerocine)。
bare_write_server_env() {
  local prefix="${1:?用法: bare_write_server_env <prefix>}"
  bare_derive_server_env "$prefix" > "$prefix/server.env"
  chmod 600 "$prefix/server.env"
  if id jerocine >/dev/null 2>&1; then
    chown jerocine:jerocine "$prefix/server.env"
  fi
}

# bare_wait_healthy —— 轮询 GET /healthz(server 自带探针端点)。
# 默认 60×5s = 5 分钟超时; BARE_HEALTH_TRIES / BARE_HEALTH_SLEEP 可覆盖(单测用)。
bare_wait_healthy() {
  local port="${SERVER_PORT:-3601}"
  local tries="${BARE_HEALTH_TRIES:-60}" sleep_s="${BARE_HEALTH_SLEEP:-5}" i
  echo "==> 等待 jerocine-server healthy(127.0.0.1:$port/healthz)..."
  for i in $(seq 1 "$tries"); do
    if curl -fsS -o /dev/null "http://127.0.0.1:$port/healthz" 2>/dev/null; then
      return 0
    fi
    sleep "$sleep_s"
  done
  echo "!! ${tries} 次探活未通过, 查日志: journalctl -u jerocine-server -n 100" >&2
  return 1
}

# bare_prune_by_name <dir> <regex> <keep> —— 目录内按名排序保留最新 keep 份(发布包/备份通用)。
bare_prune_by_name() {
  local dir="${1:?}" re="${2:?}" keep="${3:?}" f i=0
  [ -d "$dir" ] || return 0
  for f in $(ls -1 "$dir" 2>/dev/null | grep -E "$re" | sort -r || true); do
    i=$((i + 1))
    [ "$i" -le "$keep" ] && continue
    rm -f "$dir/$f"
    echo "==> 清理旧留档 $f(超出保留 $keep 份)"
  done
}

# bare_flush_caches —— 清两层缓存(与 docker 模式 clear_caches 对齐):
#   nginx proxy_cache + Redis DB0(应用缓存)。DB1 是协调态, 禁动。
bare_flush_caches() {
  echo "==> 清 nginx proxy_cache"
  rm -rf "${NGINX_CACHE_DIR:-/var/cache/nginx-jerocine}"/* 2>/dev/null || true
  echo "==> 清 Redis 应用缓存(DB0)"
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli --no-auth-warning -h 127.0.0.1 -n 0 FLUSHDB >/dev/null 2>&1 \
      || echo "   (redis-cli 清理失败, 不阻断 —— 缓存可重建)"
  fi
}

# bare_reload_nginx —— 先 -t 再 reload; 测试环境可用 NGINX_RELOAD_CMD 覆盖。
bare_reload_nginx() {
  if [ -n "${NGINX_RELOAD_CMD:-}" ]; then
    # shellcheck disable=SC2086
    eval "$NGINX_RELOAD_CMD"
    return 0
  fi
  nginx -t || bare_die "nginx -t 未通过, 保持旧配置运行 —— 检查后手动 systemctl reload nginx"
  systemctl reload nginx
}

# bare_run_migrations <dir> —— 用 golang-migrate 应用 <dir> 下的迁移(幂等, 无新迁移立即退出)。
#   bare-deploy 在重启新二进制**之前**调用: 失败即中止发布, 线上保持旧版本。
#   二进制缺失(MIGRATE_BIN 未装/离线安装失败)时响亮告警并跳过 —— 不阻断发布,
#   但必须手动跑 migrate.sh, 否则新代码读写新列会 SQL 报错。
bare_run_migrations() {
  local dir="$1" bin="${MIGRATE_BIN:-/usr/local/bin/migrate}"
  if [ ! -d "$dir" ] || ! ls "$dir"/*.sql >/dev/null 2>&1; then
    echo "==> 包内无 migrations/, 跳过迁移(旧版产物包)"
    return 0
  fi
  if [ ! -x "$bin" ]; then
    echo "!! 找不到 golang-migrate($bin) —— 本次发布跳过自动迁移。" >&2
    echo "!! 务必手动执行(迁移目录用本包, 与二进制同版; 缺省解析 git 树, 可能落后):" >&2
    echo "   MIGRATE_BIN=$bin MIGRATIONS_DIR=$dir bash $(dirname "${BASH_SOURCE[0]}")/migrate.sh" >&2
    return 0
  fi
  echo "==> 数据库迁移(golang-migrate up: $dir)"
  "$bin" -path "$dir" -database "$(bare_migrate_dsn)" up
}

# bare_migrate_dsn —— golang-migrate 的 mysql DSN(与 compose migrate 服务同一格式, 地址换 127.0.0.1)。
bare_migrate_dsn() {
  printf 'mysql://%s:%s@tcp(127.0.0.1:3306)/%s?multiStatements=true' \
    "${MYSQL_USER:-jerocine}" "${MYSQL_PASSWORD}" "${MYSQL_DATABASE:-jerocine}"
}

#!/usr/bin/env bash
# Jerocine 裸机(无 Docker)一键安装 —— Ubuntu 22.04/24.04, 需 root。
#
# 用法(在目标服务器上):
#   cp bare.env.example bare.env && vi bare.env   # 填生产值
#   sudo bash install.sh
#
# 做什么(幂等, 可重复执行; 与脚本内 [n/7] 步对应):
#   1. 预置 MySQL 配置(必须先于 apt 安装/首次初始化 —— lower_case_table_names 时序铁律)
#   2. apt 安装 mysql-server / redis-server / nginx / curl
#   3. MySQL: 建库建用户(localhost 与 127.0.0.1 两个 host 都建)
#   4. Redis: 渲染独立配置 + systemd drop-in 覆盖 ExecStart(不动发行版默认 conf)
#   5. 系统 jerocine 用户 + $PREFIX 目录树 + server.env(由 bare.env 组装) + JWT 密钥
#   6. systemd unit(jerocine-server) enable, nginx 配置落位(旧文件备份)
#   7. golang-migrate v4.17.1 二进制(缺则从 GitHub 下载; 服务器可达 GitHub)
#
# 不做什么: 不发业务包(server/前端由 bare-deploy.sh / bare-web-deploy.sh 各自发布),
# 不 start jerocine-server(包还没到位), 不动宿主机 80 端口以外的其他服务。
set -euo pipefail

cd "$(dirname "$0")"
# shellcheck source=bare-lib.sh
source ./bare-lib.sh

[ "$(id -u)" = 0 ] || bare_die "请用 root 运行(sudo bash install.sh)"
# bare.env 里的 PREFIX 指向的目录此刻可能还不存在(全新机器) —— 先建出来,
# 所以这里用 no_prefix_check 跳过存在性检查(否则鸡生蛋: 读 env 就死)。
bare_load_env no_prefix_check
mkdir -p "$PREFIX"
# 渲染前 fail-fast: 密码/密钥字符集校验(约束见 bare.env.example)。
# 被校验值会进 SQL 内插 / sed 替换串 / server.env 三处上下文,
# 任一含禁止字符都会静默破坏配置 —— 在动手前拒绝, 而不是装到一半才炸。
bare_assert_secret_charset MYSQL_PASSWORD REDIS_PASSWORD SPIDER_RESET_TOKEN
# 含生产密码的文件, 顺手收紧(脚本目录内的 bare.env; BARE_ENV 指到别处时跳过)
if [ -f bare.env ]; then chmod 600 bare.env; fi
. /etc/os-release
case "${ID:-}:${VERSION_ID%%.*}" in
  ubuntu:22|ubuntu:24) ;;
  *) echo "⚠ 未在 Ubuntu 22/24 上运行(实际: ${PRETTY_NAME:-unknown}), 继续但包名/路径可能不同" >&2 ;;
esac

MYSQL_CNF_DIR=/etc/mysql/mysql.conf.d
REDIS_CONF=/etc/redis/redis-jerocine.conf
REDIS_DROPIN=/etc/systemd/system/redis-server.service.d/jerocine.conf
NGINX_CACHE_DIR=/var/cache/nginx-jerocine

echo "==> [1/7] 预置 MySQL 配置(必须在 apt 安装/首次初始化之前)"
mkdir -p "$MYSQL_CNF_DIR"
install -m 644 mysql-jerocine.cnf "$MYSQL_CNF_DIR/jerocine.cnf"

echo "==> [2/7] apt 安装三件套"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq mysql-server redis-server nginx curl

echo "==> [3/7] MySQL 建库建用户"
systemctl enable --now mysql
for _ in $(seq 1 30); do
  mysqladmin ping --silent 2>/dev/null && break
  sleep 2
done
mysqladmin ping --silent 2>/dev/null \
  || bare_die "MySQL 60s 未就绪 —— 查 journalctl -u mysql(常见: 配置项冲突拒绝启动)"
mysql <<SQL
CREATE DATABASE IF NOT EXISTS \`${MYSQL_DATABASE:-jerocine}\`
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS '${MYSQL_USER:-jerocine}'@'localhost' IDENTIFIED BY '${MYSQL_PASSWORD}';
CREATE USER IF NOT EXISTS '${MYSQL_USER:-jerocine}'@'127.0.0.1' IDENTIFIED BY '${MYSQL_PASSWORD}';
GRANT ALL PRIVILEGES ON \`${MYSQL_DATABASE:-jerocine}\`.* TO '${MYSQL_USER:-jerocine}'@'localhost';
GRANT ALL PRIVILEGES ON \`${MYSQL_DATABASE:-jerocine}\`.* TO '${MYSQL_USER:-jerocine}'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL

echo "==> [4/7] Redis 独立配置 + systemd drop-in"
sed -e "s|@REDIS_PASSWORD@|${REDIS_PASSWORD}|g" \
    -e "s|@REDIS_MAXMEMORY@|${REDIS_MAXMEMORY:-512mb}|g" \
    redis-jerocine.conf > "$REDIS_CONF"
chmod 640 "$REDIS_CONF"
chown root:redis "$REDIS_CONF"
mkdir -p "$(dirname "$REDIS_DROPIN")"
cat > "$REDIS_DROPIN" <<'UNIT'
[Service]
ExecStart=
ExecStart=/usr/bin/redis-server /etc/redis/redis-jerocine.conf --supervised systemd --daemonize no
UNIT
systemctl daemon-reload
systemctl enable --now redis-server || systemctl restart redis-server
sleep 1
REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli ping 2>/dev/null | grep -q PONG \
  || bare_die "Redis 未就绪 —— 查 journalctl -u redis-server"

echo "==> [5/7] jerocine 用户 + 目录树 + server.env + secrets"
id jerocine >/dev/null 2>&1 || useradd --system --home "$PREFIX" --shell /usr/sbin/nologin jerocine
mkdir -p "$PREFIX"/{server,secrets,packages,backup} "$PREFIX"/webdata/{html,releases,debugmap,packages} \
         "$PREFIX/apk" "$NGINX_CACHE_DIR" /var/log/jerocine
touch /var/log/jerocine/server.log
# 前端目录给 nginx(www-data)读; 其余归 jerocine
chown -R jerocine:jerocine "$PREFIX" /var/log/jerocine
chown -R www-data:www-data "$NGINX_CACHE_DIR"
chmod 750 /var/log/jerocine
# secrets: 优先复用同仓 deploy/secrets(docker 模式在用的那对密钥), 已有则跳过
if [ ! -s "$PREFIX/secrets/jwt_private.pem" ]; then
  if [ -s ../secrets/jwt_private.pem ]; then
    cp ../secrets/jwt_*.pem "$PREFIX/secrets/"
    echo "   (已从 deploy/secrets 复制现有 JWT 密钥 —— 与 docker 模式签发的 token 互通)"
  else
    echo "⚠ $PREFIX/secrets 缺 JWT 密钥 —— 手动生成:" >&2
    echo "    openssl genrsa -out $PREFIX/secrets/jwt_private.pem 2048" >&2
    echo "    openssl rsa -in $PREFIX/secrets/jwt_private.pem -pubout -RSAPublicKey_out -out $PREFIX/secrets/jwt_public.pem" >&2
  fi
fi
chown jerocine:jerocine "$PREFIX/secrets"
chmod 700 "$PREFIX/secrets"
chmod 600 "$PREFIX"/secrets/*.pem 2>/dev/null || true
bare_write_server_env "$PREFIX"

echo "==> [6/7] systemd unit + nginx 配置"
sed "s|@PREFIX@|$PREFIX|g" jerocine-server.service > /etc/systemd/system/jerocine-server.service
systemctl daemon-reload
systemctl enable jerocine-server
# nginx: 备份发行版默认配置后替换(单机单站, 与 docker 模式同一份全量 conf; PREFIX 渲染进 conf)
[ -f /etc/nginx/nginx.conf ] && [ ! -f /etc/nginx/nginx.conf.dist-backup ] \
  && cp /etc/nginx/nginx.conf /etc/nginx/nginx.conf.dist-backup || true
sed "s|@PREFIX@|$PREFIX|g" nginx-jerocine.conf > "$PREFIX/nginx.conf.rendered"
install -m 644 "$PREFIX/nginx.conf.rendered" /etc/nginx/nginx.conf
rm -f "$PREFIX/nginx.conf.rendered"
mkdir -p /etc/nginx/jerocine-certs
chmod 750 /etc/nginx/jerocine-certs
if [ -s /etc/nginx/jerocine-certs/fullchain.pem ]; then
  nginx -t || bare_die "nginx -t 未通过"
  systemctl enable --now nginx || systemctl restart nginx
else
  echo "⚠ 证书未就位(/etc/nginx/jerocine-certs/{fullchain,privkey}.pem) —— 放好后: nginx -t && systemctl restart nginx"
fi

echo "==> [7/7] golang-migrate v4.17.1"
MIGRATE_BIN="${MIGRATE_BIN:-/usr/local/bin/migrate}"
if [ ! -x "$MIGRATE_BIN" ]; then
  case "$(uname -m)" in
    x86_64)  march=amd64 ;;
    aarch64) march=arm64 ;;
    *)       march=amd64 ;;
  esac
  curl -fsSL "https://github.com/golang-migrate/migrate/releases/download/v4.17.1/migrate.linux-${march}.tar.gz" \
    | tar xz -C /usr/local/bin migrate \
    || echo "⚠ 下载失败 —— 手动安装 golang-migrate 或设 MIGRATE_BIN 指向已有二进制" >&2
fi
if [ -x "$MIGRATE_BIN" ]; then
  echo "   migrate: $MIGRATE_BIN ($("$MIGRATE_BIN" -version 2>&1 | head -1))"
else
  echo "⚠ migrate 二进制仍缺 —— migrate.sh 会拒绝执行, 届时手动装或设 MIGRATE_BIN" >&2
fi

cat <<NEXT

==================== 安装完成, 接下来 ====================
1. 放置 TLS 证书 → /etc/nginx/jerocine-certs/{fullchain,privkey}.pem
2. 上传后端产物包并发布:
      bash bare-deploy.sh /path/to/jerocine-server-*.tar.gz
3. 跑数据库迁移(server/migrations 随仓库):
      bash migrate.sh
4. 首次启动:  systemctl start jerocine-server
5. 上传前端产物包并发布:
      bash bare-web-deploy.sh /path/to/<TS>.tar.gz
=========================================================
NEXT

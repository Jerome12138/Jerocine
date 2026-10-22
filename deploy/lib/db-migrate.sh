# deploy/lib/db-migrate.sh — 数据库迁移(golang-migrate)的同步与执行。
#
# 被 deploy.sh source; 也可被 tests/server-migrate.test.sh 单测(往 PATH 注入假 docker 即可)。
#
# 设计:
#   - 迁移文件跟二进制**同包同版**(scripts/build-server.sh 打包时带上 server/migrations/),
#     server-deploy 解包后先同步到 deploy/data/migrations/ 再执行 —— 服务器 git 树落后与否都无关。
#   - golang-migrate up 本身幂等: 没有新迁移时立即退出 0 —— "有变化才迁移, 没有就不动"由它保证,
#     这里不做自己的版本比较(避免两套判断打架)。
#   - 失败即中止发布: 调用方(server-deploy)在迁移失败时不得重启新二进制, 线上保持旧版本。

# sync_pkg_migrations <src> <dest>
#   把迁移目录(产物包内; deploy_services 备用路径则为 git 树)覆盖式拷贝到运行时目录。
#   不删除 dest 里的旧文件: 已应用的迁移记录在
#   schema_migrations, 留着旧 .sql 反而保留 down 的能力, 目录增长可忽略(几十个小文件)。
sync_pkg_migrations() {
  local src="$1" dest="$2" n_src n_dest
  [ -d "$src" ] || { echo "!! 迁移源目录不存在: $src" >&2; return 1; }
  n_src="$(find "$src" -maxdepth 1 -name '*.sql' | wc -l)"
  [ "$n_src" -gt 0 ] || { echo "!! 迁移源目录没有 .sql 文件: $src" >&2; return 1; }
  mkdir -p "$dest"
  cp -f "$src"/*.sql "$dest"/
  n_dest="$(find "$dest" -maxdepth 1 -name '*.sql' | wc -l)"
  [ "$n_dest" -ge "$n_src" ] || { echo "!! 迁移同步异常: 源 $n_src 个, 目标只剩 $n_dest 个" >&2; return 1; }
  echo "==> 迁移文件已同步到 $dest(源 $n_src 个 / 目录共 $n_dest 个)"
}

# wait_migrate_container <cid> <timeout_sec>
#   等一次性 migrate 容器退出并校验退出码(0 = 成功, 含"没有新迁移"的 no-op)。
wait_migrate_container() {
  local cid="$1" timeout="${2:-180}" i st code
  [ -n "$cid" ] || { echo "!! migrate 容器 id 为空" >&2; return 1; }
  for i in $(seq 1 "$timeout"); do
    st="$("${DOCKER[@]}" inspect --format '{{.State.Status}}' "$cid" 2>/dev/null || echo 'missing')"
    case "$st" in
      exited) break ;;
      missing) echo "!! migrate 容器不存在: $cid" >&2; return 1 ;;
    esac
    sleep 1
  done
  if [ "${st:-}" != "exited" ]; then
    echo "!! migrate 容器 ${timeout}s 内未结束(状态=${st:-unknown}) —— 查看: ${COMPOSE[*]} logs migrate" >&2
    return 1
  fi
  code="$("${DOCKER[@]}" inspect --format '{{.State.ExitCode}}' "$cid" 2>/dev/null || echo 1)"
  [ "$code" = "0" ] || { echo "!! 迁移失败(exit=$code) —— 查看: ${COMPOSE[*]} logs migrate" >&2; return 1; }
  return 0
}

# run_db_migrations <src> <dest>
#   同步迁移文件 → force-recreate 一次性 migrate 容器 → 等退出并校验。
#   cwd 必须是 deploy/(COMPOSE 的 --env-file .env 与 compose.yml 的相对挂载都按它解析);
#   DOCKER/COMPOSE 数组由 deploy.sh 定义, 单测时注入假 docker 即可。
run_db_migrations() {
  local src="$1" dest="$2" cid
  sync_pkg_migrations "$src" "$dest" || return 1
  echo "==> 数据库迁移(golang-migrate up; 无新迁移时立即退出, 不动库)"
  "${COMPOSE[@]}" up -d --force-recreate migrate >/dev/null \
    || { echo "!! migrate 容器启动失败" >&2; return 1; }
  cid="$("${COMPOSE[@]}" ps -q migrate)"
  wait_migrate_container "$cid" 180 || return 1
  echo "==> 迁移完成(数据库结构已到位)"
}

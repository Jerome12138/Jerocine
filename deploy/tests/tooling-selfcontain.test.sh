#!/usr/bin/env bash
# 模式1 自包含(工具链随包 tooling/ 自引导)的单测 + 端到端。
#
# 背景: 2026-10-22 起产物包内带 tooling/(deploy.sh + lib + compose + Dockerfile + nginx.conf,
# 由 scripts/build-{server,web}.sh 打包)。服务器端 deploy.sh 检测到它, 就同步到固定目录
# deploy/runtime/ 并**以包内版本重新执行自身** —— 模式1(本地构建 → 上传包 → 发布)全程
# 不再 git pull, 服务器 git 树落后/超前都不影响发版。
#
# §1 静态契约: build 脚本打包清单(两个脚本一致) ↔ deploy.sh 自引导识别;
#    Dockerfile.nginx 与 web/Dockerfile 运行 stage 一致; compose 挂载根化 + nginx 构建可切换;
#    .gitignore 收录 deploy/runtime/; 分发守卫名单。
# §2 server-deploy 端到端(沙箱 + 假 docker/假 git): 带 tooling 的包 ⇒ 0 次 git pull、
#    runtime/ 同步完整且是**包内版本**(带标记)、compose 走 -f runtime/docker-compose.yml、
#    镜像用 runtime/Dockerfile.runtime、"先迁移再起容器"与整套只跑一次不变。
# §3 web-deploy 端到端: 同样 0 pull; tooling 不进公开挂载目录(排除在 dist 之外);
#    nginx 重建带 JEROCINE_NGINX_* 切换变量(包内薄配方)。
# §4 对照 + 守卫: 同包**不带** tooling ⇒ legacy(pull 照旧、无标记、compose 无 -f);
#    手动直跑 runtime 副本(无 DEPLOY_ROOT)被拒; runtime 内跑模式2 命令被拒。
# §5 幂等: 带 tooling 的包再发一次, 仍然 0 pull、runtime/ 刷新、整套只跑一次。
# 跑法: bash tests/tooling-selfcontain.test.sh
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

# 注意: 不要写 WORK="$(mktemp -d)" —— Windows(Git Bash/MSYS) 下 TMPDIR 可能是
# C:\Users\...\Temp, mktemp 会吐出混了分隔符的路径。先 cd 进去再 pwd。
WORK="$(cd "$(mktemp -d)" && pwd)"
trap 'rm -rf "$WORK"' EXIT
PASS=0; FAIL=0
ok()  { PASS=$((PASS+1)); echo "  ok  $1"; }
bad() { FAIL=$((FAIL+1)); echo "  FAIL $1"; }
assert_eq()  { [ "$2" = "$3" ] && ok "$1" || bad "$1 (期望 [$3] 实得 [$2])"; }
assert_has() { [ -e "$2" ] && ok "$1" || bad "$1 (缺少 $2)"; }
assert_gone(){ [ ! -e "$2" ] && ok "$1" || bad "$1 (不该存在 $2)"; }

DEPLOY_DIR="$(cd "$(dirname "$0")/.." && pwd)"
REPO_DIR="$(cd "$DEPLOY_DIR/.." && pwd)"

# ---- 假 docker: 记录原始调用(含 -f 与 NGINX 切换变量), 不真起容器 ----
STUB="$WORK/stub"; mkdir -p "$STUB" "$WORK/state"
cat > "$STUB/docker" <<'S'
#!/usr/bin/env bash
if [ "$1" = "compose" ]; then
  echo "compose $*" >> "$STUB_STATE/log"
  echo "NGINX_CTX=${JEROCINE_NGINX_CONTEXT:-unset} NGINX_DF=${JEROCINE_NGINX_DOCKERFILE:-unset}" >> "$STUB_STATE/log"
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
# ---- 假 git: 只记账。自引导路径下应当**一次都不被调用** ----
cat > "$STUB/git" <<'S'
#!/usr/bin/env bash
echo "git $*" >> "${STUB_STATE:?}/git.log"
exit 0
S
chmod +x "$STUB/git"
export STUB_STATE="$WORK/state"
export PATH="$STUB:$PATH"
echo exited > "$WORK/state/status"; echo 0 > "$WORK/state/code"

# ---- 沙箱"仓库": deploy/ 是树内副本(旧版工具链), 包内 tooling/ 从它取但加盖版本标记 ----
ROOT="$WORK/repo"; SB="$ROOT/deploy"
mkdir -p "$SB/lib" "$SB/data/html"
cp "$DEPLOY_DIR/deploy.sh" "$DEPLOY_DIR/Dockerfile.runtime" "$DEPLOY_DIR/Dockerfile.nginx" "$SB/"
cp "$DEPLOY_DIR"/lib/*.sh "$SB/lib/"                      # 整目录: 缺一个 lib 就会在 source 时炸
cp "$DEPLOY_DIR/docker-compose.yml" "$SB/"
cp "$DEPLOY_DIR/data/nginx/nginx.conf" "$SB/nginx.conf"   # tooling 的来源(对应 build 脚本的取法)
printf 'REDIS_PASSWORD=x\n' > "$SB/.env"                  # clear_caches 要 grep 它
printf '<html>x</html>\n'  > "$SB/data/html/index.html"   # 过"挂载目录未初始化"守卫
printf 'MIT\n'             > "$ROOT/LICENSE"              # wr_sync_dist 的 ../LICENSE

# make_pkg <out> <server|web>
# 包内 tooling/ 与 scripts/build-{server,web}.sh 打的同一套文件; deploy.sh 末尾加"版本标记" ——
# 树内副本没有它, 于是"输出里出现标记"能证明跑的是**包内**版本, 而不是树内旧版。
make_pkg() {
  local out="$1" kind="$2" stage
  stage="$WORK/stage-$kind"
  rm -rf "$stage"; mkdir -p "$stage"
  case "$kind" in
    server)
      mkdir -p "$stage/static/upload" "$stage/data" "$stage/migrations"
      printf '#!/bin/sh\necho server\n' > "$stage/main"
      printf 'ipdb\n' > "$stage/data/ip2region.db"
      printf 'ddl\n'  > "$stage/migrations/000099_demo.up.sql"
      printf 'MIT\n'  > "$stage/LICENSE"
      ;;
    web)
      mkdir -p "$stage/assets/20260101-120000"
      printf '<html>web</html>\n' > "$stage/index.html"
      printf 'js\n' > "$stage/assets/20260101-120000/index-x.js"
      printf 'sw\n' > "$stage/sw.js"
      ;;
  esac
  mkdir -p "$stage/tooling/lib"
  cp "$SB/deploy.sh" "$stage/tooling/deploy.sh"
  printf '\necho "==> [tooling v2 marker]"\n' >> "$stage/tooling/deploy.sh"
  cp "$SB"/lib/*.sh "$stage/tooling/lib/"
  cp "$SB/docker-compose.yml" "$SB/Dockerfile.runtime" "$SB/Dockerfile.nginx" "$SB/nginx.conf" \
     "$stage/tooling/"
  tar czf "$out" -C "$stage" .
}

# ==================== 1. 静态契约 ====================
echo "# 1. 静态契约"

# 1.1 两个 build 脚本的 tooling 完整性断言清单必须一致(同一套文件, 防只改一边)
tooling_list() {  # 从 build 脚本抽出 `for f in ...; do` 断言清单(拼接续行)
  tr -d '\\\n' < "$1" | grep -o 'for f in [^;]*; do' | sed 's/^for f in //; s/; do$//' | tr -s ' '
}
assert_eq "build-server.sh 的 tooling 断言清单" \
  "$(tooling_list "$REPO_DIR/scripts/build-server.sh")" \
  "deploy.sh docker-compose.yml Dockerfile.runtime Dockerfile.nginx nginx.conf lib/db-migrate.sh lib/web-release.sh"
assert_eq "build-web.sh 的 tooling 断言清单与 build-server.sh 一致" \
  "$(tooling_list "$REPO_DIR/scripts/build-web.sh")" \
  "$(tooling_list "$REPO_DIR/scripts/build-server.sh")"
for b in build-server.sh build-web.sh; do
  grep -q 'Dockerfile.nginx' "$REPO_DIR/scripts/$b" \
    && ok "$b 打包 Dockerfile.nginx(nginx 薄配方)" || bad "$b 未打包 Dockerfile.nginx"
  grep -q 'data/nginx/nginx.conf' "$REPO_DIR/scripts/$b" \
    && ok "$b 打包 nginx.conf" || bad "$b 未打包 nginx.conf"
done

# 1.2 Dockerfile.nginx 与 web/Dockerfile 运行 stage 必须一致(改一侧必须同步另一侧)
from_web="$(grep '^FROM' "$REPO_DIR/web/Dockerfile" | tail -1 | awk '{print $2}')"
from_ngx="$(grep '^FROM' "$DEPLOY_DIR/Dockerfile.nginx" | tail -1 | awk '{print $2}')"
assert_eq "Dockerfile.nginx 基础镜像 == web/Dockerfile 运行 stage" "$from_ngx" "$from_web"
copy_web="$(grep -oE 'COPY [^ ]*nginx\.conf /etc/nginx/nginx\.conf' "$REPO_DIR/web/Dockerfile")"
copy_ngx="$(grep -oE 'COPY [^ ]*nginx\.conf /etc/nginx/nginx\.conf' "$DEPLOY_DIR/Dockerfile.nginx")"
[ -n "$copy_web" ] && [ -n "$copy_ngx" ] \
  && ok "两侧都有 nginx.conf 的 COPY" || bad "任缺 nginx.conf 的 COPY(web=[$copy_web] ngx=[$copy_ngx])"
assert_eq "COPY 目标路径一致" "${copy_ngx##* }" "${copy_web##* }"
src_ngx="${copy_ngx#COPY }"; src_ngx="${src_ngx%% *}"
src_web="${copy_web#COPY }"; src_web="${src_web%% *}"
assert_eq "COPY 源 basename 一致(上下文不同: 仓库根 vs deploy/runtime)" \
  "${src_ngx##*/}" "${src_web##*/}"
hc_web="$(grep -o 'wget -qO- http://127.0.0.1/' "$REPO_DIR/web/Dockerfile" | head -1)"
hc_ngx="$(grep -o 'wget -qO- http://127.0.0.1/' "$DEPLOY_DIR/Dockerfile.nginx" | head -1)"
assert_eq "HEALTHCHECK 探活命令一致" "$hc_ngx" "$hc_web"

# 1.3 compose: 5 处挂载根化 + nginx 构建可切换
for m in data/migrations secrets data/html apk certs; do
  grep -qE "^[[:space:]]*- \\\$\{JEROCINE_DEPLOY_ROOT:-\.\}/$m:" "$DEPLOY_DIR/docker-compose.yml" \
    && ok "compose 挂载根化: $m" || bad "compose 挂载未根化: $m"
done
grep -qF '${JEROCINE_NGINX_CONTEXT:-..}' "$DEPLOY_DIR/docker-compose.yml" \
  && ok "nginx 构建上下文可切换(默认树内 ..)" || bad "nginx context 不可切换"
grep -qF '${JEROCINE_NGINX_DOCKERFILE:-web/Dockerfile}' "$DEPLOY_DIR/docker-compose.yml" \
  && ok "nginx Dockerfile 可切换(默认树内 web/Dockerfile)" || bad "nginx dockerfile 不可切换"

# 1.4 deploy.sh 自引导接线: export 根 / -f 包内 compose / 包内 Dockerfile / 重 exec / 守卫名单
grep -q 'export JEROCINE_DEPLOY_ROOT=' "$DEPLOY_DIR/deploy.sh" \
  && ok "deploy.sh 自引导 export JEROCINE_DEPLOY_ROOT" || bad "未 export JEROCINE_DEPLOY_ROOT"
grep -qF -- '-f runtime/docker-compose.yml' "$DEPLOY_DIR/deploy.sh" \
  && ok "runtime 模式 compose 用包内副本" || bad "runtime 模式未指定 -f"
grep -qF 'RUNTIME_DF="runtime/Dockerfile.runtime"' "$DEPLOY_DIR/deploy.sh" \
  && ok "runtime 模式 server 镜像用包内 Dockerfile.runtime" || bad "RUNTIME_DF 未切包内副本"
grep -qF 'exec bash "runtime/deploy.sh"' "$DEPLOY_DIR/deploy.sh" \
  && ok "自引导以包内版本重新 exec" || bad "未重新 exec 包内版本"
grep -qF 'server-deploy|web-deploy|web-rollback|web-init' "$DEPLOY_DIR/deploy.sh" \
  && ok "runtime 模式白名单守卫存在" || bad "缺少 runtime 模式守卫名单"

# 1.5 .gitignore 收录 deploy/runtime/(自引导产物, 不入库)
grep -q '^deploy/runtime/$' "$REPO_DIR/.gitignore" \
  && ok ".gitignore 收录 deploy/runtime/" || bad ".gitignore 缺 deploy/runtime/"

# ==================== 2. server-deploy 带 tooling: 自引导, 0 次 git ====================
echo "# 2. server-deploy(带 tooling) 端到端"
: > "$WORK/state/log"; : > "$WORK/state/git.log"
make_pkg "$WORK/jerocine-server-20260101-120000-abc1234-linux-amd64.tar.gz" server
run_deploy() { bash "$SB/deploy.sh" "$@" > "$WORK/out" 2>&1; }

run_deploy server-deploy "$WORK/jerocine-server-20260101-120000-abc1234-linux-amd64.tar.gz" \
  && ok "带 tooling 的 server 包发布成功" || { bad "发布失败"; cat "$WORK/out"; }
assert_eq "全程 0 次 git 调用(模式1 断开 git 货币)" \
  "$(cat "$WORK/state/git.log" 2>/dev/null | wc -l | tr -d ' ')" "0"
grep -q "产物包自带部署工具链" "$WORK/out" \
  && ok "自引导同步提示出现" || bad "缺少自引导提示"
grep -q "跳过 git pull" "$WORK/out" \
  && ok "runtime 模式显式提示跳过 pull" || bad "缺少跳过 pull 提示"
grep -q "\[tooling v2 marker\]" "$WORK/out" \
  && ok "执行的是包内版本(树内副本没有的标记出现了)" || bad "标记未出现 —— 跑的怕不是树内旧版"
for f in deploy.sh docker-compose.yml Dockerfile.runtime Dockerfile.nginx nginx.conf \
         lib/db-migrate.sh lib/web-release.sh; do
  assert_has "runtime/ 同步 $f" "$SB/runtime/$f"
done
grep -q "tooling v2 marker" "$SB/runtime/deploy.sh" \
  && ok "runtime/ 里的 deploy.sh 是包内版本" || bad "runtime/ 里不是包内版本"
grep -q -- "-f runtime/docker-compose.yml up -d --force-recreate migrate" "$WORK/state/log" \
  && ok "migrate 走包内 compose 副本" || bad "migrate 未用包内 compose"
grep -q -- "-f runtime/docker-compose.yml up -d --no-build server" "$WORK/state/log" \
  && ok "server 重建走包内 compose 副本" || bad "server 未用包内 compose"
grep -q -- "build -f runtime/Dockerfile.runtime" "$WORK/state/log" \
  && ok "server 镜像用包内 Dockerfile.runtime" || bad "server 镜像未用包内 Dockerfile"
n_mig="$(grep -n 'force-recreate migrate' "$WORK/state/log" | head -1 | cut -d: -f1)"
n_srv="$(grep -n 'up -d --no-build server' "$WORK/state/log" | head -1 | cut -d: -f1)"
[ -n "$n_mig" ] && [ -n "$n_srv" ] && [ "$n_mig" -lt "$n_srv" ] \
  && ok "自引导不破坏顺序: 迁移仍先于新容器" || bad "迁移/启动顺序错误(migrate=$n_mig server=$n_srv)"
assert_eq "整套仍只跑一次(自引导的重新 exec 不重复执行)" \
  "$(grep -c 'up -d --no-build server' "$WORK/state/log")" "1"
assert_has "产物包留档(data/packages/)" "$SB/data/packages/jerocine-server-20260101-120000-abc1234-linux-amd64.tar.gz"

# ==================== 3. web-deploy 带 tooling: 自引导 + tooling 不进公开目录 ====================
echo "# 3. web-deploy(带 tooling) 端到端"
: > "$WORK/state/log"; : > "$WORK/state/git.log"
make_pkg "$WORK/jerocine-web-20260101-120000.tar.gz" web
run_deploy web-deploy "$WORK/jerocine-web-20260101-120000.tar.gz" \
  && ok "带 tooling 的 web 包发布成功" || { bad "发布失败"; cat "$WORK/out"; }
assert_eq "全程 0 次 git 调用" \
  "$(cat "$WORK/state/git.log" 2>/dev/null | wc -l | tr -d ' ')" "0"
grep -q "\[tooling v2 marker\]" "$WORK/out" \
  && ok "web-deploy 执行的也是包内版本" || bad "标记未出现"
assert_has  "dist 内容已同步进挂载目录"      "$SB/data/html/index.html"
assert_has  "assets/<TS> 已同步"            "$SB/data/html/assets/20260101-120000/index-x.js"
assert_gone "tooling/ 不进公开挂载目录(排除在 dist 外)" "$SB/data/html/tooling"
if grep -q tooling "$SB/data/root-manifest.txt" 2>/dev/null; then
  bad "根清单混入 tooling(会被当根静态管理)"
else
  ok "根清单不含 tooling"
fi
grep -q -- "-f runtime/docker-compose.yml up -d --build --no-deps nginx" "$WORK/state/log" \
  && ok "nginx 重建走包内 compose 副本" || bad "nginx 未用包内 compose"
ngx_line="$(grep '^NGINX_CTX=' "$WORK/state/log" | tail -1)"
assert_eq "nginx 构建切到包内薄配方" "${ngx_line#* }" "NGINX_DF=Dockerfile.nginx"
case "$ngx_line" in
  *"NGINX_CTX=$SB/runtime"*) ok "nginx 构建上下文 = deploy/runtime/(包内)" ;;
  *) bad "nginx 上下文不是 deploy/runtime/ (实得: $ngx_line)" ;;
esac
assert_has "web 产物包留档" "$SB/data/packages/20260101-120000.tar.gz"

# ==================== 4. 对照(legacy) + 守卫 ====================
echo "# 4. legacy 对照与守卫"
# 4.1 同一个包**不带** tooling/ ⇒ legacy: pull 照旧、无标记、compose 无 -f
: > "$WORK/state/log"; : > "$WORK/state/git.log"
stage="$WORK/stage-legacy"
rm -rf "$stage"; mkdir -p "$stage/static/upload" "$stage/data" "$stage/migrations"
printf '#!/bin/sh\necho server\n' > "$stage/main"
printf 'ipdb\n' > "$stage/data/ip2region.db"
printf 'ddl\n'  > "$stage/migrations/000099_demo.up.sql"
printf 'MIT\n'  > "$stage/LICENSE"
tar czf "$WORK/legacy-server.tar.gz" -C "$stage" .
run_deploy server-deploy "$WORK/legacy-server.tar.gz" \
  && ok "旧格式包(无 tooling)legacy 发布成功" || { bad "legacy 发布失败"; cat "$WORK/out"; }
n_pull="$(grep -c 'git -C .. pull --ff-only' "$WORK/state/git.log" 2>/dev/null || true)"
assert_eq "legacy 包仍走 git pull(父进程 + 重启后共 2 次)" "$n_pull" "2"
grep -q "\[tooling v2 marker\]" "$WORK/out" \
  && bad "legacy 包不该出现包内标记" || ok "legacy 包不带包内标记"
if grep -q -- "-f runtime/docker-compose.yml" "$WORK/state/log"; then
  bad "legacy 模式不该用 -f runtime/compose"
else
  ok "legacy 模式 compose 仍是树内默认文件"
fi

# 4.2 手动直跑 runtime 副本(无 DEPLOY_ROOT) ⇒ 拒绝并指路
out_guard="$(cd "$SB/runtime" && bash ./deploy.sh server-deploy "$WORK/legacy-server.tar.gz" 2>&1)" \
  && bad "手动直跑 runtime 副本应被拒绝" || ok "手动直跑 runtime 副本按预期被拒"
grep -q "不能直接运行" <<<"$out_guard" \
  && ok "拒绝信息指路(去 deploy/ 下跑 ./deploy.sh)" || bad "拒绝信息不明确"

# 4.3 runtime 内跑模式2 命令(server) ⇒ 拒绝并指路回树内
out_guard2="$(JEROCINE_DEPLOY_ROOT="$SB" bash "$SB/runtime/deploy.sh" server 2>&1)" \
  && bad "runtime 内跑 server 应被拒绝" || ok "runtime 内跑模式2 命令按预期被拒"
grep -q "需要服务器 git 树" <<<"$out_guard2" \
  && ok "拒绝信息指路回树内 ./deploy.sh" || bad "拒绝信息不明确"

# ==================== 5. 幂等: 再发一次仍旧 0 pull ====================
echo "# 5. 幂等(带 tooling 的包再发一次)"
: > "$WORK/state/log"; : > "$WORK/state/git.log"
make_pkg "$WORK/jerocine-server-20260101-130000-def5678-linux-amd64.tar.gz" server
run_deploy server-deploy "$WORK/jerocine-server-20260101-130000-def5678-linux-amd64.tar.gz" \
  && ok "第二次发布成功" || { bad "第二次发布失败"; cat "$WORK/out"; }
assert_eq "第二次同样 0 次 git 调用(工具链每次随包自动更新, 无需人记)" \
  "$(cat "$WORK/state/git.log" 2>/dev/null | wc -l | tr -d ' ')" "0"
assert_eq "第二次整套仍只跑一次" \
  "$(grep -c 'up -d --no-build server' "$WORK/state/log")" "1"
grep -q "tooling v2 marker" "$SB/runtime/deploy.sh" \
  && ok "runtime/ 已刷新为最新包内版本" || bad "runtime/ 未刷新"

echo
echo "===================================="
echo "  通过 $PASS / 失败 $FAIL"
echo "===================================="
[ "$FAIL" = 0 ]

#!/usr/bin/env bash
# deploy.sh「pull 后自我重启」机制的单测 —— 不需要 docker / 网络 / 真实仓库。
#   cd deploy && bash tests/deploy-reexec.test.sh
#
# 做法: 在临时目录造一个假仓库(只有 deploy/ + data/html/index.html 以过挂载守卫) 和一个假 git,
# 假 git 在 pull 时**改写 deploy.sh**(模拟"拉到新版本"), 于是可以断言:
#   1) pull 发生 2 次(父进程 + 重启后的子进程), 且只重启一次(防重入标记生效, 不会无限循环);
#   2) 重启后继续跑 pull 之后的流程, 并且读到的是**改写后**的脚本(即真的重新读盘);
#   3) 原始子命令参数被完整带过去(没退回默认 server+nginx)。
set -uo pipefail

cd "$(dirname "$0")/.."          # → deploy/
REAL_DEPLOY="$(pwd)/deploy.sh"
REAL_LIB="$(pwd)/lib/web-release.sh"

pass=0
fail=0
ok()   { pass=$((pass + 1)); echo "  ok   $*"; }
bad()  { fail=$((fail + 1)); echo "  FAIL $*"; }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || bad "$1 (期望 [$3] 实得 [$2])"; }
assert_no() { if grep -q -- "$2" "$1"; then bad "$3"; else ok "$3"; fi; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

REPO="$WORK/repo"
BIN="$WORK/bin"
mkdir -p "$REPO/deploy/lib" "$REPO/deploy/data/html" "$BIN"
cp "$REAL_DEPLOY" "$REPO/deploy/deploy.sh"
cp "$REAL_LIB"    "$REPO/deploy/lib/web-release.sh"
printf '<html>x</html>\n' > "$REPO/deploy/data/html/index.html"   # 过"挂载目录未初始化"守卫

# 假 git: 记一次调用; 若是 pull, 则改写 deploy.sh 里"pull 之后才会打印"的那行 ——
# 它出现在日志里就证明重启后的进程读到了新版本的内容(而不是旧文件的缓存/偏移)。
cat > "$BIN/git" <<'FAKEGIT'
#!/usr/bin/env bash
echo "pull" >> "$REEXEC_TEST_PULLS"
if [ "${1:-}" = "-C" ] && [ "${3:-}" = "pull" ]; then
  sed -i 's/compose up -d --build:/compose up -d --build[HOTPATCH]:/' \
      "$REEXEC_TEST_REPO/deploy/deploy.sh"
fi
exit 0
FAKEGIT
chmod +x "$BIN/git"

LOG="$WORK/run.log"
PULLS="$WORK/pulls"
# 退出码非 0 是预期的(假仓库里没有 compose/.env, 走到 docker 那步必然失败) —— 只断言日志
REEXEC_TEST_PULLS="$PULLS" REEXEC_TEST_REPO="$REPO" \
  PATH="$BIN:$PATH" bash "$REPO/deploy/deploy.sh" nginx > "$LOG" 2>&1 || true

assert_eq "pull 共 2 次(父 + 重启后的一轮)" \
          "$(cat "$PULLS" 2>/dev/null | wc -l | tr -d ' ')" "2"
assert_eq "只重启一次(防重入标记生效)" \
          "$(grep -c 'JEROCINE_DEPLOY_REEXEC=1' "$LOG" 2>/dev/null || true)" "1"
assert_eq "重启后继续执行 pull 之后的流程, 且读到的是改写后的脚本" \
          "$(grep -c 'compose up -d --build\[HOTPATCH\]: nginx' "$LOG" 2>/dev/null || true)" "1"
assert_no "$LOG" "server nginx" "子命令参数被完整带过重启(没退回默认 server+nginx)"

echo
echo "===================================="
echo "  通过 $pass / 失败 $fail"
echo "===================================="
[ "$fail" -eq 0 ]

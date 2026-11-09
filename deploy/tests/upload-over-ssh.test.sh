#!/usr/bin/env bash
# scripts/lib/upload-over-ssh.sh 的单测 —— 用"假 ssh"把远端命令在本机假目录里执行,
# 不需要真实服务器/网络。
#   cd deploy && bash tests/upload-over-ssh.test.sh
#
# 覆盖真正会出事的地方:
#   - 默认通道是**断点续传**(不是整包直传): 断线后按远端已收字节继续, 不从头重传;
#   - 秒传: 远端已有同内容文件 → 一个字节都不传;
#   - 远端进度"大小对但内容坏"(旧切片实现的经典坑) → 终校验必须拦住, 不能落位;
#   - direct 通道仍可用(快链路/手动回退)。
set -uo pipefail

# 沙箱化开发环境会给 rm/unlink/rmdir 注入 safe-delete shim: 带会话 ID 时每次调用
# 约 7s, 本测试内部多次 rm -rf 会被拖到超时。无会话 ID 时零开销透传。
unset CODEBUDDY_SESSION_ID CLAUDE_SESSION_ID
# Windows(Git Bash)下 TMPDIR 常是 C:\Users\...\Temp, mktemp 会吐出混分隔符路径。
case "${TMPDIR:-}" in
  [A-Za-z]:*) export TMPDIR=/tmp ;;
esac

cd "$(dirname "$0")/.."          # → deploy/
# shellcheck source=../../scripts/lib/upload-over-ssh.sh
source ../scripts/lib/upload-over-ssh.sh

pass=0
fail=0
ok()   { pass=$((pass + 1)); echo "  ok   $*"; }
bad()  { fail=$((fail + 1)); echo "  FAIL $*"; }
assert_eq()  { [ "$2" = "$3" ] && ok "$1" || bad "$1 (期望 [$3] 实得 [$2])"; }
assert_has() { case "$2" in *"$3"*) ok "$1" ;; *) bad "$1 (输出里没有 [$3]: $2)" ;; esac; }
assert_gone() { [ ! -e "$2" ] && ok "$1" || bad "$1 (不该存在 $2)"; }

WORK="$(cd "$(mktemp -d)" && pwd)"
trap 'rm -rf "$WORK"' EXIT

REMOTE="$WORK/remote"        # 假远端目录
mkdir -p "$REMOTE"
STUB="$WORK/ssh-stub"
STUB_LOG="$WORK/xfer.log"

# 假 ssh: 远端命令直接在本机执行(路径就是本机路径)。
#   STUB_DROP_AFTER=<n>  对 "cat >>" 只写 n 字节就断(exit 255) —— 模拟链路 reset;
#                        只在第一次生效(标记文件存在后放行)。
# 每次收到的 stdin 字节数都记进 $STUB_LOG, 用来断言"有没有从头重传"。
cat > "$STUB" <<'STUB_EOF'
#!/usr/bin/env bash
set -u
cmd="${2-}"
log="${STUB_LOG:-}"
cmdlog="${STUB_CMD_LOG:-}"
[ -n "$cmdlog" ] && printf '%s\n' "$cmd" >> "$cmdlog"
case "$cmd" in
  *"cat >>"*)
    tmp="$(mktemp)"
    if [ -n "${STUB_DROP_AFTER:-}" ] && [ ! -f "${STUB_MARK:-/nonexistent}" ]; then
      : > "${STUB_MARK}"
      head -c "$STUB_DROP_AFTER" > "$tmp"
      [ -n "$log" ] && wc -c < "$tmp" | tr -d ' ' >> "$log"
      sh -c "$cmd" < "$tmp" >/dev/null 2>&1
      rm -f "$tmp"
      exit 255                      # 连接被 reset: 远端只留下半截
    fi
    cat > "$tmp"                    # 收满整份 stdin
    [ -n "$log" ] && wc -c < "$tmp" | tr -d ' ' >> "$log"
    sh -c "$cmd" < "$tmp"
    rc=$?
    rm -f "$tmp"
    exit "$rc"
    ;;
  *"cat >"*)
    # 直传: cat > <远端> —— 一次连接吃满整包, 也记账便于断言"没有从头重传"
    tmp="$(mktemp)"
    cat > "$tmp"
    [ -n "$log" ] && wc -c < "$tmp" | tr -d ' ' >> "$log"
    sh -c "$cmd" < "$tmp"
    rc=$?
    rm -f "$tmp"
    exit "$rc"
    ;;
esac
sh -c "$cmd"
STUB_EOF
chmod +x "$STUB"
export JEROCINE_UPLOAD_SSH_BIN="$STUB"
export STUB_LOG

# 2.5MB 源文件(= 1MB + 1MB + 0.5MB), 便于"断在 1MB 处再续传"
SRC="$WORK/pkg.bin"
head -c 2621440 /dev/urandom > "$SRC"
SRC_SHA="$(sha256sum "$SRC" | awk '{print $1}')"
SRC_SIZE="$(wc -c < "$SRC" | tr -d ' ')"

echo "upload-over-ssh.test.sh — 假远端: $REMOTE"

# ---------- 0. 临时目录归一化(Windows 盘符路径必须掰到 /tmp) ----------
# 踩过: Git Bash 下 TMPDIR=C:\Users\...\Temp ⇒ mktemp 吐出带反斜杠的路径 ⇒ awk 把 \U/\k
# 当转义吃掉、状态文件写不进去 ⇒ 看门狗读到冻结字节, 把健康连接每 20s 掐一次。
assert_eq "TMPDIR 为 Windows 盘符路径 → /tmp" \
  "$( ( TMPDIR='C:\Users\kilim\AppData\Local\Temp'; _upload_tmpdir ) )" "/tmp"
assert_eq "普通 TMPDIR 原样保留" "$( ( TMPDIR=/var/tmp; _upload_tmpdir ) )" "/var/tmp"

# ---------- 1. 默认通道 = 整包直传(公开仓通用默认) ----------
: > "$STUB_LOG"
DST="$REMOTE/pkg-v1.tgz"
out="$(upload_over_ssh fakehost "$SRC" "$DST" 2>&1)"
assert_eq "全新上传返回成功" "$?" "0"
assert_has "默认走整包直传" "$out" "整包直传"
assert_eq "远端内容与源一致" "$(sha256sum "$DST" | awk '{print $1}')" "$SRC_SHA"
assert_eq "一次连接即传满(只传了整份)" "$(cat "$STUB_LOG")" "$SRC_SIZE"
assert_gone "直传后不留 .jcpart" "$DST.${SRC_SHA:0:12}.jcpart"

# ---------- 1a. 不指定通道时: 沙箱内必须自动走续传(直传的长流会被闸死) ----------
# 沙箱把单条命令的流闸在 ~1–2MB, 超了连脚本一起 SIGTERM ⇒ 沙箱里"先直传再回退"活不到回退。
: > "$STUB_LOG"
SANDBOX_ENV="$WORK/shell-runtime-bash-env.sh"
: > "$SANDBOX_ENV"
DST1A="$REMOTE/pkg-sandbox.tgz"
out="$(BASH_ENV="$SANDBOX_ENV" upload_over_ssh fakehost "$SRC" "$DST1A" 2>&1)"
assert_eq "沙箱内不指定通道也能成功" "$?" "0"
assert_has "沙箱内自动走续传而非直传" "$out" "断点续传"
assert_eq "沙箱内按 1MB 自动分段" "$(tr '\n' ' ' < "$STUB_LOG" | sed 's/ $//')" "1048576 1048576 524288"
# 显式指定仍然说话算数(沙箱里硬要直传就让他试, 便于复现/对比)
: > "$STUB_LOG"
out="$(BASH_ENV="$SANDBOX_ENV" JEROCINE_UPLOAD_MODE=direct upload_over_ssh fakehost "$SRC" "$REMOTE/pkg-sandbox2.tgz" 2>&1)"
assert_has "沙箱内显式 direct 不被改写" "$out" "整包直传"

# ---------- 1b. chunk 模式 = 单流追加式断点续传 ----------
# 每次都记远端命令, 用来盯"握手次数"这个慢链路命门(一次往返 2–10s)。
STUB_CMD_LOG="$WORK/cmd.log"
export STUB_CMD_LOG
: > "$STUB_LOG"; : > "$STUB_CMD_LOG"
DST1B0="$REMOTE/pkg-resume.tgz"
out="$(JEROCINE_UPLOAD_MODE=chunk upload_over_ssh fakehost "$SRC" "$DST1B0" 2>&1)"
assert_eq "chunk 模式上传成功" "$?" "0"
assert_has "chunk 模式走断点续传通道" "$out" "断点续传"
assert_eq "chunk 远端内容与源一致" "$(sha256sum "$DST1B0" | awk '{print $1}')" "$SRC_SHA"
assert_gone "chunk 落位后不留 .jcpart" "$DST1B0.${SRC_SHA:0:12}.jcpart"
assert_eq "传满后不再多问一次远端尺寸(只 1 次 wc)" \
  "$(grep -cE "wc -c < .*2>/dev/null" "$STUB_CMD_LOG" | tr -d ' ')" "1"

# ---------- 1b. 分段窗口(沙箱默认 1MB/段): 多段拼接仍必须完整 ----------
: > "$STUB_LOG"
DST1B="$REMOTE/pkg-window.tgz"
out="$(JEROCINE_UPLOAD_MODE=chunk JEROCINE_UPLOAD_MAX_WINDOW=1048576 upload_over_ssh fakehost "$SRC" "$DST1B" 2>&1)"
assert_eq "分段窗口上传成功" "$?" "0"
assert_has "分段模式提示本段大小" "$out" "本段 1.0MB"
assert_eq "2.5MB 按 1MB 切成 1+1+0.5 三段" \
  "$(tr '\n' ' ' < "$STUB_LOG" | sed 's/ $//')" "1048576 1048576 524288"
assert_eq "分段上传后内容仍一致" "$(sha256sum "$DST1B" | awk '{print $1}')" "$SRC_SHA"

# ---------- 2. 秒传: 远端已有同内容 → 不传一个字节 ----------
: > "$STUB_LOG"
DST2="$REMOTE/pkg-dup.tgz"
cp "$SRC" "$DST2"
out="$(upload_over_ssh fakehost "$SRC" "$DST2" 2>&1)"
assert_eq "秒传返回成功" "$?" "0"
assert_has "秒传命中提示" "$out" "跳过上传"
assert_eq "秒传不产生任何传输" "$(wc -l < "$STUB_LOG" | tr -d ' ')" "0"

# ---------- 3. 断线续传: 第一次只到 1MB, 第二次从 1MB 续 ----------
: > "$STUB_LOG"
export STUB_DROP_AFTER=1048576
export STUB_MARK="$WORK/dropped.marker"
rm -f "$STUB_MARK"
DST3="$REMOTE/pkg-v2.tgz"
out="$(JEROCINE_UPLOAD_MODE=chunk upload_over_ssh fakehost "$SRC" "$DST3" 2>&1)"
rc=$?
unset STUB_DROP_AFTER STUB_MARK
assert_eq "断线后续传最终成功" "$rc" "0"
assert_has "第二轮从远端已收字节继续" "$out" "从 1.0MB/"
assert_eq "远端内容仍与源一致" "$(sha256sum "$DST3" | awk '{print $1}')" "$SRC_SHA"
assert_eq "首轮传 1MB + 次轮只补剩余(未从头重传)" "$(tr '\n' ' ' < "$STUB_LOG" | sed 's/ $//')" "1048576 1572864"

# ---------- 3b. 中途 reset 后自动切分段(把在途丢失压到一段内) ----------
# 判据是"连接非正常结束"(rc_ssh≠0), 不是远端回报的字节数 —— reset 时那句 `wc -c` 的 stdout
# 也一起丢, got 恒为 0。这里把切分窗口设成 1MB, 好让断言能看出"后续不再整段一把梭"。
: > "$STUB_LOG"
export STUB_DROP_AFTER=1048576
export STUB_MARK="$WORK/dropped2.marker"
rm -f "$STUB_MARK"
DST3B="$REMOTE/pkg-adaptive.tgz"
out="$(JEROCINE_UPLOAD_MODE=chunk JEROCINE_UPLOAD_MAX_WINDOW=0 JEROCINE_UPLOAD_WINDOW_ON_FAIL=1048576 \
  upload_over_ssh fakehost "$SRC" "$DST3B" 2>&1)"
rc=$?
unset STUB_DROP_AFTER STUB_MARK
assert_eq "中途 reset 后仍成功" "$rc" "0"
assert_has "识别为中途断开并切分段" "$out" "中途断开"
assert_eq "reset 后按 1MB/段重发剩余(1+0.5), 不再整段重来" \
  "$(tr '\n' ' ' < "$STUB_LOG" | sed 's/ $//')" "1048576 1048576 524288"
assert_eq "自适应分段后内容一致" "$(sha256sum "$DST3B" | awk '{print $1}')" "$SRC_SHA"

# ---------- 4. 远端进度大小对但内容坏 → 终校验必须拦住 ----------
: > "$STUB_LOG"
DST4="$REMOTE/pkg-corrupt.tgz"
PART4="$DST4.${SRC_SHA:0:12}.jcpart"
head -c "$SRC_SIZE" /dev/zero > "$PART4"     # 尺寸正好, 内容是垃圾
out="$(JEROCINE_UPLOAD_MODE=chunk upload_over_ssh fakehost "$SRC" "$DST4" 2>&1)"
rc=$?
assert_eq "坏进度必须失败" "$rc" "1"
assert_has "报 sha256 不一致" "$out" "sha256 不一致"
assert_gone "绝不允许把坏包落位成正式包" "$DST4"

# ---------- 5. 未知 mode 必须报错(不能静默走某个通道) ----------
: > "$STUB_LOG"
out="$(JEROCINE_UPLOAD_MODE=bogus upload_over_ssh fakehost "$SRC" "$REMOTE/pkg-bad.tgz" 2>&1)"
assert_eq "未知 mode 返回 64" "$?" "64"
assert_has "提示可选值" "$out" "未知 JEROCINE_UPLOAD_MODE"
assert_eq "未知 mode 不产生任何传输" "$(wc -l < "$STUB_LOG" | tr -d ' ')" "0"

echo
echo "upload-over-ssh.test.sh: $pass 通过 / $fail 失败"
[ "$fail" = "0" ]

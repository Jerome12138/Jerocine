#!/usr/bin/env bash
# scripts/lib/upload-over-ssh.sh —— 产物包 ssh 上送(单向流, 禁用 scp 的仓库红线不变)。
#
# 被 deploy-{web,server,danmu}.sh source 后调用:
#   upload_over_ssh <host> <本地文件> <远端路径>
# 返回 0=成功(远端文件与本地 sha256 一致), 非 0=失败(调用方 die)。
#
# 也可 CLI 直跑: upload-over-ssh.sh <host> <本地文件> <远端路径>
# (供 JEROCINE_UPLOAD_CMD="bash <本脚本>" 这种注入用法调用)
#
# ── 两条通道 ────────────────────────────────────────────────────────────────
#   direct  整包一条流: cat <本地> | ssh 'cat > 远端'。**默认**(通用, 快链路最省事)。
#   chunk   单流追加式断点续传: 远端写 `$dst.<sha12>.jcpart`, 每次连接前先问远端
#           已收多少字节, 只补发剩余部分(`dd skip=`); 连接中断或彻底停滞(默认 45s 零进度)
#           就重连接着传, 传满后先在远端校验 sha256 再 `mv` 落位。
#           **慢 / 会中途 reset 的链路才值得开**(跨洲跨境、家宽国际出口等)。
#
# 为什么不把续传做成默认(2026-11-09 定): 通用场景下"多一次连接"就是纯开销 ——
#   国内到国内、同城机房这些链路上直传一次跑完, 续传每条连接多一次握手 + 一次尺寸核对。
#   公开仓是给别人的项目用的, 不该让所有人替少数慢链路付这个成本。
#   **本项目的私有运维仓(Harness)默认设 `JEROCINE_UPLOAD_MODE=chunk`** —— 那台源站在新加坡。
#   唯一例外是 **agent 命令沙箱**: 那里的单条命令流闸在 ~1–2MB 且超了连脚本一起 SIGTERM,
#   直传必死、还活不到"回退"那步 —— 所以不显式指定时, 沙箱里自动走 chunk(见 _upload_in_sandbox)。
#
# 续传要解决的实测问题(2026-11-09, 国内 → 新加坡 Lighthouse):
#   - 直传 8MB = 50s(164KB/s), 但**每隔十几分钟就会被 reset 一次**(30 分钟内 ssh 断连 21 次),
#     断了从 0 重来, 15MB 的后端包经常要跑两三遍。
#   - 旧的"1MB 切片续传"(每片一个独立文件 + 两次 ssh 往返 + 片间 1s)同样 8MB = **438s**,
#     比直传慢 8.7 倍 —— 分片本身把收益全吃光了。
#   - 追加式续传: 正常链路整包一条连接(与直传同速), 中途断了只丢"隧道里在途"的那一点,
#     重连从远端字节数继续, **没有"传一半全废"**。
#
# 秒传(所有通道通用): 上送前先比远端同名文件的 sha256, 一致直接跳过 ——
#   "上次传完但发布被打断"的场景重跑不重传。
#
# 可调环境变量:
#   JEROCINE_UPLOAD_MODE          direct(默认) | chunk|resume
#                                 (不设时: 沙箱内默认 chunk —— 直传的长流必被沙箱 SIGTERM)
#   JEROCINE_UPLOAD_DIRECT_TRIES  直传模式: 慢速被掐/失败后的重试次数, 默认 3
#   JEROCINE_UPLOAD_RESUME_TRIES  续传模式下最多重连/失败次数, 默认 10
#   JEROCINE_UPLOAD_STALL_SECS    续传模式: 连续多少秒零进度才重连, 默认 45
#                                 (续传**只掐停滞**, 慢但一直在动不掐 —— 掐掉会丢在途缓冲)
#   JEROCINE_UPLOAD_MAX_WINDOW    续传模式: 单条连接最多传多少字节, auto(默认) =
#                                 沙箱 1MB / 普通终端 0(不限); 也可写死数值
#   JEROCINE_UPLOAD_WINDOW_ON_FAIL 续传中途被 reset 后自动切用的窗口大小, 默认 2097152
#   JEROCINE_UPLOAD_SSH_BIN       ssh 命令名, 默认 ssh(测试注 stub 用)
#   JEROCINE_UPLOAD_MIN_RATE      直传模式吞吐下限 B/s, 默认 65536(低于此判"慢"→重连)
#   JEROCINE_UPLOAD_SAMPLE_SECS   直传模式吞吐采样窗口秒数, 默认 10
#   JEROCINE_UPLOAD_NEAR_DONE_PCT 直传模式进度超过此百分比后不再因慢速重连, 默认 90
#   JEROCINE_UPLOAD_SLOW_STREAKS  直传模式连续多少个慢速窗口才重连, 默认 2
#
# 输出层级(供调用方对齐): 调用方(deploy-*.sh)先打印第 1 层「==> 上传 <包名>」,
# 本库内部消息一律第 2 层「  ==> …」, 传输进度行第 3 层「    …」。

_upload_info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
# 第 2 层: 上送环节内的小节(deploy-*.sh 的「==> 上传 <包名>」是第 1 层)。
_upload_sub() { printf '  \033[36m==>\033[0m %s\n' "$*"; }
# 第 3 层: 明细行(分片进度等), 纯文本不带上标。
_upload_detail() { printf '    %s\n' "$*"; }
_upload_warn() { printf '  [upload] %s\n' "$*" >&2; }

_upload_mb() { awk -v b="${1:-0}" 'BEGIN{printf "%.1fMB", b/1048576}'; }

# 是否在 agent 命令沙箱里(WorkBuddy 等注入 BASH_ENV 指向 shell-runtime-bash-env.sh)。
# 沙箱把**单条命令的流**闸在 ~1–2MB, 超了直接 SIGTERM —— 而且杀的是整条命令(连同脚本),
# 所以"先直传、被掐了再回退"在沙箱里活不到回退那步(2026-10-19 实测)。两处用到它:
# ① 通道选路: 沙箱里默认改走续传; ② 续传的段大小: 沙箱里默认 1MB/段。
_upload_in_sandbox() {
  case "${BASH_ENV:-}" in
    *shell-runtime-bash-env*) return 0 ;;
  esac
  return 1
}

# 秒数 → "45s" / "2m03s"
_upload_dur() {
  local s="${1:-0}"
  if [ "$s" -ge 60 ]; then printf '%dm%02ds' $((s / 60)) $((s % 60)); else printf '%ss' "$s"; fi
}

_upload_filesize() { wc -c < "$1" | tr -d ' '; }

# 临时目录 —— **必须避开 Windows 盘符路径**: Git Bash 下 TMPDIR 常是
# `C:\Users\...\Temp`, mktemp 会吐出 `C:\...\tmp.xxx`, 一是混分隔符(awk 把它当远程主机/
# 转义序列, 状态文件写不进去 → 看门狗读到冻结字节把健康连接反复掐掉, 2026-11-09 实测
# 8MB 因此从 50s 拖到 2m54s), 二是删除守卫不认这种路径。统一掰到 /tmp(MSYS 的同一个物理目录)。
_upload_tmpdir() {
  case "${TMPDIR:-}" in
    [A-Za-z]:*) printf '/tmp' ;;
    '')         printf '/tmp' ;;
    *)          printf '%s' "$TMPDIR" ;;
  esac
}

# 远端文件字节数(不存在/读取失败 → 0)。不信任本地记账: 每次都重新问远端。
_upload_remote_size() {
  local ssh_bin="$1" host="$2" path="$3" n
  n="$("$ssh_bin" "$host" "wc -c < '$path' 2>/dev/null" 2>/dev/null | tr -d ' \r\n')"
  case "$n" in ''|*[!0-9]*) n=0 ;; esac
  printf '%s' "$n"
}

# dd 是否支持 status=progress + iflag=skip_bytes(没有就退化为 tail -c, 只是少进度条)
_upload_probe_dd() {
  dd if=/dev/zero of=/dev/null bs=1 skip=1 iflag=skip_bytes count=1 status=progress >/dev/null 2>&1
}

# dd 进度重写用的 awk 程序(供下面两种 dd 调用共用):
# dd 的进度以 \r 分隔(没有 \n), awk 必须 RS="\r" 才能逐条吃到, 否则要等结束时一整坨才吐出来;
# awk stdout 显式 >&2 —— procsub 会继承 dd 在管道里的 stdout(ssh 数据流),
# 不钉死会把进度文本混进上传数据(踩过)。
_UPLOAD_PROGRESS_AWK='
      BEGIN { RS = "\r" }
      /copied/ {
        w = 0
        for (i = 1; i < NF; i++) if ($i == "bytes") { w = $(i - 1) + 0; break }
        b = off + w
        # 实时字节落状态文件: 供看门狗采样进度。**必须 fflush** —— awk 对文件输出有缓冲,
        # 不刷会长时间停在旧值, 看门狗读到冻结字节就把健康连接当停滞掐掉(踩过)。
        if (sfile != "") { printf "%d\n", b > sfile; fflush(sfile) }
        t = ""; rate = ""
        for (i = 1; i <= NF; i++) {
          if ($i == "copied,") t = $(i + 1) + 0
          if ($i ~ /B\/s/) rate = $(i - 1) " " $i
        }
        pct = (total > 0) ? int(b * 100 / total) : 0
        printf "\r    已传 %.1fMB / %.1fMB (%d%%) | 速率 %s | 已用 %ds   ",
               b / 1048576, total / 1048576, pct, (rate == "" ? "-" : rate), t
        fflush()
        shown = 1
      }
      shown == 0 && ! /copied/ && NF { printf "%s", $0 }
      END { if (shown) printf "\n" }'

# 从偏移发送: dd skip=<偏移字节> 只读剩余部分; max>0 时只发这么多字节(分段窗口)。
_upload_send_from() {
  local src="$1" off="$2" total="$3" statf="${4:-}" max="${5:-0}"
  # 路径里的反斜杠会被 awk 当转义序列吃掉(`\U`/`\k`…), 直接把路径写坏 —— 统一正斜杠
  local statf_arg="${statf//\\//}"
  if [ "$_UPLOAD_DD_PROGRESS" = "1" ]; then
    if [ "$max" -gt 0 ]; then
      dd if="$src" bs=256k skip="$off" iflag=skip_bytes,count_bytes count="$max" status=progress \
        2> >(awk -v total="$total" -v off="$off" -v sfile="$statf_arg" "$_UPLOAD_PROGRESS_AWK" >&2)
    else
      dd if="$src" bs=256k skip="$off" iflag=skip_bytes status=progress \
        2> >(awk -v total="$total" -v off="$off" -v sfile="$statf_arg" "$_UPLOAD_PROGRESS_AWK" >&2)
    fi
  else
    # 没有 GNU dd: 退化为 tail/head(没有进度条, 但语义一致)
    if [ "$max" -gt 0 ]; then
      { if [ "$off" -gt 0 ]; then tail -c "+$((off + 1))" "$src"; else cat "$src"; fi; } | head -c "$max"
    else
      if [ "$off" -gt 0 ]; then tail -c "+$((off + 1))" "$src"; else cat "$src"; fi
    fi
  fi
}

# 停滞看门狗(续传模式用) —— 与吞吐看门狗的区别很重要:
#   续传模式下"慢但一直在动"**不该掐**: 掐掉会丢掉 ssh/管道里在途的缓冲区(实测一次丢 1.6MB),
#   而重连只是换条路径, 未必更快。真正要掐的是"完全没有进度"(对端黑洞/RTO 退避到天荒地老)。
#   因此判据从"速率低于阈值"改成"连续 N 秒字节数没变化"。
#
# 用法: _upload_stall_watchdog <状态文件> <ssh_pid>
# 返回 0 = 连接自然结束; 42 = 停滞超时(调用方应杀连接、重连续传)
_upload_stall_watchdog() {
  local statf="$1" ssh_pid="$2"
  local stall="${JEROCINE_UPLOAD_STALL_SECS:-45}"
  local b last="" idle=0
  while kill -0 "$ssh_pid" 2>/dev/null; do
    sleep 2
    kill -0 "$ssh_pid" 2>/dev/null || return 0
    b="$(tail -n 1 "$statf" 2>/dev/null)"
    case "$b" in ''|*[!0-9]*) b=0 ;; esac
    if [ "$b" = "$last" ]; then
      idle=$((idle + 2))
      if [ "$idle" -ge "$stall" ]; then
        _upload_sub "连续 ${idle}s 没有任何进度 → 重连接续"
        return 42
      fi
    else
      last="$b"
      idle=0
    fi
  done
  return 0
}

# 吞吐看门狗(直传模式用): 传输慢不报错 —— 连接活着但速率爬行, 常规"失败回退"永远等不到
# 它结束。直传断了要从 0 重来, 所以这里宁可掐掉换路径(续传模式请用停滞看门狗)。
# 慢的时候主动掐掉重连, 新 TCP 常能换到更好的路径; 续传模式下重连是**从远端字节数继续**,
# 代价只有在途缓冲, 所以这里掐得比直传更划算。
#
# 两个抑制条件:
#   - 尾段保护: 进度 ≥ NEAR_DONE_PCT(默认 90%)后不再因慢速重连 —— 只剩零头时重连不值得;
#   - 连续窗口: 需连续 SLOW_STREAKS(默认 2)个采样窗口低于阈值才动手, 单窗口抖动不重连。
#
# 用法: _upload_watchdog <状态文件> <总字节> <ssh_pid>
# 返回 0 = 连接已自然结束; 42 = 连续慢速达标(调用方应杀连接、重连续传)。
_upload_watchdog() {
  local statf="$1" total="$2" ssh_pid="$3"
  local min_rate="${JEROCINE_UPLOAD_MIN_RATE:-65536}"
  local sample="${JEROCINE_UPLOAD_SAMPLE_SECS:-10}"
  local near_done="${JEROCINE_UPLOAD_NEAR_DONE_PCT:-90}"
  local need_streaks="${JEROCINE_UPLOAD_SLOW_STREAKS:-2}"
  local prev_b=0 prev_t b now delta rate pct streak=0 near_said=0
  prev_t="$(date +%s)"
  while kill -0 "$ssh_pid" 2>/dev/null; do
    sleep 1   # 1s 一 tick, 传完能立刻察觉, 不给成功路径加固定延迟
    kill -0 "$ssh_pid" 2>/dev/null || return 0
    b="$(tail -n 1 "$statf" 2>/dev/null)"
    case "$b" in ''|*[!0-9]*) b=0 ;; esac
    now="$(date +%s)"
    if [ $((now - prev_t)) -ge "$sample" ]; then
      delta=$((b - prev_b))
      rate=$((delta / (now - prev_t)))
      pct=0
      [ "$total" -gt 0 ] 2>/dev/null && pct=$((b * 100 / total))
      if [ "$pct" -ge "$near_done" ]; then
        if [ "$near_said" = "0" ]; then
          _upload_sub "进度 ${pct}% ≥ ${near_done}% → 尾段保护, 不再因慢速重连(等自然完成)"
          near_said=1
        fi
        streak=0
      elif [ "$rate" -lt "$min_rate" ]; then
        streak=$((streak + 1))
        if [ "$streak" -ge "$need_streaks" ]; then
          _upload_sub "连续 $streak 个窗口速率过低($((rate / 1024))KB/s < $((min_rate / 1024))KB/s, 进度 ${pct}%) → 重连接续"
          return 42
        fi
        _upload_sub "窗口速率偏低($((rate / 1024))KB/s, 进度 ${pct}%), 连续 $streak/$need_streaks —— 观望下个窗口"
      else
        streak=0
      fi
      prev_b=$b
      prev_t=$now
    fi
  done
  return 0
}

# 校验远端文件 sha256 与本地一致
_upload_verify() {
  local ssh_bin="$1" host="$2" dst="$3" want_sha="$4" remote_sha
  remote_sha="$("$ssh_bin" "$host" "sha256sum '$dst'" 2>/dev/null | awk '{print $1}')"
  [ -n "$remote_sha" ] && [ "$remote_sha" = "$want_sha" ]
}

# ── 通道: 单流追加式断点续传(JEROCINE_UPLOAD_MODE=chunk 时启用; 私有仓默认) ──────────
# 远端只维护一个 `$dst.<sha12>.jcpart`; 名字掺入本地 sha 前缀 ⇒ 同名不同内容的重发包
# 不会复用陈旧进度(旧实现按字节数判定"已在远端", 重打包后必拼出坏文件且重跑永远失败)。
_upload_resume() {
  local ssh_bin="$1" host="$2" src="$3" dst="$4" local_sha="$5" total="$6"
  local tries="${JEROCINE_UPLOAD_RESUME_TRIES:-10}"
  local part="$dst.${local_sha:0:12}.jcpart"
  local tmpdir; tmpdir="$(_upload_tmpdir)"
  # 单条连接最多传多少字节(0=不限)。适配两类硬约束:
  #   ① agent 沙箱把"单条命令的流"闸在 ~1–2MB(超了连脚本一起 SIGTERM) ⇒ 沙箱里默认 1MB/段;
  #   ② 跨境链路会**中途 reset**, 隧道里在途的那 1–2MB 随连接一起丢(实测一次丢 1.6MB) ⇒
  #      第一次"中途断"之后自动切成 2MB/段, 把每次丢失的上限压到一段以内。
  # 正常链路上 window=0: 整包一条连接, 不付多余握手。
  local window="${JEROCINE_UPLOAD_MAX_WINDOW:-auto}"
  if [ "$window" = "auto" ]; then
    if _upload_in_sandbox; then window=1048576; else window=0; fi
  fi
  local t0 a=0 fails=0 rsize=-1 got=0 limit=0 seg final=""
  local fifod statf ssh_pid dd_pid rc_ssh rc_dd slow size_line
  t0="$(date +%s)"

  _upload_sub "断点续传(单流追加, 断线按远端已收字节续传)"

  while :; do
    if [ "$a" -ge 400 ]; then
      _upload_warn "连接次数异常(>400) —— 中止; 远端进度已保留"
      break
    fi
    # 远端已收字节: 上一轮正常收尾时直接复用远端自己回报的数字(省掉一次握手往返,
    # 慢链路上一次 = 2–10s); 只有中断/停滞之后才重新问远端 —— 那时本地记账已不可信。
    if [ "$rsize" -lt 0 ]; then
      rsize="$(_upload_remote_size "$ssh_bin" "$host" "$part")"
    fi

    # 远端残留比源文件还大 ⇒ 进度已不可信, 丢弃重传(比"拼出坏包"便宜)
    if [ "$rsize" -gt "$total" ]; then
      _upload_warn "远端残留 $(_upload_mb "$rsize") 大于源文件 $(_upload_mb "$total") → 丢弃重传"
      "$ssh_bin" "$host" ": > '$part'" >/dev/null 2>&1 || true
      rsize=0
    fi
    if [ "$rsize" -eq "$total" ]; then
      _upload_sub "远端已收满 $(_upload_mb "$total") → 跳过传输, 直接校验"
      final="$total"
      break
    fi

    a=$((a + 1))
    limit=0
    if [ "$window" -gt 0 ] && [ $((total - rsize)) -gt "$window" ]; then
      limit="$window"
    fi
    seg=""
    [ "$limit" -gt 0 ] && seg=", 本段 $(_upload_mb "$limit")"
    if [ "$rsize" -gt 0 ]; then
      _upload_sub "从 $(_upload_mb "$rsize")/$(_upload_mb "$total")($((rsize * 100 / total))%)继续 —— 第 $a 次连接${seg}"
    else
      _upload_sub "上传 $(_upload_mb "$total") —— 第 $a 次连接${seg}"
    fi

    fifod="$(mktemp -d "$tmpdir/jc-up-resume.XXXXXX")" || return 1
    mkfifo "$fifod/up" || { rm -rf "$fifod"; return 1; }
    statf="$(mktemp "$tmpdir/jc-up-stat.XXXXXX")" || { rm -rf "$fifod"; return 1; }
    # 远端先 `cat >>` 追加, 收满 EOF 后回报最终字节数(stdout 只有这一行, 数据全走 stdin)。
    # 这条"顺手回报"省掉了每轮一次 wc 往返 —— 慢链路上一次往返就是 2–10s。
    "$ssh_bin" "$host" "cat >> '$part'; wc -c < '$part'" < "$fifod/up" > "$fifod/size" 2>/dev/null &
    ssh_pid=$!
    _upload_send_from "$src" "$rsize" "$total" "$statf" "$limit" > "$fifod/up" &
    dd_pid=$!
    slow=0
    _upload_stall_watchdog "$statf" "$ssh_pid" || slow=1
    kill "$ssh_pid" "$dd_pid" 2>/dev/null || true
    rc_ssh=0; rc_dd=0
    wait "$ssh_pid" 2>/dev/null || rc_ssh=$?
    wait "$dd_pid" 2>/dev/null || rc_dd=$?
    rm -f "$statf"
    size_line="$(tr -d ' \r\n' < "$fifod/size" 2>/dev/null)"
    rm -rf "$fifod"
    case "$size_line" in ''|*[!0-9]*) size_line="" ;; esac
    got="${size_line:-0}"

    # 成功判据: 远端自己回报的字节数已收满(不看本地猜测)
    if [ "$got" -ge "$total" ] && [ "$rc_ssh" = "0" ]; then
      final="$got"      # 复用远端刚回报的数字, 收尾时不再多握一次手(慢链路一次 2–10s)
      break
    fi
    # 有进展且连接是正常结束的(分段窗口的常规收尾) ⇒ 立刻续下一段, 不计失败、不等待,
    # 也不再问一次远端尺寸(远端刚回报过)
    if [ "$got" -gt "$rsize" ] && [ "$rc_ssh" = "0" ] && [ "$slow" = "0" ]; then
      rsize="$got"
      continue
    fi

    fails=$((fails + 1))
    # 判据只能是"连接非正常结束"(rc_ssh≠0: 被 reset / 被停滞看门狗掐), **不能**看远端回报的
    # 字节数 —— 连接一断, 远端那句 `wc -c` 的 stdout 也一起丢, got 恒为 0, 拿 `got > rsize`
    # 判断会让自适应分段永远不触发(实测就是这么白丢了一整轮)。
    # 单流整包模式(window=0)下, 一次 reset = 在途缓冲全丢 = 从 0 重来; 所以第一轮之后
    # 就切成窗口, 把每次丢失的上限压在一段以内。
    if [ "$rc_ssh" -ne 0 ] && [ "$window" -eq 0 ] && [ "$got" -lt "$total" ]; then
      window="${JEROCINE_UPLOAD_WINDOW_ON_FAIL:-2097152}"
      _upload_sub "连接中途断开(在途缓冲随连接一起丢) → 后续按 $(_upload_mb "$window") 一段一段传"
    else
      _upload_sub "连接中断/停滞 → 重连续传(远端已收 $(_upload_mb "$got"))"
    fi
    rsize=-1
    if [ "$fails" -ge "$tries" ]; then
      _upload_warn "重试耗尽($fails 次失败 / $a 次连接) —— 远端进度已保留, 重跑本脚本从断点继续"
      break
    fi
    sleep 1
  done

  # 没传满就**不要**做落位事务 —— 留着 .jcpart 供下次续传(用户重跑即可, 不从头来)。
  # final 为空 = 上面没走到"收满"分支(重试耗尽/连接次数异常)或走到了 rsize==total 的短路,
  # 那种情况本地记账不可信, 老实问一次远端。
  [ -n "$final" ] || final="$(_upload_remote_size "$ssh_bin" "$host" "$part")"
  if [ "$final" -ne "$total" ]; then
    _upload_warn "未传满($(_upload_mb "$final")/$(_upload_mb "$total")) —— 远端进度已保留, 重跑本脚本从断点继续"
    return 1
  fi

  # 收尾: 先校验再落位(**绝不能先 mv 再验** —— 那样失败时坏包已经躺在正式路径上,
  # 下次 秒传 比对必然不一致、也容易被误发布)。远端一个事务里完成:
  #   sha256(.jcpart) 与本地一致 → mv 落位(顺带清掉旧切片实现留下的 `$dst.jcparts` 目录,
  #   兼容历史现场); 不一致 → 删掉坏进度。清目录并进这条命令, 不再多握一次手。
  local fin
  fin="$("$ssh_bin" "$host" \
    "s=\$(sha256sum '$part' 2>/dev/null | cut -d' ' -f1); if [ \"\$s\" = '$local_sha' ]; then mv -f '$part' '$dst' && rm -rf '$dst.jcparts' && echo JCUP_OK; else rm -f '$part'; echo \"JCUP_BAD:\$s\"; fi" \
    2>/dev/null | tr -d '\r')"
  case "$fin" in
    *JCUP_OK*) ;;
    *)
      _upload_warn "sha256 不一致(${fin:-远端无响应}) —— 坏进度已丢弃, 请重跑"
      return 1
      ;;
  esac
  _upload_sub "断点续传成功(sha256 ${local_sha:0:12}… 一致, 上传耗时 $(_upload_dur "$(($(date +%s) - t0))"))"
  return 0
}

# ── 通道: 整包直传(看门狗重连, 每次从 0 重来) ───────────────────────────────
_upload_direct() {
  local ssh_bin="$1" host="$2" src="$3" dst="$4" local_sha="$5" total="$6"
  local tries="${JEROCINE_UPLOAD_DIRECT_TRIES:-3}"
  local tmpdir; tmpdir="$(_upload_tmpdir)"
  local t0 a=1 fifod statf ssh_pid dd_pid rc_ssh rc_dd slow
  t0="$(date +%s)"
  fifod="$(mktemp -d "$tmpdir/jc-up-direct.XXXXXX")" || return 1
  mkfifo "$fifod/up" || { rm -rf "$fifod"; return 1; }
  while :; do
    _upload_sub "整包直传(第 $a/$tries 次尝试)…(进度: 已传/总量/百分比)"
    statf="$(mktemp "$tmpdir/jc-up-stat.XXXXXX")" || { rm -rf "$fifod"; return 1; }
    # ssh(读端)先起 dd(写端)后起, 都拿显式 pid —— 看门狗判慢时精准掐, 不留孤儿进程
    "$ssh_bin" "$host" "cat > '$dst'" < "$fifod/up" 2>/dev/null &
    ssh_pid=$!
    _upload_send_from "$src" 0 "$total" "$statf" > "$fifod/up" &
    dd_pid=$!
    slow=0
    _upload_watchdog "$statf" "$total" "$ssh_pid" || slow=1
    kill "$ssh_pid" "$dd_pid" 2>/dev/null || true
    rc_ssh=0; rc_dd=0
    wait "$ssh_pid" 2>/dev/null || rc_ssh=$?
    wait "$dd_pid" 2>/dev/null || rc_dd=$?
    rm -f "$statf"
    if [ "$slow" = "0" ] && [ "$rc_ssh" = "0" ] && [ "$rc_dd" = "0" ] \
       && _upload_verify "$ssh_bin" "$host" "$dst" "$local_sha"; then
      rm -rf "$fifod"
      _upload_sub "直传成功(sha256 ${local_sha:0:12}… 一致, 上传耗时 $(_upload_dur "$(($(date +%s) - t0))"))"
      return 0
    fi
    if [ "$a" -ge "$tries" ]; then
      rm -rf "$fifod"
      _upload_warn "直传重试耗尽 —— 建议改用续传模式(JEROCINE_UPLOAD_MODE=chunk)"
      return 1
    fi
    a=$((a + 1))
    sleep 1
  done
}

# shellcheck disable=SC2120  # 函数按位置参数取值, 允许调用方少传
upload_over_ssh() {
  local host="$1" src="$2" dst="$3"
  local ssh_bin="${JEROCINE_UPLOAD_SSH_BIN:-ssh}"
  # 未显式指定通道时按环境选: 普通终端整包直传(通用默认), **agent 命令沙箱里必须走续传**
  # (沙箱闸 ~1–2MB, 直传的长流会被 SIGTERM 掐死, 且杀了整条命令, 没有"回退"的机会)。
  local mode="${JEROCINE_UPLOAD_MODE:-}"
  if [ -z "$mode" ]; then
    if _upload_in_sandbox; then mode=chunk; else mode=direct; fi
  fi

  [ -f "$src" ] || { _upload_warn "本地文件不存在: $src"; return 1; }
  [ -n "$host" ] && [ -n "$dst" ] || { _upload_warn "缺少 host 或远端路径"; return 1; }

  local local_size local_sha
  local_size="$(_upload_filesize "$src")"
  local_sha="$(sha256sum "$src" | awk '{print $1}')"

  _upload_sub "目标 $host:$dst (本地 $(_upload_mb "$local_size"), sha256 ${local_sha:0:12}…)"

  # ---- 秒传: 远端已有同名同内容文件 → 跳过上传 ----
  local remote_sha
  remote_sha="$("$ssh_bin" "$host" "sha256sum '$dst'" 2>/dev/null | awk '{print $1}')"
  if [ -n "$remote_sha" ] && [ "$remote_sha" = "$local_sha" ]; then
    _upload_sub "远端已存在相同文件(sha256 ${local_sha:0:12}… 一致) → 跳过上传, 直接复用"
    return 0
  fi

  # dd 能力探测一次(决定有没有进度条); 探测失败不影响正确性, 只退化显示
  _UPLOAD_DD_PROGRESS=0
  _upload_probe_dd && _UPLOAD_DD_PROGRESS=1

  case "$mode" in
    direct|auto)       _upload_direct "$ssh_bin" "$host" "$src" "$dst" "$local_sha" "$local_size" ;;
    chunk|resume)      _upload_resume "$ssh_bin" "$host" "$src" "$dst" "$local_sha" "$local_size" ;;
    *) _upload_warn "未知 JEROCINE_UPLOAD_MODE=$mode(可选 direct|chunk)"; return 64 ;;
  esac
}

# ---------------------------------------------------------------------------
# CLI 直跑入口(不经 source):
#   upload-over-ssh.sh <host> <本地文件> <远端路径>
# 供 deploy-{web,server,danmu}.sh 以 JEROCINE_UPLOAD_CMD="bash <本脚本>" 注入。
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  if [ "$#" -ne 3 ]; then
    printf '[upload] 用法: upload-over-ssh.sh <host> <本地文件> <远端路径>\n' >&2
    exit 64
  fi
  upload_over_ssh "$1" "$2" "$3"
fi

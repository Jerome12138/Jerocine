import { onScopeDispose, ref, type Ref } from 'vue'
import type Player from 'video.js/dist/types/player'

/**
 * 触屏手势 composable（web 播放器专用, 仅全屏启用; TV/原生播放器不适用）。
 *
 * 语义（与原生端约定保持一致, 改动时两边同步评估）：
 * - 长按(450ms, 播放中) = 临时 3x 倍速, 松手恢复 —— 不写持久化缓存
 * - 按住横拖 = 连续刮擦进度: 渐进式步进(起步 1.2s/px 精确 → 平方项加速 → 封顶 ±10min),
 *   底部预览面板(迷你进度条: 目标位置白条 + 当前位置灰点 + 目标时间文本), 长拖节流实时 seek 跟手
 * - 短促轻扫(<300ms 且 <60px) = 固定 ±10s
 * - 控制条/菜单等 UI 上的触摸不参与手势(防误触)
 *
 * 提示 UI 由调用方渲染(模板消费 gestureRateHint / gestureSeekHint 两个 ref),
 * 覆盖层需 Teleport 进 video.js 的 .video-js 容器(全屏元素), 否则全屏时不可见。
 *
 * 使用：
 *   const { onTouchStart, onTouchMove, onTouchEnd, gestureRateHint, gestureSeekHint } =
 *     usePlayerGestures({ player, paused, buffering, fullscreen: isPlayerFullscreen, enterTempRate, exitTempRate })
 *   // 模板: @touchstart="onTouchStart" @touchmove="onTouchMove" @touchend="onTouchEnd" @touchcancel="onTouchEnd"
 */
export interface PlayerGestureOptions {
  /** video.js 实例(shallowRef 直通, 手势只做即时读写, 不依赖响应式) */
  player: Ref<Player | null>
  paused: Ref<boolean>
  buffering: Ref<boolean>
  /** 仅全屏时启用手势 */
  fullscreen: Ref<boolean>
  /** 进入临时倍速(不落持久化, 由 usePlayer 实现) */
  enterTempRate: (rate: number) => void
  /** 退出临时倍速, 恢复进入前档位 */
  exitTempRate: () => void
}

export function usePlayerGestures(opts: PlayerGestureOptions) {
  const { player, paused, buffering, fullscreen } = opts

  /* ============ 常量 ============ */
  /** 长按判定时长(ms) */
  const GESTURE_LONG_PRESS_MS = 450
  /** 横向意图判定阈值(px): 水平位移超过它且强于竖直 → 进入刮擦 */
  const GESTURE_SWIPE_TRIGGER_PX = 12
  /** 短促轻扫判定: 触摸时长 < 300ms 且位移 < 60px → 固定步进而非刮擦 */
  const GESTURE_FLICK_MS = 300
  const GESTURE_FLICK_PX = 60
  /** 短促轻扫的固定步进(秒) */
  const GESTURE_FLICK_SEEK_SEC = 10
  /** 刮擦实时 seek 的节流间隔(ms) — 预览条每次 move 都刷新, 实际 seek 节流 */
  const GESTURE_SEEK_INTERVAL_MS = 400
  /** 长按临时倍速 */
  const TEMP_RATE = 3
  /** 刮擦上限: 单次滑动最多 ±10 分钟(防长滑过冲) */
  const GESTURE_SEEK_MAX_SEC = 600
  /** 起步精确: 1px ≈ 1.2s(线性段), 保证小幅滑动可精细微调 */
  const GESTURE_SEEK_FINE_SPP = 1.2
  /** 渐进加速: 平方项除数, 越小则长滑加速越猛 */
  const GESTURE_SEEK_ACCEL_DIV = 50

  /* ============ 状态(提示 UI 供模板消费) ============ */
  /** 长按临时倍速提示(播放器顶部居中), 空串 = 隐藏 */
  const gestureRateHint = ref('')
  /** 横滑刮擦预览(播放器底部: 迷你进度条 + 目标时间), null = 隐藏 */
  const gestureSeekHint = ref<{ percent: number; curPercent: number; text: string } | null>(null)

  /* ============ 内部手势状态(非响应式) ============ */
  type GestureMode = 'none' | 'press' | 'seek'
  let gestureMode: GestureMode = 'none'
  let gestureStartX = 0
  let gestureStartY = 0
  let gestureStartTime = 0
  let gestureSeekBase = 0
  let gestureTarget = 0
  let gestureLastDx = 0
  let gestureLastSeekAt = 0
  let longPressTimer: ReturnType<typeof setTimeout> | null = null
  let seekHintTimer: ReturnType<typeof setTimeout> | null = null

  function clearLongPressTimer(): void {
    if (longPressTimer) {
      clearTimeout(longPressTimer)
      longPressTimer = null
    }
  }

  /** 刮擦步进(渐进式): 起步线性 1.2s/px 精确微调; 距离越长平方项加速; 封顶 ±10min。
   * 手感参考: 10px≈14s, 30px≈54s, 60px≈144s, 100px≈320s, ≈147px 封顶 10min。 */
  function scrubOffsetSec(nPx: number): number {
    if (nPx <= 0) return 0
    return Math.min(GESTURE_SEEK_MAX_SEC, GESTURE_SEEK_FINE_SPP * nPx + (nPx * nPx) / GESTURE_SEEK_ACCEL_DIV)
  }

  /** m:ss */
  function fmtTime(sec: number): string {
    const s = Math.max(0, Math.round(sec))
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
  }

  /** 更新刮擦预览: 迷你进度条(目标位置 + 当前位置) + 文本(快进/退量 · 目标时间 · 百分比) */
  function updateSeekHint(p: Player, target: number): void {
    const dur = p.duration() ?? 0
    const pct = (v: number) => Math.min(100, Math.max(0, Math.round((v / (dur || 1)) * 100)))
    const delta = target - gestureSeekBase
    gestureSeekHint.value = {
      percent: pct(target),
      curPercent: pct(p.currentTime() ?? 0),
      text:
        `${delta >= 0 ? '快进' : '快退'} ${fmtTime(Math.abs(delta))}` +
        ` · ${fmtTime(target)} (${pct(target)}%)`
    }
  }

  /** 短促轻扫后的短暂提示, 600ms 后自动消失 */
  function flashSeekHint(): void {
    if (seekHintTimer) clearTimeout(seekHintTimer)
    seekHintTimer = setTimeout(() => {
      gestureSeekHint.value = null
      seekHintTimer = null
    }, 600)
  }

  /** 触摸落在控制条/菜单等 UI 上时不做手势 */
  function isGestureTargetUi(e: TouchEvent): boolean {
    const el = e.target as HTMLElement | null
    return !!el?.closest?.('.vjs-control-bar, .vjs-menu, .vjs-modal-dialog')
  }

  /* ============ 触摸处理器(模板直绑) ============ */

  function onTouchStart(e: TouchEvent): void {
    if (!fullscreen.value) return
    if (!e.touches || e.touches.length !== 1) return
    if (isGestureTargetUi(e)) return
    const t = e.touches[0]
    if (!t) return
    gestureStartX = t.clientX
    gestureStartY = t.clientY
    gestureStartTime = Date.now()
    gestureMode = 'none'
    gestureLastDx = 0
    clearLongPressTimer()
    longPressTimer = setTimeout(() => {
      const p = player.value
      // 仅播放中(暂停/缓冲不触发)进入临时倍速
      if (!p || paused.value || buffering.value) return
      gestureMode = 'press'
      opts.enterTempRate(TEMP_RATE)
      gestureRateHint.value = `${TEMP_RATE}x 倍速中 ${'▶'.repeat(TEMP_RATE)}`
    }, GESTURE_LONG_PRESS_MS)
  }

  function onTouchMove(e: TouchEvent): void {
    if (!e.touches || e.touches.length !== 1) return
    if (isGestureTargetUi(e)) return
    const p = player.value
    if (!p) return
    const t = e.touches[0]
    if (!t) return
    const dx = t.clientX - gestureStartX
    const dy = t.clientY - gestureStartY
    if (gestureMode === 'none') {
      // 横向意图判定: 水平位移超阈值且强于竖直 → 进入刮擦, 同时取消长按
      if (Math.abs(dx) > GESTURE_SWIPE_TRIGGER_PX && Math.abs(dx) > Math.abs(dy)) {
        clearLongPressTimer()
        gestureMode = 'seek'
        gestureSeekBase = p.currentTime() ?? 0
        gestureTarget = gestureSeekBase
        gestureLastDx = dx
        gestureLastSeekAt = 0
      }
      return // 尚未判向或竖向滑动, 不做处理
    }
    if (gestureMode !== 'seek') return // 已长按 3x: 移动不转刮擦
    gestureLastDx = dx
    const dur = p.duration() ?? 0
    // 渐进式步进: 方向跟随手势, 步进量只看 |dx| (起步精确 → 越滑越快 → 封顶 10min)
    const offset = scrubOffsetSec(Math.abs(dx))
    gestureTarget = Math.min(
      Math.max(gestureSeekBase + (dx >= 0 ? offset : -offset), 0),
      Math.max(dur - 0.5, 0)
    )
    updateSeekHint(p, gestureTarget)
    // 长拖: 节流实时 seek 让进度条跟手(预览条本身每次 move 都刷新); 短促轻扫留给 touchend 判定
    const now = Date.now()
    if (
      now - gestureStartTime > GESTURE_FLICK_MS &&
      Math.abs(dx) > GESTURE_FLICK_PX &&
      now - gestureLastSeekAt > GESTURE_SEEK_INTERVAL_MS
    ) {
      gestureLastSeekAt = now
      try {
        p.currentTime(gestureTarget)
      } catch {
        /* ignore */
      }
    }
    if (e.cancelable) e.preventDefault()
  }

  function onTouchEnd(): void {
    clearLongPressTimer()
    const p = player.value
    if (gestureMode === 'press') {
      opts.exitTempRate()
      gestureRateHint.value = ''
    } else if (gestureMode === 'seek' && p) {
      const dt = Date.now() - gestureStartTime
      if (dt < GESTURE_FLICK_MS && Math.abs(gestureLastDx) < GESTURE_FLICK_PX) {
        // 短促轻扫: 固定 ±10s
        const step = gestureLastDx >= 0 ? GESTURE_FLICK_SEEK_SEC : -GESTURE_FLICK_SEEK_SEC
        const target = Math.max(0, (p.currentTime() ?? 0) + step)
        try {
          p.currentTime(target)
        } catch {
          /* ignore */
        }
        updateSeekHint(p, target)
        flashSeekHint()
      } else {
        // 长拖松手: 落到预览显示的目标点, 提示即清
        try {
          p.currentTime(gestureTarget)
        } catch {
          /* ignore */
        }
        gestureSeekHint.value = null
      }
    } else {
      gestureSeekHint.value = null
    }
    gestureMode = 'none'
  }

  // 组件卸载时清定时器, 防止卸载后回调摸已释放的播放器
  onScopeDispose(() => {
    clearLongPressTimer()
    if (seekHintTimer) {
      clearTimeout(seekHintTimer)
      seekHintTimer = null
    }
  })

  return {
    /** 模板直绑: @touchstart / @touchmove / @touchend|@touchcancel */
    onTouchStart,
    onTouchMove,
    onTouchEnd,
    /** 长按临时倍速提示(顶部居中) */
    gestureRateHint,
    /** 刮擦预览(底部进度条面板) */
    gestureSeekHint
  }
}

import {
  computed,
  onScopeDispose,
  ref,
  shallowRef,
  watch,
  type ComputedRef,
  type Ref,
  type ShallowRef
} from 'vue'
import videojs from 'video.js'
import type Player from 'video.js/dist/types/player'
// video.js 核心皮肤 CSS — 没有它 .vjs-tech 不会拿到 position:absolute + 100%/100%, 内层 video 会塌成默认 300x150
import 'video.js/dist/video-js.css'

/**
 * video.js 播放器 composable。
 *
 * 设计要点：
 * - 实例存 shallowRef，避免被 Vue 反应式深包裹（video.js 内部状态非常多，深包裹会爆栈/性能问题）
 * - 切换 src 不重建 player，只调 player.src(...)，保留 controls / 设置
 * - 暴露 play / pause / seek / setVolume / on / off / dispose 等命令式 API
 * - 监听 ready / play / pause / ended / timeupdate / error 五类标准事件
 * - 不在内部触发 router 跳转 / toast，由调用方决定
 *
 * 使用：
 *   const videoEl = ref<HTMLVideoElement | null>(null)
 *   const { player, ready, init, dispose, play, pause, seek, setVolume, on } = usePlayer({...})
 *   onMounted(() => init(videoEl.value!))
 */

export interface UsePlayerOptions {
  /** 播放源 url（响应式） */
  src: Ref<string> | string
  /** 视频 mime 类型，例 'application/x-mpegURL' */
  type?: Ref<string | undefined> | string
  /** 海报 */
  poster?: Ref<string | undefined> | string
  /** 自动播放（用户已交互过的场景） */
  autoplay?: boolean
  /** 默认音量 0–1 */
  volume?: number
  /** 倍速可选项 */
  playbackRates?: number[]
  /** 是否使用循环 */
  loop?: boolean
  /**
   * preload 策略 (HTMLMediaElement.preload):
   *  - 'none'      不预加载, play() 才拉数据. 极弱网用.
   *  - 'metadata' (默认) 仅拉时长 + 起播分片元数据. 弱网友好.
   *  - 'auto'      让浏览器自由预加载. 强网且首屏立刻播时用.
   *
   * 旧默认是 'auto', 在弱网下会强占带宽, 首屏比 'metadata' 慢 30-50%.
   */
  preload?: 'none' | 'metadata' | 'auto'
  /**
   * 控制条「下一集」按钮(可选, I-018):
   * 提供后会在倍速与全屏之间注册一个常驻文字按钮, 点击触发 onClick。
   * 是否真的存在下一集由回调自行判断(如 PlayView.playNext 内部已 guard hasNext)。
   */
  nextEpisode?: { onClick: () => void }
  /**
   * 弱网模式. 开启后:
   *  - VHS 初始带宽猜测 800kbps (默认 4Mbps), ABR 从低码率起步
   *  - 目标缓冲 15s (默认 30s), 起播更快
   *  - buffer-based ABR (基于缓冲长度而非带宽估算), 网络抖动下决策更稳
   *  - 复用 localStorage 上次的带宽估算, 二次播放更聪明
   */
  lowBandwidth?: boolean
}

export type PlayerEventName =
  | 'ready'
  | 'play'
  | 'pause'
  | 'ended'
  | 'timeupdate'
  | 'error'
  | 'loadedmetadata'
  | 'volumechange'
  | 'waiting'
  | 'canplay'

export interface UsePlayerReturn {
  /** video.js 实例（ready 之前为 null） */
  player: ShallowRef<Player | null>
  /** 是否已 ready */
  ready: Ref<boolean>
  /** 当前播放进度（秒） */
  currentTime: Ref<number>
  /** 视频总时长 */
  duration: Ref<number>
  /** 是否处于播放中 */
  paused: Ref<boolean>
  /** 是否正在缓冲(首帧可播前的等待态, 用于展示 loading) */
  buffering: Ref<boolean>
  /** 是否处于错误态 */
  errored: ComputedRef<boolean>
  /** 创建并挂载到目标 video 元素 */
  init: (el: HTMLVideoElement) => void
  /** 销毁实例 */
  dispose: () => void
  play: () => Promise<void> | void
  pause: () => void
  /** 跳转到指定秒数 */
  seek: (seconds: number) => void
  /** 相对快进/快退（秒） */
  seekBy: (delta: number) => void
  /** 0–1 */
  setVolume: (v: number) => void
  /** 注册事件，返回取消函数 */
  on: (event: PlayerEventName, fn: (...args: unknown[]) => void) => () => void
  /** 直接 off（与 on 配合） */
  off: (event: PlayerEventName, fn: (...args: unknown[]) => void) => void
}

function unwrap<T>(v: Ref<T> | T): T {
  return (v && typeof v === 'object' && 'value' in (v as object))
    ? (v as Ref<T>).value
    : (v as T)
}

/* ============ 倍速本地缓存 (I-019) ============ */

const PLAYBACK_RATE_LS_KEY = 'gf-playback-rate'

/** 读取上次保存的倍速; 仅接受当前播放器支持的档位, 非法/隐私模式返回 null。 */
function readSavedPlaybackRate(validRates: number[]): number | null {
  try {
    const raw = localStorage.getItem(PLAYBACK_RATE_LS_KEY)
    if (!raw) return null
    const v = Number(raw)
    return Number.isFinite(v) && validRates.includes(v) ? v : null
  } catch {
    return null
  }
}

/** 持久化倍速(隐私模式静默忽略)。 */
function savePlaybackRate(v: number): void {
  try {
    localStorage.setItem(PLAYBACK_RATE_LS_KEY, String(v))
  } catch {
    /* 隐私模式忽略 */
  }
}

/* ============ 控制条「下一集」按钮 (I-018) ============ */

/** 当前实例的点击回调(模块级单例: 一页一个 player, init 时注入, dispose 时清空)。 */
let nextEpisodeHandler: (() => void) | null = null

/** 宽松类型: 只依赖 video.js Component/Button 的运行时行为, 避免 dist types 的私有签名。 */
interface VjsButtonLike {
  createEl(tag?: string, props?: Record<string, unknown>): HTMLElement
  handleClick(): void
}
type VjsButtonCtor = new () => {
  createEl(tag?: string, props?: Record<string, unknown>): HTMLElement
}

/** 全局注册一次「下一集」按钮组件(文字按钮, 不依赖 icon font)。 */
function registerNextEpisodeButton(): boolean {
  const vjs = videojs as unknown as {
    getComponent: (name: string) => VjsButtonCtor | undefined
    registerComponent: (name: string, comp: unknown) => void
  }
  if (vjs.getComponent('NextEpisodeButton')) return true
  const Button = vjs.getComponent('Button')
  if (!Button) return false
  class NextEpisodeButton extends Button implements VjsButtonLike {
    override createEl(tag = 'button', props: Record<string, unknown> = {}): HTMLElement {
      const el = super.createEl(tag, {
        ...props,
        className: 'vjs-control vjs-next-episode-button'
      }) as HTMLElement
      el.setAttribute('title', '下一集')
      el.setAttribute('aria-label', '下一集')
      const label = document.createElement('span')
      label.className = 'vjs-next-episode-button__label'
      label.textContent = '下一集'
      el.appendChild(label)
      return el
    }
    handleClick(): void {
      nextEpisodeHandler?.()
    }
  }
  vjs.registerComponent('NextEpisodeButton', NextEpisodeButton)
  return true
}

export function usePlayer(opts: UsePlayerOptions): UsePlayerReturn {
  const player = shallowRef<Player | null>(null)
  const ready = ref(false)
  const currentTime = ref(0)
  const duration = ref(0)
  const paused = ref(true)
  /** 是否正在缓冲(点击播放后到首帧可播之间的等待态) */
  const buffering = ref(false)
  const errorMessage = ref('')

  const errored = computed(() => errorMessage.value.length > 0)

  /** 待 ready 后再注册的事件队列 */
  const pendingListeners: Array<{
    event: PlayerEventName
    fn: (...args: unknown[]) => void
  }> = []

  function init(el: HTMLVideoElement): void {
    if (player.value) {
      return
    }
    const initialSrc = unwrap(opts.src)
    const initialType = unwrap(opts.type)
    const initialPoster = unwrap(opts.poster)

    // VHS (video.js 内置 HLS 引擎) 弱网友好默认.
    // 低带宽预设 → 初始码率从低开始, 缓冲目标缩短, 二次播放复用上次带宽估算.
    const lowBw = opts.lowBandwidth ?? false
    const vhsOpts: Record<string, unknown> = {
      // 复用上一次会话估算的带宽 (localStorage), 二次播放不必从默认 4Mbps 重新摸索
      useBandwidthFromLocalStorage: true,
      // 复用上一次播放的设备像素比 / 视口大小判断, ABR 决策更快
      useDevicePixelRatio: true,
      // 优先使用 navigator.connection.downlink (如可用), 比纯带宽估算反应快
      useNetworkInformationApi: true,
      // 基于缓冲长度做 ABR (而非带宽估算). 弱网抖动下避免反复升降级
      experimentalBufferBasedABR: lowBw,
      // 弱网初始码率猜测: 800 kbps; 默认是 4 Mbps, 弱网下会立即选择高码率然后卡顿
      bandwidth: lowBw ? 800_000 : undefined,
      // 限制 m3u8 重试次数, 失败快速冒泡给我们的 error 重试逻辑
      maxPlaylistRetries: 2
    }

    // 控制条: 常规按钮 + 可选「下一集」(I-018, 插在倍速与全屏之间)
    const controlBarChildren: string[] = [
      'playToggle',
      'volumePanel',
      'currentTimeDisplay',
      'timeDivider',
      'durationDisplay',
      'progressControl',
      'liveDisplay',
      'remainingTimeDisplay',
      'playbackRateMenuButton'
    ]
    let hasNextBtn = false
    if (opts.nextEpisode) {
      hasNextBtn = registerNextEpisodeButton()
      if (hasNextBtn) {
        nextEpisodeHandler = opts.nextEpisode.onClick
        controlBarChildren.push('NextEpisodeButton')
      }
    }
    controlBarChildren.push('fullscreenToggle')

    // I-019: 倍速本地缓存 —— 记住用户选的倍速, 切源/切集/刷新后自动恢复
    const rateOptions = opts.playbackRates ?? [0.5, 1.0, 1.25, 1.5, 2.0]
    const savedRate = readSavedPlaybackRate(rateOptions)

    const p = videojs(el, {
      controls: true,
      preload: opts.preload ?? 'metadata',
      autoplay: opts.autoplay ?? false,
      loop: opts.loop ?? false,
      playbackRates: rateOptions,
      poster: initialPoster,
      // 移动 Safari 默认全屏播放, playsinline 让它内嵌在页面里播
      playsinline: true,
      sources: initialSrc
        ? [{ src: initialSrc, type: initialType || guessType(initialSrc) }]
        : [],
      html5: {
        vhs: vhsOpts
      },
      controlBar: {
        children: controlBarChildren
      }
    })

    // 弱网模式: 缩短目标缓冲, 起播更快, 也减少弱网下抢带宽
    if (lowBw) {
      try {
        // 这是 VHS 的全局静态常量, 改完对所有 player 生效
        const Vhs = (videojs as unknown as { Vhs?: { GOAL_BUFFER_LENGTH?: number; MAX_GOAL_BUFFER_LENGTH?: number } }).Vhs
        if (Vhs) {
          Vhs.GOAL_BUFFER_LENGTH = 15 // 默认 30
          Vhs.MAX_GOAL_BUFFER_LENGTH = 30 // 默认 60
        }
      } catch {
        /* VHS 版本变化时静默跳过, 不影响播放 */
      }
    }

    p.on('ready', () => {
      ready.value = true
      try {
        if (typeof opts.volume === 'number') {
          p.volume(clamp01(opts.volume))
        }
      } catch {
        // ignore
      }
      // I-019: ready 时恢复上次倍速
      if (savedRate !== null) {
        try {
          p.playbackRate(savedRate)
        } catch {
          // ignore
        }
      }
      // 监听倍速变化: 用户主动选非默认倍速时持久化(见 ratechange 内注释)
      p.on('ratechange', () => {
        // 手势临时倍速(长按2x)不持久化, 退出手势即恢复
        if (tempRateActive) return
        try {
          const r = p.playbackRate()
          // 切源/换 tech 时 video.js 会把倍速重置回 1 并触发 ratechange,
          // 若无差别保存会把缓存覆盖回 1; 1 即默认值无需记忆, 故只记非 1 倍速。
          if (typeof r === 'number' && r !== 1 && rateOptions.includes(r)) {
            savePlaybackRate(r)
          }
        } catch {
          // ignore
        }
      })
      // ready 时同步一次 paused，避免 autoplay 早于 'play' 事件挂载导致 UI 反向
      try {
        const isPaused = p.paused()
        if (typeof isPaused === 'boolean') {
          paused.value = isPaused
        }
      } catch {
        // ignore
      }
      // 注册等待中的监听
      for (const { event, fn } of pendingListeners) {
        p.on(event, fn as (...args: unknown[]) => void)
      }
      pendingListeners.length = 0
    })

    p.on('play', () => {
      paused.value = false
    })
    p.on('pause', () => {
      paused.value = true
    })
    p.on('timeupdate', () => {
      const t = p.currentTime()
      if (typeof t === 'number') {
        currentTime.value = t
      }
    })
    p.on('loadedmetadata', () => {
      const d = p.duration()
      if (typeof d === 'number' && Number.isFinite(d)) {
        duration.value = d
      }
      // I-019: 切源/切集后新 media tech 可能重置倍速, 就绪时重新套用缓存
      if (savedRate !== null) {
        try {
          const cur = p.playbackRate()
          if (typeof cur === 'number' && Math.abs(cur - savedRate) > 1e-6) {
            p.playbackRate(savedRate)
          }
        } catch {
          // ignore
        }
      }
    })
    p.on('error', () => {
      const err = p.error()
      errorMessage.value = err?.message || 'video error'
    })
    p.on('canplay', () => {
      errorMessage.value = ''
      buffering.value = false
    })
    p.on('waiting', () => {
      // 缓冲中: 显示 loading, 让用户知道点击播放后有响应
      buffering.value = true
    })
    p.on('playing', () => {
      buffering.value = false
    })

    player.value = p

    // 响应式 src / poster 切换
    if (typeof opts.src === 'object' && 'value' in (opts.src as object)) {
      watch(
        () => unwrap(opts.src),
        (next) => {
          if (!next || !player.value) {
            return
          }
          errorMessage.value = ''
          const t = unwrap(opts.type) || guessType(next)
          player.value.src({ src: next, type: t })
          // 默认重新尝试播放
          if (opts.autoplay) {
            void player.value.play()
          }
        }
      )
    }
    if (opts.poster && typeof opts.poster === 'object' && 'value' in (opts.poster as object)) {
      watch(
        () => unwrap(opts.poster),
        (next) => {
          if (player.value && next) {
            player.value.poster(next)
          }
        }
      )
    }
  }

  function dispose(): void {
    const p = player.value
    if (!p) {
      return
    }
    try {
      p.dispose()
    } catch {
      // ignore
    }
    player.value = null
    ready.value = false
    nextEpisodeHandler = null
    pendingListeners.length = 0
  }

  function play(): Promise<void> | void {
    const p = player.value
    if (!p) {
      return
    }
    const ret = p.play()
    if (ret && typeof (ret as Promise<void>).then === 'function') {
      // 静默吞掉浏览器自动播放策略导致的 reject
      return (ret as Promise<void>).catch(() => undefined)
    }
  }

  function pause(): void {
    player.value?.pause()
  }

  function seek(seconds: number): void {
    const p = player.value
    if (!p) {
      return
    }
    const d = p.duration() || 0
    const target = Math.max(0, Math.min(seconds, d > 0 ? d : seconds))
    p.currentTime(target)
  }

  function seekBy(delta: number): void {
    const p = player.value
    if (!p) {
      return
    }
    const cur = p.currentTime() || 0
    seek(cur + delta)
  }

  function setVolume(v: number): void {
    const p = player.value
    if (!p) {
      return
    }
    p.volume(clamp01(v))
  }

  function on(event: PlayerEventName, fn: (...args: unknown[]) => void): () => void {
    const p = player.value
    if (p && ready.value) {
      p.on(event, fn as (...a: unknown[]) => void)
    } else {
      pendingListeners.push({ event, fn })
    }
    return () => off(event, fn)
  }

  function off(event: PlayerEventName, fn: (...args: unknown[]) => void): void {
    const p = player.value
    if (p) {
      p.off(event, fn as (...a: unknown[]) => void)
    }
    const idx = pendingListeners.findIndex((x) => x.event === event && x.fn === fn)
    if (idx >= 0) {
      pendingListeners.splice(idx, 1)
    }
  }

  onScopeDispose(() => {
    dispose()
  })

  /** ---- 触屏手势: 临时倍速(长按 2x)。倍速变化不写持久化缓存, 退出即恢复 ---- */
  let tempRateActive = false
  let tempRateBefore = 1
  function enterTempRate(rate: number): void {
    const p = player.value
    if (!p) return
    try {
      const cur = p.playbackRate()
      tempRateBefore = typeof cur === 'number' ? cur : 1
      tempRateActive = true
      p.playbackRate(rate)
    } catch {
      tempRateActive = false
    }
  }
  function exitTempRate(): void {
    const p = player.value
    if (!p || !tempRateActive) return
    tempRateActive = false
    try {
      p.playbackRate(tempRateBefore)
    } catch {
      // ignore
    }
  }

  return {
    player,
    ready,
    currentTime,
    duration,
    paused,
    buffering,
    errored,
    init,
    dispose,
    play,
    pause,
    seek,
    seekBy,
    setVolume,
    enterTempRate,
    exitTempRate,
    on,
    off
  }
}

/** 简单根据 url 后缀猜测 mime */
function guessType(url: string): string {
  const u = url.toLowerCase()
  if (u.includes('.m3u8')) {
    return 'application/x-mpegURL'
  }
  if (u.includes('.mpd')) {
    return 'application/dash+xml'
  }
  if (u.includes('.flv')) {
    return 'video/x-flv'
  }
  if (u.includes('.webm')) {
    return 'video/webm'
  }
  return 'video/mp4'
}

function clamp01(n: number): number {
  if (Number.isNaN(n)) {
    return 0
  }
  return Math.max(0, Math.min(1, n))
}

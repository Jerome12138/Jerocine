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
import Artplayer from 'artplayer'
import Hls from 'hls.js'

/**
 * ArtPlayer + hls.js 播放器 composable（2026-10-30 由 video.js/VHS 迁移）。
 *
 * 设计要点：
 * - 实例存 shallowRef，避免被 Vue 反应式深包裹（播放器内部状态多，深包裹会爆栈/性能问题）
 * - 切换 src 不重建 player：blob/直链统一走 load()（art.type + art.url），customType 按
 *   type 分发到 hls.js 加载；mp4 等直链由 ArtPlayer 直接 $video.src
 * - 暴露 play / pause / seek / setVolume / on / off / dispose 等命令式 API（与旧 video.js 版
 *   接口一致，调用方 PlayView/useDanmaku/usePlayerGestures 只换底层实现不换契约）
 * - 不在内部触发 router 跳转 / toast，由调用方决定
 *
 * 使用：
 *   const containerEl = ref<HTMLDivElement | null>(null)
 *   const { player, ready, init, dispose, play, pause, seek, setVolume, on } = usePlayer({...})
 *   onMounted(() => init(containerEl.value!))
 */

export interface UsePlayerOptions {
  /** 播放源 url（响应式） */
  src: Ref<string> | string
  /** 视频 mime 类型，例 'application/x-mpegURL'（blob 清单无扩展名，靠它判定走 hls.js） */
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
   * 直播模式: ArtPlayer isLive（隐藏进度条/时长, 显示 LIVE 标识）,
   * hls.js 按直播清单自驱跟随。直播页专用。
   */
  isLive?: boolean
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
   * 提供后会在控制条右侧注册一个常驻文字按钮, 点击触发 onClick。
   * 是否真的存在下一集由回调自行判断(如 PlayView.playNext 内部已 guard hasNext)。
   */
  nextEpisode?: { onClick: () => void }
  /**
   * 控制条「弹幕设置」按钮(可选):
   * 提供后注册一个常驻图标按钮, 点击触发 onClick(PlayView 打开弹幕设置弹窗)。
   * 按钮常驻显示——设置项(字号/透明度等)独立于当前片源是否可用。
   */
  danmakuSettings?: { onClick: () => void }
  /**
   * 控制条「弹幕开关」按钮(可选):
   * 提供后注册一个常驻图标按钮, 点击触发 onClick 切换弹幕显隐。
   * 按钮开启态(is-on 着色)由调用方经 syncDanmakuBtnState 类机制外部同步
   * (ArtPlayer 控件是静态 DOM, 状态类不进响应式)。
   */
  danmakuToggle?: { onClick: () => void }
  /**
   * 弱网模式. 开启后(hls.js 配置):
   *  - 初始带宽估算 800kbps (默认 2Mbps), ABR 从低码率起步
   *  - 目标缓冲 25s (默认 40s), 起播更快
   *  - 复用 localStorage 上次的带宽估算(hls.bandwidthEstimate), 二次播放更聪明
   */
  lowBandwidth?: boolean
}

export type PlayerEventName =
  | 'ready'
  | 'play'
  | 'pause'
  | 'ended'
  | 'timeupdate'
  | 'seeked'
  | 'error'
  | 'loadedmetadata'
  | 'volumechange'
  | 'waiting'
  | 'canplay'
  | 'fullscreenchange'
  | 'useractive'
  | 'userinactive'

export interface UsePlayerReturn {
  /** ArtPlayer 实例（ready 之前为 null） */
  player: ShallowRef<Artplayer | null>
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
  /** 视频实际分辨率(像素宽高; loadedmetadata/码率切换后更新, 未就绪为 null) */
  videoSize: Ref<{ width: number; height: number } | null>
  /** 是否处于错误态 */
  errored: ComputedRef<boolean>
  /** 创建并挂载到目标容器元素 */
  init: (el: HTMLElement) => void
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
  /** 装载新源(不重建实例): m3u8/blob → hls.js, 其它直链 → 原生 video.src */
  load: (url: string, type?: string) => void
  /** 进入临时倍速(手势长按等场景): 直接生效, 不写持久化缓存 */
  enterTempRate: (rate: number) => void
  /** 退出临时倍速, 恢复进入前档位 */
  exitTempRate: () => void
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

const PLAYBACK_RATE_LS_KEY = 'jc-playback-rate'

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

/* ============ hls.js 带宽估算持久化(等价 VHS useBandwidthFromLocalStorage) ============ */

const HLS_BW_LS_KEY = 'jc-hls-bandwidth-estimate'

function readSavedBandwidth(): number | null {
  try {
    const raw = localStorage.getItem(HLS_BW_LS_KEY)
    if (!raw) return null
    const v = Number(raw)
    // 5kbps ~ 100Mbps 的合理区间外视为脏数据丢弃
    return Number.isFinite(v) && v >= 5_000 && v <= 100_000_000 ? v : null
  } catch {
    return null
  }
}

function saveBandwidth(v: number): void {
  try {
    if (Number.isFinite(v) && v > 0) localStorage.setItem(HLS_BW_LS_KEY, String(Math.round(v)))
  } catch {
    /* 隐私模式忽略 */
  }
}

/* ============ 播放器右上角控制条自定义按钮 ============ */

/** 当前实例的点击回调(模块级单例: 一页一个 player, init 时注入, dispose 时清空)。 */
let nextEpisodeHandler: (() => void) | null = null
/** 弹幕设置按钮回调（同 nextEpisodeHandler 单例模式） */
let danmakuSettingsHandler: (() => void) | null = null
/** 弹幕开关按钮回调（同上） */
let danmakuToggleHandler: (() => void) | null = null

/** 弹幕设置图标 SVG path（Material "tune": 调节滑块, 设置语义比气泡更明确, 2026-10-26 更换） */
const DANMAKU_ICON_PATH =
  'M3 17v2h6v-2H3zM3 5v2h10V5H3zm10 16v-2h8v-2h-8v-2h-2v6h2zM7 9v2H3v2h4v2h2V9H7zm14 4v-2H11v2h10zm-6-4h2V7h4V5h-4V3h-2v6z'
/** 弹幕开关图标 SVG path（气泡 + 弹幕行, Material "chat" 变体） */
const DANMAKU_TOGGLE_ICON_PATH =
  'M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zM6 9h12v2H6V9zm8 5H6v-2h8v2zm4-6H6V6h12v2z'

/** 创建「下一集」按钮内容节点(外层 .art-control 容器由 ArtPlayer 自建)。 */
function buildNextEpisodeButton(): HTMLElement {
  const label = document.createElement('span')
  label.className = 'art-control-next-episode__label'
  label.textContent = '下一集'
  return label
}

/** 创建「弹幕设置」图标按钮内容节点(内联 SVG, 不依赖 icon font)。 */
function buildDanmakuSettingsButton(): HTMLElement {
  return buildIconSvg(DANMAKU_ICON_PATH)
}

/** 创建「弹幕开关」图标按钮内容节点(气泡弹幕行图标)。 */
function buildDanmakuToggleButton(): HTMLElement {
  return buildIconSvg(DANMAKU_TOGGLE_ICON_PATH)
}

/** 20px 内联 SVG 图标节点(填充 currentColor)。 */
function buildIconSvg(pathD: string): HTMLElement {
  const svgNS = 'http://www.w3.org/2000/svg'
  const svg = document.createElementNS(svgNS, 'svg')
  svg.setAttribute('viewBox', '0 0 24 24')
  svg.setAttribute('fill', 'currentColor')
  svg.setAttribute('width', '20')
  svg.setAttribute('height', '20')
  svg.setAttribute('aria-hidden', 'true')
  const path = document.createElementNS(svgNS, 'path')
  path.setAttribute('d', pathD)
  svg.appendChild(path)
  return svg as unknown as HTMLElement
}

/** 模块级一次性开关: ArtPlayer 全局行为收敛(右键菜单/版本日志/内建重连与旧 video.js 行为对齐)。 */
let globalTuned = false
function tuneGlobals(): void {
  if (globalTuned) return
  globalTuned = true
  Artplayer.LOG_VERSION = false
  Artplayer.CONTEXTMENU = false
  // 关闭 ArtPlayer 内建 video:error 自动重连(1s×5 次): 恢复权完全交给上层
  // PlayView 的重试/换源链路; 且 hls.js 路径不写 video.src, 内建重连的 option.url
  // 对 blob 源是陈旧值, 双轨重连会互相打架
  Artplayer.RECONNECT_TIME_MAX = 0
}

/* ============ 对外事件名 → ArtPlayer 事件名映射 ============
 * ArtPlayer 对 <video> 原生事件只发 `video:` 前缀版(从不发裸名),
 * 全屏是 'fullscreen'(非 'fullscreenchange'), 控制条显隐是 'control'(state 参数)。 */
function mapArtEvent(event: PlayerEventName): string | null {
  switch (event) {
    case 'ready':
      return 'ready'
    case 'play':
    case 'pause':
    case 'ended':
    case 'timeupdate':
    case 'seeked':
    case 'loadedmetadata':
    case 'volumechange':
    case 'waiting':
    case 'canplay':
      return `video:${event}`
    case 'fullscreenchange':
      return 'fullscreen'
    case 'useractive':
    case 'userinactive':
      return 'control'
    case 'error':
      return null // 手动分发(见 fireErrorListeners), 不挂实例
  }
}

/** 外部事件订阅条目: fn 为调用方函数, wrapped 为实际挂到 art 的(可能包装的)函数,
 *  供 off() 对称解绑。wrapped 用 never[] 参数以同时兼容直通 fn 与 'control' 状态包装。 */
interface PlayerListener {
  event: PlayerEventName
  fn: (...args: unknown[]) => void
  wrapped?: (...args: never[]) => void
}

export function usePlayer(opts: UsePlayerOptions): UsePlayerReturn {
  tuneGlobals()

  const player = shallowRef<Artplayer | null>(null)
  const ready = ref(false)
  const currentTime = ref(0)
  const duration = ref(0)
  const paused = ref(true)
  /** 是否正在缓冲(点击播放后到首帧可播之间的等待态) */
  const buffering = ref(false)
  const errorMessage = ref('')

  const errored = computed(() => errorMessage.value.length > 0)

  /** 视频实际分辨率(像素宽高)。loadedmetadata 拿初始值; hls.js 码率切换(LEVEL_SWITCHED)
   *  与窗口 resize 后重读, 实时反映当前画质。读取失败防御性跳过(null 直到可读)。 */
  const videoSize = ref<{ width: number; height: number } | null>(null)

  /** 外部事件订阅(持久): player 重建(dispose→init)后统一重挂。
   *  曾用一次性 pendingListeners + dispose 清空 —— 旧 player 销毁后弹幕层等外部
   *  订阅全部蒸发且不再重挂(线上实测: timeupdate 链断, 弹幕静止, 2026-10-26)。 */
  const listenerRegistry: PlayerListener[] = []

  /** error 特殊分发: hls 致命错误不经过 video 元素 error 事件, 统一由此触发订阅者。
   *  1s 去抖: hls fatal destroy 后媒体层常再补一个 video:error, 双触发会把上层
   *  重试计数双扣、重复排重试任务。 */
  let lastErrorDispatchAt = 0
  function fireErrorListeners(): void {
    const now = Date.now()
    if (now - lastErrorDispatchAt < 1000) return
    lastErrorDispatchAt = now
    for (const { event, fn } of listenerRegistry) {
      if (event === 'error') fn()
    }
  }

  /** 单条注册 → ArtPlayer 事件挂载。error 不直挂(手动分发, 见 fireErrorListeners);
   *  useractive/userinactive 由 art 'control'(控制条显隐, state 参数)派生, 包装闭包
   *  存回 entry 供 off() 对称解绑。 */
  function attachEntryToArt(art: Artplayer, entry: PlayerListener): void {
    const { event, fn } = entry
    const artEvent = mapArtEvent(event)
    if (artEvent === null) return
    if (event === 'useractive') {
      entry.wrapped = (state: boolean) => {
        if (state) fn()
      }
    } else if (event === 'userinactive') {
      entry.wrapped = (state: boolean) => {
        if (!state) fn()
      }
    } else {
      entry.wrapped = fn
    }
    art.on(artEvent as Parameters<Artplayer['on']>[0], entry.wrapped as never)
  }

  /** 注册表 → ArtPlayer 事件挂载(构造后立即全量挂, 不等 ready —— 否则首个源的
   *  loadedmetadata 会在挂载前错过, 首播续播 seek 丢失)。 */
  function attachRegistryToArt(art: Artplayer): void {
    for (const entry of listenerRegistry) {
      attachEntryToArt(art, entry)
    }
  }

  /** hls 实例的 art 'destroy' 清理回调(换源时先摘旧的再挂新的, 防监听堆积)。 */
  let hlsDestroyHandler: (() => void) | null = null

  /**
   * hls.js 挂载(ArtPlayer customType 入口)。
   * MSE 可用 → hls.js 接管(端侧 blob 清单唯一可播路径); 否则退化原生 HLS
   * (iPhone Safari, blob 清单播不了, 上层 PlayView 已按 isHlsSupported() 决定喂直链)。
   */
  function attachHls(this: Artplayer, video: HTMLVideoElement, url: string): void {
    const art = this
    // 换源前保存上一条 hls 会话的带宽估算, 供下次起播直接复用
    const prevHls = art.hls as Hls | undefined
    if (prevHls) {
      saveBandwidth(prevHls.bandwidthEstimate)
      try {
        prevHls.destroy()
      } catch {
        /* ignore */
      }
    }
    if (hlsDestroyHandler) {
      try {
        art.off('destroy', hlsDestroyHandler)
      } catch {
        /* ignore */
      }
      hlsDestroyHandler = null
    }

    if (Hls.isSupported()) {
      const lowBw = opts.lowBandwidth ?? false
      const hls = new Hls({
        enableWorker: true,
        // 抗卡顿缓冲(对齐旧 VHS GOAL_BUFFER_LENGTH 40s/上限 80s; 弱网 25s/50s):
        // 实测源站 TTFB 0.4-0.9s/分片, 更深缓冲显著减少 rebuffer
        maxBufferLength: lowBw ? 25 : 40,
        maxMaxBufferLength: lowBw ? 50 : 80,
        backBufferLength: 60,
        // 初始带宽估算: 弱网 800k 起播选低码率; 正常网 2M — 实测源站 1080p 变体约
        // 800k~2M, 估算偏高会"起播即选高码率→带宽不足→立刻降级", 起播反糊
        abrEwmaDefaultEstimate: readSavedBandwidth() ?? (lowBw ? 800_000 : 2_000_000),
        startLevel: -1
      })

      // 致命错误自愈(先于 loadSource 注册): 网络错重试(上限 3 次, FRAG_BUFFERED 复位),
      // 媒体错 recoverMediaError(上限 2 次, 第二次轮换音频编解码), 超限销毁并冒泡
      // error 走上层重试/换源链路
      let netRetries = 0
      let mediaRetries = 0
      hls.on(Hls.Events.ERROR, (_evt, data) => {
        if (!data.fatal) return
        if (data.type === Hls.ErrorTypes.NETWORK_ERROR && netRetries < 3) {
          netRetries += 1
          hls.startLoad()
          return
        }
        if (data.type === Hls.ErrorTypes.MEDIA_ERROR && mediaRetries < 2) {
          mediaRetries += 1
          try {
            if (mediaRetries === 2) {
              hls.swapAudioCodec()
            }
            hls.recoverMediaError()
            return
          } catch {
            /* 落到销毁 */
          }
        }
        errorMessage.value = data.details || 'hls fatal error'
        try {
          hls.destroy()
        } catch {
          /* ignore */
        }
        fireErrorListeners()
      })
      hls.on(Hls.Events.FRAG_BUFFERED, () => {
        netRetries = 0
      })
      // 码率切换后分辨率可能变化 → 重读宽高(等价旧 VHS 的 video resize 监听)
      hls.on(Hls.Events.LEVEL_SWITCHED, () => readVideoSize(art))

      hls.loadSource(url)
      hls.attachMedia(video)

      art.hls = hls
      hlsDestroyHandler = () => {
        saveBandwidth(hls.bandwidthEstimate)
        try {
          hls.destroy()
        } catch {
          /* ignore */
        }
      }
      art.on('destroy', hlsDestroyHandler)
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      // 原生 HLS(iOS Safari): blob 清单不可播, 喂进来的只会是直链(见 PlayView isHlsSupported 门)
      video.src = url
    } else {
      art.notice.show = '当前浏览器不支持 HLS 播放'
    }
  }

  function readVideoSize(art: Artplayer | null): void {
    const v = art?.video
    if (!v) return
    try {
      const w = v.videoWidth
      const h = v.videoHeight
      if (typeof w === 'number' && typeof h === 'number' && w > 0 && h > 0) {
        videoSize.value = { width: w, height: h }
      }
    } catch {
      // ignore
    }
  }

  /** url 是否按 HLS 装载(端侧 blob 清单无扩展名, 由调用方 mime 或扩展名共同判定)。 */
  function isHlsUrl(url: string, type?: string): boolean {
    if (type) return /mpegurl/i.test(type)
    return /\.m3u8(\?|#|$)/i.test(url)
  }

  /** 装载新源(不重建实例)。blob: 清单无扩展名, 必须显式 art.type 才能命中 customType。 */
  function loadInternal(art: Artplayer, url: string, type?: string): void {
    if (!url) return
    errorMessage.value = ''
    if (isHlsUrl(url, type)) {
      art.type = 'm3u8'
    } else {
      art.type = ''
      // 非 HLS 源: 销毁残留的 hls 会话(否则旧实例带着已关闭的 MediaSource 空转到下次 m3u8)
      const staleHls = art.hls as Hls | undefined
      if (staleHls) {
        try {
          staleHls.destroy()
        } catch {
          /* ignore */
        }
        art.hls = undefined
      }
    }
    art.url = url
  }

  function init(el: HTMLElement): void {
    if (player.value) {
      return
    }
    const initialSrc = unwrap(opts.src)
    const initialType = unwrap(opts.type)
    const initialPoster = unwrap(opts.poster)

    // I-019: 倍速本地缓存 —— 记住用户选的倍速, 切源/切集/刷新后自动恢复
    const rateOptions = opts.playbackRates ?? [0.5, 1.0, 1.25, 1.5, 2.0]
    const savedRate = readSavedPlaybackRate(rateOptions)
    // 倍速面板档位交给调用方配置(不随 ArtPlayer 静态默认漂移), 保证与持久化校验一致
    Artplayer.PLAYBACK_RATE = rateOptions.slice()

    // 控制条自定义按钮: 「弹幕开关」+「弹幕设置」(倍速区左侧) + 「下一集」(全屏按钮之前)
    // (ArtPlayer 自建 .art-control 容器, html 只放内容节点; 点击回调绑在容器上)
    const customControls: Artplayer['option']['controls'] = []
    if (opts.danmakuToggle) {
      danmakuToggleHandler = opts.danmakuToggle.onClick
      customControls.push({
        name: 'danmaku-toggle',
        position: 'right',
        index: 10,
        tooltip: '弹幕开关',
        html: buildDanmakuToggleButton(),
        click: () => danmakuToggleHandler?.()
      })
    }
    if (opts.danmakuSettings) {
      danmakuSettingsHandler = opts.danmakuSettings.onClick
      customControls.push({
        name: 'danmaku-settings',
        position: 'right',
        index: 15,
        tooltip: '弹幕设置',
        html: buildDanmakuSettingsButton(),
        click: () => danmakuSettingsHandler?.()
      })
    }
    if (opts.nextEpisode) {
      nextEpisodeHandler = opts.nextEpisode.onClick
      customControls.push({
        name: 'next-episode',
        position: 'right',
        index: 18,
        tooltip: '下一集',
        html: buildNextEpisodeButton(),
        click: () => nextEpisodeHandler?.()
      })
    }

    const art = new Artplayer({
      container: el as HTMLDivElement,
      url: initialSrc,
      poster: initialPoster,
      // 初始 mime 已知且为 HLS 时直接声明 type, 首个 blob 清单才能命中 customType
      type: initialSrc && isHlsUrl(initialSrc, initialType) ? 'm3u8' : '',
      customType: {
        m3u8: attachHls
      },
      autoplay: opts.autoplay ?? false,
      volume: typeof opts.volume === 'number' ? clamp01(opts.volume) : 0.6,
      muted: false,
      loop: opts.loop ?? false,
      // 直播模式: 隐藏进度/时长, hls.js 直播跟随
      isLive: opts.isLive ?? false,
      // 移动 Safari 默认全屏播放, playsinline 让它内嵌在页面里播
      playsInline: true,
      moreVideoAttr: {
        // 弱网友好: 仅拉时长 + 起播分片元数据, 不整段预载(hls.js 分片节奏由 maxBufferLength 管)
        preload: opts.preload ?? 'metadata'
      },
      // 键盘/移动端横滑手势全部关闭: 播放控制由自家 usePlayerGestures + handleKeydown
      // 统一约定(单击播停/双击全屏/长按 3x/渐进刮擦), 避免双重响应
      hotkey: false,
      gesture: false,
      fastForward: false,
      mutex: true,
      backdrop: true,
      // 直播模式不带倍速/画面比例（点播语义, 直播改倍速会造成缓冲漂移）
      setting: !(opts.isLive ?? false),
      playbackRate: !(opts.isLive ?? false),
      aspectRatio: !(opts.isLive ?? false),
      fullscreen: true,
      fullscreenWeb: true,
      pip: true,
      miniProgressBar: true,
      controls: customControls
    })

    // 外部订阅构造后立即挂载(不等 ready —— art 的 ready 在首个源 canplay 才触发,
    // 首播的 loadedmetadata 若等 ready 再挂会错过, 首次进页续播 seek 会丢)
    attachRegistryToArt(art)
    suppressProgrammaticNotices(art)

    player.value = art

    art.on('ready', () => {
      ready.value = true
      // I-019: ready 时恢复上次倍速
      if (savedRate !== null) {
        try {
          art.playbackRate = savedRate
        } catch {
          // ignore
        }
      }
      // ready 时同步一次 paused，避免 autoplay 早于 'play' 事件挂载导致 UI 反向
      try {
        paused.value = art.video.paused
      } catch {
        // ignore
      }
    })

    art.on('video:play', () => {
      paused.value = false
    })
    art.on('video:pause', () => {
      paused.value = true
    })
    art.on('video:timeupdate', () => {
      currentTime.value = art.currentTime
    })
    art.on('video:loadedmetadata', () => {
      const d = art.duration
      if (Number.isFinite(d)) {
        duration.value = d
      }
      readVideoSize(art)
      // I-019: 切源/切集后新源可能重置倍速, 就绪时重新套用缓存
      if (savedRate !== null) {
        try {
          if (Math.abs(art.playbackRate - savedRate) > 1e-6) {
            art.playbackRate = savedRate
          }
        } catch {
          // ignore
        }
      }
    })
    art.on('video:durationchange', () => {
      const d = art.duration
      if (Number.isFinite(d) && d > 0) {
        duration.value = d
      }
    })
    art.on('video:error', () => {
      // video:error 收到的是 DOM Event(message 恒 undefined), 真实媒体错误读 video.error
      const mediaErr = art.video?.error
      errorMessage.value = mediaErr?.message || 'video error'
      fireErrorListeners()
    })
    art.on('video:canplay', () => {
      errorMessage.value = ''
      buffering.value = false
    })
    art.on('video:waiting', () => {
      // 缓冲中: 显示 loading, 让用户知道点击播放后有响应
      buffering.value = true
    })
    art.on('video:playing', () => {
      buffering.value = false
    })
    // 窗口/容器尺寸变化后重读分辨率
    art.on('resize', () => readVideoSize(art))
    // 倍速持久化: 用户主动选非默认倍速时保存(手势临时倍速不落盘)
    art.on('video:ratechange', () => {
      if (tempRateActive) return
      try {
        const r = art.playbackRate
        // 切源/换流时底层会把倍速重置回 1 并触发 ratechange,
        // 若无差别保存会把缓存覆盖回 1; 1 即默认值无需记忆, 故只记非 1 倍速。
        if (typeof r === 'number' && r !== 1 && rateOptions.includes(r)) {
          savePlaybackRate(r)
        }
      } catch {
        // ignore
      }
    })

    // 响应式 src / poster 切换
    if (typeof opts.src === 'object' && 'value' in (opts.src as object)) {
      watch(
        () => unwrap(opts.src),
        (next) => {
          if (!next || !player.value) {
            return
          }
          loadInternal(player.value, next, unwrap(opts.type))
          // 默认重新尝试播放
          if (opts.autoplay) {
            // 切源/切集时旧 play() promise 可能已被 pause() 打断 → reject(浏览器标准行为),
            // 不 catch 会产生 unhandledrejection 并被埋点当成 js-error 上报。静默吞掉。
            const ret = player.value.play() as Promise<void> | undefined
            void ret?.catch(() => undefined)
          }
        }
      )
    }
    if (opts.poster && typeof opts.poster === 'object' && 'value' in (opts.poster as object)) {
      watch(
        () => unwrap(opts.poster),
        (next) => {
          if (player.value && next) {
            player.value.poster = next
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
      p.destroy(true)
    } catch {
      // ignore
    }
    player.value = null
    ready.value = false
    currentTime.value = 0
    duration.value = 0
    paused.value = true
    buffering.value = false
    videoSize.value = null
    errorMessage.value = ''
    nextEpisodeHandler = null
    danmakuSettingsHandler = null
    danmakuToggleHandler = null
    // listenerRegistry 保留: 外部订阅随 composable 生命周期, 重建后由 init 全量重挂
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
    const d = p.duration || 0
    // ArtPlayer 的 currentTime setter 会 clamp 到 [0, duration], duration 未就绪时
    // 任何 seek 都会被钳成 0(等于重置播放) —— 元数据到达前直接忽略
    if (d <= 0) {
      return
    }
    const target = Math.max(0, Math.min(seconds, d))
    p.currentTime = target
  }

  function seekBy(delta: number): void {
    const p = player.value
    if (!p) {
      return
    }
    const cur = p.currentTime || 0
    seek(cur + delta)
  }

  function setVolume(v: number): void {
    const p = player.value
    if (!p) {
      return
    }
    p.volume = clamp01(v)
    // ArtPlayer volume setter 自带 notice 提示; 程序化设置(键盘/手势)不打扰, 立即清掉
    try {
      p.notice.show = ''
    } catch {
      /* ignore */
    }
  }

  function load(url: string, type?: string): void {
    const p = player.value
    if (!p) {
      return
    }
    loadInternal(p, url, type)
  }

  function on(event: PlayerEventName, fn: (...args: unknown[]) => void): () => void {
    const entry: PlayerListener = { event, fn: fn as (...a: unknown[]) => void }
    listenerRegistry.push(entry)
    const p = player.value
    if (p) {
      attachEntryToArt(p, entry)
    }
    return () => off(event, fn)
  }

  function off(event: PlayerEventName, fn: (...args: unknown[]) => void): void {
    const idx = listenerRegistry.findIndex((x) => x.event === event && x.fn === fn)
    if (idx < 0) {
      return
    }
    const [entry] = listenerRegistry.splice(idx, 1)
    // 从实例对称解绑(否则一次性监听变永久监听: 重试/切过滤的 seek-back 会在
    // 后续每次 loadedmetadata 用过期位置重复 seek)
    const p = player.value
    if (p && entry && entry.wrapped) {
      const artEvent = mapArtEvent(event)
      if (artEvent) {
        try {
          p.off(artEvent as Parameters<Artplayer['off']>[0], entry.wrapped as never)
        } catch {
          /* ignore */
        }
      }
    }
  }

  onScopeDispose(() => {
    dispose()
  })

  /** ---- 触屏手势: 临时倍速(长按 3x)。倍速变化不写持久化缓存, 退出即恢复 ---- */
  let tempRateActive = false
  let tempRateBefore = 1
  function enterTempRate(rate: number): void {
    const p = player.value
    if (!p) return
    try {
      const cur = p.playbackRate
      tempRateBefore = typeof cur === 'number' ? cur : 1
      tempRateActive = true
      p.playbackRate = rate
    } catch {
      tempRateActive = false
    }
  }
  function exitTempRate(): void {
    const p = player.value
    if (!p || !tempRateActive) return
    tempRateActive = false
    try {
      p.playbackRate = tempRateBefore
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
    videoSize,
    errored,
    init,
    dispose,
    play,
    pause,
    seek,
    seekBy,
    setVolume,
    load,
    enterTempRate,
    exitTempRate,
    on,
    off
  }
}

/** 程序化 play/pause/倍速的内置 notice(切集自动续播/键盘/手势高频触发)是视觉噪音:
 *  在实例上包装 notice.show setter 过滤这几类英文文案, 其余(音量等用户操作反馈)保留。 */
function suppressProgrammaticNotices(art: Artplayer): void {
  try {
    const notice = art.notice as unknown as { show: string }
    const desc = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(notice), 'show')
    if (!desc?.set || !desc.get) return
    Object.defineProperty(notice, 'show', {
      configurable: true,
      get: desc.get,
      set(this: unknown, value: string) {
        if (typeof value === 'string' && /^(Play|Pause|Rate:)/i.test(value.trim())) {
          return
        }
        desc.set!.call(this, value)
      }
    })
  } catch {
    /* 包装失败仅损失体验, 不影响功能 */
  }
}

/** 当前浏览器是否具备 MSE/HLS.js 播放条件(端侧 blob 清单可播的前提)。 */
export function isHlsSupported(): boolean {
  return Hls.isSupported()
}

function clamp01(n: number): number {
  if (Number.isNaN(n)) {
    return 0
  }
  return Math.max(0, Math.min(1, n))
}

/** 分辨率打标: 按视频实际宽高给出展示画质(行业通用阈值, 与源站标称无直接关系)。 */
export function resolutionLabel(width: number, height: number): string {
  if (width >= 3800 || height >= 2100) return '4K'
  if (width >= 2500 || height >= 1400) return '2K'
  if (width >= 1800 || height >= 950) return '1080P'
  if (width >= 1200 || height >= 680) return '720P'
  if (width >= 800 || height >= 460) return '480P'
  if (height > 0) return `${height}P`
  return '标清'
}

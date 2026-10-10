/**
 * jerocineNative - 封装与 Jerocine TV APK Native Bridge 的通信.
 *
 * 通信约定:
 *   - 调用 native:   const result = JerocineNative.invoke(method, jsonArgsString)
 *                    返回 jsonResultString, parse 后 { ok, ...其他字段 }
 *   - native 事件:   native 内调 window.__JerocineEvents(name, payload)
 *                    本模块在 init() 时挂 window.__JerocineEvents 作为派发器
 *
 * 接口尽量稳定 — 设计原则: web 端用 invoke('xxx', args), 未来 native 只要注册
 * 新 method handler 就能用, 不必重发 APK.
 *
 * 用法:
 *   import { jerocine, isNative } from '@/utils/jerocineNative'
 *   if (isNative()) {
 *     await jerocine.call('toast', { message: 'hi' })
 *     jerocine.on('keyMenu', () => openSettings())
 *   }
 */

interface NativeBridge {
  invoke?: (method: string, jsonArgs: string) => string
  // 兼容旧 v1 接口
  playVideo?: (url: string, title: string) => void
  playPlaylist?: (cfgJson: string) => void
  setAuthToken?: (token: string) => void
  clearAuthToken?: () => void
  checkUpdate?: () => void
  openServerSettings?: () => void
  /** 打开原生设置抽屉(TV 胶囊行"设置"按钮 / 首页"我的"卡, 方案 §6) */
  openSettings?: () => void
  getPlatform?: () => string
}

interface CallResult {
  ok: boolean
  error?: string
  [key: string]: unknown
}

type EventHandler = (payload: unknown) => void

/**
 * 绝对 API base(到 /api)。派发原生播放器时作为 proxyBase 传入, 原生据此拼 /v1/m3u8/filter
 * 做端侧广告过滤。三个派发入口(PlayView / 详情页直跳 / 路由快捷路径)统一用它,
 * 避免某些入口漏传致原生"代理地址未传"、广告过滤不触发。
 */
export function absApiBase(): string {
  const apiBase = (import.meta.env.VITE_API_BASE as string | undefined) || '/api'
  const isAbs = /^https?:\/\//i.test(apiBase)
  return isAbs ? apiBase : (typeof window !== 'undefined' ? window.location.origin : '') + apiBase
}

function getBridge(): NativeBridge | undefined {
  if (typeof window === 'undefined') return undefined
  const w = window as unknown as { JerocineNative?: NativeBridge; JerocinePlayer?: NativeBridge }
  return w.JerocineNative ?? w.JerocinePlayer
}

export function isNative(): boolean {
  const b = getBridge()
  return !!(b && (b.invoke || b.playVideo))
}

/**
 * 壳内**能否真的调起原生设置抽屉** —— 比 isNative() 严一档：还要求 bridge 暴露了调用入口。
 *
 * 用途：顶栏「刷新 / 设置」按钮在**非 TV 模式**下的显隐判断（见
 * `useTvSettingsEntry.shouldShowHeaderActions`）。桌面布局里首页金刚区的「设置」卡不渲染
 * （v-if="isTV"），顶栏那个按钮是用户切回 TV 的唯一入口 —— 但在老 APK 上按钮点了没反应，
 * 等于给了个假入口，所以这里要能区分"桥在但调不动设置"。
 *
 * 两个分支都认的原因：
 *   - 现版 bridge(`JerocineBridge`) 把 openSettings 做成 `invoke('openSettings')` 的
 *     一个 case，**没有**单独的 @JavascriptInterface 方法 ⇒ 必须认 `invoke`；
 *   - 旧 v1 bridge 只有 playVideo/playPlaylist（没有 invoke）⇒ 返回 false，顶栏就不画
 *     这个假装能用的按钮；
 *   - 若将来把 openSettings 提升为独立的 @JavascriptInterface 方法，第一个分支照样认得。
 */
export function canOpenSettings(): boolean {
  const b = getBridge()
  if (!b) return false
  if (typeof b.openSettings === 'function') return true
  return typeof b.invoke === 'function'
}

const subscribers = new Map<string, Set<EventHandler>>()

function dispatchEvent(name: string, payload: unknown): void {
  const set = subscribers.get(name)
  if (!set) return
  for (const cb of set) {
    try {
      cb(payload)
    } catch (e) {
      // 关键: native evaluateJavascript 注入的 dispatcher 走到这里出错, window.onerror
      // 拿不到 source (退化成 "Script error."), 所以必须在这里主动上报真实 stack.
      // 延迟 import 避免与 App.vue 循环引用.
      console.error('[jerocineNative] handler error', name, e)
      void import('@/utils/telemetry').then(({ telemetry }) => {
        telemetry.trackError(e as Error, 'native-dispatch-error', {
          event: name,
          payload: typeof payload === 'object' ? JSON.stringify(payload).slice(0, 200) : String(payload)
        })
      }).catch(() => {})
    }
  }
}

function ensureGlobalDispatcher(): void {
  if (typeof window === 'undefined') return
  const w = window as unknown as { __JerocineEvents?: (n: string, p: unknown) => void }
  if (w.__JerocineEvents) return
  w.__JerocineEvents = (name: string, payload: unknown) => dispatchEvent(name, payload)
}

ensureGlobalDispatcher()

export const jerocine = {
  /** 通用 invoke. 自动 JSON.stringify args, JSON.parse 返回. native 不在场抛错 */
  call(method: string, args: Record<string, unknown> = {}): CallResult {
    const b = getBridge()
    if (!b?.invoke) {
      return { ok: false, error: 'native bridge not available' }
    }
    try {
      const resultStr = b.invoke(method, JSON.stringify(args))
      if (!resultStr) return { ok: true }
      return JSON.parse(resultStr) as CallResult
    } catch (e) {
      return { ok: false, error: e instanceof Error ? e.message : String(e) }
    }
  },

  /** 订阅 native 事件. 返回 unsubscribe 函数 */
  on(eventName: string, handler: EventHandler): () => void {
    let set = subscribers.get(eventName)
    if (!set) {
      set = new Set()
      subscribers.set(eventName, set)
    }
    set.add(handler)
    return () => {
      set?.delete(handler)
    }
  },

  // ============ 类型化便捷方法 (常用动作) ============

  toast(message: string, longDuration = false): void {
    this.call('toast', { message, duration: longDuration ? 1 : 0 })
  },

  exitApp(): void {
    this.call('exitApp')
  },

  keepScreenOn(on: boolean): void {
    this.call('keepScreenOn', { on })
  },

  openExternal(url: string): void {
    this.call('openExternal', { url })
  },

  storageGet(key: string): string | null {
    const r = this.call('storageGet', { key })
    if (!r.ok) return null
    return r.value == null ? null : String(r.value)
  },

  storageSet(key: string, value: string): boolean {
    return this.call('storageSet', { key, value }).ok
  },

  storageRemove(key: string): boolean {
    return this.call('storageRemove', { key }).ok
  },

  getDeviceInfo(): Record<string, unknown> | null {
    const r = this.call('getDeviceInfo')
    if (!r.ok) return null
    return r.device as Record<string, unknown> | null
  },

  getNetworkStatus(): { connected: boolean; type: string } {
    const r = this.call('getNetworkStatus') as { ok: boolean; connected?: boolean; type?: string }
    return { connected: !!r.connected, type: r.type ?? 'unknown' }
  },

  // ============ 视频播放器 ============

  /** 播单集 (兼容老 API). 推荐用 playPlaylist */
  playVideo(url: string, title = ''): void {
    this.call('playVideo', { url, title })
  },

  /**
   * 播 playlist (含跳片头片尾 / 自动续集 / 历史回传 / 多源切换).
   *
   * v3 多源模式 (推荐): 传 sources + currentSourceId → native 播放器内菜单可切源
   * v2 单源兼容: 仅传 episodes → 播放器内菜单无"切换源" 项
   */
  playPlaylist(cfg: {
    /** v3 多源: 整片所有播放源 + 集列表 */
    sources?: Array<{
      id: string
      name: string
      /**
       * 服务端 m3u8 可达性(play 接口标注): false ⇒ 原生不包装 /m3u8/proxy, 走直链 +
       * 端侧混合过滤。undefined(未测) 时保持旧行为(仍走代理)。
       */
      adFilterOk?: boolean
      episodes: Array<{ url: string; title?: string }>
    }>
    /** v3 多源: 初始选中的源 id (找不到时默认 sources[0]) */
    currentSourceId?: string
    /** v2 单源兼容: 当前源的集列表 (与 sources 二选一) */
    episodes?: Array<{ url: string; title?: string }>
    startIndex?: number
    resumeAtSec?: number
    skipIntroSec?: number
    skipOutroSec?: number
    /** 自动连播开关(播放页开关持久化值). false → 原生不自动跳片尾; 跳片头不受影响 */
    autoNext?: boolean
    filmId?: string
    filmName?: string
    /** 方案B: 传原始 m3u8 + 代理 base, 由原生每集自行包装代理过滤(不再 web 预包装) */
    proxyBase?: string
    /** 初始广告过滤开关(原生以自身持久化为准, 仅作初值参考) */
    adFilter?: boolean
  }): void {
    this.call('playPlaylist', cfg as unknown as Record<string, unknown>)
  },

  stopPlayer(): void {
    this.call('stopPlayer')
  },

  setPlayerSpeed(speed: number): void {
    this.call('setPlayerSpeed', { speed })
  },

  // ============ 自升级 / 服务器设置 ============

  checkUpdate(): void {
    this.call('checkUpdate')
  },

  openServerSettings(): void {
    this.call('openServerSettings')
  },

  /** 打开原生设置抽屉(壳层 MENU 键的等价入口, 供无 MENU 键的遥控器 / 触屏使用) */
  openSettings(): void {
    this.call('openSettings')
  },

  setAuthToken(token: string): void {
    this.call('setAuthToken', { token })
  },

  clearAuthToken(): void {
    this.call('clearAuthToken')
  }
}

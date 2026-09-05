/**
 * telemetry - 客户端埋点上报.
 *
 * 设计:
 *   - 单例 + 内存队列, 节流 flush (5s 或 20 条满)
 *   - 失败重试 1 次, 再失败丢弃 (防止失败堆积撑爆内存)
 *   - 自动 sessionId (uuid 风格随机串, 持久化 sessionStorage)
 *   - 字段隐私: 只上报 type/category/action/label/value/path/userId/platform
 *
 * 用法:
 *   import { telemetry } from '@/utils/telemetry'
 *   telemetry.track('action', { category: 'search', action: 'submit', label: keyword })
 *   telemetry.trackError(err, 'render')
 *   telemetry.trackApi(path, durationMs, status)
 *   telemetry.trackPv(path)   // router.afterEach 已自动接入
 *
 * 接入位置:
 *   - main.ts 顶层 import './utils/telemetry' (触发 install 副作用)
 *   - api/http.ts response 拦截器: trackApi + 4xx/5xx → trackError
 *   - router/index.ts router.afterEach: trackPv
 *   - App.vue window.onerror / unhandledrejection: trackError
 *   - App.vue jerocine.on('playerError'): trackError
 */

/**
 * chunk 加载失败特征: 部署后 WebView/浏览器缓存的旧 index.html 仍指向旧 hash 的 chunk →
 * 404/返回 index.html(text/html) → 动态 import 失败。router.onError 已自动 reload 自愈,
 * 属正常现象, 不作为异常上报(命中即在 trackError 丢弃)。
 */
const CHUNK_NOISE_RE =
  /dynamically imported module|Importing a module script failed|valid JavaScript MIME type|ChunkLoadError|Loading (CSS )?chunk|preload CSS/i

interface TelemetryEvent {
  ts: number
  type: 'pv' | 'error' | 'action' | 'perf' | 'api'
  category?: string
  action?: string
  label?: string
  value?: number
  path?: string
  userId?: number
  sessionId: string
  platform: string
  appVersion: string
  extra?: string
}

interface TrackOpts {
  category?: string
  action?: string
  label?: string
  value?: number
  extra?: Record<string, unknown>
}

/** 后端 ingest 契约 (server/internal/service.IngestEvent) — extra 须为对象 */
interface IngestPayload {
  category: string
  path: string
  label: string
  value: number
  duration: number
  clientTs: number
  sessionId: string
  extra: Record<string, unknown>
}

/**
 * 富事件 → 后端 IngestEvent 形状:
 *  - category = category || type(后端以 category 为主分类 + 问题去重键 category|path|label)
 *  - clientTs = ts; api 事件耗时(value)同时写入 duration 供 ApiPerf 聚合
 *  - extra 必须是对象(后端 map[string]any); 后端无列的 type/action/platform/appVersion/userId
 *    与原 extra 一并并入 extra, 不丢数据
 */
export function toIngestEvent(e: TelemetryEvent): IngestPayload {
  let extraObj: Record<string, unknown> = {}
  if (e.extra) {
    try {
      const parsed: unknown = JSON.parse(e.extra)
      extraObj =
        parsed && typeof parsed === 'object'
          ? (parsed as Record<string, unknown>)
          : { raw: e.extra }
    } catch {
      extraObj = { raw: e.extra }
    }
  }
  return {
    category: e.category || e.type,
    path: e.path ?? '',
    label: e.label ?? '',
    value: e.value ?? 0,
    duration: e.type === 'api' ? Math.round(e.value ?? 0) : 0,
    clientTs: e.ts,
    sessionId: e.sessionId,
    extra: {
      type: e.type,
      ...(e.action ? { action: e.action } : {}),
      platform: e.platform,
      appVersion: e.appVersion,
      ...(e.userId ? { userId: e.userId } : {}),
      ...extraObj
    }
  }
}

const FLUSH_INTERVAL_MS = 5000
const FLUSH_BATCH_SIZE = 20
const MAX_QUEUE = 200
// 与 http 实例同源: baseURL = (VITE_API_BASE||'/api') + '/v1'; 埋点入库走 /telemetry/events
const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) || '/api'
export const ENDPOINT = API_BASE + '/v1/telemetry/events'

function genSessionId(): string {
  if (typeof window === 'undefined') return 'ssr'
  try {
    const KEY = 'gf-telemetry-sid'
    let v = sessionStorage.getItem(KEY)
    if (!v) {
      v = 's_' + Math.random().toString(36).slice(2) + Date.now().toString(36)
      sessionStorage.setItem(KEY, v)
    }
    return v
  } catch {
    return 's_' + Math.random().toString(36).slice(2)
  }
}

function detectPlatform(): string {
  if (typeof window === 'undefined') return 'web'
  const w = window as unknown as { JerocineNative?: unknown; JerocinePlayer?: unknown }
  if (w.JerocineNative || w.JerocinePlayer) return 'android-tv'
  return 'web'
}

function detectAppVersion(): string {
  // Web build mode 或者 native 内可以通过 bridge 拿真实 versionCode/Name. 第一版用 build mode.
  return (import.meta.env.MODE ?? 'production').slice(0, 16)
}

/** 当前视口/模式/dpr — error 自动附带, 便于关联"出错时设备态" */
function collectViewport(): Record<string, unknown> {
  if (typeof window === 'undefined') return {}
  try {
    return {
      innerW: window.innerWidth,
      innerH: window.innerHeight,
      docW: document.documentElement.scrollWidth,
      docH: document.documentElement.scrollHeight,
      dpr: window.devicePixelRatio,
      mode: document.documentElement.getAttribute('data-mode')
    }
  } catch {
    return {}
  }
}

/**
 * Native bridge 探活 — diag snapshot 内拷一份, 后台直接看 bridge 是否注入 + invoke
 * 返回值. (web→native 单向验证, native→web 验证靠后续 echoReply 事件落 telemetry.)
 */
function collectBridge(): Record<string, unknown> {
  if (typeof window === 'undefined') return {}
  const w = window as unknown as {
    JerocineNative?: { invoke?: (m: string, a: string) => string }
    JerocinePlayer?: unknown
    Capacitor?: { getPlatform?: () => string; platform?: string }
  }
  const out: Record<string, unknown> = {
    hasJerocineNative: !!w.JerocineNative,
    hasJerocinePlayer: !!w.JerocinePlayer,
    hasCapacitor: !!w.Capacitor,
    hasInvoke: !!w.JerocineNative?.invoke
  }
  // 用 getPlatform 这种无副作用的 invoke 做 web→native 一次性 ping
  try {
    if (w.JerocineNative?.invoke) {
      const r = w.JerocineNative.invoke('getPlatform', '')
      out.invokeProbe = r ? r.slice(0, 200) : ''
    }
  } catch (e) {
    out.invokeProbeError = e instanceof Error ? e.message : String(e)
  }
  try {
    if (w.Capacitor?.getPlatform) out.capacitorPlatform = w.Capacitor.getPlatform()
  } catch {
    // ignore
  }
  return out
}

/** 屏幕物理参数 + UA, 仅 diag snapshot 用, 避免每条 error 都重复存 */
function collectScreen(): Record<string, unknown> {
  if (typeof window === 'undefined') return {}
  try {
    return {
      screenW: window.screen?.width,
      screenH: window.screen?.height,
      availW: window.screen?.availWidth,
      availH: window.screen?.availHeight,
      ua: navigator.userAgent.slice(0, 200),
      lang: navigator.language,
      online: navigator.onLine,
      cookie: navigator.cookieEnabled
    }
  } catch {
    return {}
  }
}

class Telemetry {
  private queue: TelemetryEvent[] = []
  /** error 类事件在 queue 内按 key 去重, value 累计计数. 出 queue (flush) 即清. */
  private errorIndex = new Map<string, TelemetryEvent>()
  private timer: number | undefined
  private sessionId: string = genSessionId()
  private platform: string = detectPlatform()
  private appVersion: string = detectAppVersion()
  private userId = 0

  /** 由 userStore 登录后调; 0 = 匿名 */
  setUserId(id: number): void {
    this.userId = id || 0
  }

  /** 通用 track API */
  track(type: TelemetryEvent['type'], opts: TrackOpts = {}): void {
    const event: TelemetryEvent = {
      ts: Date.now(),
      type,
      category: opts.category,
      action: opts.action,
      label: opts.label,
      value: opts.value,
      path: typeof window !== 'undefined' ? window.location.pathname : '',
      userId: this.userId,
      sessionId: this.sessionId,
      platform: this.platform,
      appVersion: this.appVersion,
      extra: opts.extra ? JSON.stringify(opts.extra) : undefined
    }
    this.enqueue(event)
  }

  trackPv(path: string): void {
    this.track('pv', { action: 'route', label: path })
  }

  trackApi(path: string, durationMs: number, status: number): void {
    // 只记 api 事件(已含 status/耗时, 供"API 性能"统计)。
    // HTTP 4xx/5xx 不再额外造 error 事件 —— 它们多为预期(401/404)或会重复刷屏错误列表,
    // 真正的服务端故障(5xx)仍可从 api 事件的 status 看到; 错误列表只留 JS/应用异常。
    this.track('api', { action: path, label: String(status), value: durationMs })
  }

  trackError(err: unknown, category = 'js-error', extra?: Record<string, unknown>): void {
    let message = ''
    let stack = ''
    let errorName = ''
    if (err instanceof Error) {
      message = err.message
      stack = err.stack ?? ''
      errorName = err.name
      // 跟 ES2022 Error.cause 链, 最多展开 5 层避免循环
      let cause: unknown = (err as { cause?: unknown }).cause
      let depth = 0
      while (cause && depth < 5) {
        if (cause instanceof Error) {
          stack += '\nCaused by: ' + (cause.stack ?? cause.message)
          cause = (cause as { cause?: unknown }).cause
        } else {
          stack += '\nCaused by (non-Error): ' + String(cause)
          break
        }
        depth++
      }
    } else if (err && typeof err === 'object') {
      try {
        message = JSON.stringify(err).slice(0, 500)
      } catch {
        message = String(err)
      }
    } else {
      message = String(err)
    }
    // 上层拿不到 stack (传字符串 / unhandledrejection 非 Error / 第三方 callback throw 普通值),
    // 这里 synth 一个 Error 把当前调用栈抓下来 — 至少能定位 trackError 的调用方.
    if (!stack) {
      const synth = new Error('synthetic-stack').stack ?? ''
      stack = '[synthetic, original was not Error]\n' + synth.split('\n').slice(2).join('\n')
    }
    // stack 限到 20k 字符: 留出 12k 给其它 extra 字段, JSON.stringify 转义后总长仍在
    // 服务端 32k cap 内, 避免被截断撑爆 JSON.
    // chunk 加载失败 = 部署后旧 index.html 指向旧 chunk hash 的"正常自愈"现象(router.onError 已自动
    // reload), 不是异常 → 不上报, 否则每次部署都刷屏埋点错误列表(vue-error/unhandled-rejection/
    // js-error-opaque/chunk-load-reload 各路径都汇到这里, 单点过滤即可全覆盖)。
    if (CHUNK_NOISE_RE.test(message)) return
    // 所有 error 自动带 viewport/mode/dpr/UA — 不用再每个 caller 单独传, 后台一眼看到
    // 出错时的设备态.
    this.track('error', {
      category,
      action: category,
      label: message.slice(0, 1024),
      extra: {
        ...(extra ?? {}),
        errorName: errorName || undefined,
        ...collectViewport(),
        stack: stack.slice(0, 20000)
      }
    })
  }

  /** 开机/手动触发: 上报一条 diag 事件, 内含完整设备/视口/原生壳信息 + bridge 探活. */
  reportDiag(extraCtx?: Record<string, unknown>): void {
    this.track('action', {
      category: 'diag',
      action: 'snapshot',
      label: 'startup',
      extra: {
        ...collectViewport(),
        ...collectScreen(),
        ...collectBridge(),
        ...(extraCtx ?? {})
      }
    })
  }

  private enqueue(e: TelemetryEvent): void {
    // error 类事件按 (category|path|label) 去重: 同 key 在未 flush 的 queue 内只占 1 行,
    // value 字段当作"本批发生次数"累加, 服务端按 value 拿到次数. 避免单页 setInterval
    // 反复 throw 把队列撑爆 + DB 存几百条同样的 row.
    if (e.type === 'error') {
      const key = `${e.category ?? ''}|${e.path ?? ''}|${e.label ?? ''}`
      const exist = this.errorIndex.get(key)
      if (exist) {
        exist.value = (exist.value ?? 1) + 1
        exist.ts = e.ts // 更新到最近一次时间
        return
      }
      // 首次出现, value=1 起步
      e.value = 1
      this.errorIndex.set(key, e)
    }
    if (this.queue.length >= MAX_QUEUE) {
      const dropped = this.queue.shift()
      if (dropped && dropped.type === 'error') {
        const key = `${dropped.category ?? ''}|${dropped.path ?? ''}|${dropped.label ?? ''}`
        this.errorIndex.delete(key)
      }
    }
    this.queue.push(e)
    if (this.queue.length >= FLUSH_BATCH_SIZE) {
      this.flush()
    } else if (e.type === 'error') {
      // error 优先级 — 500ms 内 flush, 不等 5s 批次. 避免"toast 出来了但 telemetry
      // 还在 queue 里没传, 用户看不到数据".
      this.scheduleFlush(500)
    } else {
      this.scheduleFlush()
    }
  }

  private scheduleFlush(delayMs: number = FLUSH_INTERVAL_MS): void {
    if (typeof window === 'undefined') return
    // 若已有定时器但比新请求还慢, 抢占成快的
    if (this.timer !== undefined) {
      if (delayMs >= FLUSH_INTERVAL_MS) return // 现有 5s 就够, 不用重排
      window.clearTimeout(this.timer)
    }
    this.timer = window.setTimeout(() => {
      this.timer = undefined
      this.flush()
    }, delayMs)
  }

  /** 批量发送, 失败 1 次重试 (未发送 events 留队列等下次) */
  private flush(): void {
    if (this.queue.length === 0) return
    if (typeof window === 'undefined') return
    const batch = this.queue.slice()
    this.queue = []
    this.errorIndex.clear()
    if (this.timer !== undefined) {
      window.clearTimeout(this.timer)
      this.timer = undefined
    }
    void this.sendWithRetry(batch, 1)
  }

  private async sendWithRetry(events: TelemetryEvent[], retriesLeft: number): Promise<void> {
    try {
      const res = await fetch(ENDPOINT, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ events: events.map(toIngestEvent) }),
        // 上报失败不要阻塞主线程
        credentials: 'omit',
        keepalive: true
      })
      if (!res.ok) throw new Error('telemetry HTTP ' + res.status)
    } catch (e) {
      if (retriesLeft > 0) {
        // 简单退避: 2s 后重试
        setTimeout(() => {
          void this.sendWithRetry(events, retriesLeft - 1)
        }, 2000)
      }
      // 终失败丢弃 — 不打扰用户主流程
    }
  }
}

export const telemetry = new Telemetry()

// 进程退出 / 切后台时尽量 flush (Beacon API)
if (typeof window !== 'undefined') {
  const flushBeacon = (): void => {
    const t = telemetry as unknown as { queue: TelemetryEvent[] }
    if (t.queue.length === 0) return
    if (typeof navigator !== 'undefined' && navigator.sendBeacon) {
      try {
        const blob = new Blob([JSON.stringify({ events: t.queue.map(toIngestEvent) })], {
          type: 'application/json'
        })
        navigator.sendBeacon(ENDPOINT, blob)
        t.queue = []
      } catch {
        /* ignore */
      }
    }
  }
  window.addEventListener('pagehide', flushBeacon)
  window.addEventListener('beforeunload', flushBeacon)
}

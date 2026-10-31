import { onScopeDispose, ref, watch, type Ref, type ShallowRef } from 'vue'
import type Artplayer from 'artplayer'
import type { Danmu } from 'artplayer-plugin-danmuku'
import { ApiError } from '@/types/api'
import { filmApi } from '@/api'
import type { DanmuComment, DanmuAggregateResp } from '@/types/film'
import { toDanmuInputs, type DanmuMapFilter } from '@/utils/danmu'
import { SETTINGS_APPLIED_EVENT } from '@/utils/settingEvents'

/**
 * 弹幕（D3）—— 渲染层 artplayer-plugin-danmuku + 数据层聚合弹幕服务（2026-10-30 换引擎）：
 *
 *  - 渲染引擎：**artplayer-plugin-danmuku**（nextTV 同款），插件自带控制条弹幕开关
 *    （apd-toggle）与弹幕设置面板（apd-config-panel: 类型屏蔽/防重叠/透明度/区域/字号/速度），
 *    UI 与 nextTV 完全一致；渲染/暂停跟随/seek 清屏/防重叠全部由插件托管。
 *  - 数据层：服务端聚合弹幕（dandanplay v2 协议按标题匹配，404→unsupported/空→empty）。
 *  - 开关持久化：jc-danmaku（用户级总开关，与 /settings 配置页、齿轮面板共用）。
 *  - 显示设置持久化：jc-danmaku-settings（字号/透明度/区域/速度/描边等）→ 插件 config()。
 *
 * 本 composable 职责 = 数据获取 + 状态机 + 设置持久化映射；渲染层不再自管。
 */

/** 插件 Result 上需要的最小接口（避免 import 插件 Result 类型造成循环依赖）。 */
interface DanmukuPluginLike {
  load: (danmuku?: Danmu[]) => Promise<unknown>
  config: (option: Record<string, unknown>) => unknown
  show: () => unknown
  hide: () => unknown
  reset: () => unknown
  /** 插件当前配置(visible 供换集后判断是否恢复显示) */
  readonly option: { visible?: boolean }
}

export interface DanmakuSettings {
  /** 字号：0 = 自适应；>0 = 固定像素（插件 fontSize 数字） */
  fontSize: number
  /** 弹幕层整体透明度 0-1 */
  opacity: number
  /** 显示区域高度占比 0-1（0.25/0.5/0.75/1 对应 1/4 屏·半屏·3/4 屏·全屏；映射插件 margin 底部偏移） */
  area: number
  /** 滚动速度档位（数值越大越慢）。映射插件 speed（弹幕持续时间秒, 1-10）：
   *  档位 [1,1.5,2,2.5,3] → 持续秒 [5,6,7.5,9,10]，默认档 2 = 7.5s 与 nextTV 一致 */
  speed: number
  /** 同屏弹幕上限 —— **已废弃**（插件 antiOverlap 防重叠替代）；字段保留兼容旧 localStorage */
  limit: number
  /** 底部弹幕（源 mode 4）开关 */
  bottomMode: boolean
  /** 顶部弹幕（源 mode 5）开关 */
  topMode: boolean
  /** 彩色（false = 全部白色） */
  colored: boolean
  /** 防重叠（插件 antiOverlap；插件设置面板可改, 经 :config 事件回写持久化） */
  antiOverlap: boolean
}

export const DANMAKU_SETTINGS_KEY = 'jc-danmaku-settings'
export const DANMAKU_DEFAULT_SETTINGS: DanmakuSettings = {
  fontSize: 0,
  opacity: 0.8,
  area: 0.5,
  // 速度默认 2（"正常"档 = 插件持续 7.5s 走完屏, 与 nextTV speed:7.5 一致）
  speed: 2,
  // 已废弃(插件 antiOverlap 替代), 默认 30 仅兼容旧数据
  limit: 30,
  bottomMode: true,
  topMode: true,
  colored: true,
  antiOverlap: true
}
/** 字号档位（自适应档 + 4 档可调；B站/腾讯均为档位式） */
export const DANMAKU_FONT_STEPS = [0, 16, 20, 24, 28, 34] as const
/** 显示区域档位（1/4 屏 → 全屏） */
export const DANMAKU_AREA_STEPS = [0.25, 0.5, 0.75, 1] as const
/**
 * 速度档位（数值越大弹幕滚动越慢）。映射插件 speed（持续时间秒）：
 * 档位 [1,1.5,2,2.5,3] → [5,6,7.5,9,10]。默认档 2 = 7.5s（nextTV 同款）。
 */
export const DANMAKU_SPEED_STEPS = [1, 1.5, 2, 2.5, 3] as const
/** 旧速度档位（仅用于 readDanmakuSettings 迁移: 旧值 ×2 映射到新档） */
export const DANMAKU_OLD_SPEED_STEPS = [0.5, 0.75, 1, 1.25, 1.5] as const
/** 档位值 → 插件 speed（弹幕持续时间秒, 越大越慢; 上限 10）。 */
export function danmakuSpeedOf(speed: number): number {
  return Math.min(10, 5 + (speed - 1) * 2)
}

/** 单个弹幕请求超时（ms）：整集聚合容错更大（服务端聚合最坏数秒 + 4MB 级响应）。
 *  60s: 4MB 大响应在弱链路/代理下实测 9-13s, 30s 曾出现偶发超时(E2E 发现) —— 放宽到 60s。
 *  导出为可变对象便于测试注入极小超时。 */
export const DANMAKU_FETCH_TIMEOUTS = {
  aggregate: 60_000
}

/** 聚合请求自动重试次数（仅超时/网络错; 404 不重试）:
 *  E2E 实测生产/代理链路偶发 4MB 响应中途失败(21.7s/4.2s transferSize=0), 一次重试可恢复。 */
export const DANMAKU_AGG_RETRIES = 1

/** 读取弹幕显示设置（缺省/损坏回默认值） */
export function readDanmakuSettings(): DanmakuSettings {
  try {
    const raw = localStorage.getItem(DANMAKU_SETTINGS_KEY)
    if (!raw) return { ...DANMAKU_DEFAULT_SETTINGS }
    const p = JSON.parse(raw) as Partial<DanmakuSettings>
    // 速度档迁移: 旧档值(0.5/0.75/1/1.25/1.5) ×2 → 新档(整体慢一倍, 语义见 DANMAKU_SPEED_STEPS)
    let speed = DANMAKU_DEFAULT_SETTINGS.speed
    if (typeof p.speed === 'number') {
      if ((DANMAKU_OLD_SPEED_STEPS as readonly number[]).includes(p.speed)) {
        speed = p.speed * 2
      } else if ((DANMAKU_SPEED_STEPS as readonly number[]).includes(p.speed)) {
        speed = p.speed
      }
    }
    return {
      ...DANMAKU_DEFAULT_SETTINGS,
      ...p,
      // 数值越界/非法字段钳制到合法范围
      fontSize: typeof p.fontSize === 'number' && p.fontSize >= 0 ? Math.min(p.fontSize, 64) : 0,
      opacity: typeof p.opacity === 'number' ? Math.min(Math.max(p.opacity, 0.1), 1) : 0.8,
      area: (DANMAKU_AREA_STEPS as readonly number[]).includes(p.area as number) ? (p.area as number) : 0.5,
      speed,
      limit: typeof p.limit === 'number' && p.limit >= 0 ? p.limit : DANMAKU_DEFAULT_SETTINGS.limit
    }
  } catch {
    return { ...DANMAKU_DEFAULT_SETTINGS }
  }
}

function writeDanmakuSettings(s: DanmakuSettings): void {
  try {
    localStorage.setItem(DANMAKU_SETTINGS_KEY, JSON.stringify(s))
  } catch {
    /* localStorage 不可用时仅本轮生效 */
    return
  }
  // 账号记忆: 设置持久化即通知云同步(useCloudSettings 防抖上报 /me/web-settings)
  try {
    window.dispatchEvent(new CustomEvent('jc-setting-changed', { detail: { key: DANMAKU_SETTINGS_KEY } }))
  } catch {
    /* ignore */
  }
}

/** 带超时的请求封装：超时（或外部 abort）自动中止底层请求。
 *  controllers 为本 composable 实例的活跃请求集合，供卸载时统一中止。 */
function withTimeout<T>
(
  factory: (signal: AbortSignal) => Promise<T>,
  timeoutMs: number,
  controllers: Set<AbortController>
): { promise: Promise<T>; ctrl: AbortController } {
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), timeoutMs)
  const promise = factory(ctrl.signal).finally(() => {
    clearTimeout(timer)
    controllers.delete(ctrl)
  })
  controllers.add(ctrl)
  return { promise, ctrl }
}

/**
 * 弹幕 UI 状态（无源/查无/失败都必须有显示, 不许静默）:
 *   idle         开关关 / 无 mid
 *   loading      查询装载中
 *   unsupported  查无作品/该集（服务端 404）或影片名缺失 —— "该片暂无弹幕"
 *   empty        匹配到集但弹幕内容为空（或服务端弹幕开关未开放） —— "未找到弹幕"
 *   failed       装载链路异常（请求失败/超时/插件缺失） —— "弹幕加载失败", 可重试
 *   ready        已装载, 弹幕可见
 */
export type DanmuState = 'idle' | 'loading' | 'unsupported' | 'empty' | 'failed' | 'ready'

export interface UseDanmakuOptions {
  /** 影片 mid（空 = 无弹幕；也作内存缓存键） */
  mid: Ref<number | undefined>
  /** 影片名（聚合服务按标题匹配；空 = 无弹幕） */
  title: Ref<string>
  /** 年份（可选，提高作品匹配精度） */
  year: Ref<number | undefined>
  /** 集号（1 起；对应我们自己的集序, 服务端按集号定位） */
  episode: Ref<number>
  /** 播放器实例（ready 前为 null；插件实例从 art.plugins 取） */
  player: ShallowRef<Artplayer | null>
  /** 开关 */
  enabled: Ref<boolean>
}

/** 弹幕插件实例名（artplayer-plugin-danmuku 注册名）。 */
export const DANMUKU_PLUGIN_NAME = 'artplayerPluginDanmuku'

export function useDanmaku(opts: UseDanmakuOptions) {
  /** 是否有弹幕可显示（装载非空; 供 UI 显示状态） */
  const hasDanmaku = ref(false)
  /** UI 状态机（三态显示的单一来源, 语义见 {@link DanmuState}） */
  const danmuState = ref<DanmuState>('idle')

  /** 弹幕显示设置（localStorage 持久化, 实时应用到插件） */
  const settings = ref<DanmakuSettings>(readDanmakuSettings())

  /** 拿弹幕插件实例（播放器未建/未传 danmuku 配置时为 null）。 */
  function plugin(): DanmukuPluginLike | null {
    const art = opts.player.value
    if (!art) return null
    try {
      const p = (art.plugins as unknown as Record<string, unknown>)[DANMUKU_PLUGIN_NAME]
      return p ? (p as DanmukuPluginLike) : null
    } catch {
      return null
    }
  }

  /** 当前过滤项（底部/顶部/彩色开关）。 */
  function currentMapFilter(): DanmuMapFilter {
    const s = settings.value
    return { bottomMode: s.bottomMode, topMode: s.topMode, colored: s.colored }
  }

  /** 把可运行时生效的设置项应用到插件（透明度/区域/字号/速度/防重叠）。 */
  function applyPluginSettings(): void {
    const p = plugin()
    if (!p) return
    const s = settings.value
    try {
      p.config({
        opacity: s.opacity,
        // 插件 margin: [纵向间距, 底部偏移]; 显示区域 = 1 - 底部偏移
        margin: [10, `${Math.round((1 - s.area) * 100)}%`],
        fontSize: s.fontSize > 0 ? s.fontSize : 23,
        speed: danmakuSpeedOf(s.speed),
        antiOverlap: s.antiOverlap
      })
    } catch {
      /* 插件未就绪时静默 */
    }
  }

  /* ---------------- 取数（内存缓存 + in-flight 去重 + 超时中止） ----------------
   * 服务端已有 Redis 缓存；这里再加一层内存缓存：命中直接装载，连缓存命中往返都省掉。
   * 隔离：缓存 key 带 mid+episode，跨集/跨片自动 miss；生命周期 = composable，onScopeDispose 清空。 */
  /** 已加载段弹幕池（key=`${mid}:${episode}`）—— 分片懒加载: 池随段追加, 装载时全量重建 */
  const aggCache = new Map<string, DanmuComment[]>()
  /** FIFO 上限: 整集弹幕可到 MB 级, 长剧来回切集防内存膨胀 */
  const AGG_CACHE_MAX = 8
  /** 整集 in-flight 去重 */
  const aggReq = new Map<string, Promise<DanmuComment[]>>()
  /** 本实例活跃请求控制器集合（卸载/超时统一中止，避免悬挂连接） */
  const activeControllers = new Set<AbortController>()

  /** 分片大小（秒）: 15min/段。首拉起播段, 播放中按进度预取 —— 4MB 整集响应在弱链路
   *  实测 9-30s(E2E 曾 30s 超时), 分片后单段 ≈ 数百 KB, 首屏弹幕秒级可用。 */
  const SEGMENT_SEC = 900
  /** 分片取数范围状态（每集重置）: 已连续加载到的秒数 / 整集总条数（0 = 未知）。 */
  let segLoadedTo = 0
  let segTotal = 0
  /** 分片取数 in-flight 去重（同段并发只发一次）。 */
  let segInflight: Promise<void> | null = null
  /** 该集后续段永久停预取标记(404 语义态); 换集重置。 */
  let segFinished = false

  /** 取弹幕分片（内存池 + 并发去重 + 超时重试）。返回该片的弹幕（已并入池）。 */
  function ensureAggregate(from = 0, to = 0): Promise<DanmuComment[]> {
    const mid = opts.mid.value
    if (mid === undefined) return Promise.reject(new Error('no mid'))
    const ep = Math.max(1, opts.episode.value)
    const key = `${mid}:${ep}`
    // 复审提示项: in-flight 去重 key 掺 range —— 预取在途时 force 重载同集
    // 若共享 key 会拿到错误 range 的响应造成池缺段
    const flightKey = `${key}#${from}-${to}`
    const existing = aggReq.get(flightKey)
    if (existing) return existing
    console.log('[danmaku] agg request START', key, `range=[${from},${to})`, 'timeout =', DANMAKU_FETCH_TIMEOUTS.aggregate)
    const t0 = performance.now()
    /** 单次请求（带超时）。失败(非 404)时由外层重试 DANMAKU_AGG_RETRIES 次。 */
    const attempt = (): Promise<DanmuAggregateResp> => {
      const { promise } = withTimeout(
        (signal) =>
          filmApi.getDanmuAggregate(
            { title: opts.title.value, episode: ep, year: opts.year.value, from, to },
            { signal }
          ),
        DANMAKU_FETCH_TIMEOUTS.aggregate,
        activeControllers
      )
      return promise
    }
    /** 重试包装: 404 立即上抛(查无作品, 语义态); 其余(超时/网络/5xx)退避后重试。 */
    const withRetry = async (): Promise<DanmuAggregateResp> => {
      let lastErr: unknown
      for (let i = 0; i <= DANMAKU_AGG_RETRIES; i++) {
        if (i > 0) {
          console.warn('[danmaku] agg retry', i, '/', DANMAKU_AGG_RETRIES, 'key =', key)
          await new Promise((r) => setTimeout(r, 1000 * i))
        }
        try {
          return await attempt()
        } catch (err) {
          lastErr = err
          // 404 = 查无作品/该集 —— 语义结果, 重试无意义
          if (err instanceof ApiError && err.status === 404) throw err
        }
      }
      throw lastErr
    }
    const req = withRetry()
      .then((resp) => {
        console.log('[danmaku] agg request OK', key, Math.round(performance.now() - t0) + 'ms, comments =', resp?.comments?.length ?? 0, 'total =', resp?.total ?? '?')
        aggReq.delete(flightKey)
        const data = resp?.comments ?? []
        // 分片池合并: 追加新段（去重——同 time+text 的重复条目丢弃, 服务端段边界半开区间理论不重, 双保险）
        const pool = aggCache.get(key) ?? []
        const seen = new Set(pool.map((c) => `${c.time}:${c.text}`))
        let added = 0
        for (const c of data) {
          const sig = `${c.time}:${c.text}`
          if (seen.has(sig)) continue
          seen.add(sig)
          pool.push(c)
          added++
        }
        aggCache.set(key, pool)
        if (resp?.total) segTotal = resp.total
        // FIFO 淘汰最旧集（当前 key 刚 set 置于末尾, 淘汰从头开始且跳过当前 key）
        if (aggCache.size > AGG_CACHE_MAX) {
          const first = aggCache.keys().next().value
          if (first !== undefined && first !== key) aggCache.delete(first)
        }
        console.log('[danmaku] pool merged:', added, 'added,', pool.length, 'total')
        return data
      })
      .catch((err) => {
        const isAbort = (err as { code?: string })?.code === 'ERR_CANCELED'
        console.warn('[danmaku] agg request FAIL', key, Math.round(performance.now() - t0) + 'ms', isAbort ? '(timeout abort)' : (err instanceof ApiError ? `ApiError ${err.status}` : String(err).slice(0, 80)))
        aggReq.delete(flightKey)
        throw err
      })
    aggReq.set(flightKey, req)
    return req
  }

  /* ---------------- 装载 ----------------
   * 插件在 usePlayer 构造时注册（控制条开关/设置面板随之常驻）; 这里只喂数据与显隐。 */
  let loadToken = 0

  /** 当前集 key（`mid:episode`）；关→开同集即时恢复用（不重新拉取装载） */
  function currentLoadKey(): string | null {
    const mid = opts.mid.value
    return mid === undefined ? null : `${mid}:${opts.episode.value}`
  }
  /** 已成功装载内容的集 key；null = 尚未装载/需重装 */
  let loadedKey: string | null = null
  /** 已装载内容对应的弹幕池（当前集的分片合并池; 顶部/底部/彩色切换时内存重映射, 零请求零重载） */
  let lastComments: DanmuComment[] | null = null

  /** 把当前池装载进插件（分片追加/设置重映射共用入口）。池空 → empty 态。 */
  async function loadPoolIntoPlugin(p: DanmukuPluginLike, token: number): Promise<boolean> {
    const pool = aggCache.get(currentLoadKey() ?? '') ?? []
    lastComments = pool.length > 0 ? pool : null
    const items = toDanmuInputs(pool, currentMapFilter())
    if (items.length === 0) {
      // empty 边缘: 首段(0-900s)无弹幕但整集可能有(后段) —— 预取拉到数据时由
      // prefetchUpcoming 回写 ready/loadedKey, 这里仅当"池空且无后续段可拉"才是真 empty
      if (!(segTotal > pool.length && segFinished !== true)) {
        danmuState.value = 'empty'
      }
      return false
    }
    try {
      await p.load(items)
    } catch {
      if (token === loadToken) danmuState.value = 'failed'
      return false
    }
    return true
  }

  /** 换集装载后是否恢复显示: 尊重用户在插件控制条上的显隐选择(apd-toggle 是会话级
   *  开关 —— 用户刚关掉弹幕, 换集后被 show() 强制重开属于状态覆盖)。 */
  function pluginWantsShow(): boolean {
    const p = plugin()
    if (!p) return true
    try {
      return p.option.visible !== false
    } catch {
      return true
    }
  }

  async function loadDanmaku(force = false): Promise<void> {
    const key = currentLoadKey()
    const token = ++loadToken
    const p = plugin()
    if (!opts.enabled.value || !key) {
      // 关闭: 仅隐藏弹幕层(插件 hide), 不清内容 —— 再开同集即时恢复显示
      if (p) p.hide()
      danmuState.value = 'idle'
      return
    }
    if (!force && loadedKey === key && p) {
      // 同集已装载(关→开): 直接恢复显示, 即时出现(避免 clear+重拉造成"开弹幕等好久")
      p.show()
      hasDanmaku.value = true
      danmuState.value = 'ready'
      return
    }
    if (!opts.title.value) {
      danmuState.value = 'unsupported'
      return
    }
    hasDanmaku.value = false
    danmuState.value = 'loading'
    if (!p) {
      danmuState.value = 'failed' // 弹幕插件未注册
      return
    }
    applyPluginSettings() // 每次装载（首装或 bottom/top/colored 重载）都刷一遍运行时设置
    // 新集: 重置分片游标, 首拉起播段 [0, SEGMENT_SEC)
    segLoadedTo = 0
    segTotal = 0
    segFinished = false
    try {
      await ensureAggregate(0, SEGMENT_SEC)
    } catch (e) {
      if (token !== loadToken) return
      // 404 = 查无作品/该集 → "该片暂无弹幕"；其余（网络/超时/服务端）→ failed 可重试
      danmuState.value =
        e instanceof ApiError && e.status === 404 ? 'unsupported' : 'failed'
      return
    }
    if (token !== loadToken) return
    segLoadedTo = SEGMENT_SEC
    if (!(await loadPoolIntoPlugin(p, token))) return
    if (token !== loadToken) return
    loadedKey = currentLoadKey()
    hasDanmaku.value = true
    danmuState.value = 'ready'
    if (opts.enabled.value && pluginWantsShow()) p.show()
  }

  /**
   * 分片预取（节流入口, 由播放器 timeupdate/seeked 事件驱动）:
   * 播放进入"已加载尾部 - 300s"缓冲区或 seek 跳出已加载区 → 拉下一段并合并装载。
   * total 已知且 loadedTo 越过 total 对应时长（近似: 本段返回 0 条）→ 拉完, 不再请求。
   */
  let prefetchBusy = false
  async function prefetchUpcoming(currentSec: number): Promise<void> {
    if (prefetchBusy || !opts.enabled.value || segTotal === 0 || segFinished) return
    if (currentSec < segLoadedTo - 300) return // 未进缓冲区（含 seek 回跳到已加载区）
    const key = currentLoadKey()
    if (!key || aggCache.get(key) === undefined) return // 主装载未完成/已换集
    const prefetchedOnce = segLoadedTo > SEGMENT_SEC
    if (prefetchedOnce && lastComments !== null && lastComments.length >= segTotal && segTotal > 0) return
    prefetchBusy = true
    const token = loadToken
    let failFrom = -1 // 本次预取循环最后尝试的段起点(失败回滚用; 声明在 try 外供 catch 引用)
    try {
      // 循环补段直到追上播放位置 + 一段缓冲(第1轮坑②: 大跨度 seek 旧实现一次只补一段,
      // 靠 10s tick 逐段追, 追帧期间目标区弹幕缺失) —— 上限 8 段(2h)防极端长视频长循环。
      let pulled = 0
      for (let i = 0; i < 8; i++) {
        const from = segLoadedTo
        const to = from + SEGMENT_SEC
        // 追上播放位置 + 一段缓冲 → 停
        if (from > currentSec + SEGMENT_SEC) break
        failFrom = from
        const seg = await ensureAggregate(from, to)
        if (token !== loadToken) return
        segLoadedTo = to
        pulled += seg.length
        // 404/拉完判定: 本段 0 条且 pool 已达 total → 无后续
        const pool = aggCache.get(currentLoadKey() ?? '') ?? []
        if (segTotal > 0 && pool.length >= segTotal) break
      }
      if (pulled === 0) return
      const p = plugin()
      if (!p) return
      const loaded = await loadPoolIntoPlugin(p, token)
      // empty 边缘修复: 首段无弹幕停 empty, 预取拉到数据 → 回写 ready(按钮态恢复)
      if (loaded && token === loadToken && danmuState.value === 'empty') {
        loadedKey = currentLoadKey()
        hasDanmaku.value = true
        danmuState.value = 'ready'
        if (opts.enabled.value && pluginWantsShow()) p.show()
      }
      console.log('[danmaku] segments loaded, pulled =', pulled, ', pool =', lastComments?.length ?? 0)
    } catch (e) {
      // 404 = 查无作品/该集语义态 —— 该集后续段必然同样 404, 置 segFinished 永久停止预取
      // (否则短集片尾缓冲区里每 10s tick 循环请求到切集); 其余(网络/超时)回滚游标下个 tick 重试
      // 复审项13: 404 分支也需 token 守卫 —— 换集后旧集在途请求迟到 404 会污染新集的
      // segFinished(新集预取永久停止 + 空态误判)
      if (e instanceof ApiError && e.status === 404) {
        if (token === loadToken) {
          segFinished = true
          console.warn('[danmaku] prefetch 404 → 该集后续段停止预取', `[${failFrom},+900)`)
        }
      } else {
        console.warn('[danmaku] prefetch FAIL', String(e).slice(0, 60))
        if (token === loadToken) segLoadedTo = failFrom
      }
    } finally {
      prefetchBusy = false
    }
  }

  /** 更新弹幕显示设置（局部 patch）。
   *  - 底部/顶部/彩色/描边 命中 → 原始数据在内存, 直接重映射装载（零请求）
   *  - 其余（透明度/区域/速度/字号）→ 插件 config() 即时生效，无需重载 */
  function updateSettings(patch: Partial<DanmakuSettings>): void {
    const next = { ...settings.value, ...patch }
    settings.value = next
    writeDanmakuSettings(next)
    const affectsContent =
      patch.bottomMode !== undefined ||
      patch.topMode !== undefined ||
      patch.colored !== undefined
    if (affectsContent) {
      // 顶部/底部/彩色切换: 整集原始数据在内存, 直接重映射+load, 零请求、不清状态机(即时生效)
      if (
        plugin() &&
        lastComments &&
        loadedKey !== null &&
        loadedKey === currentLoadKey()
      ) {
        const items = toDanmuInputs(lastComments, currentMapFilter())
        // 插件 load 失败会 reject(内部 validator/转换抛错), 不 catch 会变 unhandledrejection
        plugin()?.load(items).catch(() => undefined)
        applyPluginSettings() // 混合 patch(如同时改字号)也要应用运行时设置
      } else {
        loadedKey = null // 内容映射参数变了, 已装载内容失效, 重开需重装
        void loadDanmaku(true)
      }
    } else {
      applyPluginSettings()
    }
  }

  /**
   * 插件设置面板 → 本地持久化回写。
   * 插件面板所有改动(滑条/开关)都经 danmuku.config() → art.emit('artplayerPluginDanmuku:config')。
   * 我们 applyPluginSettings() 主动 config 时 option 与 settings 一致 → 无差异 → 不回写(天然防回环);
   * 用户在插件面板改动 → 有差异 → 回写 jc-danmaku-settings, 下次进页初始 config 不丢。
   * visible(显示开关)/modes(类型屏蔽) 为会话级显示层状态, 有意不持久化。
   */
  function onPluginConfig(artOption: Record<string, unknown>): void {
    const s = { ...settings.value }
    let changed = false
    const num = (v: unknown): number | null => (typeof v === 'number' && Number.isFinite(v) ? v : null)
    const op = num(artOption.opacity)
    if (op !== null && Math.abs(op - s.opacity) > 1e-3) {
      s.opacity = Math.min(Math.max(op, 0.1), 1)
      changed = true
    }
    const fs = num(artOption.fontSize)
    if (fs !== null && fs !== (s.fontSize > 0 ? s.fontSize : 23)) {
      // 插件面板字号为连续滑条(非档位), 直接落值; 0(自适应档) 只能从我们这边设
      s.fontSize = Math.round(fs)
      changed = true
    }
    const margin = artOption.margin
    if (Array.isArray(margin) && typeof margin[1] === 'string' && margin[1].endsWith('%')) {
      const bottomPct = Number.parseFloat(margin[1])
      if (Number.isFinite(bottomPct)) {
        const area = Math.round((1 - Math.min(Math.max(bottomPct, 0), 100) / 100) * 100) / 100
        if (!DANMAKU_AREA_STEPS.includes(area as 0.25 | 0.5 | 0.75 | 1)) {
          // 插件滑条是连续值, 吸附到最近档位(与设置面板档位语义一致)
          const nearest = DANMAKU_AREA_STEPS.reduce((a, b) =>
            Math.abs(b - area) < Math.abs(a - area) ? b : a
          )
          if (nearest !== s.area) {
            s.area = nearest
            changed = true
          }
        } else if (area !== s.area) {
          s.area = area
          changed = true
        }
      }
    }
    const sp = num(artOption.speed)
    if (sp !== null) {
      // 反查档位: 持续秒 [5,6,7.5,9,10] → [1,1.5,2,2.5,3]; 非档位值吸附最近档
      const step = (DANMAKU_SPEED_STEPS as readonly number[]).find(
        (st) => Math.abs(danmakuSpeedOf(st) - sp) < 0.3
      )
      if (step !== undefined && step !== s.speed) {
        s.speed = step
        changed = true
      }
    }
    if (typeof artOption.antiOverlap === 'boolean' && artOption.antiOverlap !== s.antiOverlap) {
      s.antiOverlap = artOption.antiOverlap
      changed = true
    }
    if (changed) {
      settings.value = s
      writeDanmakuSettings(s)
    }
  }

  /** 分片预取的 timeupdate/seeked handler（具名引用, 供重建实例时对称 off） */
  let onPrefetchTick: ((...a: unknown[]) => void) | null = null
  let onPrefetchSeek: ((...a: unknown[]) => void) | null = null

  /** :config 事件 handler（具名函数, 供重建实例时对称 off） */
  const onPluginConfigHandler = (option: unknown): void =>
    onPluginConfig((option ?? {}) as Record<string, unknown>)

  /* ---- 开关 / 集号 / 影片（纳入触发条件; immediate: 初始 enabled=true 的老用户进页即装载） ---- */
  watch(
    () => [opts.enabled.value, opts.mid.value, opts.episode.value, opts.title.value] as const,
    () => {
      void loadDanmaku()
    },
    { immediate: true }
  )
  /** 播放器实例变化（init 完成/重建）后插件才可用 → 补一次装载 + 挂 :config 持久化回写。
   *  immediate: player 已就绪时（测试桩/热重建）也要挂上监听。 */
  watch(
    opts.player,
    (art, prevArt) => {
      if (prevArt) {
        try {
          prevArt.off('artplayerPluginDanmuku:config', onPluginConfigHandler)
        } catch {
          /* ignore */
        }
        if (onPrefetchTick) {
          try {
            prevArt.off('video:timeupdate', onPrefetchTick as never)
          } catch {
            /* ignore */
          }
        }
        if (onPrefetchSeek) {
          try {
            prevArt.off('video:seeked', onPrefetchSeek as never)
          } catch {
            /* ignore */
          }
        }
      }
      if (!art) return
      try {
        art.on('artplayerPluginDanmuku:config', onPluginConfigHandler)
      } catch {
        /* ignore */
      }
      // 分片预取触发: timeupdate 节流 10s + seeked(跳出已加载区立即补)
      let lastTick = 0
      onPrefetchTick = () => {
        const now = Date.now()
        if (now - lastTick < 10_000) return
        lastTick = now
        try {
          const t = art.currentTime
          if (typeof t === 'number' && Number.isFinite(t)) void prefetchUpcoming(t)
        } catch {
          /* ignore */
        }
      }
      onPrefetchSeek = () => {
        try {
          const t = art.currentTime
          if (typeof t === 'number' && Number.isFinite(t)) void prefetchUpcoming(t)
        } catch {
          /* ignore */
        }
      }
      try {
        art.on('video:timeupdate', onPrefetchTick as never)
        art.on('video:seeked', onPrefetchSeek as never)
      } catch {
        /* ignore */
      }
      void loadDanmaku()
    },
    { immediate: true }
  )
  /** 诊断日志: 弹幕状态机每次切换都打印(便于排查"弹幕长期不出现") */
  watch(danmuState, (to, from) => {
    console.log('[danmaku] state:', from, '→', to)
  })
  /** 云端设置合并到此设备(useCloudSettings 广播): 重读设置; 底部/顶部/彩色类变更需重载映射 */
  const onCloudApplied = (): void => {
    const prev = { ...settings.value }
    settings.value = readDanmakuSettings()
    applyPluginSettings()
    if (
      prev.bottomMode !== settings.value.bottomMode ||
      prev.topMode !== settings.value.topMode ||
      prev.colored !== settings.value.colored
    ) {
      loadedKey = null
      void loadDanmaku(true)
    }
  }
  window.addEventListener(SETTINGS_APPLIED_EVENT, onCloudApplied)

  onScopeDispose(() => {
    loadToken++
    // 中止仍在飞行的弹幕请求（超时/卸载都走这里），避免悬挂连接
    for (const c of activeControllers) {
      try {
        c.abort()
      } catch {
        /* ignore */
      }
    }
    activeControllers.clear()
    aggCache.clear()
    aggReq.clear()
    loadedKey = null
    lastComments = null
    window.removeEventListener(SETTINGS_APPLIED_EVENT, onCloudApplied)
  })

  return {
    hasDanmaku,
    danmuState,
    settings,
    updateSettings,
    reload: () => void loadDanmaku(true)
  }
}

import {
  onScopeDispose,
  ref,
  watch,
  type Ref,
  type ShallowRef
} from 'vue'
import type Player from 'video.js/dist/types/player'
import { createEngine, type DanmakuEngine, type DanmakuItem } from 'danmaku-lite'
import { filmApi } from '@/api'
import type { DanmuComment, DanmuProvider, DanmuTencentSegment } from '@/types/film'
import {
  resolveDanmuProvider,
  toDanmuInput,
  toDanmuInputs,
  type DanmuMapFilter
} from '@/utils/danmu'

/**
 * 弹幕（D3）—— 渲染层迁移到 danmaku-lite（方案 A, 2026-10-26）：
 *
 *  - 数据链路（不变）：mid → GET /films/:mid/danmu/source（平台候选）→
 *      - bilibili（路径 A 整集聚合）：seasonId → GET /danmu/bilibili 一次拉整集
 *      - tencent（路径 B 分段点播）：resourceId → episodes 匹配 → segments 索引 →
 *        按 currentTime 由引擎 dataSource 流式预拉 comments
 *  - 渲染引擎：danmaku-lite（DOM 引擎），叠加在 video.js 容器内独立弹幕层。
 *    引擎**自驱**：通过 PlayerAdapter（position/paused/duration）每帧读进度，无需手动 time/start/stop/seek。
 *    - 重叠：内置轨道分配器（scrollGap / maxVisible 控制密度）
 *    - 截断/被挡：overflow:'drop'（无空闲轨道时静默丢弃，不再挤到屏外截断）
 *    - 拖动进度一窝蜂：引擎按 seekThreshold 检测大跳，重置后从新位置按时间重发（非整段补发）
 *    - 腾讯分段：DataSourceAdapter.fetch(start,end) 由引擎按 preBuffer 自驱预拉（scheduler.add 追加，不清除屏上弹幕）
 *  - 开关与源偏好持久化（不变）：开关走 useWebSettings；源偏好 jc-danmaku-provider。
 */

export const DANMAKU_PROVIDER_KEY = 'jc-danmaku-provider'
export const DANMAKU_DEFAULT_PROVIDER = 'bilibili'

/* ---------------- 弹幕显示设置（2026-10-26, 参考 B站/腾讯设置面板） ---------------- */

export interface DanmakuSettings {
  /** 字号(px)。0 = 自适应：按 stage 宽度推算（见 resolveFontSize），覆盖全屏/移动端差异 */
  fontSize: number
  /** 弹幕层整体透明度 0-1 */
  opacity: number
  /** 显示区域高度占比 0-1（0.25/0.5/0.75/1 对应 1/4 屏·半屏·3/4 屏·全屏） */
  area: number
  /** 滚动速度倍率（>1 快、<1 慢）。danmaku-lite 的 speed 数值越大弹幕越快，本项目档位越大越慢，
   *  故映射为 1/speed；运行时 setSpeed 即时生效，无需重载 */
  speed: number
  /** 同屏弹幕上限（0 = 不限；maxVisible 控制） */
  limit: number
  /** 底部弹幕（源 mode 4 → 6） */
  bottomMode: boolean
  /** 顶部弹幕（源 mode 5） */
  topMode: boolean
  /** 彩色（false = 全部白色） */
  colored: boolean
  /** 描边/阴影 */
  outline: boolean
}

export const DANMAKU_SETTINGS_KEY = 'jc-danmaku-settings'
export const DANMAKU_DEFAULT_SETTINGS: DanmakuSettings = {
  fontSize: 0,
  opacity: 0.8,
  area: 0.5,
  speed: 2,
  // 同屏上限默认 30（2026-10-26: 0=不限时高密度剧集"一窝蜂"铺满; 档位 15/30/50/不限 取 30）
  limit: 30,
  bottomMode: true,
  topMode: true,
  colored: true,
  outline: true
}
/** 字号档位（自适应档 + 4 档可调；B站/腾讯均为档位式） */
export const DANMAKU_FONT_STEPS = [0, 16, 20, 24, 28, 34] as const
/** 显示区域档位（1/4 屏 → 全屏） */
export const DANMAKU_AREA_STEPS = [0.25, 0.5, 0.75, 1] as const
/**
 * 速度档位（数值越大弹幕滚动越慢——与 danmaku-lite 的 speed 方向相反, 映射为 1/speed）：
 * 2026-10-26 修正: 旧档位 [0.5,0.75,1,1.25,1.5] 标签方向反了(0.5 实为最快), 且用户反馈整体太快。
 * 新档 = 旧档 ×2（整体慢一倍）, 标签 极快/快/正常/较慢/慢。运行时 setSpeed 生效, 无需重载。
 */
export const DANMAKU_SPEED_STEPS = [1, 1.5, 2, 2.5, 3] as const
/** 旧速度档位（仅用于 readDanmakuSettings 迁移: 旧值 ×2 映射到新档） */
export const DANMAKU_OLD_SPEED_STEPS = [0.5, 0.75, 1, 1.25, 1.5] as const
/** 同屏上限档位 */
export const DANMAKU_LIMIT_STEPS = [15, 30, 50, 0] as const

/** 单个弹幕请求超时（ms）：整集聚合容错更大（路径A 29段串行最坏6-10s，服务端本身30s），
 *  分片/源列表 15s。过久即丢弃（AbortController 中止请求），调用方标 failed 可重试。
 *  导出为可变对象便于测试注入极小超时。 */
export const DANMAKU_FETCH_TIMEOUTS = {
  source: 15_000,
  segment: 15_000,
  bilibili: 30_000
}

/** 取消类错误判定（AbortController 中止 / axios 超时 ECONNABORTED / axios.isCancel）。 */
function isAbortErr(err: unknown): boolean {
  if (!err || typeof err !== "object") return false
  const e = err as { code?: string; name?: string; __CANCEL__?: unknown }
  return (
    e.code === "ERR_CANCELED" ||
    e.code === "ECONNABORTED" ||
    e.name === "CanceledError" ||
    e.__CANCEL__ !== undefined
  )
}

/** 业务无数据（无匹配集/无分段）哨兵：区别于取数失败，标 empty 而非 failed。 */
class DanmakuEmptyError extends Error {}

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
 * 弹幕 UI 状态（用户要求 2026-10-25: 无源/查无/失败都必须有显示, 不许静默）:
 *   idle         开关关 / 无 mid
 *   loading      查询装载中
 *   unsupported  无候选源（接口失败或平台无此片）—— "该片暂无弹幕"
 *   empty        有候选源但拉不到内容（平台无对应集/分段/内容为空）—— "未找到弹幕"
 *   failed       装载链路异常（请求失败/弹幕引擎未创建）—— "弹幕加载失败", 可重试
 *   ready        已装载, 弹幕可见
 */
export type DanmuState = 'idle' | 'loading' | 'unsupported' | 'empty' | 'failed' | 'ready'

export interface UseDanmakuOptions {
  /** 影片 mid（空 = 无弹幕） */
  mid: Ref<number | undefined>
  /** 集号（1 起；对应我们自己的集序, 服务端按标题匹配平台集） */
  episode: Ref<number>
  /** video.js 实例（ready 前为 null；切集后重拉依赖 episode watch 而非实例） */
  player: ShallowRef<Player | null>
  /** 播放状态（联动 adapter.paused 用） */
  playing: Ref<boolean>
  /** 开关 */
  enabled: Ref<boolean>
  /**
   * 订阅播放器事件（usePlayer.on 的返回签名）。
   * 注意：渲染已由 danmaku-lite 经 adapter 自驱，无需手动 play/pause/seek 联动；
   * 该字段保留以兼容 PlayView 调用约定，当前不再订阅。
   */
  onPlayerEvent: (event: 'play' | 'pause' | 'seeked' | 'timeupdate', fn: () => void) => () => void
}

/** 读取/写入源偏好（localStorage, 与开关分开存） */
function readProviderPref(): string {
  try {
    return localStorage.getItem(DANMAKU_PROVIDER_KEY) || DANMAKU_DEFAULT_PROVIDER
  } catch {
    return DANMAKU_DEFAULT_PROVIDER
  }
}

function writeProviderPref(provider: string): void {
  try {
    localStorage.setItem(DANMAKU_PROVIDER_KEY, provider)
  } catch {
    /* localStorage 不可用时仅本轮生效 */
  }
}

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
  }
}

export function useDanmaku(opts: UseDanmakuOptions) {
  const stage = ref<HTMLElement | null>(null)
  /** 当前影片可用的弹幕源（全平台候选; 无候选时为空列表） */
  const providers = ref<DanmuProvider[]>([])
  /** 当前选中的平台（持久化偏好；候选里没有时回退 bilibili/首个） */
  const provider = ref<string>(readProviderPref())
  /** 是否有弹幕可显示（源存在 && 拉取非空; 供 UI 显示状态） */
  const hasDanmaku = ref(false)
  /** UI 状态机（三态显示的单一来源, 语义见 {@link DanmuState}） */
  const danmuState = ref<DanmuState>('idle')

  /** 弹幕显示设置（localStorage 持久化, 实时应用到引擎） */
  const settings = ref<DanmakuSettings>(readDanmakuSettings())

  /** danmaku-lite 引擎实例（stage 挂载后创建；composable 生命周期内常驻，切集/开关不销毁） */
  let engine: DanmakuEngine | null = null
  /** 容器尺寸变化观察（全屏/移动端旋屏时重算尺寸 + 自适应字号） */
  let resizeObserver: ResizeObserver | null = null

  /** 把可运行时生效的设置项应用到引擎（透明度/区域/速度/上限/字号/描边）。 */
  function applyEngineSettings(): void {
    if (!engine) return
    const s = settings.value
    engine.setArea(s.area)
    engine.setOpacity(s.opacity)
    engine.setSpeed(1 / s.speed) // 档位越大越慢 → 取倒数
    engine.setMaxVisible(s.limit)
    engine.setFontSize(resolveFontSize())
    engine.setStrokeWidth(s.outline ? 1.25 : 0)
    engine.setUseTextShadow(s.outline)
  }

  /** 自适应字号：用户手选固定值优先；否则按 stage 宽度推算（窄屏更小、宽屏更大）。 */
  function resolveFontSize(): number {
    const s = settings.value
    if (s.fontSize > 0) return s.fontSize
    const w = stage.value?.clientWidth ?? 0
    return Math.min(34, Math.max(16, Math.round(w / 64)))
  }

  /** 当前过滤项（底部/顶部/彩色开关），供映射与 dataSource.fetch 共用。 */
  function currentMapFilter(): DanmuMapFilter {
    const s = settings.value
    return { bottomMode: s.bottomMode, topMode: s.topMode, colored: s.colored }
  }

  /** 创建引擎（stage 挂载后调用一次）。adapter 自驱进度，dataSource 自驱腾讯分段预拉。 */
  function ensureEngine(): void {
    if (engine || !stage.value) return
    try {
      engine = createEngine('dom', {
        container: stage.value,
        adapter: {
          get position() {
            return playerSeconds()
          },
          get paused() {
            return !opts.playing.value
          },
          get duration() {
            return opts.player.value?.duration?.() ?? undefined
          }
        },
        area: settings.value.area,
        opacity: settings.value.opacity,
        fontSize: resolveFontSize(),
        speed: 1 / settings.value.speed,
        maxVisible: settings.value.limit,
        overflow: 'drop',
        strokeWidth: settings.value.outline ? 1.25 : 0,
        useTextShadow: settings.value.outline,
        fontFamily: "'Inter', 'PingFang SC', 'Microsoft YaHei', system-ui, sans-serif",
        scrollGap: 96,
        seekThreshold: 0.5,
        // 腾讯分段：引擎按进度窗口自驱 fetch（scheduler.add 追加，不清除屏上弹幕，无闪烁）
        dataSource: { fetch: fetchTencentRange }
      })
      resizeObserver = new ResizeObserver(() => {
        engine?.resize()
        engine?.setFontSize(resolveFontSize())
      })
      resizeObserver.observe(stage.value)
      applyEngineSettings()
    } catch {
      // 引擎创建失败（容器异常等）：降级为 failed 态，不影响播放
      engine = null
    }
  }

  /** 清空当前弹幕（切集/关闭/重载统一入口）。引擎自驱，无需停动画。 */
  function clearDanmaku(): void {
    try {
      engine?.clear()
    } catch {
      /* 引擎异常时静默降级 */
    }
  }

  /** 当前播放秒数（video.js 实例可用时读实时值，否则 0）。 */
  function playerSeconds(): number {
    try {
      const t = opts.player.value?.currentTime()
      return typeof t === 'number' && Number.isFinite(t) ? t : 0
    } catch {
      return 0
    }
  }

  /* ---------------- 腾讯路径 B（分段点播）状态 ---------------- */
  interface TencentCtx {
    episodeId: string
    segments: DanmuTencentSegment[]
  }
  let tencentCtx: TencentCtx | null = null

  /* ---------------- 前端内存缓存（请求去重 2026-10-26） ----------------
   * 目标：同一集内"开关关→开 / 配置(底部·顶部·彩色)变更重载 / seek 回已装载区域 / 来回切集"不再重复发 HTTP。
   * 服务端已有 Redis 缓存；这里再加一层内存缓存：命中直接装载，连缓存命中往返都省掉。
   * 隔离：所有缓存 key 带 mid（+episode+provider），跨集/跨片自动 miss；生命周期 = composable，onScopeDispose 清空。 */
  interface SrcCacheEntry {
    providers: DanmuProvider[]
  }
  const srcCache = new Map<number, SrcCacheEntry>()
  /** B站整集弹幕（key=`${mid}:${episode}`） */
  const biliCache = new Map<string, DanmuComment[]>()
  interface TencentMetaEntry {
    episodeId: string
    segments: DanmuTencentSegment[]
  }
  /** 腾讯 meta（key=`${mid}:${episode}:${provider}`）：匹配集 + 分段索引 */
  const tencentMeta = new Map<string, TencentMetaEntry>()
  /** 腾讯段弹幕数据缓存（key=`${mid}:${episode}:${segmentId}`）：dataSource 重复 fetch 命中不重拉 */
  const segCache = new Map<string, DanmuComment[]>()

  /** 本实例活跃请求控制器集合（卸载/超时统一中止，避免悬挂连接） */
  const activeControllers = new Set<AbortController>()
  /** 源列表 in-flight 去重 */
  let sourceReq: Promise<DanmuProvider[]> | null = null
  /** B站整集 in-flight 去重（key=mid:ep） */
  const biliReq = new Map<string, Promise<DanmuComment[]>>()
  /** 腾讯 meta in-flight 去重（key=mid:ep:provider） */
  const tencentMetaReq = new Map<string, Promise<TencentMetaEntry>>()
  /** 腾讯单段 in-flight 去重（key=mid:ep:segmentId） */
  const segReq = new Map<string, Promise<DanmuComment[]>>()
  /** 已触发内容预取的 (mid:ep) 集合，避免重复后台预热 */
  const prefetchedKeys = new Set<string>()

  function resetTencent(): void {
    tencentCtx = null
  }

  /** 取单段腾讯弹幕（带段缓存 + in-flight 去重 + 15s 超时；失败/超时返回空，不阻断其它段）。 */
  async function getSegmentComments(
    episodeId: string,
    seg: DanmuTencentSegment
  ): Promise<DanmuComment[]> {
    const mid = opts.mid.value
    if (mid === undefined) return []
    const key = `${mid}:${opts.episode.value}:${seg.segmentId}`
    const hit = segCache.get(key)
    if (hit) return hit
    const existing = segReq.get(key)
    if (existing) return existing
    const { promise } = withTimeout(
      (signal) =>
        filmApi.getDanmuTencentComments(
          { episodeId, segmentId: seg.segmentId },
          { signal }
        ),
      DANMAKU_FETCH_TIMEOUTS.segment,
      activeControllers
    )
    const req = promise
      .then((resp) => {
        segReq.delete(key)
        if (!resp?.ok || !resp.data) return [] as DanmuComment[]
        segCache.set(key, resp.data)
        return resp.data
      })
      .catch(() => {
        segReq.delete(key)
        return [] as DanmuComment[] // 单段失败/超时：丢弃该段弹幕，不阻断流式
      })
    segReq.set(key, req)
    return req
  }

  /** 引擎 dataSource.fetch：把 [start,end) 秒窗口映射到重叠的腾讯段，取各段弹幕并按时间过滤。
   *  返回 danmaku-lite 的 DanmakuItem[]（已应用底部/顶部/彩色过滤）；引擎排序后调度。 */
  async function fetchTencentRange(start: number, end: number): Promise<DanmakuItem[]> {
    const ctx = tencentCtx
    if (!ctx) return []
    const myToken = loadToken
    const filter = currentMapFilter()
    const items: DanmakuItem[] = []
    const segs = ctx.segments
    for (let i = 0; i < segs.length; i += 1) {
      const seg = segs[i]!
      if (seg.startTime >= end) break // 段按 startTime 升序，其后皆不重叠
      const nextStart = i + 1 < segs.length ? segs[i + 1]!.startTime : Infinity
      if (nextStart <= start) continue // 该段整体在窗口之前
      const data = await getSegmentComments(ctx.episodeId, seg)
      if (myToken !== loadToken) return [] // 竞态：已切集/重载，丢弃陈旧结果
      for (let j = 0; j < data.length; j += 1) {
        const c = data[j]!
        const tt = c.time / 1000
        if (tt >= start && tt < end) {
          const it = toDanmuInput(c, j, filter)
          if (it) items.push(it)
        }
      }
    }
    return items
  }

  /* ---------------- 装载（按平台分叉） ----------------
   * 顺序铁律: **先落 providers（UI 立刻有所指）, 再建引擎装载内容**。
   * 曾反过来（先建引擎后取源）叠加模板弹幕层 v-if=providers 非空:
   * providers 空 → stage 不渲染 → 引擎建不起来 → providers 永远为空 ⇒
   * 按钮/弹幕层/源切换全不显示（2026-10-25 线上实测, 回归见 useDanmaku.spec.ts）。 */
  /* ---------------- 源/内容预取（缓存去重 + 超时中止） ----------------
   * ensureSource / ensureBilibili / ensureTencentMeta: 取数并落入内存缓存；
   * 同 key 并发复用同一 in-flight Promise（去重）；单请求带超时(整集30s/分片·源15s)，
   * 过久 AbortController 中止（丢弃），由调用方决定标 failed 还是静默降级。
   * prefetchContent: 后台预热内容缓存（开启弹幕或播放中时），首开即命中、瞬时显示。 */
  let loadToken = 0

  /** 取源列表（缓存 + 并发去重）。mid 缺失返回空。 */
  function ensureSource(): Promise<DanmuProvider[]> {
    const mid = opts.mid.value
    if (mid === undefined) return Promise.resolve([])
    const hit = srcCache.get(mid)
    if (hit) return Promise.resolve(hit.providers)
    if (sourceReq) return sourceReq
    const { promise } = withTimeout(
      (signal) => filmApi.getDanmuSource(mid, { signal }),
      DANMAKU_FETCH_TIMEOUTS.source,
      activeControllers
    )
    const req = promise
      .then((resp) => {
        sourceReq = null
        const list = resp?.providers ?? []
        srcCache.set(mid, { providers: list })
        return list
      })
      .catch((err) => {
        sourceReq = null
        throw err
      })
    sourceReq = req
    return req
  }

  /** 取 B站整集弹幕（缓存 + 并发去重 + 30s 超时）。 */
  function ensureBilibili(sel: DanmuProvider): Promise<DanmuComment[]> {
    const mid = opts.mid.value
    if (mid === undefined) return Promise.reject(new Error('no mid'))
    const ep = Math.max(1, opts.episode.value)
    const key = `${mid}:${ep}`
    const hit = biliCache.get(key)
    if (hit) return Promise.resolve(hit)
    const existing = biliReq.get(key)
    if (existing) return existing
    const { promise } = withTimeout(
      (signal) =>
        filmApi.getDanmuBilibili({ seasonId: sel.resourceId, episode: ep }, { signal }),
      DANMAKU_FETCH_TIMEOUTS.bilibili,
      activeControllers
    )
    const req = promise
      .then((resp) => {
        biliReq.delete(key)
        const data = resp?.comments ?? []
        biliCache.set(key, data)
        return data
      })
      .catch((err) => {
        biliReq.delete(key)
        throw err
      })
    biliReq.set(key, req)
    return req
  }

  /** 取腾讯 meta（episodes 匹配 + segments 索引；缓存 + 并发去重 + 15s 超时）。 */
  function ensureTencentMeta(sel: DanmuProvider): Promise<TencentMetaEntry> {
    const mid = opts.mid.value
    if (mid === undefined) return Promise.reject(new Error('no mid'))
    const ep = Math.max(1, opts.episode.value)
    const key = `${mid}:${ep}:${sel.provider}`
    const hit = tencentMeta.get(key)
    if (hit) return Promise.resolve(hit)
    const existing = tencentMetaReq.get(key)
    if (existing) return existing
    const { promise } = withTimeout(
      async (signal) => {
        const eps = await filmApi.getDanmuTencentEpisodes(
          { resourceId: sel.resourceId, episode: ep },
          { signal }
        )
        if (!eps?.ok || !eps.data.matched?.episodeId) {
          throw new DanmakuEmptyError() // 无匹配集 → 标 empty，不判 failed
        }
        const episodeId = eps.data.matched.episodeId
        const segs = await filmApi.getDanmuTencentSegments(episodeId, { signal })
        if (!segs?.ok || segs.data.length === 0) {
          throw new DanmakuEmptyError()
        }
        const entry: TencentMetaEntry = { episodeId, segments: segs.data }
        tencentMeta.set(key, entry)
        return entry
      },
      DANMAKU_FETCH_TIMEOUTS.segment,
      activeControllers
    )
    const req = promise
      .then((entry) => {
        tencentMetaReq.delete(key)
        return entry
      })
      .catch((err) => {
        tencentMetaReq.delete(key)
        throw err
      })
    tencentMetaReq.set(key, req)
    return req
  }

  /** 后台预热内容缓存：开启弹幕或播放中时触发，命中后首开瞬时显示。
   *  仅在 enabled||playing 时拉内容；源列表由 ensureSource 另行预热。 */
  function prefetchContent(): void {
    if (!opts.enabled.value && !opts.playing.value) return
    const mid = opts.mid.value
    if (mid === undefined) return
    const key = `${mid}:${opts.episode.value}`
    if (prefetchedKeys.has(key)) return
    prefetchedKeys.add(key)
    void ensureSource()
      .then((list) => {
        const picked = resolveDanmuProvider(list, provider.value, DANMAKU_DEFAULT_PROVIDER)
        if (!picked) return
        if (picked.provider === 'tencent') {
          void ensureTencentMeta(picked.entry).catch(() => {})
        } else {
          void ensureBilibili(picked.entry).catch(() => {})
        }
      })
      .catch(() => {})
  }

  async function loadDanmaku(): Promise<void> {
    const token = ++loadToken
    clearDanmaku()
    resetTencent()
    hasDanmaku.value = false
    if (!opts.enabled.value || !opts.mid.value) {
      danmuState.value = 'idle'
      return
    }
    danmuState.value = 'loading'
    let list: DanmuProvider[]
    try {
      list = await ensureSource()
    } catch {
      if (token !== loadToken) return
      danmuState.value = 'unsupported' // 源列表取数失败（网络/超时） → 暂无候选
      return
    }
    if (token !== loadToken) return
    providers.value = list
    if (list.length === 0) {
      danmuState.value = 'unsupported'
      return
    }
    ensureEngine()
    if (!engine) {
      danmuState.value = 'failed' // 弹幕引擎未创建
      return
    }
    applyEngineSettings() // 每次装载（首装或 bottom/top/colored 重载）都刷一遍运行时设置
    const picked = resolveDanmuProvider(list, provider.value, DANMAKU_DEFAULT_PROVIDER)
    if (!picked) {
      danmuState.value = 'unsupported'
      return
    }
    if (picked.fellBack) {
      provider.value = picked.provider // 持久化回写，下次进来不再走回退链
    }
    const sel = picked.entry
    if (!sel.resourceId) {
      danmuState.value = 'unsupported'
      return
    }
    if (picked.provider === 'tencent') {
      await loadTencentEpisode(sel, token)
      return
    }
    await loadBilibili(sel, token)
  }

  async function loadBilibili(sel: DanmuProvider, token: number): Promise<void> {
    let commentsData: DanmuComment[]
    try {
      commentsData = await ensureBilibili(sel)
    } catch {
      if (token !== loadToken) return
      danmuState.value = 'failed' // 取数失败/超时 → 可重试
      return
    }
    if (token !== loadToken) return
    const items = toDanmuInputs(commentsData, currentMapFilter())
    if (items.length === 0) {
      danmuState.value = 'empty'
      return
    }
    engine?.load(items)
    hasDanmaku.value = true
    danmuState.value = 'ready'
  }

  async function loadTencentEpisode(sel: DanmuProvider, token: number): Promise<void> {
    let entry: TencentMetaEntry
    try {
      entry = await ensureTencentMeta(sel)
    } catch (e) {
      if (token !== loadToken) return
      if (e instanceof DanmakuEmptyError) {
        danmuState.value = 'empty'
      } else {
        danmuState.value = 'failed' // 取数失败/超时 → 可重试
      }
      return
    }
    if (token !== loadToken) return
    tencentCtx = { episodeId: entry.episodeId, segments: entry.segments }
    // 引擎经 dataSource 按进度自驱预拉；此处只需确认有段可拉即可标 ready。
    hasDanmaku.value = true
    danmuState.value = 'ready'
  }

  /** 用户手动切换弹幕源（UI 调用） */
  function setProvider(next: string): void {
    if (next === provider.value) {
      return
    }
    provider.value = next
    writeProviderPref(next)
    void loadDanmaku()
  }

  /** 更新弹幕显示设置（局部 patch）。
   *  - 底部/顶部/彩色 命中 → 重新映射装载（bilibili 重 load；tencent 清+重流式，均命中缓存零 HTTP）
   *  - 其余（透明度/区域/速度/字号/描边/上限）→ 引擎运行时 setX 即时生效，无需重载 */
  function updateSettings(patch: Partial<DanmakuSettings>): void {
    const next = { ...settings.value, ...patch }
    settings.value = next
    writeDanmakuSettings(next)
    const affectsContent =
      patch.bottomMode !== undefined ||
      patch.topMode !== undefined ||
      patch.colored !== undefined
    if (affectsContent) {
      void loadDanmaku()
    } else {
      applyEngineSettings()
    }
  }

  /* ---- 开关 / 集号 / 源变化 / stage 挂载（Teleport 后才有值, 纳入触发条件） ---- */
  watch(
    () => [opts.enabled.value, opts.playing.value, opts.mid.value, opts.episode.value, !!stage.value] as const,
    () => {
      // 后台预热：源列表必拉（便宜）；内容在开启弹幕或播放中时预拉，首开即命中缓存瞬时显示
      void ensureSource().catch(() => {})
      prefetchContent()
      void loadDanmaku()
    }
  )

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
    clearDanmaku()
    resizeObserver?.disconnect()
    engine?.destroy()
    engine = null
    resetTencent()
    srcCache.clear()
    biliCache.clear()
    tencentMeta.clear()
    segCache.clear()
    sourceReq = null
    biliReq.clear()
    tencentMetaReq.clear()
    segReq.clear()
    prefetchedKeys.clear()
  })

  return {
    stage,
    providers,
    provider,
    setProvider,
    hasDanmaku,
    danmuState,
    settings,
    updateSettings,
    reload: () => void loadDanmaku()
  }
}

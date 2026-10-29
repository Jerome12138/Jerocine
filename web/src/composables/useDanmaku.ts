import {
  onScopeDispose,
  ref,
  watch,
  type Ref,
  type ShallowRef
} from 'vue'
import type Artplayer from 'artplayer'
import { createEngine, type DanmakuEngine, type DanmakuItem } from 'danmaku-lite/canvas'
import { SETTINGS_APPLIED_EVENT } from '@/utils/settingEvents'
import { filmApi } from '@/api'
import { ApiError } from '@/types/api'
import type { DanmuComment } from '@/types/film'
import { toDanmuInputs, type DanmuMapFilter } from '@/utils/danmu'

/**
 * 弹幕（D3）—— 渲染层 danmaku-lite + 数据层聚合弹幕服务（2026-10-30）：
 *
 *  - 数据链路：影片名 + 集号 → GET /danmu/aggregate（服务端按标题匹配作品 → 定位集 →
 *    整集聚合弹幕，两段缓存 7d）→ 一次拉整集。
 *    404 = 查无作品/该集（标 unsupported「该片暂无弹幕」）；空列表 = 标 empty；其余错误 = failed 可重试。
 *  - 渲染引擎：danmaku-lite（Canvas 引擎），叠加在播放器容器内独立弹幕层。
 *    引擎**自驱**：通过 PlayerAdapter（position/paused/duration）每帧读进度，无需手动 time/start/stop/seek。
 *    - 重叠：内置轨道分配器（scrollGap / maxVisible 控制密度）
 *    - 截断/被挡：overflow:'drop'（无空闲轨道时静默丢弃，不再挤到屏外截断）
 *    - 拖动进度一窝蜂：引擎按 seekThreshold 检测大跳，重置后从新位置按时间重发（非整段补发）
 *  - 显示设置持久化：jc-danmaku-settings（字号/透明度/区域/速度/上限/底部/顶部/彩色/描边）。
 */

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
  // 速度默认 1（"极快"档 = danmaku speed 1.0, 8s 走完屏）：用户反馈 1.5(12s) 仍偏慢
  speed: 1,
  // 同屏上限默认 30（2026-10-26: 0=不限时高密度剧集"一窝蜂"铺满; 档位 15/30/50/不限 取 30 = 中）
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
/** 速度档→引擎倍率系数：2026-10-26 用户要求所有档位整体再提一档（每档都快约 40%）,
 *  映射从 1/speed 改为 1.4/speed（默认档 1 → 引擎 1.4 倍, 约 5.7s 走完屏）。 */
export const DANMAKU_SPEED_FACTOR = 1.4
/** 档位值 → danmaku-lite 滚动速度倍率（档位数值越大越慢, 与引擎方向相反）。 */
export function danmakuSpeedOf(speed: number): number {
  return DANMAKU_SPEED_FACTOR / speed
}
/** 同屏上限档位 */
export const DANMAKU_LIMIT_STEPS = [15, 30, 50, 0] as const

/** 单个弹幕请求超时（ms）：整集聚合容错更大（服务端聚合最坏数秒 + 4MB 级响应）。
 *  导出为可变对象便于测试注入极小超时。 */
export const DANMAKU_FETCH_TIMEOUTS = {
  aggregate: 30_000
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
 *   failed       装载链路异常（请求失败/超时/引擎未创建） —— "弹幕加载失败", 可重试
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
  /** 播放器实例（ready 前为 null；adapter 每帧读进度） */
  player: ShallowRef<Artplayer | null>
  /** 播放状态（联动 adapter.paused 用） */
  playing: Ref<boolean>
  /** 开关 */
  enabled: Ref<boolean>
}

export function useDanmaku(opts: UseDanmakuOptions) {
  const stage = ref<HTMLElement | null>(null)
  /** 是否有弹幕可显示（装载非空; 供 UI 显示状态） */
  const hasDanmaku = ref(false)
  /** UI 状态机（三态显示的单一来源, 语义见 {@link DanmuState}） */
  const danmuState = ref<DanmuState>('idle')

  /** 弹幕显示设置（localStorage 持久化, 实时应用到引擎） */
  const settings = ref<DanmakuSettings>(readDanmakuSettings())

  /** danmaku-lite 引擎实例（stage 挂载后创建；composable 生命周期内常驻，切集/开关不销毁） */
  let engine: DanmakuEngine | null = null
  /** 容器尺寸变化观察（全屏/移动端旋屏时重算尺寸 + 自适应字号） */
  let resizeObserver: ResizeObserver | null = null
  /** 最近一次应用到引擎的字号（ResizeObserver 去抖用） */
  let lastFontSize = 0

  /** 把可运行时生效的设置项应用到引擎（透明度/区域/速度/上限/字号/描边）。 */
  function applyEngineSettings(): void {
    if (!engine) return
    const s = settings.value
    engine.setArea(s.area)
    engine.setOpacity(s.opacity)
    engine.setSpeed(danmakuSpeedOf(s.speed))
    engine.setMaxVisible(s.limit)
    lastFontSize = resolveFontSize()
    engine.setFontSize(lastFontSize)
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

  /** 当前过滤项（底部/顶部/彩色开关）。 */
  function currentMapFilter(): DanmuMapFilter {
    const s = settings.value
    return { bottomMode: s.bottomMode, topMode: s.topMode, colored: s.colored }
  }

  /** 创建引擎（stage 挂载后调用一次）。adapter 自驱进度。 */
  function ensureEngine(): void {
    if (engine || !stage.value) return
    try {
      engine = createEngine('canvas', { // Canvas 引擎: 500+ 并发/原生 strokeText 描边/位图缓存, 低端设备更顺滑(DOM 引擎 <200 条)
        container: stage.value,
        adapter: {
          get position() {
            return playerSeconds()
          },
          get paused() {
            return !opts.playing.value
          },
          get duration() {
            return opts.player.value?.duration ?? undefined
          }
        },
        area: settings.value.area,
        opacity: settings.value.opacity,
        fontSize: (lastFontSize = resolveFontSize()),
        speed: danmakuSpeedOf(settings.value.speed),
        maxVisible: settings.value.limit,
        overflow: 'drop',
        strokeWidth: settings.value.outline ? 1.25 : 0,
        // useTextShadow 仅 DOM 引擎有效, Canvas 用原生 strokeText, 已移除
        fontFamily: "'Inter', 'PingFang SC', 'Microsoft YaHei', system-ui, sans-serif",
        // 滚动弹幕最小纵向间距：加大以降低同屏密集重叠；配合 overflow:'drop' 无空闲轨道时静默丢弃
        scrollGap: 140,
        seekThreshold: 0.5
      })
      resizeObserver = new ResizeObserver(() => {
        engine?.resize()
        // 字号去抖: 仅在自适应字号实际变化时才 set, 避免容器尺寸微抖引发频繁重排(视觉抖动)
        const fs = resolveFontSize()
        if (fs !== lastFontSize) {
          lastFontSize = fs
          engine?.setFontSize(fs)
        }
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

  /** 当前播放秒数（播放器实例可用时读实时值，否则 0）。 */
  function playerSeconds(): number {
    try {
      const t = opts.player.value?.currentTime
      return typeof t === 'number' && Number.isFinite(t) ? t : 0
    } catch {
      return 0
    }
  }

  /* ---------------- 取数（内存缓存 + in-flight 去重 + 超时中止） ----------------
   * 服务端已有 Redis 缓存；这里再加一层内存缓存：命中直接装载，连缓存命中往返都省掉。
   * 隔离：缓存 key 带 mid+episode，跨集/跨片自动 miss；生命周期 = composable，onScopeDispose 清空。 */
  /** 整集弹幕（key=`${mid}:${episode}`） */
  const aggCache = new Map<string, DanmuComment[]>()
  /** 整集 in-flight 去重 */
  const aggReq = new Map<string, Promise<DanmuComment[]>>()
  /** 本实例活跃请求控制器集合（卸载/超时统一中止，避免悬挂连接） */
  const activeControllers = new Set<AbortController>()

  /** 取整集聚合弹幕（缓存 + 并发去重 + 30s 超时）。 */
  function ensureAggregate(): Promise<DanmuComment[]> {
    const mid = opts.mid.value
    if (mid === undefined) return Promise.reject(new Error('no mid'))
    const ep = Math.max(1, opts.episode.value)
    const key = `${mid}:${ep}`
    const hit = aggCache.get(key)
    if (hit) return Promise.resolve(hit)
    const existing = aggReq.get(key)
    if (existing) return existing
    const { promise } = withTimeout(
      (signal) =>
        filmApi.getDanmuAggregate(
          { title: opts.title.value, episode: ep, year: opts.year.value },
          { signal }
        ),
      DANMAKU_FETCH_TIMEOUTS.aggregate,
      activeControllers
    )
    const req = promise
      .then((resp) => {
        aggReq.delete(key)
        const data = resp?.comments ?? []
        aggCache.set(key, data)
        return data
      })
      .catch((err) => {
        aggReq.delete(key)
        throw err
      })
    aggReq.set(key, req)
    return req
  }

  /* ---------------- 装载 ----------------
   * 顺序铁律: **先建引擎再装载内容**（stage 由模板常驻渲染, 引擎创建不依赖取数结果）。 */
  let loadToken = 0

  /** 当前集 key（`mid:episode`）；关→开同集即时恢复用（不重新拉取装载） */
  function currentLoadKey(): string | null {
    const mid = opts.mid.value
    return mid === undefined ? null : `${mid}:${opts.episode.value}`
  }
  /** 已成功装载内容的集 key；null = 尚未装载/需重装 */
  let loadedKey: string | null = null
  /** 最近一次装载的原始弹幕（顶部/底部/彩色切换时内存重映射, 零请求零重载） */
  let lastComments: DanmuComment[] | null = null

  async function loadDanmaku(force = false): Promise<void> {
    const key = currentLoadKey()
    console.log('[danmaku] loadDanmaku', { force, enabled: opts.enabled.value, key, loadedKey })
    const token = ++loadToken
    if (!opts.enabled.value || !key) {
      // 关闭: 仅停绘(引擎 enabled=false 保留数据), 不清内容不重置 —— 再开同集即时恢复显示
      engine?.setEnabled(false)
      danmuState.value = 'idle'
      return
    }
    if (!force && loadedKey === key && engine) {
      // 同集已装载(关→开): 直接恢复绘制, 即时显示(避免 clear+重拉造成"开弹幕等好久")
      console.log('[danmaku] resume loaded content, no reload:', key)
      engine.setEnabled(true)
      hasDanmaku.value = true
      danmuState.value = 'ready'
      return
    }
    if (!opts.title.value) {
      danmuState.value = 'unsupported'
      return
    }
    clearDanmaku()
    hasDanmaku.value = false
    danmuState.value = 'loading'
    ensureEngine()
    if (!engine) {
      danmuState.value = 'failed' // 弹幕引擎未创建
      return
    }
    applyEngineSettings() // 每次装载（首装或 bottom/top/colored 重载）都刷一遍运行时设置
    let commentsData: DanmuComment[]
    try {
      commentsData = await ensureAggregate()
    } catch (e) {
      if (token !== loadToken) return
      // 404 = 查无作品/该集 → "该片暂无弹幕"；其余（网络/超时/服务端）→ failed 可重试
      danmuState.value = e instanceof ApiError && e.status === 404 ? 'unsupported' : 'failed'
      return
    }
    if (token !== loadToken) return
    const items = toDanmuInputs(commentsData, currentMapFilter())
    if (items.length === 0) {
      danmuState.value = 'empty'
      return
    }
    engine.load(items)
    lastComments = commentsData
    loadedKey = currentLoadKey()
    hasDanmaku.value = true
    danmuState.value = 'ready'
    console.log('[danmaku] aggregate loaded:', items.length, 'items, key =', loadedKey)
  }

  /** 更新弹幕显示设置（局部 patch）。
   *  - 底部/顶部/彩色 命中 → 原始数据在内存, 直接重映射装载（零请求）
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
      // 顶部/底部/彩色切换: 整集原始数据在内存, 直接重映射+load, 零请求、不清状态机(即时生效)
      if (
        engine &&
        lastComments &&
        loadedKey !== null &&
        loadedKey === currentLoadKey()
      ) {
        const items = toDanmuInputs(lastComments, currentMapFilter())
        engine.load(items)
        applyEngineSettings() // 混合 patch(如同时改描边)也要应用运行时设置
        console.log('[danmaku] in-memory remap (bottom/top/colored):', items.length, 'items')
      } else {
        loadedKey = null // 内容映射参数变了, 已装载内容失效, 重开需重装
        void loadDanmaku(true)
      }
    } else {
      applyEngineSettings()
    }
  }

  /* ---- 开关 / 集号 / 影片 / stage 挂载（Teleport 后才有值, 纳入触发条件） ---- */
  watch(
    () => [opts.enabled.value, opts.playing.value, opts.mid.value, opts.episode.value, opts.title.value, !!stage.value] as const,
    (cur, prev) => {
      // 播放/暂停切换: 引擎经 adapter.paused 自停/自走(冻结不清屏), 这里绝不重载 ——
      // 否则 loadDanmaku 的 clearDanmaku 会把屏上弹幕清空(表现为"暂停弹幕消失")。
      if (
        prev &&
        cur[0] === prev[0] && cur[2] === prev[2] && cur[3] === prev[3] &&
        cur[4] === prev[4] && cur[5] === prev[5] &&
        cur[1] !== prev[1]
      ) {
        return
      }
      void loadDanmaku()
    }
  )
  /** 诊断日志: 弹幕状态机每次切换都打印(便于排查"弹幕长期不出现") */
  watch(danmuState, (to, from) => {
    console.log('[danmaku] state:', from, '→', to)
  })
  /** 云端设置合并到此设备(useCloudSettings 广播): 重读设置; 底部/顶部/彩色类变更需重载映射 */
  const onCloudApplied = (): void => {
    const prev = { ...settings.value }
    settings.value = readDanmakuSettings()
    applyEngineSettings()
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
    clearDanmaku()
    resizeObserver?.disconnect()
    engine?.destroy()
    engine = null
    aggCache.clear()
    aggReq.clear()
    loadedKey = null
    lastComments = null
    window.removeEventListener(SETTINGS_APPLIED_EVENT, onCloudApplied)
  })

  return {
    stage,
    hasDanmaku,
    danmuState,
    settings,
    updateSettings,
    reload: () => void loadDanmaku(true)
  }
}

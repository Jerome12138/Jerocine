import { onScopeDispose, ref, watch, type Ref, type ShallowRef } from 'vue'
import type Artplayer from 'artplayer'
import type { Danmu } from 'artplayer-plugin-danmuku'
import { ApiError } from '@/types/api'
import { filmApi } from '@/api'
import type { DanmuComment } from '@/types/film'
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
  /** 描边（插件 Danmu.border） */
  outline: boolean
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
  outline: true,
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
  /** 播放状态（插件自驱跟随, 这里仅作语义保留/测试桩用） */
  playing: Ref<boolean>
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

  /** 当前过滤项（底部/顶部/彩色/描边开关）。 */
  function currentMapFilter(): DanmuMapFilter {
    const s = settings.value
    return { bottomMode: s.bottomMode, topMode: s.topMode, colored: s.colored, outline: s.outline }
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
  /** 整集弹幕（key=`${mid}:${episode}`） */
  const aggCache = new Map<string, DanmuComment[]>()
  /** FIFO 上限: 整集弹幕可到 MB 级, 长剧来回切集防内存膨胀 */
  const AGG_CACHE_MAX = 8
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
        // FIFO 淘汰最旧集（当前 key 刚 set 置于末尾, 淘汰从头开始且跳过当前 key）
        if (aggCache.size > AGG_CACHE_MAX) {
          const first = aggCache.keys().next().value
          if (first !== undefined && first !== key) aggCache.delete(first)
        }
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
   * 插件在 usePlayer 构造时注册（控制条开关/设置面板随之常驻）; 这里只喂数据与显隐。 */
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
    let commentsData: DanmuComment[]
    try {
      commentsData = await ensureAggregate()
    } catch (e) {
      if (token !== loadToken) return
      // 404 = 查无作品/该集 → "该片暂无弹幕"；其余（网络/超时/服务端）→ failed 可重试
      danmuState.value =
        e instanceof ApiError && e.status === 404 ? 'unsupported' : 'failed'
      return
    }
    if (token !== loadToken) return
    const items = toDanmuInputs(commentsData, currentMapFilter())
    if (items.length === 0) {
      danmuState.value = 'empty'
      return
    }
    try {
      await p.load(items)
    } catch {
      if (token !== loadToken) return
      danmuState.value = 'failed'
      return
    }
    lastComments = commentsData
    loadedKey = currentLoadKey()
    hasDanmaku.value = true
    danmuState.value = 'ready'
    if (opts.enabled.value) p.show()
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
      patch.colored !== undefined ||
      patch.outline !== undefined
    if (affectsContent) {
      // 顶部/底部/彩色/描边切换: 整集原始数据在内存, 直接重映射+load, 零请求、不清状态机(即时生效)
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
      }
      if (!art) return
      try {
        art.on('artplayerPluginDanmuku:config', onPluginConfigHandler)
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
      prev.colored !== settings.value.colored ||
      prev.outline !== settings.value.outline
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

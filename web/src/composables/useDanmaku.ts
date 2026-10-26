import {
  onScopeDispose,
  ref,
  watch,
  type Ref,
  type ShallowRef
} from 'vue'
import type Player from 'video.js/dist/types/player'
import { filmApi } from '@/api'
import type { DanmuComment, DanmuProvider, DanmuTencentSegment } from '@/types/film'
import { prefetchWindow, resolveDanmuProvider, shouldPrefetchNext, toDanmuInputs } from '@/utils/danmu'

/**
 * 弹幕（D3）—— 方案 §8.2 双路径：
 *
 *  - 数据链路：mid → GET /films/:mid/danmu/source（平台候选, 不暴露 db_id）
 *    → 选中平台：
 *      - bilibili（路径 A 整集聚合）：seasonId → GET /danmu/bilibili 一次拉整集
 *      - tencent（路径 B 分段点播）：resourceId(cid) → GET /danmu/tencent/episodes
 *        拿 matched.episodeId 复合串 → GET /danmu/tencent/segments 分段索引(startTime 秒)
 *        → 按 currentTime 预拉"当前段 + 后续 2 段"（GET /danmu/tencent/comments）
 *  - 播放器：不换引擎, 在 video.js 容器内叠加独立弹幕层（comment-core-library,
 *    DOM/CSS 渲染 div.cmt）。只通过 play/pause/seeked/切集事件联动, 不侵入引擎。
 *  - 联动契约：
 *      play  → manager.start()   （动画计时恢复）
 *      pause → manager.stop()    （动画暂停）
 *      seeked→ clear + seek(t) + time(t) 重放到目标时刻；腾讯路径另重置分段游标重新预拉
 *      timeupdate → time(t)      投递 t 秒前应出现的弹幕；腾讯路径顺带做跨段预拉
 *      ratechange：库无倍速概念, 不做速率补偿（倍速下弹幕相对视频时间漂移, 可接受）
 *
 * 开关与源偏好持久化：开关走 useWebSettings（localStorage `jc-danmaku`, 默认开, 与 /settings 共用）；
 * 源偏好 `jc-danmaku-provider`（默认 bilibili, D3 起可选 tencent）。
 */

export const DANMAKU_PROVIDER_KEY = 'jc-danmaku-provider'
export const DANMAKU_DEFAULT_PROVIDER = 'bilibili'

/* ---------------- 弹幕显示设置（2026-10-26, 参考 B站/腾讯设置面板） ---------------- */

export interface DanmakuSettings {
  /** 字号(px)。0 = 自适应：非全屏小号 / 全屏大号 / 移动端更小（见 PlayView CSS --jc-danmu-size） */
  fontSize: number
  /** 弹幕层整体透明度 0-1 */
  opacity: number
  /** 显示区域高度占比 0-1（0.25/0.5/0.75/1 对应 1/4 屏·半屏·3/4 屏·全屏） */
  area: number
  /** 滚动速度倍率（>1 快、<1 慢；库 dur *= scroll.scale，已渲染弹幕不变，变更触发重载） */
  speed: number
  /** 同屏弹幕上限（0 = 不限；库 time() 用 runline+本次数量判断） */
  limit: number
  /** 底部弹幕（mode 4） */
  bottomMode: boolean
  /** 顶部弹幕（mode 5） */
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
  limit: 0,
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
 * 速度档位（= 库 dur 倍率, **数值越大弹幕滚动越慢**——ScrollComment 构造时
 * dur *= scroll.scale, dur 大 = 穿越时间久 = 慢）。
 * 2026-10-26 修正: 旧档位 [0.5,0.75,1,1.25,1.5] 标签方向反了(0.5 实为最快),
 * 且用户反馈整体太快。新档 = 旧档 ×2（整体慢一倍）, 标签 极快/快/正常/较慢/慢。
 */
export const DANMAKU_SPEED_STEPS = [1, 1.5, 2, 2.5, 3] as const
/** 旧速度档位（仅用于 readDanmakuSettings 迁移: 旧值 ×2 映射到新档） */
export const DANMAKU_OLD_SPEED_STEPS = [0.5, 0.75, 1, 1.25, 1.5] as const
/** 同屏上限档位 */
export const DANMAKU_LIMIT_STEPS = [15, 30, 50, 0] as const

/**
 * 弹幕 UI 状态（用户要求 2026-10-25: 无源/查无/失败都必须有显示, 不许静默）:
 *   idle         开关关 / 无 mid
 *   loading      查询装载中
 *   unsupported  无候选源（接口失败或平台无此片）—— "该片暂无弹幕"
 *   empty        有候选源但拉不到内容（平台无对应集/分段/内容为空）—— "未找到弹幕"
 *   failed       装载链路异常（请求失败/弹幕库未加载）—— "弹幕加载失败", 可重试
 *   ready        已装载, 弹幕可见
 */
export type DanmuState = 'idle' | 'loading' | 'unsupported' | 'empty' | 'failed' | 'ready'
/** 路径 B 预拉窗口：当前段 + 后续 3 段（2026-10-26 从 3 提到 4：腾讯段较短时跨段空档更少, 单请求 3s 短超时+内容缓存, 成本可控） */
const TENCENT_PREFETCH_SPAN = 4
/** 预拉提前量（秒）：距下段边界 <1s 就开始拉, 避免边界卡顿 */
const TENCENT_PREFETCH_LEAD = 1

export interface UseDanmakuOptions {
  /** 影片 mid（空 = 无弹幕） */
  mid: Ref<number | undefined>
  /** 集号（1 起；对应我们自己的集序, 服务端按标题匹配平台集） */
  episode: Ref<number>
  /** video.js 实例（ready 前为 null；切集后重拉依赖 episode watch 而非实例） */
  player: ShallowRef<Player | null>
  /** 播放状态（联动 start/stop 用） */
  playing: Ref<boolean>
  /** 开关 */
  enabled: Ref<boolean>
  /** 订阅播放器事件（usePlayer.on 的返回签名） */
  onPlayerEvent: (event: 'play' | 'pause' | 'seeked' | 'timeupdate', fn: () => void) => () => void
}

/** 取全局库（index.html 已加载 script）。加载失败返回 null（弹幕静默降级, 不影响播放）。
 *  ⚠️ 官方 dist(0.11.0, 与 npm 包逐字节同源)暴露的是**顶层 var CommentManager**,
 *  并不存在 `window.CommentCoreLibrary` 聚合对象 —— 首版按聚合对象读导致弹幕库
 *  "加载成功却永远取不到"(线上 2026-10-26 实测, 全部影片归因 failed)。 */
function library(): CommentCoreLibraryLike | null {
  if (typeof window === 'undefined') {
    return null
  }
  const CM = window.CommentManager
  return CM ? { CommentManager: CM } : null
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
      limit: typeof p.limit === 'number' && p.limit >= 0 ? p.limit : 0
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

  /** 弹幕显示设置（localStorage 持久化, 实时应用到 manager/CSS） */
  const settings = ref<DanmakuSettings>(readDanmakuSettings())

  let manager: CommentManagerLike | null = null
  /** 白色模式改写器（filter.addModifier 累加, 保存引用以便移除/替换） */
  let colorModifier: ((c: DanmakuInputComment) => DanmakuInputComment) | null = null

  /** 把 settings 应用到 manager 与 stage（新弹幕生效项 + CSS 实时项）。
   *  ensureManager 后与 updateSettings 时调用。 */
  function applySettings(): void {
    if (!manager || !stage.value) {
      return
    }
    const s = settings.value
    // CSS 实时项（整层/全部弹幕节点立即生效）
    stage.value.style.opacity = String(s.opacity)
    stage.value.style.height = `${Math.round(s.area * 100)}%`
    if (s.fontSize > 0) {
      stage.value.style.setProperty('--jc-danmu-size', `${s.fontSize}px`)
    } else {
      stage.value.style.removeProperty('--jc-danmu-size') // 自适应（随全屏/移动端）
    }
    stage.value.classList.toggle('jc-danmaku-stage--no-outline', !s.outline)
    // 库配置项（新弹幕生效）
    manager.options.scroll = { ...manager.options.scroll, scale: s.speed }
    manager.options.limit = s.limit
    manager.filter = manager.filter ?? {}
    manager.filter.allowTypes = {
      1: true, // 滚动
      2: true, // 底部滚动（反向）
      6: true, // 反向滚动
      4: s.bottomMode,
      5: s.topMode,
      7: false,
      8: false,
      17: false
    }
    // 彩色/白色：modifier 改写每条 color（先移除旧的再添加, 防累加）
    if (colorModifier) {
      manager.filter.removeModifier?.(colorModifier)
      colorModifier = null
    }
    if (!s.colored) {
      colorModifier = (c) => {
        c.color = 0xffffff
        return c
      }
      manager.filter.addModifier?.(colorModifier)
    }
  }

  /** 更新弹幕显示设置（局部 patch）；speed 变更触发重载（已渲染弹幕 dur 固定） */
  function updateSettings(patch: Partial<DanmakuSettings>): void {
    const next = { ...settings.value, ...patch }
    const speedChanged = patch.speed !== undefined && patch.speed !== settings.value.speed
    settings.value = next
    writeDanmakuSettings(next)
    applySettings()
    if (speedChanged) {
      void loadDanmaku()
    }
  }

  /** 重建 manager（stage 挂载后 / 切集重拉前） */
  function ensureManager(): void {
    if (manager || !stage.value) {
      return
    }
    const lib = library()
    if (!lib) {
      return
    }
    manager = new lib.CommentManager(stage.value)
    manager.init('css')
    applySettings()
  }

  /** 清空当前弹幕并停掉动画（切集/关闭/卸载统一入口） */
  function clearDanmaku(): void {
    try {
      manager?.stop()
      manager?.clear()
    } catch {
      /* 库内部状态异常时静默降级 */
    }
  }

  /** 装载对齐：load 后强制 seek 到当前播放秒再 time。
   *  ⚠️ 不能只调 time(t)：弹幕装载（异步拉取）期间 timeupdate 已经把
   *  lastPosition 推进到当前播放时间，load 完成后 time(t) 的 |lastPosition-t|
   *  会 < seekTrigger(2000ms) 而**不触发 seek** → 从 position=0 起把整集/已装载
   *  段落里所有 stime<=t 的弹幕一次性补发（2026-10-26 线上截图"一窝蜂"）。
   *  seek 先把 position 定位到 stime>=t-1，time 只补发边界那条，之后随播放推进逐条出现。 */
  function alignManager(t: number): void {
    try {
      manager?.seek(t)
      manager?.time(t)
    } catch {
      /* 库状态异常时静默降级 */
    }
  }

  /** 当前播放秒数（video.js 实例可用时读实时值） */
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
  /** 已装载段标记（seek 会 clear 后重装；只决定"装不装", 不决定"拉不拉"） */
  const loadedSegs = new Set<string>()

  /* ---------------- 前端内存缓存（请求去重 2026-10-26） ----------------
   * 目标：同一集内"开关关→开 / 配置(速度)变更重载 / seek 回已装载区域 / 来回切集"不再重复发 HTTP。
   * 服务端已有 Redis 缓存（DanmuSource 正7d/负2h、B站整集、腾讯段 v1:danmu:seg:* 正7d/负2h），
   * 这里再加一层内存缓存：命中直接装载，连缓存命中往返都省掉。
   * 隔离：所有缓存 key 带 mid（+episode+provider），跨集/跨片自动 miss，不存在装错集；
   * 生命周期 = composable（播放页会话），onScopeDispose 统一清空。 */
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
  /** 腾讯段弹幕数据缓存（key=`${mid}:${episode}:${segmentId}`）：seek 后 loadedSegs.clear 重装时命中不重拉 */
  const segCache = new Map<string, DanmuComment[]>()

  function resetTencent(): void {
    tencentCtx = null
    loadedSegs.clear()
  }

  /** 从 t 所在段起装载"当前段 + 后续 2 段"（已装载的跳过；token 竞态保护）。
   *  返回本窗口是否有请求失败（内容空 ≠ 失败, 供上层区分 empty/failed 态）。 */
  async function loadTencentFrom(t: number, token: number): Promise<boolean> {
    if (!tencentCtx) {
      return false
    }
    const { episodeId, segments } = tencentCtx
    const { start, end } = prefetchWindow(segments, t, TENCENT_PREFETCH_SPAN)
    let anyFail = false
    for (let i = start; i < end; i += 1) {
      const seg = segments[i]
      if (!seg || loadedSegs.has(seg.segmentId)) {
        continue
      }
      // 段数据缓存命中 ⇒ 直接装载（seek 回已装区域 / 配置切换重载时零请求）
      let data = segCache.get(`${opts.mid.value}:${opts.episode.value}:${seg.segmentId}`)
      if (data === undefined) {
        const resp = await filmApi
          .getDanmuTencentComments({ episodeId, segmentId: seg.segmentId })
          .catch(() => null)
        if (token !== loadToken) {
          return false
        }
        if (!resp?.ok || !resp.data) {
          anyFail = true
          continue
        }
        data = resp.data
        segCache.set(`${opts.mid.value}:${opts.episode.value}:${seg.segmentId}`, data)
      }
      const input = toDanmuInputs(data)
      if (input.length > 0) {
        manager?.load(input)
        // 段 load 会整体替换 timeline（position 失效）→ 每段装载后立即对齐，
        // 否则下段 load 后 position 指向新 timeline 的错误位置（漏发或补发）。
        alignManager(playerSeconds())
      }
      loadedSegs.add(seg.segmentId)
    }
    return anyFail
  }

  /** 跨段预拉：距下段边界 < 提前量且未装载时，从当前时间起再拉一段窗口 */
  function maybePrefetchTencent(): void {
    if (!tencentCtx || loadedSegs.size === 0) {
      return
    }
    const t = playerSeconds()
    if (!shouldPrefetchNext(tencentCtx.segments, t, TENCENT_PREFETCH_LEAD, loadedSegs)) {
      return
    }
    void loadTencentFrom(t, loadToken)
  }

  /* ---------------- 装载（按平台分叉） ---------------- */

  let loadToken = 0
  /**
   * 拉取并装载当前集弹幕（token 防竞态：快速切集时旧请求结果不落地）。
   *
   * 顺序铁律: **先落 providers（UI 立刻有所指）, 再建 manager 装载内容**。
   * 曾反过来（先 ensureManager 后取源）叠加模板弹幕层 v-if=providers 非空:
   * providers 空 → stage 不渲染 → manager 建不起来 → providers 永远为空 ⇒
   * 按钮/弹幕层/源切换全不显示（2026-10-25 线上实测, 回归见 useDanmaku.spec.ts）。
   */
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
    const mid = opts.mid.value
    // 源候选缓存命中 ⇒ 零请求（同一集开关/配置切换重载时复用）
    let list: DanmuProvider[]
    const srcHit = mid !== undefined ? srcCache.get(mid) : undefined
    if (srcHit) {
      list = srcHit.providers
    } else {
      const src = await filmApi.getDanmuSource(mid).catch(() => null)
      if (token !== loadToken) {
        return
      }
      if (!src) {
        // 接口失败也按"暂无弹幕"显示 —— 任何查不到弹幕的情况都要有明确反馈, 不许静默
        providers.value = []
        danmuState.value = 'unsupported'
        return
      }
      list = src.providers ?? []
      if (mid !== undefined) {
        srcCache.set(mid, { providers: list })
      }
    }
    providers.value = list
    if (list.length === 0) {
      danmuState.value = 'unsupported'
      return
    }
    ensureManager()
    if (!manager) {
      danmuState.value = 'failed' // 弹幕库(window.CommentCoreLibrary)未加载
      return
    }
    // 选中平台：偏好优先；不在候选里时回退 bilibili → 首个（纯函数 resolveDanmuProvider，逐分支单测覆盖）
    const picked = resolveDanmuProvider(list, provider.value, DANMAKU_DEFAULT_PROVIDER)
    if (!picked) {
      danmuState.value = 'unsupported' // list 非空（上方已 return），双保险
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

  /** 路径 A：B站整集聚合（一次拉全, 服务端 29 段串行） */
  async function loadBilibili(sel: DanmuProvider, token: number): Promise<void> {
    // loadDanmaku 前置已保证 opts.mid.value 非空（!opts.mid.value 分支 return）
    const mid = opts.mid.value!
    const ep = Math.max(1, opts.episode.value)
    // 整集缓存命中 ⇒ 零请求（同一集开关/配置切换/来回切集重载时复用）
    let commentsData: DanmuComment[]
    const biliKey = `${mid}:${ep}`
    const biliHit = biliCache.get(biliKey)
    if (biliHit) {
      commentsData = biliHit
    } else {
      const resp = await filmApi
        .getDanmuBilibili({ seasonId: sel.resourceId, episode: ep })
        .catch(() => null)
      if (token !== loadToken) {
        return
      }
      if (!resp) {
        danmuState.value = 'failed'
        return
      }
      commentsData = resp.comments
      biliCache.set(biliKey, commentsData)
    }
    const input = toDanmuInputs(commentsData)
    if (input.length === 0) {
      danmuState.value = 'empty'
      return
    }
    manager?.load(input)
    // 装载后强制 seek 对齐（见 alignManager 注释：防 timeupdate 已推进 lastPosition 导致补发一窝蜂）
    alignManager(playerSeconds())
    if (!opts.playing.value) {
      manager?.stop()
    }
    hasDanmaku.value = true
    danmuState.value = 'ready'
  }

  /** 路径 B：腾讯分段点播（episodes 匹配 → segments 索引 → 按进度预拉 3 段） */
  async function loadTencentEpisode(sel: DanmuProvider, token: number): Promise<void> {
    const mid = opts.mid.value!
    const ep = Math.max(1, opts.episode.value)
    // meta（匹配集 + 分段索引）缓存命中 ⇒ 零请求
    const metaKey = `${mid}:${ep}:${sel.provider}`
    let episodeId: string
    let segsData: DanmuTencentSegment[]
    const metaHit = tencentMeta.get(metaKey)
    if (metaHit) {
      episodeId = metaHit.episodeId
      segsData = metaHit.segments
    } else {
      const eps = await filmApi
        .getDanmuTencentEpisodes({ resourceId: sel.resourceId, episode: ep })
        .catch(() => null)
      if (token !== loadToken) {
        return
      }
      if (!eps?.ok) {
        danmuState.value = 'failed'
        return
      }
      if (!eps.data.matched?.episodeId) {
        danmuState.value = 'empty' // 平台无对应集
        return
      }
      episodeId = eps.data.matched.episodeId
      const segs = await filmApi.getDanmuTencentSegments(episodeId).catch(() => null)
      if (token !== loadToken) {
        return
      }
      if (!segs?.ok) {
        danmuState.value = 'failed'
        return
      }
      if (segs.data.length === 0) {
        danmuState.value = 'empty'
        return
      }
      segsData = segs.data
      tencentMeta.set(metaKey, { episodeId, segments: segsData })
    }
    tencentCtx = { episodeId, segments: segsData }
    const anyFail = await loadTencentFrom(playerSeconds(), token)
    if (token !== loadToken) {
      return
    }
    if (loadedSegs.size > 0) {
      // 对齐播放进度（loadTencentFrom 内已逐段对齐，这里兜底一次）
      alignManager(playerSeconds())
      if (!opts.playing.value) {
        manager?.stop()
      }
      hasDanmaku.value = true
      danmuState.value = 'ready'
    } else {
      danmuState.value = anyFail ? 'failed' : 'empty'
    }
  }

  /* ---- 播放器事件联动 ---- */
  opts.onPlayerEvent('play', () => {
    manager?.start()
  })
  opts.onPlayerEvent('pause', () => {
    manager?.stop()
  })
  opts.onPlayerEvent('seeked', () => {
    if (!manager) {
      return
    }
    const t = playerSeconds()
    try {
      manager.stop()
      manager.clear()
      manager.seek(t)
      manager.time(t)
      if (opts.playing.value) {
        manager.start()
      }
    } catch {
      /* 库状态异常时静默降级 */
    }
    // 腾讯路径：跳转后分段游标重置，按新进度重新预拉
    if (tencentCtx) {
      loadedSegs.clear()
      void loadTencentFrom(t, loadToken)
    }
  })
  opts.onPlayerEvent('timeupdate', () => {
    // 库内部 seekTrigger=2s, 密集调用无害；切集窗口的旧 timeupdate 由 token/clear 兜底
    if (manager && opts.enabled.value) {
      manager.time(playerSeconds())
      maybePrefetchTencent()
    }
  })

  /** 用户手动切换弹幕源（UI 调用） */
  function setProvider(next: string): void {
    if (next === provider.value) {
      return
    }
    provider.value = next
    writeProviderPref(next)
    void loadDanmaku()
  }

  /* ---- 开关 / 集号 / 源变化 / stage 挂载（Teleport 后才有值, 纳入触发条件） ---- */
  watch(
    () => [opts.enabled.value, opts.mid.value, opts.episode.value, !!stage.value] as const,
    () => {
      void loadDanmaku()
    }
  )

  onScopeDispose(() => {
    loadToken++
    clearDanmaku()
    resetTencent()
    srcCache.clear()
    biliCache.clear()
    tencentMeta.clear()
    segCache.clear()
    manager = null
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

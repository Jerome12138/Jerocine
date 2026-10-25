import {
  onScopeDispose,
  ref,
  watch,
  type Ref,
  type ShallowRef
} from 'vue'
import type Player from 'video.js/dist/types/player'
import { filmApi } from '@/api'
import type { DanmuProvider, DanmuTencentSegment } from '@/types/film'
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
/** 路径 B 预拉窗口：当前段 + 后续 2 段（§六 风险表：腾讯 67 段×100~250KB, 窗口控制请求量） */
const TENCENT_PREFETCH_SPAN = 3
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

/** 取全局库（index.html 已加载 script）。加载失败返回 null（弹幕静默降级, 不影响播放）。 */
function library(): CommentCoreLibraryLike | null {
  return (typeof window !== 'undefined' && window.CommentCoreLibrary) || null
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

export function useDanmaku(opts: UseDanmakuOptions) {
  const stage = ref<HTMLElement | null>(null)
  /** 当前影片可用的弹幕源（全平台候选; 无候选时为空列表） */
  const providers = ref<DanmuProvider[]>([])
  /** 当前选中的平台（持久化偏好；候选里没有时回退 bilibili/首个） */
  const provider = ref<string>(readProviderPref())
  /** 是否有弹幕可显示（源存在 && 拉取非空; 供 UI 显示状态） */
  const hasDanmaku = ref(false)

  let manager: CommentManagerLike | null = null

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
  const loadedSegs = new Set<string>()

  function resetTencent(): void {
    tencentCtx = null
    loadedSegs.clear()
  }

  /** 从 t 所在段起装载"当前段 + 后续 2 段"（已装载的跳过；token 竞态保护） */
  async function loadTencentFrom(t: number, token: number): Promise<void> {
    if (!tencentCtx) {
      return
    }
    const { episodeId, segments } = tencentCtx
    const { start, end } = prefetchWindow(segments, t, TENCENT_PREFETCH_SPAN)
    for (let i = start; i < end; i += 1) {
      const seg = segments[i]
      if (!seg || loadedSegs.has(seg.segmentId)) {
        continue
      }
      const resp = await filmApi
        .getDanmuTencentComments({ episodeId, segmentId: seg.segmentId })
        .catch(() => null)
      if (token !== loadToken || !resp?.ok || !resp.data) {
        return
      }
      const input = toDanmuInputs(resp.data)
      if (input.length > 0) {
        manager?.load(input)
      }
      loadedSegs.add(seg.segmentId)
    }
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
  /** 拉取并装载当前集弹幕（token 防竞态：快速切集时旧请求结果不落地） */
  async function loadDanmaku(): Promise<void> {
    const token = ++loadToken
    clearDanmaku()
    resetTencent()
    hasDanmaku.value = false
    if (!opts.enabled.value || !opts.mid.value) {
      return
    }
    ensureManager()
    if (!manager) {
      return
    }
    const src = await filmApi.getDanmuSource(opts.mid.value).catch(() => null)
    if (token !== loadToken || !src) {
      return
    }
    const list = src.providers ?? []
    providers.value = list
    if (list.length === 0) {
      return
    }
    // 选中平台：偏好优先；不在候选里时回退 bilibili → 首个（纯函数 resolveDanmuProvider，逐分支单测覆盖）
    const picked = resolveDanmuProvider(list, provider.value, DANMAKU_DEFAULT_PROVIDER)
    if (!picked) {
      return // list 非空（上方已 return），双保险
    }
    if (picked.fellBack) {
      provider.value = picked.provider // 持久化回写，下次进来不再走回退链
    }
    const sel = picked.entry
    if (!sel.resourceId) {
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
    const resp = await filmApi
      .getDanmuBilibili({ seasonId: sel.resourceId, episode: Math.max(1, opts.episode.value) })
      .catch(() => null)
    if (token !== loadToken || !resp) {
      return
    }
    const input = toDanmuInputs(resp.comments)
    if (input.length === 0) {
      return
    }
    manager?.load(input)
    // 装载后对齐当前播放进度（若已暂停则保持静止）
    manager?.time(playerSeconds())
    if (!opts.playing.value) {
      manager?.stop()
    }
    hasDanmaku.value = true
  }

  /** 路径 B：腾讯分段点播（episodes 匹配 → segments 索引 → 按进度预拉 3 段） */
  async function loadTencentEpisode(sel: DanmuProvider, token: number): Promise<void> {
    const eps = await filmApi
      .getDanmuTencentEpisodes({ resourceId: sel.resourceId, episode: Math.max(1, opts.episode.value) })
      .catch(() => null)
    if (token !== loadToken || !eps?.ok || !eps.data.matched?.episodeId) {
      return // 无匹配集 → 静默无弹幕
    }
    const episodeId = eps.data.matched.episodeId
    const segs = await filmApi.getDanmuTencentSegments(episodeId).catch(() => null)
    if (token !== loadToken || !segs?.ok || segs.data.length === 0) {
      return
    }
    tencentCtx = { episodeId, segments: segs.data }
    await loadTencentFrom(playerSeconds(), token)
    if (loadedSegs.size > 0) {
      manager?.time(playerSeconds())
      if (!opts.playing.value) {
        manager?.stop()
      }
      hasDanmaku.value = true
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
    manager = null
  })

  return { stage, providers, provider, setProvider, hasDanmaku, reload: () => void loadDanmaku() }
}

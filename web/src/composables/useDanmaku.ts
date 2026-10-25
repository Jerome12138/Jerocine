import {
  onScopeDispose,
  ref,
  watch,
  type Ref,
  type ShallowRef
} from 'vue'
import type Player from 'video.js/dist/types/player'
import { filmApi } from '@/api'
import type { DanmuComment, DanmuProvider } from '@/types/film'

/**
 * 弹幕（P0）—— 方案 §四之二 / §四之三：
 *
 *  - 数据链路：mid → GET /films/:mid/danmu/source（平台候选, 不暴露 db_id）
 *    → 取 B站 seasonId → GET /danmu/bilibili?seasonId&episode（服务端按集号匹配）
 *  - 播放器：不换引擎, 在 video.js 容器内叠加独立弹幕层（comment-core-library,
 *    DOM/CSS 渲染 div.cmt）。只通过 play/pause/seeked/切集事件联动, 不侵入引擎。
 *  - 联动契约：
 *      play  → manager.start()   （动画计时恢复）
 *      pause → manager.stop()    （动画暂停）
 *      seeked→ clear + seek(t) + time(t) 重放到目标时刻
 *      timeupdate → time(t)      投递 t 秒前应出现的弹幕（库内部 2s seekTrigger 防抖）
 *      ratechange：库无倍速概念, P0 不做速率补偿（倍速下弹幕相对视频时间漂移, 可接受）
 *
 * 开关与源偏好持久化：开关走 useWebSettings（localStorage `jc-danmaku`, 默认开, 与 /settings 共用）；
 * 源偏好 `jc-danmaku-provider`（默认 bilibili）。
 * 多平台切换（腾讯/爱奇艺/优酷弹幕拉取）留 P1（评审 7.8：腾讯 mzc→vid 链路未验证）。
 */

export const DANMAKU_PROVIDER_KEY = 'jc-danmaku-provider'
export const DANMAKU_DEFAULT_PROVIDER = 'bilibili'

export interface UseDanmakuOptions {
  /** 影片 mid（空 = 无弹幕） */
  mid: Ref<number | undefined>
  /** 集号（1 起；对应我们自己的集序, 服务端按标题匹配 B站集） */
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

export function useDanmaku(opts: UseDanmakuOptions) {
  const stage = ref<HTMLElement | null>(null)
  /** 当前可用的弹幕源（B站 seasonId 等; 无 bilibili 时为空） */
  const available = ref<DanmuProvider | null>(null)
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

  let loadToken = 0
  /** 拉取并装载当前集弹幕（token 防竞态：快速切集时旧请求结果不落地） */
  async function loadDanmaku(): Promise<void> {
    const token = ++loadToken
    clearDanmaku()
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
    const bili = src.providers.find((p) => p.provider === DANMAKU_DEFAULT_PROVIDER) ?? null
    available.value = bili
    if (!bili || !bili.resourceId) {
      return
    }
    const resp = await filmApi
      .getDanmuBilibili({ seasonId: bili.resourceId, episode: Math.max(1, opts.episode.value) })
      .catch(() => null)
    if (token !== loadToken || !resp) {
      return
    }
    // 毫秒 → 秒; stime 字段名对齐库契约
    const input: DanmakuInputComment[] = resp.comments.map((c: DanmuComment) => ({
      stime: c.time / 1000,
      mode: c.mode,
      color: c.color,
      text: c.text
    }))
    if (input.length === 0) {
      return
    }
    manager.load(input)
    // 装载后对齐当前播放进度（若已暂停则保持静止）
    manager.time(playerSeconds())
    if (!opts.playing.value) {
      manager.stop()
    }
    hasDanmaku.value = true
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
  })
  opts.onPlayerEvent('timeupdate', () => {
    // 库内部 seekTrigger=2s, 密集调用无害；切集窗口的旧 timeupdate 由 token/clear 兜底
    if (manager && opts.enabled.value) {
      manager.time(playerSeconds())
    }
  })

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
    manager = null
  })

  return { stage, available, hasDanmaku, reload: () => void loadDanmaku() }
}

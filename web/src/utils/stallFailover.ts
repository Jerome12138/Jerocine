/**
 * 卡顿看门狗换源决策(纯函数) —— 高峰慢源治标 PlayView 接线用。
 *
 * 背景: 高峰期第三方源 CDN 拥堵表现为「长时间缓冲但不报错」(video:waiting 持续),
 * error 状态机(handleVideoError)只在真错误时触发, 覆盖不到这种慢速卡顿。
 * 看门狗在连续缓冲超阈值时发起换源评估, 目标从线路测速(usePlaySpeedTest)结果里挑
 * 显著更快的线路, 而不是 error 路径的"下一个源"盲轮换 —— 换去一个同样慢的源
 * 只是换一种卡。
 */

/** 连续缓冲多久算 stall。阈值要盖过 seek 后正常起播与码率切换的缓冲(常见 2~6s), 短抖动不触发。 */
export const STALL_THRESHOLD_MS = 12000
/** 触发一次换源后的冷却: 新源也可能慢, 防止 A→B→A 来回横跳。 */
export const STALL_COOLDOWN_MS = 30000
/** 单集内 stall 换源上限(与 error 路径的 fallbackRounds 语义独立, 各管各的)。 */
export const MAX_STALL_FAILOVERS_PER_EPISODE = 2
/** 「显著更快」才值得换: 目标耗时 < 当前耗时 × 0.6(至少省 40%), 否则体感无差别白折腾。 */
export const FASTER_RATIO = 0.6

export interface StallContext {
  /** 本轮连续缓冲时长(ms) */
  bufferingMs: number
  /** 距上次 stall 换源的毫秒数; 从未触发过传 Infinity */
  msSinceLastFailover: number
  /** 本集内已发生的 stall 换源次数 */
  failoverCount: number
  /** 用户暂停中(waiting 不该在暂停态计时) */
  paused: boolean
  /** 拖动进度条中(seek 引起的缓冲不归看门狗管) */
  seeking: boolean
  /** 切集/切源互斥窗口(旧源残余事件期, 不评估) */
  switching: boolean
  /** 除当前线路外的可换源数量 */
  otherSourceCount: number
}

/** 看门狗是否应发起换源评估(各守卫独立可测)。 */
export function shouldStallFailover(c: StallContext): boolean {
  if (c.paused || c.seeking || c.switching) return false
  if (c.otherSourceCount <= 0) return false
  if (c.failoverCount >= MAX_STALL_FAILOVERS_PER_EPISODE) return false
  if (c.msSinceLastFailover < STALL_COOLDOWN_MS) return false
  return c.bufferingMs >= STALL_THRESHOLD_MS
}

/** 测速表里某线路的耗时(ms); 未测过/测失败(-1)/无此线路一律 Infinity(不可信)。 */
export function speedOf(speeds: Record<string, number>, id: string | undefined): number {
  if (!id) return Infinity
  const v = speeds[id]
  return typeof v === 'number' && v >= 0 ? v : Infinity
}

/**
 * 从测速结果挑 stall 换源目标。
 * 规则: 只有存在「显著更快」(FASTER_RATIO)的候选才返回其 id, 否则 null ——
 * 当前线路已是可选最快时换源只会一样卡, 还打断观看。
 * 特例: 当前线路没测过/测挂(Infinity)时, 任一有有效测速结果的候选都算更优
 * (清单拿不到/拿得极慢正是 stall 的常见原因)。
 */
export function pickStallTarget(
  speeds: Record<string, number>,
  currentSourceId: string | undefined,
  candidateIds: string[],
): string | null {
  let best: string | null = null
  let bestMs = Infinity
  for (const id of candidateIds) {
    const ms = speedOf(speeds, id)
    if (ms < bestMs) {
      bestMs = ms
      best = id
    }
  }
  if (best === null) return null
  const cur = speedOf(speeds, currentSourceId)
  if (cur === Infinity) return best
  return bestMs < cur * FASTER_RATIO ? best : null
}

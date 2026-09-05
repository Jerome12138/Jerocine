/**
 * 时间分桶 (用于历史/收藏页按时间分组展示).
 *
 * 桶定义 (相对"现在"):
 *  - today    今天 0:00 起
 *  - week     最近 7 天内 (但不含今天)
 *  - month    最近 30 天内 (但不含本周)
 *  - earlier  更早
 *
 * 抽成纯函数 + 接受 now 参数, 方便单测 (固定 now 拿确定结果).
 */

export type TimeBucket = 'today' | 'week' | 'month' | 'earlier'

export const BUCKET_LABEL: Record<TimeBucket, string> = {
  today: '今天',
  week: '本周',
  month: '本月',
  earlier: '更早'
}

const BUCKET_ORDER: TimeBucket[] = ['today', 'week', 'month', 'earlier']

/** 取目标时间戳 ts 在 now 参考下属于哪个桶. ts 单位 ms. */
export function bucketOf(ts: number, now: number = Date.now()): TimeBucket {
  if (!ts || ts <= 0) return 'earlier'
  // 计算"今天 0:00" (本地时间)
  const today0 = new Date(now)
  today0.setHours(0, 0, 0, 0)
  if (ts >= today0.getTime()) return 'today'
  const day = 24 * 60 * 60 * 1000
  if (now - ts < 7 * day) return 'week'
  if (now - ts < 30 * day) return 'month'
  return 'earlier'
}

/**
 * 将一组带 timeStamp 字段的记录按桶分组, 返回有序数组 (today → earlier).
 * 空桶不出现在结果里; 每个桶内保持原始顺序 (调用方应已按时间倒序排好).
 */
export function groupByTimeBucket<T extends { timeStamp?: number }>(
  records: T[],
  now: number = Date.now()
): Array<{ bucket: TimeBucket; label: string; items: T[] }> {
  const map: Record<TimeBucket, T[]> = {
    today: [],
    week: [],
    month: [],
    earlier: []
  }
  for (const r of records) {
    const b = bucketOf(r.timeStamp ?? 0, now)
    map[b].push(r)
  }
  return BUCKET_ORDER.filter((b) => map[b].length > 0).map((b) => ({
    bucket: b,
    label: BUCKET_LABEL[b],
    items: map[b]
  }))
}

/**
 * 计算观看进度百分比 (0-100).
 * currentTime / duration 都是秒. duration 缺失或 <= 0 返回 0.
 * 已看完 (currentTime >= duration - 30) 视为 100% (减去片尾 buffer).
 */
export function progressPercent(currentTime?: number, duration?: number): number {
  if (!currentTime || !duration || duration <= 0) return 0
  if (currentTime >= duration - 30) return 100
  const pct = (currentTime / duration) * 100
  if (!Number.isFinite(pct)) return 0
  return Math.max(0, Math.min(100, Math.round(pct)))
}

/**
 * "当前看到第几集" 的显示标签.
 * - episode: 集名(可能是显示名如 "第01集"/"HD", 也可能是 remote 历史里存的纯数字索引字符串)
 * - episodeIndex: 0-based 索引(原生播放器 updateProgress 会持续更新, 最可靠的"当前集")
 * 取舍: 优先用"非纯数字"的集名(真实集名); 否则用 episodeIndex → 第N集; 都缺则空串。
 */
export function episodeLabel(episode?: string, episodeIndex?: number): string {
  const name = (episode ?? '').trim()
  // 真实集名(非纯数字, 且不是 "第0集" 这类 0-based 遗留串)直接用
  if (name && !/^\d+$/.test(name) && !/^第\s*0+\s*集$/.test(name)) return name
  // 优先用 0-based index → 1-based 显示
  if (typeof episodeIndex === 'number' && episodeIndex >= 0) return `第 ${episodeIndex + 1} 集`
  // 退化: 纯数字串 / "第0集" 遗留串当 0-based 索引, 至少显示第 1 集(杜绝"第0集")
  if (name) {
    const n = parseInt(name.replace(/[^\d]/g, ''), 10)
    return `第 ${Number.isFinite(n) ? n + 1 : 1} 集`
  }
  return ''
}

/**
 * 弹幕纯函数（useDanmaku 的可测内核）。
 *
 * 从 composable 闭包中提取的无副作用逻辑：腾讯分段游标 / 跨段预拉判定 /
 * 源选择回退 / 毫秒→秒映射。useDanmaku 只做装配（播放器事件、请求、状态），
 * 这里只做决策 —— 表驱动用例见 danmu.spec.ts。
 *
 * 契约要点：
 * - 后端/壳返回毫秒（time），danmaku-lite 入参 time 是秒，换算唯一入口在 toDanmuInput
 * - segments 入参须按 startTime 升序（danmu-service 壳保证）
 */
import type { DanmuComment, DanmuProvider, DanmuTencentSegment } from '@/types/film'
import type { DanmakuItem } from 'danmaku-lite'

/**
 * 弹幕映射过滤项（避免与 DanmakuSettings 形成循环依赖，用本地结构而非 import 后者）。
 */
export interface DanmuMapFilter {
  /** 是否保留底部弹幕（源 mode 4 映射后 = 6） */
  bottomMode?: boolean
  /** 是否保留顶部弹幕（源 mode 5 映射后 = 5） */
  topMode?: boolean
  /** false = 忽略原色，全部改写为白色 */
  colored?: boolean
}

/**
 * 模式归一（danmaku-lite 约定：1 滚动 / 5 顶部 / 6 底部）：
 *   - 源 1/2/6（普通滚动 / 反向滚动 / 底部反向滚动）→ 1
 *     （danmaku-lite 无逆向滚动，统一归并进同一滚动轨道分配器，避免与 1 同屏交叉重叠）
 *   - 源 4（底部固定）→ 6
 *   - 源 5（顶部固定）→ 5
 *   - 其余（3/7/8/17 等后端可能返回的非法值）→ 1（兜底，避免非法值直达引擎）
 */
function normalizeDanmuMode(mode: number): number {
  if (mode === 4) return 6
  if (mode === 5) return 5
  return 1
}

/**
 * 单条弹幕映射：毫秒 → 秒（danmaku-lite 约定 time 为秒），产出 DanmakuItem。
 * 源 DanmuComment 无 id，用 `time:text:index` 生成稳定唯一 id（同列表内唯一、同数据稳定，供 load 去重）。
 * filter 命中（关闭底部/顶部/彩色）返回 null，调用方负责跳过。
 */
export function toDanmuInput(
  c: DanmuComment,
  index = 0,
  filter?: DanmuMapFilter,
): DanmakuItem | null {
  const mapped = normalizeDanmuMode(c.mode)
  if (filter) {
    if (mapped === 6 && filter.bottomMode === false) return null
    if (mapped === 5 && filter.topMode === false) return null
  }
  const color = filter && filter.colored === false ? 0xffffff : c.color
  return {
    id: `${c.time}:${c.text}:${index}`,
    text: c.text,
    time: c.time / 1000,
    // DanmakuMode 是 const enum（1/5/6）；esbuild 不支持其运行时，
    // 故不 import 该枚举，仅以 `as DanmakuItem['mode']` 收敛数值类型（类型导入无运行时副作用）。
    mode: mapped as DanmakuItem['mode'],
    color
  }
}

/** 批量映射（保持顺序；filter 命中的条目跳过；空列表返回空数组）。 */
export function toDanmuInputs(
  list: readonly DanmuComment[],
  filter?: DanmuMapFilter,
): DanmakuItem[] {
  const out: DanmakuItem[] = []
  for (let i = 0; i < list.length; i += 1) {
    const item = toDanmuInput(list[i]!, i, filter)
    if (item) out.push(item)
  }
  return out
}

/**
 * 腾讯分段游标：t 所在段索引（最后一段 startTime <= t；没有任何段 <= t 时取 0，
 * 与"0 段起始为 0"约定一致 —— 未命中不应返回 -1 让预拉从负索引开始）。
 */
export function segmentIndexAt(segments: readonly DanmuTencentSegment[], t: number): number {
  let idx = 0
  for (let i = 0; i < segments.length; i += 1) {
    const seg = segments[i]
    if (seg && seg.startTime <= t) idx = i
  }
  return idx
}

/** 预拉窗口 [start, end)：从 t 所在段起共 span 段，末尾收敛到 segments.length。 */
export function prefetchWindow(
  segments: readonly DanmuTencentSegment[],
  t: number,
  span: number,
): { start: number; end: number } {
  const start = segmentIndexAt(segments, t)
  return { start, end: Math.min(start + span, segments.length) }
}

/** 下一段索引（startTime > t 的首段；没有则 -1）。 */
export function nextSegmentIndex(segments: readonly DanmuTencentSegment[], t: number): number {
  for (let i = 0; i < segments.length; i += 1) {
    const seg = segments[i]
    if (seg && seg.startTime > t) return i
  }
  return -1
}

/**
 * 跨段预拉判定（§8.2 提前量 LEAD）：下一段存在、未装载、
 * 且进度已进入"距边界 < lead"窗口。三者缺一不拉。
 */
export function shouldPrefetchNext(
  segments: readonly DanmuTencentSegment[],
  t: number,
  lead: number,
  loaded: ReadonlySet<string>,
): boolean {
  const i = nextSegmentIndex(segments, t)
  if (i < 0) return false
  const seg = segments[i]
  if (!seg || loaded.has(seg.segmentId)) return false
  return t >= seg.startTime - lead
}

export interface DanmuProviderSelection {
  /** 最终选中的平台名（偏好命中时等于 pref） */
  provider: string
  /** 对应候选条目（providers 非空时恒有） */
  entry: DanmuProvider
  /** 偏好失效发生了回退（调用方据此持久化回写 localStorage） */
  fellBack: boolean
}

/**
 * 源选择（与 useDanmaku 原闭包逻辑逐分支等价）：
 * 偏好优先 → 偏好不在候选时回退默认平台 → 默认也不在时取首个。
 * 候选为空返回 null。
 */
export function resolveDanmuProvider(
  providers: readonly DanmuProvider[],
  pref: string,
  defaultProvider: string,
): DanmuProviderSelection | null {
  if (providers.length === 0) return null
  const hit = providers.find((it) => it.provider === pref)
  if (hit) return { provider: pref, entry: hit, fellBack: false }
  const name = providers.some((it) => it.provider === defaultProvider)
    ? defaultProvider
    : providers[0]!.provider
  const entry = providers.find((it) => it.provider === name) ?? providers[0]!
  return { provider: name, entry, fellBack: true }
}

/**
 * 弹幕纯函数（useDanmaku 的可测内核）。
 *
 * 从 composable 闭包中提取的无副作用逻辑：腾讯分段游标 / 跨段预拉判定 /
 * 源选择回退 / 毫秒→秒映射。useDanmaku 只做装配（播放器事件、请求、状态），
 * 这里只做决策 —— 表驱动用例见 danmu.spec.ts。
 *
 * 契约要点（方案 §8.2）：
 * - 后端/壳返回毫秒（time），CommentCoreLibrary 入参是秒（stime），换算唯一入口在下方
 * - segments 入参须按 startTime 升序（danmu-service 壳保证）
 */
import type { DanmuComment, DanmuProvider, DanmuTencentSegment } from '@/types/film'

/**
 * CommentCoreLibrary 装载入参（stime 秒）。
 * mode 归一（2026-10-26, 流式防重叠）:
 *   CCL 的滚动/反向滚动(mode 1/2/6)各自独立 CommentSpaceAllocator, 轨道都从顶部/底部起始,
 *   互不知情 → mode 6(反向滚动)/2(底部反向)与 mode 1(普通滚动)同屏时**交叉重叠**。
 *   统一归一为 mode 1: 所有滚动弹幕进同一 allocator, 走内置 pathCheck 防重叠轨道分配。
 *   固定弹幕 mode 4/5 保留（AnchorCommentSpaceAllocator 内部已按行堆叠防重叠）。
 */
export interface DanmuLibraryInput {
  stime: number
  mode: number
  color: number
  text: string
}

function normalizeDanmuMode(mode: number): number {
  if (mode === 2 || mode === 6) return 1
  return mode
}

/** 单条弹幕映射：毫秒 → 秒（库约定）。 */
export function toDanmuInput(c: DanmuComment): DanmuLibraryInput {
  return { stime: c.time / 1000, mode: normalizeDanmuMode(c.mode), color: c.color, text: c.text }
}

/** 批量映射（保持顺序；空列表返回空数组，调用方据此跳过 manager.load）。 */
export function toDanmuInputs(list: readonly DanmuComment[]): DanmuLibraryInput[] {
  return list.map(toDanmuInput)
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

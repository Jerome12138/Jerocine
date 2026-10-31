/**
 * 弹幕纯函数（useDanmaku 的可测内核）。
 *
 * 从 composable 闭包中提取的无副作用逻辑：弹幕条目映射（模式归一/过滤/毫秒→秒）与
 * 整秒打散。useDanmaku 只做装配（播放器事件、请求、状态），这里只做决策 ——
 * 表驱动用例见 danmu.spec.ts。
 *
 * 契约要点：后端返回毫秒（time），artplayer-plugin-danmuku 入参 time 是秒，
 * 换算唯一入口在 toDanmuInput。输出为插件 Danmu 格式（mode 0 滚动 / 1 顶部 / 2 底部,
 * color '#RRGGBB'）。
 */
import type { DanmuComment } from '@/types/film'
import type { Danmu } from 'artplayer-plugin-danmuku'

/**
 * 弹幕映射过滤项（避免与 DanmakuSettings 形成循环依赖，用本地结构而非 import 后者）。
 */
export interface DanmuMapFilter {
  /** 是否保留底部弹幕（源 mode 4） */
  bottomMode?: boolean
  /** 是否保留顶部弹幕（源 mode 5） */
  topMode?: boolean
  /** false = 忽略原色，全部改写为白色 */
  colored?: boolean
}

/**
 * 模式归一（artplayer-plugin-danmuku 约定：0 滚动 / 1 顶部 / 2 底部）：
 *   - 源 1/2/3/6/7/8（普通滚动 / 反向滚动 / 各种滚动变体）→ 0（插件无逆向滚动, 统一归并）
 *   - 源 4（底部固定）→ 2
 *   - 源 5（顶部固定）→ 1
 *   - 其余非法值 → 0（兜底）
 */
function normalizeDanmuMode(mode: number): 0 | 1 | 2 {
  if (mode === 4) return 2
  if (mode === 5) return 1
  return 0
}

/** 十进制颜色 → '#RRGGBB'（插件 Danmu.color 为 CSS 颜色字符串）。 */
export function danmuColorToHex(color: number): string {
  const v = Number.isFinite(color) && color > 0 ? Math.floor(color) : 0xffffff
  return `#${(v & 0xffffff).toString(16).padStart(6, '0')}`
}

/**
 * 单条弹幕映射：毫秒 → 秒（插件约定 time 为秒），产出插件 Danmu。
 * filter 命中（关闭底部/顶部）返回 null，调用方负责跳过。
 */
export function toDanmuInput(
  c: DanmuComment,
  index = 0,
  filter?: DanmuMapFilter,
): Danmu | null {
  const mapped = normalizeDanmuMode(c.mode)
  if (filter) {
    if (mapped === 2 && filter.bottomMode === false) return null
    if (mapped === 1 && filter.topMode === false) return null
  }
  const color = filter && filter.colored === false ? '#ffffff' : danmuColorToHex(c.color)
  return {
    text: c.text,
    time: c.time / 1000,
    mode: mapped,
    color
  }
}

/** 整秒随机打散幅度（秒）：用户指定前后 0.5s。 */
export const DANMU_SCATTER_RANGE = 0.5

/** FNV-1a 32bit → [0,1) 确定性伪随机；同一输入恒得同一偏移（重看/重拉不漂移）。 */
function hashUnit(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i += 1) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return ((h >>> 0) % 1000) / 1000
}

/**
 * 整秒聚堆随机打散：弹幕源时间基本是整秒精度，同秒（及邻近秒）弹幕齐发会列成整齐一列
 * （固定步长错开仍显规律、不够自然）。对 time 为整秒的条目叠加确定性伪随机偏移
 * ±DANMU_SCATTER_RANGE；非整秒条目已有毫秒级分散，不动。偏移种子取条目序号
 * （同数据稳定）。处理后按 time 重新升序。
 */
export function scatterTimes(items: Danmu[]): void {
  for (let i = 0; i < items.length; i += 1) {
    const it = items[i]!
    const t = it.time ?? 0
    const rounded = Math.round(t)
    if (Math.abs(t - rounded) > 1e-6) continue
    const jitter = (hashUnit(`${t}:${it.text}:${i}`) - 0.5) * 2 * DANMU_SCATTER_RANGE
    it.time = Math.max(0, Number((rounded + jitter).toFixed(3)))
  }
  items.sort((a, b) => (a.time ?? 0) - (b.time ?? 0))
}

/** 批量映射（保持顺序；filter 命中的条目跳过；空列表返回空数组）。
 *  整秒弹幕随机打散（scatterTimes），消除齐发列队。 */
export function toDanmuInputs(
  list: readonly DanmuComment[],
  filter?: DanmuMapFilter,
): Danmu[] {
  if (list.length === 0) return []
  const sorted = [...list].sort((a, b) => a.time - b.time)
  const out: Danmu[] = []
  for (const c of sorted) {
    const item = toDanmuInput(c, out.length, filter)
    if (item) out.push(item)
  }
  scatterTimes(out)
  return out
}

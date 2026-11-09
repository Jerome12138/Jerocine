/**
 * 跨源集对齐（切源用）—— 只做「落点决策」，解析器复用 `./episodeKey`（三端同规的 v2）。
 * 方案见 `JerocineHarness/docs/方案-epid集身份与跨源对齐-2026-11-06.md`。
 *
 * 背景：切源（含看门狗自动换源 / error 轮换）此前只做「保同下标」——
 * 剧/动漫各源同构时等价，但**综艺各源分集不一致时会静默切到错集**。
 */
import { epidOf, partCompatible, parseEpisodeKey, sameEpisode, type EpisodeKey } from './episodeKey'

/** 对齐档位：exact=身份串全等；samePeriod=同期号；sameDate=同日期；byOrdinal=同期序；position=按位置。 */
export type AlignTier =
  | 'exact'
  | 'samePeriod'
  | 'sameDate'
  | 'byOrdinal'
  | 'bySeries'
  | 'position'
  | 'failed'

export interface AlignResult {
  /** 目标源里的下标；-1 = 完全对不上 */
  index: number
  tier: AlignTier
}

/** 能否据此**自动**换源：只有精确档（exact/samePeriod/sameDate）放行；
 *  byOrdinal / position 是启发式兜底 ⇒ 只提示、不自动切（宁可不切，不乱切）。 */
export function canAutoSwitch(tier: AlignTier): boolean {
  // bySeries（同栏目组内唯一，L3a 客户端等价档）也放行：目标侧该组**只有一条** ⇒ 无歧义，
  // 衍生栏目换源也能保持同一集。
  return tier === 'exact' || tier === 'samePeriod' || tier === 'sameDate' || tier === 'bySeries'
}

/**
 * 身份相容（不含期号/日期）：栏目 + 变体 + 主次部分；**日期双有且不等 → 否决**（硬证据，同 SameEpisode′）。
 * 第 ④ 档（跨命名体系：一侧日期、一侧期号）靠它，不能用 `sameEpisode`（那要求有共同期号/日期）。
 */
function identityCompatible(a: EpisodeKey, b: EpisodeKey): boolean {
  if (a.variant !== b.variant || a.series !== b.series) return false
  if (!partCompatible(a, b)) return false
  if (a.date && b.date && a.date !== b.date) return false
  return true
}

/** 「期槽」：期号优先、无期号用日期（无则空串 = 不参与期序）。 */
function periodKey(k: EpisodeKey): string {
  if (k.num > 0) return `n${k.num}`
  return k.date ? `d${k.date}` : ''
}

function comparePeriodKey(a: string, b: string): number {
  if (a[0] === 'n' && b[0] === 'n') return Number(a.slice(1)) - Number(b.slice(1))
  return a < b ? -1 : a > b ? 1 : 0
}

/** 期序：同 (栏目, 变体) 分组内，按「期槽」去重排序后的序号（1 起；无期槽 = 0）。 */
function periodOrdinals(list: EpisodeKey[]): number[] {
  const out = list.map(() => 0)
  const groups = new Map<string, number[]>()
  list.forEach((k, i) => {
    const g = `${k.series}\u0001${k.variant}`
    const arr = groups.get(g) ?? []
    arr.push(i)
    groups.set(g, arr)
  })
  for (const idxs of groups.values()) {
    const keys = [...new Set(idxs.map((i) => periodKey(list[i]!)).filter((s) => s !== ''))].sort(
      comparePeriodKey
    )
    const rank = new Map(keys.map((s, n) => [s, n + 1]))
    for (const i of idxs) out[i] = rank.get(periodKey(list[i]!)) ?? 0
  }
  return out
}

/**
 * L3a（客户端等价档，与服务端 `pickSeriesUnique` 同口径）：同 `(series, variant)` 组内**唯一定位**。
 * 适用：目标侧该集**既无期号也无日期**的衍生栏目（心动的旅行 / 线上集结篇 / 夏日清凉特辑…）。
 * 安全边界：正片（series/variant 皆空）不适用；作用域被 (series,variant) 锁死 + 主/次相容；
 * 期号两侧都有须相等；日期两侧都有须相等；**目标侧该组仅 1 条**才返回，否则 -1（宁缺勿错）。
 */
function seriesUniqueIndex(c: EpisodeKey, tgt: EpisodeKey[]): number {
  if (!c.series && !c.variant) return -1
  let only = -1
  for (let i = 0; i < tgt.length; i++) {
    const t = tgt[i]!
    if (t.series !== c.series || t.variant !== c.variant) continue
    if (!partCompatible(c, t)) continue
    if (c.num > 0 && t.num > 0 && c.num !== t.num) continue
    if (c.date && t.date && c.date !== t.date) continue
    if (only >= 0) return -1 // 组内不唯一 ⇒ 不配
    only = i
  }
  return only
}

/**
 * 跨源集对齐（5 档降级）。`curEps` / `targetEps` 为两侧完整集名列表（下标即播放用的集号）。
 * 全部对不上时返回 `{ index: -1, tier: 'failed' }`（调用方自行决定提示）。
 */
export function alignEpisode(curIndex: number, curEps: string[], targetEps: string[]): AlignResult {
  const cur = curEps.map((t) => parseEpisodeKey(t ?? ''))
  const tgt = targetEps.map((t) => parseEpisodeKey(t ?? ''))
  const c = cur[curIndex]
  if (!c || tgt.length === 0) return { index: -1, tier: 'failed' }

  // ① 身份串全等
  const curEpid = epidOf(c)
  for (let i = 0; i < tgt.length; i++) {
    if (epidOf(tgt[i]!) === curEpid) return { index: i, tier: 'exact' }
  }
  // ② 同集（SameEpisode′）+ 期号相同 —— 期号优先，此时不看日期
  if (c.num > 0) {
    for (let i = 0; i < tgt.length; i++) {
      const t = tgt[i]!
      if (t.num > 0 && t.num === c.num && sameEpisode(c, t)) return { index: i, tier: 'samePeriod' }
    }
  }
  // ③ 同集 + 日期相同（两侧都没期号、都有日期）
  if (c.num === 0 && c.date !== '') {
    for (let i = 0; i < tgt.length; i++) {
      const t = tgt[i]!
      if (t.num === 0 && t.date === c.date && sameEpisode(c, t)) return { index: i, tier: 'sameDate' }
    }
  }
  // ④ 身份相容 + 期序相同（跨命名体系：源 A 用日期、源 B 用期号）
  const ord = periodOrdinals(cur)[curIndex] ?? 0
  if (ord > 0) {
    const tgtOrd = periodOrdinals(tgt)
    for (let i = 0; i < tgt.length; i++) {
      if (tgtOrd[i] === ord && identityCompatible(c, tgt[i]!)) return { index: i, tier: 'byOrdinal' }
    }
  }
  // ⑤ 同栏目组内唯一（L3a）—— 衍生栏目（目标侧无期号无日期）；与服务端 pickSeriesUnique 同口径
  const uniq = seriesUniqueIndex(c, tgt)
  if (uniq >= 0) return { index: uniq, tier: 'bySeries' }
  // ⑥ 整体位置（原有兜底：第 i 个 ↔ 第 i 个，超出取最后一集）
  return { index: Math.min(curIndex, tgt.length - 1), tier: 'position' }
}

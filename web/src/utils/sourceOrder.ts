/**
 * 源 tab 展示排序：测速完成后按实测延时「由快到慢」排。
 *
 * 规则（用户 2026-11-09）：
 *  - 有有效实测值的源(ms >= 0)排在前面，按 ms 升序（最快在前）；
 *  - 无实测值(undefined) 或 失败(-1) 的源排在后面，保持各自原有相对顺序；
 *  - 两侧相等（同为有效或同为无效）时用原下标做稳定 tie-break。
 *  - 完全没有测速结果时 == 原顺序（即"测速之前"不改变顺序）。
 *
 * 只影响展示顺序：EpisodeTabs 的选中/播放都按 source id 传递，重排不影响语义。
 */
export function sortSourcesBySpeed<T extends { id: string }>(
  sources: readonly T[],
  speeds: Record<string, number> | undefined
): T[] {
  const sp = speeds ?? {}
  return sources
    .map((source, index) => {
      const ms = sp[source.id]
      const valid = typeof ms === 'number' && ms >= 0
      return { source, index, ms: valid ? (ms as number) : Number.POSITIVE_INFINITY }
    })
    .sort((a, b) => {
      if (a.ms !== b.ms) return a.ms - b.ms
      return a.index - b.index
    })
    .map((x) => x.source)
}

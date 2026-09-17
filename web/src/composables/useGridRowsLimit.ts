import { onBeforeUnmount, ref, type Ref } from 'vue'
import { useViewMode } from './useViewMode'

/**
 * 「猜你喜欢 / 相关推荐」类网格模块的行数上限：
 *   默认最多 maxRows 行(2), 移动端(<768)放宽到 mobileRows 行(3)。
 * 列数跟随 theme.css --gf-list-cols 阶梯(3/4/5/6/7, TV 恒 6) —— 与 CSS 保持同断点,
 * resize 时联动, 保证"截断条数 = 渲染列数 × 行数"。
 */
export function useGridRowsLimit(maxRows = 2, mobileRows = 3): {
  cols: Ref<number>
  limitToRows: <T>(items: T[] | undefined) => T[]
} {
  const { isTV } = useViewMode()

  const cols = ref(6)
  function calcCols(): number {
    if (isTV.value) return 6
    const w = window.innerWidth
    if (w < 480) return 3
    if (w < 768) return 4
    if (w < 1024) return 5
    if (w < 1152) return 6
    return 7
  }
  function update(): void {
    cols.value = calcCols()
  }
  update()
  window.addEventListener('resize', update)
  onBeforeUnmount(() => window.removeEventListener('resize', update))

  function limitToRows<T>(items: T[] | undefined): T[] {
    if (!Array.isArray(items)) return []
    const rows = window.innerWidth < 768 ? mobileRows : maxRows
    return items.slice(0, cols.value * rows)
  }

  return { cols, limitToRows }
}

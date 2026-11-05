import { describe, it, expect } from 'vitest'
import { ref, nextTick } from 'vue'
import { useClientPagedList } from './useClientPagedList'

/**
 * useClientPagedList(§C8#3) — 前端切片分页状态收敛(参照 usePagedList.spec 口径):
 *   DashboardView(在线明细, 原款带末页钳制)与 LiveSourceListView(频道面板, 原款**无**钳制
 *   —— 本次顺带修的 bug)两处手写样板收编为单点实现。
 * 行为口径:
 *   - paged = source 切片 [(page-1)*size, page*size)
 *   - source 收缩(筛选/删除)后 page 越界 → 钳制回最后一个有效页(≥1)
 *   - reset() 回第 1 页(筛选条件变化由宿主调, 与原两处 watch 行为一致)
 */
describe('useClientPagedList 前端切片分页', () => {
  const makeSource = (n: number) => ref(Array.from({ length: n }, (_, i) => ({ id: i + 1, name: `r${i + 1}` })))

  it('验收1: 初值 page=1, paged 为首窗切片, total/pageCount 就位', () => {
    const src = makeSource(25)
    const l = useClientPagedList(src, 10)
    expect(l.page.value).toBe(1)
    expect(l.paged.value).toHaveLength(10)
    expect(l.paged.value[0]).toEqual({ id: 1, name: 'r1' })
    expect(l.total.value).toBe(25)
    expect(l.pageCount.value).toBe(3)
  })

  it('验收2: 翻到末页 → 切片为余量(不足一页)', () => {
    const src = makeSource(25)
    const l = useClientPagedList(src, 10)
    l.page.value = 3
    expect(l.paged.value).toHaveLength(5)
    expect(l.paged.value[0]).toEqual({ id: 21, name: 'r21' })
  })

  it('验收3: 越界钳制 — 数据收缩后 page 超出末页 → 回到最后一个有效页', async () => {
    const src = makeSource(25)
    const l = useClientPagedList(src, 10)
    l.page.value = 3
    // 数据收缩到只剩 12 条(2 页) → page 钳回 2
    src.value = src.value.slice(0, 12)
    await nextTick()
    expect(l.page.value).toBe(2)
    expect(l.paged.value).toHaveLength(2)
    // 再收缩到 0 条 → 钳回第 1 页(不落 0)
    src.value = []
    await nextTick()
    expect(l.page.value).toBe(1)
    expect(l.paged.value).toHaveLength(0)
    expect(l.pageCount.value).toBe(1)
  })

  it('验收4: page 未越界时数据变化不钳制(保持原 DashboardView 行为)', async () => {
    const src = makeSource(25)
    const l = useClientPagedList(src, 10)
    l.page.value = 2
    src.value = src.value.slice(0, 25)
    await nextTick()
    expect(l.page.value).toBe(2)
  })

  it('验收5: reset() 回第 1 页(宿主筛选 watch 调用)', () => {
    const src = makeSource(25)
    const l = useClientPagedList(src, 10)
    l.page.value = 2
    l.reset()
    expect(l.page.value).toBe(1)
    expect(l.paged.value).toHaveLength(10)
  })

  it('验收6: 支持传 getter(computed 源), 与 ref 源等价', async () => {
    const src = makeSource(25)
    const l = useClientPagedList(() => src.value, 10)
    l.page.value = 3
    expect(l.paged.value).toHaveLength(5)
    src.value = src.value.slice(0, 5)
    await nextTick()
    expect(l.page.value).toBe(1)
  })

  it('验收7: 非整除页数 pageCount 向上取整', () => {
    const l = useClientPagedList(makeSource(101), 100)
    expect(l.pageCount.value).toBe(2)
  })
})

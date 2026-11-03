import { describe, it, expect, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { usePagedList } from './usePagedList'

interface Row {
  id: number
  name: string
}

/** 受控 fetch 桩: 用 defer 收放每个请求的 resolve/reject 时机 */
function defer<T>(): { promise: Promise<T>; resolve: (v: T) => void; reject: (e: unknown) => void } {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('usePagedList 分页状态收敛', () => {
  it('验收1: 默认 immediate → fetch 被调 (1, 20) 一次', () => {
    const fetch = vi.fn(async () => ({ list: [], total: 0 }))
    usePagedList<Row>({ fetch })
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch).toHaveBeenCalledWith(1, 20)
  })

  it('验收2: resolve {list, total} → rows/total/loading/loadError 就位', async () => {
    const fetch = vi.fn(async () => ({
      list: [{ id: 1, name: 'a' }, { id: 2, name: 'b' }, { id: 3, name: 'c' }],
      total: 64
    }))
    const paged = usePagedList<Row>({ fetch })
    expect(paged.loading.value).toBe(true)
    await flushPromises()
    expect(paged.rows.value).toHaveLength(3)
    expect(paged.total.value).toBe(64)
    expect(paged.loading.value).toBe(false)
    expect(paged.loadError.value).toBe('')
  })

  it('验收3: fetch reject → loadError=e.message, rows 保留上次值', async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce({ list: [{ id: 1, name: 'a' }], total: 1 })
      .mockRejectedValueOnce(new Error('boom'))
    const paged = usePagedList<Row>({ fetch, immediate: false })
    await paged.load()
    expect(paged.rows.value).toHaveLength(1)
    // current 变化 → watch 自动重拉(唯一触发器), 本次失败
    paged.current.value = 2
    await flushPromises()
    expect(paged.loadError.value).toBe('boom')
    expect(paged.rows.value).toHaveLength(1)
    expect(paged.loading.value).toBe(false)
  })

  it('验收4: current=3 返回空 list 且 pageCount=2 → 末页回夹自动 fetch(2)', async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce({ list: [{ id: 1, name: 'a' }], total: 40 }) // 初载
      .mockResolvedValueOnce({ list: [], total: 40 }) // 第 3 页空(数据被删空)
      .mockResolvedValueOnce({ list: [{ id: 9, name: 'last' }], total: 40 }) // 回夹第 2 页
    const paged = usePagedList<Row>({ fetch, immediate: false })
    await paged.load()
    paged.current.value = 3
    await flushPromises()
    // 回夹触发: current 被置 2 且自动重拉
    expect(paged.current.value).toBe(2)
    expect(fetch).toHaveBeenLastCalledWith(2, 20)
    expect(paged.rows.value).toEqual([{ id: 9, name: 'last' }])
  })

  it('验收5: 快速两次 load, 慢响应后到 → 序号守卫丢弃, rows 是快响应的', async () => {
    const slow = defer<{ list: Row[]; total: number }>()
    const fast = defer<{ list: Row[]; total: number }>()
    const fetch = vi.fn().mockReturnValueOnce(slow.promise).mockReturnValueOnce(fast.promise)
    const paged = usePagedList<Row>({ fetch, immediate: false })
    void paged.load()
    void paged.load()
    fast.resolve({ list: [{ id: 2, name: 'fast' }], total: 1 })
    await flushPromises()
    slow.resolve({ list: [{ id: 1, name: 'slow' }], total: 2 })
    await flushPromises()
    expect(paged.rows.value).toEqual([{ id: 2, name: 'fast' }])
    expect(paged.total.value).toBe(1)
    expect(paged.loading.value).toBe(false)
  })

  it('验收6: current=3 时 search() → current 变 1 且 fetch 以 (1,…) 追加一次', async () => {
    const fetch = vi.fn().mockResolvedValue({ list: [], total: 0 })
    const paged = usePagedList<Row>({ fetch, immediate: false })
    paged.current.value = 3
    await flushPromises()
    expect(fetch).toHaveBeenCalledTimes(1)
    paged.search()
    await flushPromises()
    expect(paged.current.value).toBe(1)
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(fetch).toHaveBeenLastCalledWith(1, 20)
  })

  it('验收7: current=1 时 search() → 直接 load, 不改页码', async () => {
    const fetch = vi.fn().mockResolvedValue({ list: [], total: 0 })
    const paged = usePagedList<Row>({ fetch, immediate: false })
    await paged.load()
    expect(fetch).toHaveBeenCalledTimes(1)
    paged.search()
    await flushPromises()
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(fetch).toHaveBeenLastCalledWith(1, 20)
    expect(paged.current.value).toBe(1)
  })

  it('验收8: pageSize: 50 → fetch 首参 (1, 50)', () => {
    const fetch = vi.fn(async () => ({ list: [], total: 0 }))
    usePagedList<Row>({ fetch, pageSize: 50 })
    expect(fetch).toHaveBeenCalledWith(1, 50)
  })

  it('验收9: immediate: false → setup 后不自动 fetch', () => {
    const fetch = vi.fn(async () => ({ list: [], total: 0 }))
    usePagedList<Row>({ fetch, immediate: false })
    expect(fetch).not.toHaveBeenCalled()
  })

  it('取消类错误(CanceledError) 静默: 不写 loadError', async () => {
    const canceled = Object.assign(new Error('canceled'), { name: 'CanceledError' })
    const fetch = vi.fn()
      .mockResolvedValueOnce({ list: [{ id: 1, name: 'a' }], total: 1 })
      .mockRejectedValueOnce(canceled)
    const paged = usePagedList<Row>({ fetch, immediate: false })
    await paged.load()
    paged.current.value = 2
    await flushPromises()
    expect(paged.loadError.value).toBe('')
    expect(paged.rows.value).toHaveLength(1)
  })

  it('current watch 是唯一加载触发器: current 变化自动 load', async () => {
    const fetch = vi.fn(async () => ({ list: [], total: 0 }))
    const paged = usePagedList<Row>({ fetch, immediate: false })
    paged.current.value = 5
    await flushPromises()
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch).toHaveBeenCalledWith(5, 20)
  })
})

import { afterEach, describe, expect, it, vi } from 'vitest'
import { createSegmentPrefetcher } from './segmentPrefetch'

/**
 * 预取的三个关键契约（2026-11-25 线上问题回归）：
 *  1. 锚点 = **hls 正在加载的分片**，预取它后面的 N 个（不能锚在"正在播放"，否则全打在已缓冲区间）；
 *  2. `no-cache` 不禁用（浏览器仍会存，最多多一次 304）；只有 `no-store` 才禁用；
 *  3. 连续加载不重复预取同一个后续分片（水位 prefetchedUpTo）。
 */

const frags = (n: number): { url: string }[] =>
  Array.from({ length: n }, (_, i) => ({ url: `https://cdn.test/s${i}.ts` }))
const url = (i: number): string => `https://cdn.test/s${i}.ts`

function resp(cacheControl: string): Response {
  return {
    headers: { get: (k: string) => (k.toLowerCase() === 'cache-control' ? cacheControl : null) },
    arrayBuffer: async () => new ArrayBuffer(0)
  } as unknown as Response
}

/** 让已 resolve 的 promise 链跑完 */
const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 0))

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('segmentPrefetch', () => {
  it('锚在"正在加载的分片"，预取其后 N 个（不是播放位置）', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u: string) => {
        calls.push(u)
        return resp('no-cache')
      })
    )
    const p = createSegmentPrefetcher({ ahead: 2, maxInflight: 3 })
    p.onLevel(frags(10))
    p.onLoading(3)

    await tick()
    expect(calls).toEqual([url(4), url(5)])
    // no-cache 不应禁用
    expect(p.active).toBe(true)
    p.destroy()
  })

  it('连续加载不重复预取同一分片（水位生效）', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u: string) => {
        calls.push(u)
        return resp('no-cache')
      })
    )
    const p = createSegmentPrefetcher({ ahead: 2, maxInflight: 3 })
    p.onLevel(frags(10))
    p.onLoading(3)
    await tick()
    calls.length = 0

    p.onLoading(4) // 期望：只补预取 s6，不重复 s5
    await tick()
    expect(calls).toEqual([url(6)])
    p.destroy()
  })

  it('no-store 才禁用；禁用后不再发请求', async () => {
    let calls = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        calls += 1
        return resp('no-store')
      })
    )
    const p = createSegmentPrefetcher({ ahead: 2 })
    p.onLevel(frags(6))
    p.onLoading(0)
    await tick()

    expect(p.active).toBe(false)
    const before = calls
    p.onLoading(1)
    await tick()
    expect(calls).toBe(before)
    p.destroy()
  })

  it('跳转（seek）会取消在途预取', async () => {
    const signals: AbortSignal[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((_u: string, init?: RequestInit) => {
        if (init?.signal) signals.push(init.signal)
        return new Promise<Response>(() => {}) // 永不完成，模拟在途
      })
    )
    const p = createSegmentPrefetcher({ ahead: 2 })
    p.onLevel(frags(200))
    p.onLoading(0)
    await Promise.resolve()
    expect(signals.length).toBeGreaterThan(0)

    const before = signals.length
    p.onLoading(100) // 远跳 ⇒ 取消旧区间在途请求（随后会给新区间发新的）
    expect(signals.slice(0, before).every((s) => s.aborted)).toBe(true)
    p.destroy()
  })


  it('并发上限挡住的分片不会被水位吞掉（下轮补上）', async () => {
    const calls: string[] = []
    let release: (() => void) | null = null
    vi.stubGlobal(
      'fetch',
      vi.fn((u: string) => {
        calls.push(u)
        return new Promise<Response>((resolve) => {
          release = () => resolve(resp('no-cache'))
        })
      })
    )
    const p = createSegmentPrefetcher({ ahead: 4, maxInflight: 1 })
    p.onLevel(frags(10))
    p.onLoading(0)
    await Promise.resolve()
    expect(calls).toEqual([url(1)]) // 只发出 1 个

    release?.()
    await tick()
    p.onLoading(1)
    await tick()
    // s2 必须在下一轮补上，而不是因为水位被跳过
    expect(calls).toContain(url(2))
    p.destroy()
  })

  it('关掉再开（setAhead 0→2）能重新预取', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u: string) => {
        calls.push(u)
        return resp('no-cache')
      })
    )
    const p = createSegmentPrefetcher({ ahead: 2 })
    p.onLevel(frags(10))
    p.onLoading(3)
    await tick()

    p.setAhead(0)
    expect(p.active).toBe(false)
    calls.length = 0

    p.setAhead(2) // 水位应被重置，重新按当前位置补
    await tick()
    expect(calls.length).toBeGreaterThan(0)
    p.destroy()
  })

  it('往回 seek 也会重置水位（幅度很小时同样生效）', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u: string) => {
        calls.push(u)
        return resp('no-cache')
      })
    )
    const p = createSegmentPrefetcher({ ahead: 2 })
    p.onLevel(frags(20))
    p.onLoading(8)
    await tick()
    calls.length = 0

    p.onLoading(6) // 往回 2（小于跳变阈值）
    await tick()
    expect(calls).toEqual([url(7), url(8)])
    p.destroy()
  })

  it('setAhead(0) 关闭并发（清空在途）', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((_u: string, init?: RequestInit) => {
        void init
        return new Promise<Response>(() => {})
      })
    )
    const p = createSegmentPrefetcher({ ahead: 2 })
    p.onLevel(frags(10))
    p.onLoading(0)
    await Promise.resolve()
    expect(p.active).toBe(true)

    p.setAhead(0)
    expect(p.active).toBe(false)
    p.destroy()
  })
})
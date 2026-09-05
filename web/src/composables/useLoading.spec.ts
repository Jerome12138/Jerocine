import { describe, it, expect } from 'vitest'
import { useLoading } from './useLoading'

describe('useLoading', () => {
  it('loading 初始 false', () => {
    const { loading } = useLoading()
    expect(loading.value).toBe(false)
  })

  it('run() 期间 loading=true, 完成后 loading=false', async () => {
    const { loading, run } = useLoading()
    let midState = false
    const p = run(async () => {
      midState = loading.value
      return 42
    })
    expect(loading.value).toBe(true)
    const result = await p
    expect(midState).toBe(true)
    expect(result).toBe(42)
    expect(loading.value).toBe(false)
  })

  it('run() 异常时 loading 也归零', async () => {
    const { loading, run } = useLoading()
    await expect(run(async () => { throw new Error('boom') })).rejects.toThrow('boom')
    expect(loading.value).toBe(false)
  })

  it('多个 useLoading 实例互相独立', () => {
    const a = useLoading()
    const b = useLoading()
    expect(a.loading).not.toBe(b.loading)
  })
})

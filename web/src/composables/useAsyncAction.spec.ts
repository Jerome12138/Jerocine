import { describe, it, expect, beforeEach, vi } from 'vitest'

// toast 单点来自 http.ts(实际由拦截器/调用方共用), spec 里 mock 掉观察口径
vi.mock('@/api/http', () => ({ toast: vi.fn() }))

import { toast } from '@/api/http'
import { useAsyncAction } from './useAsyncAction'
import { confirm, answerConfirm, confirmState } from './useConfirm'

/** 可控 deferred: 手动 resolve/reject */
function deferred<T>(): { promise: Promise<T>; resolve: (v: T) => void; reject: (e: unknown) => void } {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('useAsyncAction', () => {
  beforeEach(() => {
    vi.mocked(toast).mockClear()
    confirmState.visible = false
    confirmState.resolve = null
  })

  it('run 成功 → action 调用 + toast(success, successMsg) + after 调用 + 返回 true + busy 复位', async () => {
    const { busy, run } = useAsyncAction()
    const action = vi.fn().mockResolvedValue(undefined)
    const after = vi.fn()
    const ok = await run({ action, successMsg: '已移入回收站', after })
    expect(ok).toBe(true)
    expect(action).toHaveBeenCalledTimes(1)
    expect(toast).toHaveBeenCalledWith('success', '已移入回收站')
    expect(after).toHaveBeenCalledTimes(1)
    expect(busy.value).toBe(false)
  })

  it('action reject Error("x") → toast(error,"x") + after 未调用 + 返回 false', async () => {
    const { busy, run } = useAsyncAction()
    const after = vi.fn()
    const ok = await run({
      action: () => Promise.reject(new Error('x')),
      successMsg: '成功',
      after
    })
    expect(ok).toBe(false)
    expect(toast).toHaveBeenCalledWith('error', 'x')
    expect(after).not.toHaveBeenCalled()
    expect(busy.value).toBe(false)
  })

  it('给 confirm 且用户取消(answerConfirm(false)) → action 未调用 + 返回 false', async () => {
    const { run } = useAsyncAction()
    const action = vi.fn().mockResolvedValue(undefined)
    const p = run({
      confirm: { title: '删除？' },
      action
    })
    await Promise.resolve()
    expect(confirmState.visible).toBe(true)
    answerConfirm(false)
    const ok = await p
    expect(ok).toBe(false)
    expect(action).not.toHaveBeenCalled()
  })

  it('busy 中第二次 run → 立即 false, action 不二次执行', async () => {
    const { busy, run } = useAsyncAction()
    const d = deferred<void>()
    const action = vi.fn().mockReturnValue(d.promise)
    const p1 = run({ action })
    expect(busy.value).toBe(true)
    const ok2 = await run({ action })
    expect(ok2).toBe(false)
    expect(action).toHaveBeenCalledTimes(1)
    d.resolve(undefined)
    const ok1 = await p1
    expect(ok1).toBe(true)
    expect(busy.value).toBe(false)
  })

  it('successMsg 省略 → 无 success toast(仍返回 true)', async () => {
    const { run } = useAsyncAction()
    const ok = await run({ action: () => Promise.resolve() })
    expect(ok).toBe(true)
    expect(toast).not.toHaveBeenCalled()
  })

  it('action reject CanceledError → 静默(无 error toast) + 返回 false', async () => {
    const { run } = useAsyncAction()
    const e = new Error('canceled')
    e.name = 'CanceledError'
    const ok = await run({ action: () => Promise.reject(e) })
    expect(ok).toBe(false)
    expect(toast).not.toHaveBeenCalled()
  })
})

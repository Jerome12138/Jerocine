import { describe, it, expect } from 'vitest'
import { effectScope } from 'vue'
import { useAbortable } from './useAbortable'

describe('useAbortable', () => {
  it('signal 初始未 aborted', () => {
    const scope = effectScope()
    scope.run(() => {
      const { signal } = useAbortable()
      expect(signal.aborted).toBe(false)
    })
    scope.stop()
  })

  it('调 abort() → signal.aborted=true', () => {
    const scope = effectScope()
    scope.run(() => {
      const { signal, abort } = useAbortable()
      abort('test')
      expect(signal.aborted).toBe(true)
    })
    scope.stop()
  })

  it('已 aborted 后再调 abort() 不重复', () => {
    const scope = effectScope()
    scope.run(() => {
      const { abort, signal } = useAbortable()
      abort('first')
      const reasonBefore = signal.reason
      abort('second')
      // signal reason 应保持第一次
      expect(signal.reason).toBe(reasonBefore)
    })
    scope.stop()
  })

  it('refresh() → 旧 signal aborted, 返回新 signal 可用', () => {
    const scope = effectScope()
    scope.run(() => {
      const { signal: oldSignal, refresh } = useAbortable()
      const newSignal = refresh()
      expect(oldSignal.aborted).toBe(true)
      expect(newSignal.aborted).toBe(false)
      expect(newSignal).not.toBe(oldSignal)
    })
    scope.stop()
  })

  it('scope dispose → signal 自动 abort', () => {
    const scope = effectScope()
    let signal: AbortSignal | null = null
    scope.run(() => {
      signal = useAbortable().signal
    })
    expect(signal!.aborted).toBe(false)
    scope.stop()
    expect(signal!.aborted).toBe(true)
  })
})

import { onScopeDispose } from 'vue'

/**
 * 自动 abort 的请求 helper
 * 在组件 / effect scope dispose 时自动取消未完成请求
 *
 * 使用：
 *   const { signal, abort, refresh } = useAbortable()
 *   const data = await filmApi.getIndex({ signal })
 *
 * refresh() —— 复位 controller 用于重新发起请求
 */
export function useAbortable(): {
  signal: AbortSignal
  abort: (reason?: string) => void
  refresh: () => AbortSignal
} {
  let controller = new AbortController()

  function abort(reason?: string): void {
    if (!controller.signal.aborted) {
      controller.abort(reason)
    }
  }

  function refresh(): AbortSignal {
    abort('refresh')
    controller = new AbortController()
    return controller.signal
  }

  onScopeDispose(() => {
    abort('scope-dispose')
  })

  return {
    get signal(): AbortSignal {
      return controller.signal
    },
    abort,
    refresh
  }
}

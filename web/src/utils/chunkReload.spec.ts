import { describe, it, expect, vi } from 'vitest'
import {
  CHUNK_RELOAD_FALLBACK_MS,
  CHUNK_RELOAD_GUARD_KEY,
  handleChunkLoadError,
  isChunkLoadError,
  reloadToLatest,
  type ChunkReloadDeps
} from './chunkReload'

const CHUNK_ERR = 'Failed to fetch dynamically imported module: /assets/x.js'

function makeGuardStore() {
  const map = new Map<string, string>()
  return {
    map,
    getItem: (k: string): string | null => map.get(k) ?? null,
    setItem: (k: string, v: string): void => {
      map.set(k, v)
    },
    removeItem: (k: string): void => {
      map.delete(k)
    }
  }
}

interface Harness {
  deps: ChunkReloadDeps
  reload: ReturnType<typeof vi.fn>
  track: ReturnType<typeof vi.fn>
  guard: ReturnType<typeof makeGuardStore>
  timers: Array<() => void>
  fireTimers: () => void
}

function makeHarness(overrides: Partial<ChunkReloadDeps> = {}): Harness {
  const reload = vi.fn()
  const track = vi.fn()
  const guard = makeGuardStore()
  const timers: Array<() => void> = []
  const deps: ChunkReloadDeps = {
    guardStore: guard,
    reload,
    getRegistration: () => undefined,
    track,
    setTimer: (fn) => {
      timers.push(fn)
    },
    ...overrides
  }
  return {
    deps,
    reload,
    track,
    guard,
    timers,
    fireTimers: () => {
      while (timers.length) timers.shift()?.()
    }
  }
}

const flush = (): Promise<void> => new Promise((r) => setTimeout(r, 0))

describe('isChunkLoadError', () => {
  it('识别 4 类 chunk 加载失败文案', () => {
    expect(isChunkLoadError(new Error('dynamically imported module'))).toBe(true)
    expect(isChunkLoadError(new Error("'text/html' is not a valid JavaScript MIME type"))).toBe(true)
    expect(isChunkLoadError(new Error('Failed to fetch dynamically imported module'))).toBe(true)
    expect(isChunkLoadError(new Error('Importing a module script failed'))).toBe(true)
  })

  it('普通错误不误判', () => {
    expect(isChunkLoadError(new Error('Network Error'))).toBe(false)
    expect(isChunkLoadError('some string error')).toBe(false)
    expect(isChunkLoadError(undefined)).toBe(false)
  })
})

describe('handleChunkLoadError', () => {
  it('非 chunk 错误 → ignored, 不刷新不埋点', () => {
    const h = makeHarness()
    expect(handleChunkLoadError(new Error('boom'), h.deps)).toBe('ignored')
    expect(h.reload).not.toHaveBeenCalled()
    expect(h.track).not.toHaveBeenCalled()
    expect(h.timers).toHaveLength(0)
  })

  it('首次 chunk 失败(无 SW 注册) → 直接刷新一次', () => {
    const h = makeHarness()
    expect(handleChunkLoadError(new Error(CHUNK_ERR), h.deps)).toBe('reloaded')
    expect(h.guard.map.get(CHUNK_RELOAD_GUARD_KEY)).toBe('1')
    expect(h.reload).not.toHaveBeenCalled() // 走 setTimer（留 50ms flush）
    h.fireTimers()
    expect(h.reload).toHaveBeenCalledTimes(1)
    expect(h.track).toHaveBeenCalledWith(expect.any(Error), 'chunk-load-reload', { msg: CHUNK_ERR })
  })

  it('同会话第二次失败 → guarded, 只上报不刷新(防死循环)', () => {
    const h = makeHarness()
    expect(handleChunkLoadError(new Error(CHUNK_ERR), h.deps)).toBe('reloaded')
    h.fireTimers()
    h.reload.mockClear()
    h.track.mockClear()

    expect(handleChunkLoadError(new Error(CHUNK_ERR), h.deps)).toBe('guarded')
    expect(h.reload).not.toHaveBeenCalled()
    expect(h.track).toHaveBeenCalledWith(
      expect.any(Error),
      'chunk-load-reload-guarded',
      { msg: CHUNK_ERR }
    )
  })

  it('有 SW 注册 → 先 update 再刷新, 且只刷一次(兜底定时器不重复触发)', async () => {
    const update = vi.fn().mockResolvedValue(undefined)
    const h = makeHarness({ getRegistration: () => Promise.resolve({ update }) })
    expect(handleChunkLoadError(new Error(CHUNK_ERR), h.deps)).toBe('reloaded')

    await flush()
    expect(update).toHaveBeenCalledTimes(1)

    // 兜底定时器(1500ms) + update 完成后排入的 50ms 刷新定时器, 都跑一遍
    h.fireTimers()
    h.fireTimers()
    expect(h.reload).toHaveBeenCalledTimes(1)
  })

  it('update 挂在兜底延时上时, 兜底定时器仍会刷新', async () => {
    // 永不 resolve 的 update
    const h = makeHarness({ getRegistration: () => new Promise(() => {}) })
    expect(handleChunkLoadError(new Error(CHUNK_ERR), h.deps)).toBe('reloaded')
    expect(h.timers.length).toBe(1) // 只有兜底定时器
    expect(CHUNK_RELOAD_FALLBACK_MS).toBeGreaterThan(0)
    h.fireTimers()
    expect(h.reload).toHaveBeenCalledTimes(1)
  })

  it('getRegistration 抛错 → 仍刷新', () => {
    const h = makeHarness({
      getRegistration: () => {
        throw new Error('no sw')
      }
    })
    expect(handleChunkLoadError(new Error(CHUNK_ERR), h.deps)).toBe('reloaded')
    h.fireTimers()
    expect(h.reload).toHaveBeenCalledTimes(1)
  })

  it('update 被 reject → 仍刷新', async () => {
    const update = vi.fn().mockRejectedValue(new Error('offline'))
    const h = makeHarness({ getRegistration: () => Promise.resolve({ update }) })
    handleChunkLoadError(new Error(CHUNK_ERR), h.deps)
    await flush()
    h.fireTimers()
    expect(h.reload).toHaveBeenCalledTimes(1)
  })

  it('闸门存储不可用 → 当作已闸门, 不刷新', () => {
    const h = makeHarness({
      guardStore: {
        getItem: () => null,
        setItem: () => {
          throw new Error('quota')
        },
        removeItem: () => {}
      }
    })
    expect(handleChunkLoadError(new Error(CHUNK_ERR), h.deps)).toBe('guarded')
    expect(h.reload).not.toHaveBeenCalled()
  })
})

describe('reloadToLatest (TV 胶囊行「刷新」显式动作)', () => {
  it('清掉一次性闸门后再刷新(用户显式意图 ⇒ 恢复自愈能力)', () => {
    const h = makeHarness()
    h.guard.map.set(CHUNK_RELOAD_GUARD_KEY, '1') // 模拟本会话已自愈过一次
    reloadToLatest(h.deps)
    expect(h.guard.map.has(CHUNK_RELOAD_GUARD_KEY)).toBe(false)
    h.fireTimers()
    expect(h.reload).toHaveBeenCalledTimes(1)
    // 清闸门后, 后续 chunk 404 仍能自愈(不被 guarded 卡住)
    expect(handleChunkLoadError(new Error(CHUNK_ERR), h.deps)).toBe('reloaded')
  })

  it('有 SW 注册 → 先 update 再刷新', async () => {
    const update = vi.fn().mockResolvedValue(undefined)
    const h = makeHarness({ getRegistration: () => Promise.resolve({ update }) })
    reloadToLatest(h.deps)
    await flush()
    expect(update).toHaveBeenCalledTimes(1)
    h.fireTimers()
    h.fireTimers()
    expect(h.reload).toHaveBeenCalledTimes(1)
  })

  it('无 SW 注册(纯网页/不支持) → 仍然刷新', () => {
    const h = makeHarness({ getRegistration: () => undefined })
    reloadToLatest(h.deps)
    h.fireTimers()
    expect(h.reload).toHaveBeenCalledTimes(1)
  })

  it('闸门存储不可用 → 不影响刷新', () => {
    const h = makeHarness({
      guardStore: {
        getItem: () => null,
        setItem: () => {},
        removeItem: () => {
          throw new Error('quota')
        }
      }
    })
    reloadToLatest(h.deps)
    h.fireTimers()
    expect(h.reload).toHaveBeenCalledTimes(1)
  })
})

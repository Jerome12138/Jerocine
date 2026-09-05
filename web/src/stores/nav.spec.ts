import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useNavStore } from './nav'

vi.mock('@/api/film', () => ({
  getCategories: vi.fn()
}))

import { getCategories } from '@/api/film'

describe('useNavStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('list 初始空, loaded=false, loading=false', () => {
    const s = useNavStore()
    expect(s.list).toEqual([])
    expect(s.loaded).toBe(false)
    expect(s.loading).toBe(false)
  })

  it('ensureLoaded 调用 api 并填充 list', async () => {
    ;(getCategories as ReturnType<typeof vi.fn>).mockResolvedValue([
      { id: 1, name: '电影', children: [] }
    ])
    const s = useNavStore()
    await s.ensureLoaded()
    expect(s.list).toHaveLength(1)
    expect(s.loaded).toBe(true)
    expect(s.loading).toBe(false)
  })

  it('已 loaded → ensureLoaded 不再调 api', async () => {
    ;(getCategories as ReturnType<typeof vi.fn>).mockResolvedValue([])
    const s = useNavStore()
    await s.ensureLoaded()
    await s.ensureLoaded()
    expect(getCategories).toHaveBeenCalledTimes(1)
  })

  it('findById 递归查子节点', () => {
    const s = useNavStore()
    s.list = [
      { id: 1, name: 'A', children: [{ id: 11, name: 'A-1', children: [] }] } as never,
      { id: 2, name: 'B', children: [] } as never
    ]
    expect(s.findById(11)?.name).toBe('A-1')
    expect(s.findById(2)?.name).toBe('B')
  })

  it('findById 不存在 → undefined', () => {
    const s = useNavStore()
    expect(s.findById(999)).toBeUndefined()
  })

  it('loading 中 → 并发 ensureLoaded 不重复 fetch', async () => {
    ;(getCategories as ReturnType<typeof vi.fn>).mockImplementation(
      () => new Promise((r) => setTimeout(() => r([]), 30))
    )
    const s = useNavStore()
    const p1 = s.ensureLoaded()
    const p2 = s.ensureLoaded()
    await Promise.all([p1, p2])
    expect(getCategories).toHaveBeenCalledTimes(1)
  })
})

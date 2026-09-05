import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useFavoriteStore } from './favorite'

vi.mock('@/api/favorite', () => ({
  listFavorite: vi.fn().mockResolvedValue([]),
  addFavorite: vi.fn().mockResolvedValue(undefined),
  removeFavorite: vi.fn().mockResolvedValue(undefined),
  checkFavorite: vi.fn().mockResolvedValue(false)
}))

describe('useFavoriteStore (本地模式)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('初始 map={}, list=[]', () => {
    const s = useFavoriteStore()
    expect(s.map).toEqual({})
    expect(s.list).toEqual([])
  })

  it('localStorage 已有 → 初始化加载', () => {
    localStorage.setItem(
      'filmFavorite',
      JSON.stringify({
        '1': { id: '1', name: 'A', timeStamp: 100 }
      })
    )
    const s = useFavoriteStore()
    expect(s.map['1']?.name).toBe('A')
  })

  it('add() 加收藏 + 持久化', async () => {
    const s = useFavoriteStore()
    await s.add({ id: 1 as never, name: 'X' } as never)
    expect(s.map['1']?.name).toBe('X')
    expect(s.isFavorited(1)).toBe(true)
    expect(s.isFavorited('1')).toBe(true)
    // 持久化
    expect(localStorage.getItem('filmFavorite')).toContain('"X"')
  })

  it('isFavorited 不存在 → false', () => {
    const s = useFavoriteStore()
    expect(s.isFavorited(999)).toBe(false)
  })

  it('add 重复 ID → 幂等 (只刷新 timeStamp)', async () => {
    const s = useFavoriteStore()
    await s.add({ id: '1' as never, name: 'A' } as never)
    const t1 = s.map['1'].timeStamp
    await new Promise((r) => setTimeout(r, 5))
    await s.add({ id: '1' as never, name: 'A again' } as never)
    expect(Object.keys(s.map).length).toBe(1)
    expect(s.map['1'].timeStamp).toBeGreaterThanOrEqual(t1)
  })

  it('add 空 id → no-op', async () => {
    const s = useFavoriteStore()
    await s.add({ id: '' as never, name: 'X' } as never)
    expect(Object.keys(s.map).length).toBe(0)
  })

  it('损坏的 localStorage JSON → 安全 fallback 空对象', () => {
    localStorage.setItem('filmFavorite', '{not json')
    expect(() => useFavoriteStore()).not.toThrow()
    const s = useFavoriteStore()
    expect(s.map).toEqual({})
  })

  it('数组形式 localStorage (非 object) → 安全忽略', () => {
    localStorage.setItem('filmFavorite', '[1,2,3]')
    const s = useFavoriteStore()
    expect(s.map).toEqual({})
  })
})

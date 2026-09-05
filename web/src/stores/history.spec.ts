import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useHistoryStore, buildPlayLink } from './history'

vi.mock('@/api/history', () => ({
  listHistory: vi.fn().mockResolvedValue([]),
  upsertHistory: vi.fn().mockResolvedValue(undefined),
  removeHistory: vi.fn().mockResolvedValue(undefined),
  clearHistory: vi.fn().mockResolvedValue(undefined)
}))

function clearAllCookies(): void {
  document.cookie.split(';').forEach((c) => {
    const eq = c.indexOf('=')
    const name = (eq > -1 ? c.slice(0, eq) : c).trim()
    document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`
  })
}

describe('useHistoryStore (本地模式)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    clearAllCookies()
    vi.clearAllMocks()
  })

  it('初始 map={}, list=[]', () => {
    const s = useHistoryStore()
    expect(s.list.length).toBe(0)
  })

  it('record() 写入 + 持久化到 localStorage', () => {
    const s = useHistoryStore()
    s.record({
      id: '100',
      name: '电影 A',
      link: '/play?id=100&source=x&episode=0',
      episode: '第 1 集',
      source: 'jerocine',
      episodeIndex: 0
    } as never)
    expect(s.get('100')?.name).toBe('电影 A')
    expect(localStorage.getItem('filmHistory')).toContain('100')
  })

  it('record 重复 id → 更新 timeStamp + 不增长 list 长度', () => {
    const s = useHistoryStore()
    s.record({
      id: '1',
      name: 'X',
      link: '/play?id=1&source=s&episode=0',
      episode: 'E1'
    } as never)
    s.record({
      id: '1',
      name: 'X again',
      link: '/play?id=1&source=s&episode=0',
      episode: 'E1'
    } as never)
    expect(s.list.length).toBe(1)
    expect(s.get('1')?.name).toBe('X again')
  })

  it('record 不安全 link (不是 /play?...) → link 字段被清空', () => {
    const s = useHistoryStore()
    s.record({
      id: '2',
      name: 'X',
      link: 'javascript:alert(1)' as never,
      episode: 'E'
    } as never)
    // 记录仍写入, 但 link 字段被清空 (避免 XSS)
    expect(s.get('2')?.link).toBe('')
  })

  it('remove() 删除单条', async () => {
    const s = useHistoryStore()
    s.record({
      id: '1',
      name: 'A',
      link: '/play?id=1&source=s&episode=0',
      episode: 'E'
    } as never)
    await s.remove('1')
    expect(s.get('1')).toBeUndefined()
  })

  it('clear() 清空全部', async () => {
    const s = useHistoryStore()
    s.record({
      id: '1',
      name: 'A',
      link: '/play?id=1&source=s&episode=0',
      episode: 'E'
    } as never)
    s.record({
      id: '2',
      name: 'B',
      link: '/play?id=2&source=s&episode=0',
      episode: 'E'
    } as never)
    await s.clear()
    expect(s.list.length).toBe(0)
  })

  it('损坏的 localStorage JSON → 安全 fallback', () => {
    localStorage.setItem('filmHistory', '{broken')
    expect(() => useHistoryStore()).not.toThrow()
  })

  it('localStorage 数组形式 (老格式) → 转 map', () => {
    localStorage.setItem(
      'filmHistory',
      JSON.stringify([
        { id: 'old1', name: 'Old A', link: '/play?id=old1&source=x', episode: 'E', timeStamp: 100 }
      ])
    )
    const s = useHistoryStore()
    expect(s.get('old1')?.name).toBe('Old A')
  })
})

describe('buildPlayLink (续播链接实时拼)', () => {
  it('由当前字段拼 /play 链接(带 currentTime)', () => {
    expect(buildPlayLink({ id: '10', source: 'lz', episodeIndex: 4, currentTime: 320 })).toBe(
      '/play?id=10&source=lz&episode=4&currentTime=320'
    )
  })
  it('无进度时不带 currentTime', () => {
    expect(buildPlayLink({ id: '10', source: 'lz', episodeIndex: 0, currentTime: 0 })).toBe(
      '/play?id=10&source=lz&episode=0'
    )
  })
  it('缺 source/episodeIndex → 兜底空源/第0集索引', () => {
    expect(buildPlayLink({ id: '7' })).toBe('/play?id=7&source=&episode=0')
  })
})

describe('updateProgress 重建 link(续播接当前集)', () => {
  it('updateProgress 后 record.link 指向新 episodeIndex/currentTime', () => {
    const s = useHistoryStore()
    s.record({
      id: '100',
      name: '片',
      link: '/play?id=100&source=lz&episode=0',
      episode: '第1集',
      source: 'lz',
      episodeIndex: 0,
      currentTime: 0
    })
    s.updateProgress('100', 5, 600, undefined, 'lz')
    expect(s.get('100')?.link).toBe('/play?id=100&source=lz&episode=5&currentTime=600')
    expect(s.get('100')?.episodeIndex).toBe(5)
  })
})

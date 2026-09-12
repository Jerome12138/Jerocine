import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useHistoryStore, buildPlayLink, recordToCard } from './history'

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

describe('clearEpisode 不删影片级记录(2026-09-12 线上 bug 回归)', () => {
  it('切集(自动/手动): 清上一集独立进度, 影片级保留且进度清零, 随后 updateProgress 可推进到新集', async () => {
    const s = useHistoryStore()
    s.record({
      id: '200',
      name: '剧',
      link: '/play?id=200&source=lz&episode=2&currentTime=1200',
      episode: '第3集',
      source: 'lz',
      episodeIndex: 2,
      currentTime: 1200
    })
    // 原生播放器 onMediaItemTransition: 从第2集切到第3集(自动或手动都带 fromIndex)
    await s.clearEpisode('200', 'lz', 2)
    // 影片级记录必须还在(旧实现被删 → 后续 updateProgress 全部 no-op → 影片从历史消失)
    expect(s.get('200')).toBeDefined()
    expect(s.get('200')?.currentTime).toBe(0)
    // 该集独立进度已清(getEpisode 回退命中的是影片级清零记录 → 重开从头播)
    expect(s.getEpisode('200', 'lz', 2)?.currentTime).toBe(0)
    // 紧跟的切集通知: updateProgress 能正常推进到新集(不再因记录缺失而丢失)
    s.updateProgress('200', 3, 0, undefined, 'lz')
    expect(s.get('200')?.episodeIndex).toBe(3)
    expect(s.get('200')?.currentTime).toBe(0)
  })

  it('影片级指向别的集 → clearEpisode 不动影片级', async () => {
    const s = useHistoryStore()
    s.record({
      id: '201',
      name: '剧2',
      link: '/play?id=201&source=lz&episode=5',
      episode: '第6集',
      source: 'lz',
      episodeIndex: 5,
      currentTime: 300
    })
    await s.clearEpisode('201', 'lz', 2)
    expect(s.get('201')?.currentTime).toBe(300)
  })

  it('播完退出(ended): 影片级保留、进度清零, 重开从头播(而非从历史消失)', async () => {
    const s = useHistoryStore()
    s.record({
      id: '300',
      name: '电影',
      link: '/play?id=300&source=lz&episode=0&currentTime=5000',
      episode: '正片',
      source: 'lz',
      episodeIndex: 0,
      currentTime: 5000
    })
    await s.clearEpisode('300', 'lz', 0)
    expect(s.get('300')).toBeDefined()
    expect(s.get('300')?.currentTime).toBe(0)
    expect(s.get('300')?.name).toBe('电影')
  })
})

describe('recordToCard (历史卡复用 FilmCard)', () => {
  it('remarks 取影片自身更新状态(HD / 更新至 N 集), 与普通影片卡同位', () => {
    const card = recordToCard({
      id: '12',
      name: '片',
      link: '/play?id=12&source=lz&episode=0',
      episode: '第3集',
      timeStamp: 1,
      remarks: 'HD'
    })
    expect(card.remarks).toBe('HD')
    expect(card.name).toBe('片')
    expect(card.mid).toBe(12)
  })

  it('老记录无 remarks → 空串(卡片左下角不显示), 不退回集数', () => {
    const card = recordToCard({
      id: '13',
      name: '片2',
      link: '/play?id=13',
      episode: '第1集',
      timeStamp: 1
    })
    expect(card.remarks).toBe('')
  })
})

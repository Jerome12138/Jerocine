import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest'
import { buildLivePlaylistCfg, dispatchNativeLive, dispatchNativeLiveForRoute } from './nativeLive'
import type { LiveChannel, LiveSource } from '@/types/live'

const ch = (name: string, url = `http://s/${name}`): LiveChannel => ({ name, group: '', logo: '', url })
const src = (id: number, name: string): LiveSource => ({ id, name })

const BRIDGE_KEY = 'JerocinePlayer' as const

function installBridge(invoke: (m: string, a: string) => string): void {
  ;(window as unknown as Record<string, unknown>)[BRIDGE_KEY] = { invoke }
}

describe('buildLivePlaylistCfg：直播 → playPlaylist 映射', () => {
  it('源 → 播放器源, 频道 → 集; 选中当前源, 起始下标透传', () => {
    const cfg = buildLivePlaylistCfg(
      [src(1, 'A'), src(2, 'B')],
      new Map([
        [1, [ch('a1'), ch('a2')]],
        [2, [ch('b1')]]
      ]),
      1,
      1
    )!
    expect(cfg.sources.map((s) => s.id)).toEqual(['1', '2'])
    expect(cfg.currentSourceId).toBe('1')
    expect(cfg.startIndex).toBe(1)
    // 非数字 filmId：不写影视历史
    expect(cfg.filmId).toBe('')
    expect(cfg.sources[0]!.episodes[0]).toEqual({ url: 'http://s/a1', title: 'a1' })
  })

  it('未命中当前源 → 回落第一个可用源; 越界下标钳制', () => {
    const cfg = buildLivePlaylistCfg([src(1, 'A'), src(2, 'B')], new Map([[2, [ch('b1')]]]), 1, 99)!
    expect(cfg.currentSourceId).toBe('2')
    expect(cfg.startIndex).toBe(0)
  })

  it('剔除无频道/空地址的源; 全不可播 → null', () => {
    expect(buildLivePlaylistCfg([src(1, 'A')], new Map([[1, [ch('x', '')]]]), 1, 0)).toBeNull()
    expect(buildLivePlaylistCfg([], new Map(), 0, 0)).toBeNull()
    const cfg = buildLivePlaylistCfg([src(1, 'A'), src(2, 'B')], new Map([[2, [ch('b1')]]]), 1, 0)!
    expect(cfg.sources.map((s) => s.id)).toEqual(['2'])
  })

  it('频道名为空 → 兜底「频道」', () => {
    const cfg = buildLivePlaylistCfg([src(1, 'A')], new Map([[1, [ch('')]]]), 1, 0)!
    expect(cfg.sources[0]!.episodes[0]!.title).toBe('频道')
  })
})

describe('dispatchNativeLive：桥在场才派发', () => {
  afterEach(() => {
    delete (window as unknown as Record<string, unknown>)[BRIDGE_KEY]
  })

  it('无原生桥 → 不派发(false)', () => {
    expect(dispatchNativeLive([src(1, 'A')], new Map([[1, [ch('a1')]]]), 1, 0)).toBe(false)
  })

  it('有原生桥 → invoke("playPlaylist", cfg) 并返回 true', () => {
    const invoke = vi.fn(() => '{"ok":true}')
    installBridge(invoke)

    expect(dispatchNativeLive([src(1, 'A')], new Map([[1, [ch('a1')]]]), 1, 0, 'CCTV1')).toBe(true)

    expect(invoke).toHaveBeenCalledWith('playPlaylist', expect.any(String))
    const cfg = JSON.parse(invoke.mock.calls[0]![1] as string) as ReturnType<typeof buildLivePlaylistCfg> & {
      filmName: string
    }
    expect(cfg!.currentSourceId).toBe('1')
    expect(cfg!.filmName).toBe('CCTV1')
  })

  it('有桥但无可用频道 → 不派发', () => {
    const invoke = vi.fn(() => '{"ok":true}')
    installBridge(invoke)
    expect(dispatchNativeLive([src(1, 'A')], new Map(), 1, 0)).toBe(false)
    expect(invoke).not.toHaveBeenCalled()
  })
})

/* ---------------- 路由守卫用：拉数据 + 派发 ---------------- */

const api = vi.hoisted(() => ({
  getLiveSources: vi.fn(),
  getLiveChannels: vi.fn()
}))

vi.mock('@/api', () => ({ liveApi: api }))

describe('dispatchNativeLiveForRoute：/live 守卫派发', () => {
  beforeEach(() => {
    localStorage.clear()
    api.getLiveSources.mockReset()
    api.getLiveChannels.mockReset()
    api.getLiveSources.mockResolvedValue({ sources: [src(1, 'A'), src(2, 'B')] })
    api.getLiveChannels.mockImplementation((id: number) =>
      Promise.resolve({ id, name: '', channels: id === 1 ? [ch('a1'), ch('a2')] : [ch('b0'), ch('b1')] })
    )
  })

  afterEach(() => {
    delete (window as unknown as Record<string, unknown>)[BRIDGE_KEY]
  })

  it('无原生桥 → false 且不拉数据', async () => {
    expect(await dispatchNativeLiveForRoute()).toBe(false)
    expect(api.getLiveSources).not.toHaveBeenCalled()
  })

  it('有桥 → 拉全源频道并派发, 起点为第一个源的第一个频道, 且回写记忆', async () => {
    const invoke = vi.fn(() => '{"ok":true}')
    installBridge(invoke)

    expect(await dispatchNativeLiveForRoute()).toBe(true)

    expect(api.getLiveChannels).toHaveBeenCalledTimes(2)
    const cfg = JSON.parse(invoke.mock.calls[0]![1] as string) as {
      currentSourceId: string
      startIndex: number
    }
    expect(cfg.currentSourceId).toBe('1')
    expect(cfg.startIndex).toBe(0)
    const last = JSON.parse(localStorage.getItem('jc-live-last-channel') ?? '{}') as Record<string, unknown>
    expect(last.sourceId).toBe(1)
    expect(last.url).toBe('http://s/a1')
    expect(last.sourceName).toBe('A')
    expect(last.channelName).toBe('a1')
    expect(typeof last.ts).toBe('number')
  })

  it('按上次观看(源 + 频道地址)定位起始频道', async () => {
    localStorage.setItem('jc-live-last-channel', JSON.stringify({ sourceId: 2, url: 'http://s/b1' }))
    const invoke = vi.fn(() => '{"ok":true}')
    installBridge(invoke)

    expect(await dispatchNativeLiveForRoute()).toBe(true)
    const cfg = JSON.parse(invoke.mock.calls[0]![1] as string) as {
      currentSourceId: string
      startIndex: number
    }
    expect(cfg.currentSourceId).toBe('2')
    expect(cfg.startIndex).toBe(1) // 记忆里的 b1 在源 2 的频道列表里是第 2 条(索引 1)
  })

  it('无直播源 → false(放行 web /live 兜底)', async () => {
    installBridge(vi.fn(() => '{"ok":true}'))
    api.getLiveSources.mockResolvedValue({ sources: [] })
    expect(await dispatchNativeLiveForRoute()).toBe(false)
  })

  it('单源频道拉取失败 → 该源剔除, 其余仍可派发', async () => {
    const invoke = vi.fn(() => '{"ok":true}')
    installBridge(invoke)
    api.getLiveChannels.mockImplementation((id: number) =>
      id === 1 ? Promise.reject(new Error('boom')) : Promise.resolve({ id, name: '', channels: [ch('b0')] })
    )

    expect(await dispatchNativeLiveForRoute()).toBe(true)
    const cfg = JSON.parse(invoke.mock.calls[0]![1] as string) as { sources: Array<{ id: string }> }
    expect(cfg.sources.map((s) => s.id)).toEqual(['2'])
  })
})

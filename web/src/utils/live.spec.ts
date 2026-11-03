import { describe, it, expect, beforeEach } from 'vitest'
import {
  groupChannels,
  LIVE_DEFAULT_GROUP,
  probeLiveChannel,
  probeLiveChannelsLimited,
  readLiveLast,
  writeLiveLast,
  clearLiveLast,
  liveLastLabel
} from './live'
import type { LiveChannel } from '@/types/live'

function ch(name: string, group = ''): LiveChannel {
  return { name, group, logo: '', url: `http://s/${name}` }
}

describe('groupChannels：频道分组聚合', () => {
  it('空列表 → 空分组', () => {
    expect(groupChannels([])).toEqual([])
  })

  it('按首次出现顺序归组, 组内保序', () => {
    const out = groupChannels([ch('CCTV1', '央视'), ch('BTV', '卫视'), ch('CCTV5', '央视')])
    expect(out.map((g) => g.name)).toEqual(['央视', '卫视'])
    expect(out[0]!.channels.map((c) => c.name)).toEqual(['CCTV1', 'CCTV5'])
    expect(out[1]!.channels.map((c) => c.name)).toEqual(['BTV'])
  })

  it('无分组/空白分组 → 默认组', () => {
    const out = groupChannels([ch('A'), ch('B', '  '), ch('C', LIVE_DEFAULT_GROUP)])
    expect(out).toHaveLength(1)
    expect(out[0]!.name).toBe(LIVE_DEFAULT_GROUP)
    expect(out[0]!.channels).toHaveLength(3)
  })

  it('分组名首尾空白裁剪后再归组', () => {
    const out = groupChannels([ch('A', '央视'), ch('B', ' 央视 ')])
    expect(out).toHaveLength(1)
    expect(out[0]!.name).toBe('央视')
  })
})

describe('probeLiveChannel：端侧两段式探源', () => {
  const okFetch = () => Promise.resolve({ ok: true } as Response)

  it('cors 模式成功 → 可达且可跨域', async () => {
    const r = await probeLiveChannel('http://x/1.m3u8', 1000, okFetch)
    expect(r).toEqual({ url: 'http://x/1.m3u8', alive: true, cors: true })
  })

  it('cors 被拒但 no-cors 可达 → 可达但跨域受限', async () => {
    let calls = 0
    const fetchImpl = (() => {
      calls++
      // 第一次 cors 模式 → 无 ACAO(TypeError); 第二次 no-cors → 成功
      return calls === 1 ? Promise.reject(new TypeError('Failed to fetch')) : Promise.resolve({} as Response)
    }) as unknown as typeof fetch
    const r = await probeLiveChannel('http://x/2.m3u8', 1000, fetchImpl)
    expect(r).toEqual({ url: 'http://x/2.m3u8', alive: true, cors: false })
    expect(calls).toBe(2)
  })

  it('两段都失败 → 不可达', async () => {
    const fetchImpl = (() => Promise.reject(new TypeError('down'))) as unknown as typeof fetch
    const r = await probeLiveChannel('http://x/3.m3u8', 1000, fetchImpl)
    expect(r).toEqual({ url: 'http://x/3.m3u8', alive: false, cors: false })
  })

  it('超时(AbortError) → 不可达且不做二次复测', async () => {
    let calls = 0
    const fetchImpl = (() => {
      calls++
      return Promise.reject(new DOMException('aborted', 'AbortError'))
    }) as unknown as typeof fetch
    const r = await probeLiveChannel('http://x/4.m3u8', 1000, fetchImpl)
    expect(r).toEqual({ url: 'http://x/4.m3u8', alive: false, cors: false })
    expect(calls).toBe(1)
  })
})

describe('probeLiveChannelsLimited：限流并发池', () => {
  it('并发数不超过上限, 全部完成且结果齐全', async () => {
    let active = 0
    let maxActive = 0
    const fetchImpl = ((_url: RequestInfo | URL) => {
      active++
      maxActive = Math.max(maxActive, active)
      return new Promise<Response>((resolve) => {
        setTimeout(() => {
          active--
          resolve({ ok: true } as Response)
        }, 5)
      })
    }) as unknown as typeof fetch
    const urls = Array.from({ length: 12 }, (_, i) => `http://x/${i}`)
    const rs = await probeLiveChannelsLimited(urls, 3, 1000, fetchImpl)
    expect(rs).toHaveLength(12)
    expect(maxActive).toBeLessThanOrEqual(3)
    expect(new Set(rs.map((r) => r.url))).toEqual(new Set(urls))
  })

  it('onResult 随到随回调, 结果数与输入一致', async () => {
    const seen: string[] = []
    const urls = ['http://x/a', 'http://x/b', 'http://x/c']
    await probeLiveChannelsLimited(
      urls,
      2,
      1000,
      (() => Promise.resolve({ ok: true } as Response)) as unknown as typeof fetch,
      (r) => seen.push(r.url),
    )
    expect(new Set(seen)).toEqual(new Set(urls))
  })

  it('单频道 fetch 同步抛出按不可达处理, 不拖垮整池', async () => {
    const fetchImpl = ((_url: RequestInfo | URL) => {
      const u = String(_url)
      if (u.endsWith('bad')) throw new Error('sync boom')
      return Promise.resolve({ ok: true } as Response)
    }) as unknown as typeof fetch
    const rs = await probeLiveChannelsLimited(['http://x/ok1', 'http://x/bad', 'http://x/ok2'], 2, 1000, fetchImpl)
    expect(rs).toHaveLength(3)
    expect(rs.find((r) => r.url.endsWith('bad'))!.alive).toBe(false)
    expect(rs.filter((r) => r.alive)).toHaveLength(2)
  })

  it('delayMs>0: 每个请求之间插入间隔(节流), 仍全部完成', async () => {
    const urls = ['http://x/d0', 'http://x/d1', 'http://x/d2']
    const delayMs = 25
    const times: number[] = []
    const fetchImpl = ((_url: RequestInfo | URL) => {
      times.push(performance.now())
      return Promise.resolve({ ok: true } as Response)
    }) as unknown as typeof fetch
    const rs = await probeLiveChannelsLimited(urls, 1, 500, fetchImpl, undefined, delayMs)
    expect(rs).toHaveLength(3)
    for (let i = 1; i < times.length; i++) {
      expect(times[i]! - times[i - 1]!).toBeGreaterThanOrEqual(delayMs - 5)
    }
  })
})

describe('上次观看记忆 (read/write/clear/label)', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('往返读写；损坏数据返回 null', () => {
    expect(readLiveLast()).toBeNull()
    writeLiveLast({ sourceId: 3, url: 'http://s/c1', sourceName: 'iptv-org', channelName: 'CCTV-1' })
    expect(readLiveLast()).toEqual({
      sourceId: 3,
      url: 'http://s/c1',
      sourceName: 'iptv-org',
      channelName: 'CCTV-1'
    })
    localStorage.setItem('jc-live-last-channel', '{bad json')
    expect(readLiveLast()).toBeNull()
  })

  it('clearLiveLast 清除记忆', () => {
    writeLiveLast({ sourceId: 1, url: 'u' })
    clearLiveLast()
    expect(readLiveLast()).toBeNull()
  })

  it('liveLastLabel 拼「源 · 频道」，缺项自动省略', () => {
    expect(liveLastLabel({ sourceName: 'iptv-org', channelName: 'CCTV-1' })).toBe('iptv-org · CCTV-1')
    expect(liveLastLabel({ sourceName: 'iptv-org' })).toBe('iptv-org')
    expect(liveLastLabel({ channelName: 'CCTV-1' })).toBe('CCTV-1')
    expect(liveLastLabel(null)).toBe('')
    expect(liveLastLabel({})).toBe('')
  })
})

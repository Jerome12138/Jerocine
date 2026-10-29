import { describe, it, expect } from 'vitest'
import { groupChannels, LIVE_DEFAULT_GROUP } from './live'
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

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import EpisodeTabs from './EpisodeTabs.vue'
import type { PlaySource } from '@/types/film'

function makeSource(id: string, count: number): PlaySource {
  return {
    id,
    name: id,
    episodes: Array.from({ length: count }, (_, i) => ({
      episode: String(i + 1),
      link: `${id}-${i + 1}`
    }))
  }
}

describe('EpisodeTabs 分段切换', () => {
  it('集数 <= pageSize 时不显示分段, 直接渲染全部 chip', () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 20)] }
    })
    expect(wrapper.find('.jc-episode-segments').exists()).toBe(false)
    expect(wrapper.findAll('.jc-episode-chip')).toHaveLength(20)
  })

  it('集数 > pageSize 时显示分段, 默认只渲染第一段', () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 75)] }
    })
    expect(wrapper.find('.jc-episode-segments').exists()).toBe(true)
    const segs = wrapper.findAll('.jc-episode-seg')
    expect(segs).toHaveLength(3) // 75 / 30 = ceil 3
    expect(segs[0]!.text()).toBe('1-30')
    expect(segs[1]!.text()).toBe('31-60')
    expect(segs[2]!.text()).toBe('61-75')
    // 默认第一段, 30 个 chip
    expect(wrapper.findAll('.jc-episode-chip')).toHaveLength(30)
  })

  it('点分段切换后只渲染对应段的 chip, 触发 select 携带原始全局 idx', async () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 50)] }
    })
    const segs = wrapper.findAll('.jc-episode-seg')
    await segs[1]!.trigger('click') // 31-50
    const chips = wrapper.findAll('.jc-episode-chip')
    expect(chips).toHaveLength(20) // 50 - 30 = 20
    expect(chips[0]!.text()).toContain('31')
    // 点第一个 (即第 31 集, 全局 idx=30)
    await chips[0]!.trigger('click')
    const emits = wrapper.emitted('select')
    expect(emits).toBeTruthy()
    expect(emits![0]![0]).toMatchObject({ sourceId: 's1', episodeIndex: 30 })
  })

  it('currentEpisode 在第二段时, 默认定位到第二段而不是从头', () => {
    const src = makeSource('s1', 75)
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [src],
        currentEpisode: 's1-45' // 全局 idx=44, 落在 31-60 段
      }
    })
    const segs = wrapper.findAll('.jc-episode-seg')
    expect(segs[1]!.classes()).toContain('jc-episode-seg--active')
    expect(segs[0]!.classes()).not.toContain('jc-episode-seg--active')
  })

  it('pageSize=0 关闭分段, 即使集数很多也不分段', () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 100)], pageSize: 0 }
    })
    expect(wrapper.find('.jc-episode-segments').exists()).toBe(false)
    expect(wrapper.findAll('.jc-episode-chip')).toHaveLength(100)
  })
})

describe('EpisodeTabs 播放源切换', () => {
  it('单源时不渲染 source tab', () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 5)] }
    })
    expect(wrapper.find('.jc-source-tabs').exists()).toBe(false)
  })

  it('多源时渲染 tab; 点击非 active 源触发 change-source', async () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 5), makeSource('s2', 5)],
        currentSourceId: 's1'
      }
    })
    const tabs = wrapper.findAll('.jc-source-tab')
    expect(tabs).toHaveLength(2)
    await tabs[1]!.trigger('click')
    expect(wrapper.emitted('change-source')![0]).toEqual(['s2'])
    // 点击当前 active 源不应再触发
    await tabs[0]!.trigger('click')
    // 仍只一次
    expect(wrapper.emitted('change-source')).toHaveLength(1)
  })
})

describe('EpisodeTabs 片源集数徽标', () => {
  it('多源时每个 tab 右上角显示该源集数徽标, 不再显示"共 N 集"', () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 12), makeSource('s2', 40)],
        currentSourceId: 's1'
      }
    })
    const badges = wrapper.findAll('.jc-source-tab__count-badge')
    expect(badges).toHaveLength(2)
    expect(badges[0]!.text()).toBe('12')
    expect(badges[1]!.text()).toBe('40')
    // 旧的"共 N 集"汇总已移除
    expect(wrapper.find('.jc-source-count').exists()).toBe(false)
  })

  it('单源不渲染源条(无 tab / 无徽标 / 无共N集)', () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 5)] }
    })
    expect(wrapper.find('.jc-source-bar').exists()).toBe(false)
    expect(wrapper.find('.jc-source-count').exists()).toBe(false)
  })
})

describe('EpisodeTabs 已观看标记', () => {
  it('watchedLinks 中的 episode 渲染绿点, currentEpisode 那个不渲染绿点', () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 5)],
        currentEpisode: 's1-3',
        watchedLinks: ['s1-1', 's1-2', 's1-3']
      }
    })
    const chips = wrapper.findAll('.jc-episode-chip')
    expect(chips[0]!.find('.jc-episode-chip__dot').exists()).toBe(true)
    expect(chips[1]!.find('.jc-episode-chip__dot').exists()).toBe(true)
    expect(chips[2]!.find('.jc-episode-chip__dot').exists()).toBe(false)
    expect(chips[2]!.classes()).toContain('jc-episode-chip--active')
  })
})

// ===== 2026-11-06 方案 §C6 表 9 项(测速色点/播放中态/热区) =====
describe('EpisodeTabs 测速色点(方案 §C6 #1)', () => {
  it('已测结果渲染色点: 最快=--fast 绿 / 其余=--mid 黄 / 失败=--fail 红; 数值进 title 不进文本', () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 5), makeSource('s2', 5), makeSource('s3', 5)],
        speeds: { s1: 100, s2: 300, s3: -1 }
      }
    })
    const tabs = wrapper.findAll('.jc-source-tab')
    expect(tabs).toHaveLength(3)
    const dots = tabs.map((t) => t.find('.jc-source-tab__speed'))
    expect(dots.every((d) => d.exists())).toBe(true)
    expect(dots[0]!.classes()).toContain('jc-source-tab__speed--fast')
    expect(dots[1]!.classes()).toContain('jc-source-tab__speed--mid')
    expect(dots[2]!.classes()).toContain('jc-source-tab__speed--fail')
    // 数值降级进 title 悬浮, 失败给语义 title
    expect(dots[0]!.attributes('title')).toBe('100ms')
    expect(dots[1]!.attributes('title')).toBe('300ms')
    expect(dots[2]!.attributes('title')).toBe('测速失败')
    // ⚡数值文本已移除: 点内不再有文本
    expect(dots[0]!.text()).toBe('')
  })

  it('未测(speeds 无该源)且未在测 → 不渲染点', () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 5), makeSource('s2', 5)],
        speeds: {}
      }
    })
    expect(wrapper.find('.jc-source-tab__speed').exists()).toBe(false)
  })

  it('测速进行中(speed-testing) → 全部脉冲点 --loading, title=测速中…(含带上一轮旧值的复测)', () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 5), makeSource('s2', 5)],
        speeds: {},
        speedTesting: true
      }
    })
    let dots = wrapper.findAll('.jc-source-tab__speed')
    expect(dots).toHaveLength(2)
    expect(dots.every((d) => d.classes().includes('jc-source-tab__speed--loading'))).toBe(true)
    expect(dots.every((d) => d.attributes('title') === '测速中…')).toBe(true)
    // 复测: 上一轮旧值仍在 → 复测期间同样转脉冲+统一 title, 不亮旧值
    const retest = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 5), makeSource('s2', 5)],
        speeds: { s1: 100, s2: -1 },
        speedTesting: true
      }
    })
    dots = retest.findAll('.jc-source-tab__speed')
    expect(dots).toHaveLength(2)
    expect(dots.every((d) => d.classes().includes('jc-source-tab__speed--loading'))).toBe(true)
    expect(dots.every((d) => d.attributes('title') === '测速中…')).toBe(true)
  })
})

describe('EpisodeTabs 播放中源标 ▶(方案 §C6 #3)', () => {
  it('playingSourceId 与浏览态(active)不同 → 该源 tab 出 ▶ 标, 其余不出', () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 5), makeSource('s2', 5)],
        currentSourceId: 's1', // 详情页浏览态停在 s1
        playingSourceId: 's2' // 播放在 s2
      }
    })
    const tabs = wrapper.findAll('.jc-source-tab')
    expect(tabs[0]!.find('.jc-source-tab__playing').exists()).toBe(false)
    expect(tabs[1]!.find('.jc-source-tab__playing').exists()).toBe(true)
  })

  it('playingSourceId == active(播放页语义: 播放态即激活态) → 不出 ▶ 标', () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 5), makeSource('s2', 5)],
        currentSourceId: 's1',
        playingSourceId: 's1'
      }
    })
    expect(wrapper.find('.jc-source-tab__playing').exists()).toBe(false)
  })

  it('未传 playingSourceId → 不出 ▶ 标', () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 5), makeSource('s2', 5)],
        currentSourceId: 's1'
      }
    })
    expect(wrapper.find('.jc-source-tab__playing').exists()).toBe(false)
  })
})

describe('EpisodeTabs 触达热区(方案 §C6 #2)', () => {
  it('源 tab 与集 chip 均带 jc-hotarea 类(命中面积靠热区补偿)', () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 3), makeSource('s2', 3)]
      }
    })
    const tabs = wrapper.findAll('.jc-source-tab')
    expect(tabs.length).toBeGreaterThan(0)
    expect(tabs.every((t) => t.classes().includes('jc-hotarea'))).toBe(true)
    const chips = wrapper.findAll('.jc-episode-chip')
    expect(chips.length).toBeGreaterThan(0)
    expect(chips.every((c) => c.classes().includes('jc-hotarea'))).toBe(true)
  })
})

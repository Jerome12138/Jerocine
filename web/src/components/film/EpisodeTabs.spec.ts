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
    expect(wrapper.find('.gf-episode-segments').exists()).toBe(false)
    expect(wrapper.findAll('.gf-episode-chip')).toHaveLength(20)
  })

  it('集数 > pageSize 时显示分段, 默认只渲染第一段', () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 75)] }
    })
    expect(wrapper.find('.gf-episode-segments').exists()).toBe(true)
    const segs = wrapper.findAll('.gf-episode-seg')
    expect(segs).toHaveLength(3) // 75 / 30 = ceil 3
    expect(segs[0]!.text()).toBe('1-30')
    expect(segs[1]!.text()).toBe('31-60')
    expect(segs[2]!.text()).toBe('61-75')
    // 默认第一段, 30 个 chip
    expect(wrapper.findAll('.gf-episode-chip')).toHaveLength(30)
  })

  it('点分段切换后只渲染对应段的 chip, 触发 select 携带原始全局 idx', async () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 50)] }
    })
    const segs = wrapper.findAll('.gf-episode-seg')
    await segs[1]!.trigger('click') // 31-50
    const chips = wrapper.findAll('.gf-episode-chip')
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
    const segs = wrapper.findAll('.gf-episode-seg')
    expect(segs[1]!.classes()).toContain('gf-episode-seg--active')
    expect(segs[0]!.classes()).not.toContain('gf-episode-seg--active')
  })

  it('pageSize=0 关闭分段, 即使集数很多也不分段', () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 100)], pageSize: 0 }
    })
    expect(wrapper.find('.gf-episode-segments').exists()).toBe(false)
    expect(wrapper.findAll('.gf-episode-chip')).toHaveLength(100)
  })
})

describe('EpisodeTabs 播放源切换', () => {
  it('单源时不渲染 source tab', () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 5)] }
    })
    expect(wrapper.find('.gf-source-tabs').exists()).toBe(false)
  })

  it('多源时渲染 tab; 点击非 active 源触发 change-source', async () => {
    const wrapper = mount(EpisodeTabs, {
      props: {
        sources: [makeSource('s1', 5), makeSource('s2', 5)],
        currentSourceId: 's1'
      }
    })
    const tabs = wrapper.findAll('.gf-source-tab')
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
    const badges = wrapper.findAll('.gf-source-tab__count-badge')
    expect(badges).toHaveLength(2)
    expect(badges[0]!.text()).toBe('12')
    expect(badges[1]!.text()).toBe('40')
    // 旧的"共 N 集"汇总已移除
    expect(wrapper.find('.gf-source-count').exists()).toBe(false)
  })

  it('单源不渲染源条(无 tab / 无徽标 / 无共N集)', () => {
    const wrapper = mount(EpisodeTabs, {
      props: { sources: [makeSource('s1', 5)] }
    })
    expect(wrapper.find('.gf-source-bar').exists()).toBe(false)
    expect(wrapper.find('.gf-source-count').exists()).toBe(false)
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
    const chips = wrapper.findAll('.gf-episode-chip')
    expect(chips[0]!.find('.gf-episode-chip__dot').exists()).toBe(true)
    expect(chips[1]!.find('.gf-episode-chip__dot').exists()).toBe(true)
    expect(chips[2]!.find('.gf-episode-chip__dot').exists()).toBe(false)
    expect(chips[2]!.classes()).toContain('gf-episode-chip--active')
  })
})

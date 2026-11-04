import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageErrorState from './ManageErrorState.vue'

/**
 * ManageErrorState(§6 组合组件, R1 评审裁决实施)用例:
 *   card 形态(FilmListView 同款: BaseEmpty hide-icon + outline sm 重试钮) /
 *   plain 形态(LiveSourceListView 同款: 红字「标题：详情」段落) / retry emit / retryLabel 定制。
 */

describe('ManageErrorState', () => {
  it('card 形态(默认): 标题/描述渲染 + 重试钮点击 emit retry', async () => {
    const w = mount(ManageErrorState, { props: { title: '加载失败', description: '网络超时' } })
    expect(w.find('section').exists()).toBe(true)
    expect(w.text()).toContain('加载失败')
    expect(w.text()).toContain('网络超时')
    expect(w.text()).toContain('重试')
    // card 形态重试钮带 refresh 图标
    expect(w.find('button svg').exists()).toBe(true)
    await w.find('button').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
  })

  it('plain 形态: 标题+详情拼一段红字段落', async () => {
    const w = mount(ManageErrorState, {
      props: { variant: 'plain', title: '直播源加载失败', description: '500 Internal Error' }
    })
    expect(w.find('section').exists()).toBe(false)
    expect(w.find('p').text()).toBe('直播源加载失败：500 Internal Error')
    await w.find('button').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
  })

  it('retryLabel 可定制; description 缺省时 plain 段落只显标题, retryLabel 缺省为「重试」', () => {
    const custom = mount(ManageErrorState, {
      props: { variant: 'plain', title: '加载失败', retryLabel: '重新加载' }
    })
    expect(custom.find('p').text()).toBe('加载失败')
    expect(custom.text()).toContain('重新加载')

    const def = mount(ManageErrorState, { props: { variant: 'plain', title: '加载失败' } })
    expect(def.text()).toContain('重试')
  })
})

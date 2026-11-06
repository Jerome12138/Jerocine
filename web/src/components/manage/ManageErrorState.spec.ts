import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageErrorState from './ManageErrorState.vue'

/**
 * ManageErrorState(§6 组合组件, R1 评审裁决实施)用例:
 *   card 形态(FilmListView 同款: BaseEmpty hide-icon + outline sm 重试钮) /
 *   plain 形态(LiveSourceListView 同款: 红字「标题：详情」段落) /
 *   banner 形态(§C8#1: CronList/FailureList/CollectList 三处横幅原款 —— info 图标 +
 *   「标题: 详情」半角冒号红字 + 右侧重试钮) / retry emit / retryLabel 定制。
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

  it('banner 形态: info 图标 + 「标题: 详情」半角冒号红字 + 右侧重试钮(三处横幅原款)', async () => {
    const w = mount(ManageErrorState, {
      props: { variant: 'banner', title: '加载失败', description: '500 Internal Error', retryLabel: '点击重试' }
    })
    // 不是 card 的 section 结构, 是横向单行
    expect(w.find('section').exists()).toBe(false)
    // 原款文案是 span.text-sm(非 card 的 BaseEmpty、非 plain 的 p)
    const text = w.find('span.text-sm')
    expect(text.exists()).toBe(true)
    // 文案差异保留: 横幅是半角冒号「标题: 详情」(≠ plain 的全角「标题：详情」)
    expect(text.text()).toBe('加载失败: 500 Internal Error')
    // info 图标(非 card 的 refresh)
    expect(w.find('svg').exists()).toBe(true)
    // 重试钮在右侧(ml-auto)
    const btn = w.find('button')
    expect(btn.classes()).toContain('ml-auto')
    expect(btn.text()).toContain('点击重试')
    await btn.trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
  })

  it('banner 形态: description 缺省时只显标题', () => {
    const w = mount(ManageErrorState, { props: { variant: 'banner', title: '加载失败' } })
    expect(w.find('span.text-sm').text()).toBe('加载失败')
  })
})

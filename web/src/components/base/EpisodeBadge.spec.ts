import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import EpisodeBadge from './EpisodeBadge.vue'

/**
 * W1d 「看到第 N 集」渐变角标 — 两处桌面视觉收敛(规格 §1 W1d):
 *   rail(默认) = 首页继续观看行原款(.jc-continue__ep: z-2, 全宽减 2×space-2 截断)
 *   page       = 历史页原款(.jc-history-ep: z-4, 右侧再留 28px 删除钮位)
 * 公共部分(品牌渐变胶囊+白字+超长省略)两处逐字相同。
 * TV 版(jc-tv-card .ep)不在本件范围(DOM 与 tv-cards.css 全局样式不动)。
 */
describe('EpisodeBadge', () => {
  it('rail(默认)渲染 rail 形态类', () => {
    const w = mount(EpisodeBadge, { slots: { default: '看到第 3 集' } })
    const badge = w.find('span')
    expect(badge.classes()).toContain('jc-ep-badge--rail')
    expect(badge.classes()).not.toContain('jc-ep-badge--page')
    expect(badge.text()).toBe('看到第 3 集')
  })

  it('variant=page 渲染 page 形态类(历史页, 预留删除钮位)', () => {
    const w = mount(EpisodeBadge, { props: { variant: 'page' }, slots: { default: '看到第 12 集' } })
    expect(w.find('span').classes()).toContain('jc-ep-badge--page')
  })

  it('空文案由调用方 v-if 控制, 本件只负责渲染 slot', () => {
    const w = mount(EpisodeBadge, { slots: { default: '' } })
    expect(w.find('span').text()).toBe('')
  })
})

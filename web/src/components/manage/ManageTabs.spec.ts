import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageTabs from './ManageTabs.vue'

const TABS = [
  { value: 0, label: '待补采' },
  { value: 1, label: '已处理' },
  { value: -1, label: '全部' }
]

describe('ManageTabs 页签', () => {
  it('渲染 tabs 数量, 容器 role=tablist', () => {
    const w = mount(ManageTabs, { props: { modelValue: 0, tabs: TABS } })
    expect(w.findAll('[role="tab"]')).toHaveLength(3)
    expect(w.find('[role="tablist"]').exists()).toBe(true)
  })

  it('点击第 2 项 → emit update:modelValue 原类型值', async () => {
    const w = mount(ManageTabs, { props: { modelValue: 0, tabs: TABS } })
    await w.findAll('[role="tab"]')[1]!.trigger('click')
    expect(w.emitted('update:modelValue')![0]).toEqual([1])
  })

  it('active 项 aria-selected=true, 其余 false', () => {
    const w = mount(ManageTabs, { props: { modelValue: 1, tabs: TABS } })
    const tabs = w.findAll('[role="tab"]')
    expect(tabs[1]!.attributes('aria-selected')).toBe('true')
    expect(tabs[0]!.attributes('aria-selected')).toBe('false')
    expect(tabs[2]!.attributes('aria-selected')).toBe('false')
  })

  it('恒有 data-focusable=true (TV 遥控聚焦)', () => {
    const w = mount(ManageTabs, { props: { modelValue: 0, tabs: TABS } })
    for (const t of w.findAll('[role="tab"]')) {
      expect(t.attributes('data-focusable')).toBe('true')
    }
  })

  it('variant=underline → 容器类含 jc-tabs--underline, active 项含 jc-tab--active', () => {
    const w = mount(ManageTabs, { props: { modelValue: 1, tabs: TABS, variant: 'underline' } })
    expect(w.find('[role="tablist"]').classes()).toContain('jc-tabs--underline')
    expect(w.findAll('[role="tab"]')[1]!.classes()).toContain('jc-tab--active')
  })

  it('pill 态(默认): active 项 BaseButton gradient, 非 active ghost', () => {
    const w = mount(ManageTabs, { props: { modelValue: 0, tabs: TABS } })
    expect(w.find('[role="tablist"]').classes()).not.toContain('jc-tabs--underline')
    const btns = w.findAll('[role="tab"]')
    expect(btns[0]!.classes()).toContain('jc-btn-gradient')
    expect(btns[1]!.classes()).toContain('bg-transparent')
  })

  it('count > 0 渲染数字徽标, 无 count 不渲染', () => {
    const w = mount(ManageTabs, {
      props: { modelValue: 0, tabs: [{ value: 0, label: '待补采', count: 5 }, { value: 1, label: '已处理' }] }
    })
    const first = w.findAll('[role="tab"]')[0]!
    expect(first.text()).toContain('5')
    expect(first.find('.jc-tabs__count').exists()).toBe(true)
    expect(w.findAll('[role="tab"]')[1]!.find('.jc-tabs__count').exists()).toBe(false)
  })
})

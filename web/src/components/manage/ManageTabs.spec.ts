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

  it('segment 态(默认): 容器胶囊底, active 项 bg-surface+shadow+text-primary, 非 active text-secondary', () => {
    // 视觉契约(规格 §3.3/D①): 逐像素照抄仪表盘手写胶囊, 类名即契约本身
    const w = mount(ManageTabs, { props: { modelValue: 0, tabs: TABS } })
    expect(w.find('[role="tablist"]').classes()).not.toContain('jc-tabs--underline')
    const list = w.find('[role="tablist"]')
    for (const c of ['flex', 'rounded-full', 'bg-elevated', 'p-0.5', 'text-xs']) {
      expect(list.classes()).toContain(c)
    }
    const btns = w.findAll('[role="tab"]')
    expect(btns[0]!.classes()).toContain('bg-surface')
    expect(btns[0]!.classes()).toContain('shadow')
    expect(btns[0]!.classes()).toContain('text-primary')
    expect(btns[1]!.classes()).toContain('text-secondary')
    expect(btns[1]!.classes()).not.toContain('bg-surface')
    expect(btns[1]!.classes()).not.toContain('shadow')
  })

  it('segment 项: min-h-[28px] 定值(D⑨)、无 hover 类(照抄现状), 且不再有按钮组样式(jc-btn-gradient 随 pill 删除)', () => {
    const w = mount(ManageTabs, { props: { modelValue: 0, tabs: TABS } })
    for (const b of w.findAll('[role="tab"]')) {
      expect(b.classes()).toContain('min-h-[28px]')
      expect(b.classes()).toContain('rounded-full')
      // 无 hover 类: 未激活项恒 text-secondary, 与仪表盘原版一致
      for (const c of b.classes()) expect(c.startsWith('hover:')).toBe(false)
    }
    expect(w.html()).not.toContain('jc-btn-gradient')
  })

  it('variant="segment" 显式传入与默认一致', () => {
    const w = mount(ManageTabs, { props: { modelValue: 0, tabs: TABS, variant: 'segment' } })
    expect(w.find('[role="tablist"]').classes()).toContain('bg-elevated')
    expect(w.findAll('[role="tab"]')[0]!.classes()).toContain('bg-surface')
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

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageIconButton from './ManageIconButton.vue'

/**
 * ManageIconButton(§6 组合组件, R1 评审裁决实施)用例:
 *   icon 形态(trash+aria-label+36px 定值) / 文字形态(槽内容+hover danger) /
 *   hover 变 danger 契约类 / focus-visible 焦点环 / disabled 摘 data-focusable / click emit。
 */

describe('ManageIconButton', () => {
  it('icon 形态: BaseIcon 渲染 + aria-label + data-focusable + 36px 定值类(D⑨ 不走 --jc-ctrl-h)', () => {
    const w = mount(ManageIconButton, { props: { icon: 'trash', label: '删除 分类A' } })
    const btn = w.find('button')
    expect(btn.find('svg').exists()).toBe(true)
    expect(btn.attributes('aria-label')).toBe('删除 分类A')
    expect(btn.attributes('data-focusable')).toBe('true')
    expect(btn.attributes('type')).toBe('button')
    expect(btn.classes()).toContain('w-[36px]')
    expect(btn.classes()).toContain('h-[36px]')
    expect(btn.classes()).toContain('jc-icon-btn--icon')
  })

  it('文字形态: 默认槽内容渲染, 无 aria-label(槽文字即无障碍名)', () => {
    const w = mount(ManageIconButton, { slots: { default: '删除' } })
    const btn = w.find('button')
    expect(btn.text()).toBe('删除')
    expect(btn.find('svg').exists()).toBe(false)
    expect(btn.attributes('aria-label')).toBeUndefined()
    expect(btn.attributes('data-focusable')).toBe('true')
    expect(btn.classes()).not.toContain('jc-icon-btn--icon')
  })

  it('统一契约: hover 变 danger + focus-visible 焦点环类挂在根钮上', () => {
    const w = mount(ManageIconButton, { props: { icon: 'trash', label: '删除' } })
    const cls = w.find('button').classes()
    expect(cls).toContain('jc-icon-btn')
    expect(cls).toContain('outline-none')
    // 契约由 scoped CSS(.jc-icon-btn:hover/:focus-visible)承载, 类名锚点存在即挂钩
  })

  it('disabled: 按钮禁用 + data-focusable 摘除(TV 空间导航跳过)', () => {
    const w = mount(ManageIconButton, { props: { disabled: true }, slots: { default: '删除' } })
    const btn = w.find('button')
    expect((btn.element as HTMLButtonElement).disabled).toBe(true)
    expect(btn.attributes('data-focusable')).toBeUndefined()
  })

  it('click 转发 emit(两形态同契约)', async () => {
    const icon = mount(ManageIconButton, { props: { icon: 'trash', label: '删除' } })
    await icon.find('button').trigger('click')
    expect(icon.emitted('click')).toHaveLength(1)

    const text = mount(ManageIconButton, { slots: { default: '删除' } })
    await text.find('button').trigger('click')
    expect(text.emitted('click')).toHaveLength(1)
  })
})

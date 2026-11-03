import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageSwitch from './ManageSwitch.vue'
import { useViewMode } from '@/composables/useViewMode'

describe('ManageSwitch', () => {
  afterEach(() => useViewMode().setMode('desktop'))
  it('modelValue=false → aria-checked=false', () => {
    const w = mount(ManageSwitch, { props: { modelValue: false } })
    expect(w.find('button').attributes('aria-checked')).toBe('false')
  })

  it('modelValue=true → aria-checked=true', () => {
    const w = mount(ManageSwitch, { props: { modelValue: true } })
    expect(w.find('button').attributes('aria-checked')).toBe('true')
  })

  it('role=switch', () => {
    const w = mount(ManageSwitch, { props: { modelValue: false } })
    expect(w.find('button').attributes('role')).toBe('switch')
  })

  it('click 翻转 modelValue (emit !modelValue)', async () => {
    const w = mount(ManageSwitch, { props: { modelValue: false } })
    await w.find('button').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([true])
  })

  it('disabled 时 click 不 emit', async () => {
    const w = mount(ManageSwitch, { props: { modelValue: false, disabled: true } })
    await w.find('button').trigger('click')
    expect(w.emitted('update:modelValue')).toBeUndefined()
  })

  it('开/关 视觉: knob class on/off', () => {
    const w = mount(ManageSwitch, { props: { modelValue: true } })
    expect(w.find('.jc-switch__knob').classes()).toContain('jc-switch__knob--on')
  })

  it('尺寸档: 桌面默认 default, size="large" 透传', () => {
    expect(mount(ManageSwitch, { props: { modelValue: false } }).find('button').attributes('data-size')).toBe('default')
    expect(mount(ManageSwitch, { props: { modelValue: false, size: 'large' } }).find('button').attributes('data-size')).toBe('large')
  })

  it('移动端(<768) 自动小尺寸', () => {
    useViewMode().setMode('mobile')
    const w = mount(ManageSwitch, { props: { modelValue: false } })
    expect(w.find('button').attributes('data-size')).toBe('small')
  })

  it('loading: 不 emit 且按钮禁用', async () => {
    const w = mount(ManageSwitch, { props: { modelValue: false, loading: true } })
    await w.find('button').trigger('click')
    expect(w.emitted('update:modelValue')).toBeUndefined()
    expect(w.find('button').attributes('disabled')).toBeDefined()
  })

  it('内联文字: activeText/inactiveText 按状态渲染', async () => {
    const w = mount(ManageSwitch, { props: { modelValue: false, activeText: '开', inactiveText: '关' } })
    expect(w.text()).toContain('关')
    await w.setProps({ modelValue: true })
    expect(w.text()).toContain('开')
  })
})

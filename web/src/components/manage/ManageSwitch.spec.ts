import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageSwitch from './ManageSwitch.vue'

describe('ManageSwitch', () => {
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
    expect(w.find('.gf-switch__knob').classes()).toContain('gf-switch__knob--on')
  })
})

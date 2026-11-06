import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseSwitch from './BaseSwitch.vue'

/**
 * W1a 开关视觉件 — 两尺寸视觉契约(规格 §1 W1a):
 *   lg = --jc-switch-* 三档自适应(scale=1; SettingsView 行内开关原款类 jc-settings-page__switch/__knob)
 *   sm = --jc-switch-* 三档自适应(scale=0.85; PlayView 跳过弹窗原款类 jc-skip-switch__track/__thumb)
 * 两档圆角/底色/滑块尺寸/位移各不相同, 视觉契约以类名逐字锁定。
 * 交互语义(role=switch/aria-checked)归外层按钮(BaseSwitchRow/调用方), 本件纯视觉 aria-hidden。
 */
describe('BaseSwitch', () => {
  it('lg(默认)渲染 SettingsView 原款类: jc-settings-page__switch > __knob, aria-hidden', () => {
    const w = mount(BaseSwitch, { props: { modelValue: false } })
    const track = w.find('.jc-settings-page__switch')
    expect(track.exists()).toBe(true)
    expect(track.find('.jc-settings-page__knob').exists()).toBe(true)
    expect(track.attributes('aria-hidden')).toBe('true')
  })

  it('sm 渲染 PlayView 跳过弹窗原款类: jc-skip-switch__track > __thumb, 且不再有 lg 类', () => {
    const w = mount(BaseSwitch, { props: { modelValue: false, size: 'sm' } })
    const track = w.find('.jc-skip-switch__track')
    expect(track.exists()).toBe(true)
    expect(track.find('.jc-skip-switch__thumb').exists()).toBe(true)
    expect(w.find('.jc-settings-page__switch').exists()).toBe(false)
  })

  it('开态 is-on: lg 落在 switch 上 / sm 落在 track 上(与两处原实现状态类位置一致)', () => {
    const lg = mount(BaseSwitch, { props: { modelValue: true } })
    expect(lg.find('.jc-settings-page__switch').classes()).toContain('is-on')
    const lgOff = mount(BaseSwitch, { props: { modelValue: false } })
    expect(lgOff.find('.jc-settings-page__switch').classes()).not.toContain('is-on')

    const sm = mount(BaseSwitch, { props: { modelValue: true, size: 'sm' } })
    expect(sm.find('.jc-skip-switch__track').classes()).toContain('is-on')
    const smOff = mount(BaseSwitch, { props: { modelValue: false, size: 'sm' } })
    expect(smOff.find('.jc-skip-switch__track').classes()).not.toContain('is-on')
  })
})

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseSwitchRow from './BaseSwitchRow.vue'

/**
 * W1a 开关行 — 标题+描述+开关整行按钮(规格 §1 W1a):
 * role=switch / aria-checked 跟随值 / data-focusable 保留 / 行类名 jc-settings-page__row*
 * 逐字不变(SettingsView 视觉契约) / 点击 emit update:modelValue(翻转值)。
 * 数据驱动沿用 SettingsView toggles 数组。
 */
describe('BaseSwitchRow', () => {
  it('role=switch + aria-checked 跟随 modelValue + data-focusable + 行类名', () => {
    const on = mount(BaseSwitchRow, { props: { title: '广告过滤', desc: 'd', modelValue: true } })
    const row = on.find('button')
    expect(row.attributes('role')).toBe('switch')
    expect(row.attributes('aria-checked')).toBe('true')
    expect(row.attributes('data-focusable')).toBe('true')
    expect(row.classes()).toContain('jc-settings-page__row')

    const off = mount(BaseSwitchRow, { props: { title: '广告过滤', modelValue: false } })
    expect(off.find('button').attributes('aria-checked')).toBe('false')
  })

  it('渲染标题与描述, 内嵌 lg 开关(jc-settings-page__switch)', () => {
    const w = mount(BaseSwitchRow, {
      props: { title: '自动连播', desc: '一集播完自动播放下一集', modelValue: false }
    })
    expect(w.text()).toContain('自动连播')
    expect(w.text()).toContain('一集播完自动播放下一集')
    expect(w.find('.jc-settings-page__switch').exists()).toBe(true)
    expect(w.find('.jc-settings-page__row-title').text()).toBe('自动连播')
    expect(w.find('.jc-settings-page__row-desc').text()).toBe('一集播完自动播放下一集')
  })

  it('无描述时不渲染描述节点', () => {
    const w = mount(BaseSwitchRow, { props: { title: '弹幕', modelValue: false } })
    expect(w.find('.jc-settings-page__row-desc').exists()).toBe(false)
  })

  it('点击行 → emit update:modelValue(翻转值)', async () => {
    const off = mount(BaseSwitchRow, { props: { title: 'x', modelValue: false } })
    await off.find('button').trigger('click')
    expect(off.emitted('update:modelValue')![0]).toEqual([true])

    const on = mount(BaseSwitchRow, { props: { title: 'x', modelValue: true } })
    await on.find('button').trigger('click')
    expect(on.emitted('update:modelValue')![0]).toEqual([false])
  })
})

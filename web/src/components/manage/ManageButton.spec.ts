import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageButton from './ManageButton.vue'
import { useViewMode } from '@/composables/useViewMode'

describe('ManageButton (EP 风格 + 旧 API 兼容)', () => {
  afterEach(() => useViewMode().setMode('desktop'))

  it('默认: 原生 button, data-size=default, data-type=default', () => {
    const w = mount(ManageButton)
    const btn = w.find('button')
    expect(btn.attributes('type')).toBe('button')
    expect(btn.attributes('data-size')).toBe('default')
    expect(btn.attributes('data-type')).toBe('default')
  })

  it('兼容旧 variant="gradient": 保留 jc-btn-gradient 类', () => {
    const w = mount(ManageButton, { props: { variant: 'gradient' } })
    expect(w.find('button').classes()).toContain('jc-btn-gradient')
    expect(w.find('button').attributes('data-type')).toBe('primary')
  })

  it('EP 风格 type="danger" + plain: data-type=danger 且非 variant 分支', () => {
    const w = mount(ManageButton, { props: { type: 'danger', plain: true } })
    const btn = w.find('button')
    expect(btn.attributes('data-type')).toBe('danger')
    expect(btn.classes()).not.toContain('jc-btn-gradient')
    expect(btn.attributes('class')).toContain('var(--jc-danger)')
  })

  it('原生 type="submit" 兼容: 落到原生 type 属性, 样式按 default', () => {
    const w = mount(ManageButton, { props: { type: 'submit' } })
    const btn = w.find('button')
    expect(btn.attributes('type')).toBe('submit')
    expect(btn.attributes('data-type')).toBe('default')
  })

  it('size="sm"(旧) 与 size="small"(EP) 均归一为 small', () => {
    expect(mount(ManageButton, { props: { size: 'sm' } }).find('button').attributes('data-size')).toBe('small')
    expect(mount(ManageButton, { props: { size: 'small' } }).find('button').attributes('data-size')).toBe('small')
  })

  it('尺寸由 size 决定, 不再按设备强制小尺寸(移动端未传 size 仍为 default)', () => {
    useViewMode().setMode('mobile')
    const w = mount(ManageButton)
    expect(w.find('button').attributes('data-size')).toBe('default')
  })

  it('loading: 按钮禁用且 click 不 emit', async () => {
    const w = mount(ManageButton, { props: { loading: true } })
    const btn = w.find('button')
    expect(btn.attributes('disabled')).toBeDefined()
    await btn.trigger('click')
    expect(w.emitted('click')).toBeUndefined()
  })

  it('disabled: click 不 emit', async () => {
    const w = mount(ManageButton, { props: { disabled: true } })
    await w.find('button').trigger('click')
    expect(w.emitted('click')).toBeUndefined()
  })

  it('block: 含 w-full; 正常点击 emit', async () => {
    const w = mount(ManageButton, { props: { block: true } })
    expect(w.find('button').classes()).toContain('w-full')
    await w.find('button').trigger('click')
    expect(w.emitted('click')?.length).toBe(1)
  })
})

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseButton from './BaseButton.vue'

describe('BaseButton', () => {
  it('默认渲染 button[type=button]', () => {
    const w = mount(BaseButton, { slots: { default: '点我' } })
    const btn = w.find('button')
    expect(btn.exists()).toBe(true)
    expect(btn.attributes('type')).toBe('button')
    expect(btn.text()).toContain('点我')
  })

  it('type=submit 透传', () => {
    const w = mount(BaseButton, { props: { type: 'submit' } })
    expect(w.find('button').attributes('type')).toBe('submit')
  })

  it('disabled 阻止 click emit', async () => {
    const w = mount(BaseButton, { props: { disabled: true } })
    await w.find('button').trigger('click')
    expect(w.emitted('click')).toBeFalsy()
  })

  it('loading 期间 click 不 emit', async () => {
    const w = mount(BaseButton, { props: { loading: true } })
    await w.find('button').trigger('click')
    expect(w.emitted('click')).toBeFalsy()
  })

  it('正常 click 触发 emit', async () => {
    const w = mount(BaseButton)
    await w.find('button').trigger('click')
    expect(w.emitted('click')).toBeTruthy()
  })

  it('block=true 含 w-full class', () => {
    const w = mount(BaseButton, { props: { block: true } })
    expect(w.find('button').classes()).toContain('w-full')
  })

  it('size 三档映射高度令牌: sm→--jc-ctrl-h-sm / md→--jc-ctrl-h / lg→--jc-ctrl-h-lg', () => {
    // 2026-11-05 三档统一(A′ 派生档: sm=标准−4/lg=标准+8)后锁定 size→令牌映射, 防回归定值高度
    const sm = mount(BaseButton, { props: { size: 'sm' } })
    expect(sm.find('button').classes()).toContain('h-[var(--jc-ctrl-h-sm)]')

    const md = mount(BaseButton) // 默认 md
    expect(md.find('button').classes()).toContain('h-[var(--jc-ctrl-h)]')

    const lg = mount(BaseButton, { props: { size: 'lg' } })
    expect(lg.find('button').classes()).toContain('h-[var(--jc-ctrl-h-lg)]')
  })
})

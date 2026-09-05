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
})

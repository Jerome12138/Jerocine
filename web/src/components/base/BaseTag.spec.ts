import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseTag from './BaseTag.vue'

describe('BaseTag', () => {
  it('默认渲染 slot', () => {
    const w = mount(BaseTag, { slots: { default: 'NEW' } })
    expect(w.text()).toContain('NEW')
  })

  it('size=sm 默认高度 class', () => {
    const w = mount(BaseTag, { props: { size: 'sm' }, slots: { default: 'x' } })
    // size class 含 h-* (具体值不绑死, 防 token 改了误失败)
    const cls = w.classes().join(' ')
    expect(cls).toMatch(/h-/)
  })

  it('outlined=true 影响 class', () => {
    const w1 = mount(BaseTag, { props: { outlined: true }, slots: { default: 'x' } })
    const w2 = mount(BaseTag, { props: { outlined: false }, slots: { default: 'x' } })
    expect(w1.classes()).not.toEqual(w2.classes())
  })

  it('不同 variant 应用不同 class', () => {
    const a = mount(BaseTag, { props: { variant: 'success' }, slots: { default: 'x' } })
    const b = mount(BaseTag, { props: { variant: 'danger' }, slots: { default: 'x' } })
    expect(a.classes().join(' ')).not.toBe(b.classes().join(' '))
  })
})

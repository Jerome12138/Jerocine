import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import AuthLayout from './AuthLayout.vue'

describe('AuthLayout', () => {
  it('渲染 slot', () => {
    const w = mount(AuthLayout, { slots: { default: () => h('div', { class: 'login-form' }, 'X') } })
    expect(w.find('.login-form').exists()).toBe(true)
  })

  it('根容器 min-h-screen + flex-center 居中', () => {
    const w = mount(AuthLayout)
    const cls = w.find('div').classes()
    expect(cls).toContain('min-h-screen')
    expect(cls).toContain('flex-center')
  })

  it('背景渐变 inline style (CSS gradient, 无外部 URL)', () => {
    const w = mount(AuthLayout)
    const style = w.find('div').attributes('style') ?? ''
    expect(style).toContain('linear-gradient')
    expect(style).not.toContain('http')
  })

  it('卡片容器 max-w-[440px] 居中', () => {
    const w = mount(AuthLayout)
    expect(w.find('.max-w-\\[440px\\]').exists()).toBe(true)
  })
})

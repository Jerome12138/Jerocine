import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import MinimalLayout from './MinimalLayout.vue'

describe('MinimalLayout', () => {
  it('渲染 slot 内容', () => {
    const w = mount(MinimalLayout, {
      slots: { default: () => h('div', { class: 'inner' }, 'X') }
    })
    expect(w.find('.inner').exists()).toBe(true)
    expect(w.text()).toContain('X')
  })

  it('根元素 min-h-screen 撑满', () => {
    const w = mount(MinimalLayout)
    expect(w.find('div').classes()).toContain('min-h-screen')
  })

  it('应用 dark theme class (bg-base text-primary)', () => {
    const w = mount(MinimalLayout)
    const cls = w.find('div').classes()
    expect(cls).toContain('bg-base')
    expect(cls).toContain('text-primary')
  })
})

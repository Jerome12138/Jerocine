import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseIcon from './BaseIcon.vue'

describe('BaseIcon', () => {
  it('已知 name 渲染 SVG', () => {
    const w = mount(BaseIcon, { props: { name: 'play' } })
    expect(w.find('svg').exists()).toBe(true)
  })

  it('未知 name 不报错 (回退或返回空 svg)', () => {
    const w = mount(BaseIcon, { props: { name: 'nope-xxx' as never } })
    expect(w.exists()).toBe(true)
  })

  it('size prop 应用到 wrapper span style (svg 撑满 100%)', () => {
    const w = mount(BaseIcon, { props: { name: 'play', size: '24px' } })
    // size 控制外层 span 尺寸, svg 内部 100% 撑满
    const style = w.find('span.gf-icon').attributes('style') ?? ''
    expect(style).toContain('24px')
  })
})

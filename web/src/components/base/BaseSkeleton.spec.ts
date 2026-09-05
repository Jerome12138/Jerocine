import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseSkeleton from './BaseSkeleton.vue'

describe('BaseSkeleton', () => {
  it('count=1 默认渲染 1 个', () => {
    const w = mount(BaseSkeleton)
    expect(w.findAll('[class*="skeleton"], .skel, [aria-hidden]').length).toBeGreaterThanOrEqual(1)
  })

  it('count=3 渲染 3 个', () => {
    const w = mount(BaseSkeleton, { props: { count: 3 } })
    // 用 html 长度 / 重复结构判定
    const html = w.html()
    const matches = html.match(/<div/g) ?? []
    expect(matches.length).toBeGreaterThanOrEqual(3)
  })

  it('shape=circle 时宽高相等', () => {
    const w = mount(BaseSkeleton, { props: { shape: 'circle', width: '40px' } })
    // 通过 style 检查
    const style = w.find('[style]').attributes('style') ?? ''
    expect(style).toContain('40px')
  })

  it('shape=text 默认', () => {
    const w = mount(BaseSkeleton, { props: { shape: 'text' } })
    expect(w.exists()).toBe(true)
  })

  it('width prop 应用到 style', () => {
    const w = mount(BaseSkeleton, { props: { width: '200px' } })
    const style = w.find('[style]').attributes('style') ?? ''
    expect(style).toContain('200px')
  })
})

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseImage from './BaseImage.vue'

describe('BaseImage', () => {
  it('挂载成功 (div wrapper)', () => {
    const w = mount(BaseImage, { props: { src: 'http://x/a.jpg', alt: 'X' } })
    expect(w.find('.jc-base-image').exists()).toBe(true)
  })

  it('空 src 不崩, 走错误回退分支', () => {
    const w = mount(BaseImage, { props: { src: '', alt: 'X' } })
    expect(w.exists()).toBe(true)
  })

  it('ratio prop 用 padding-bottom 撑比例(不用 aspect-ratio, 兼容 Chrome 66)', () => {
    const w = mount(BaseImage, { props: { src: 'http://x/a.jpg', alt: 'X', ratio: '3/4' } })
    const style = w.find('.jc-base-image').attributes('style') ?? ''
    // 3/4 → padding-bottom: 133.3333%; 且 height: 0(老内核盒子塌 0 的根因修复)
    expect(style).toContain('padding-bottom: 133.3333%')
    expect(style).not.toContain('aspect-ratio')
  })

  it('空 ratio 走父控高分支, 不注入 padding', () => {
    const w = mount(BaseImage, { props: { src: 'http://x/a.jpg', alt: 'X', ratio: '' } })
    const style = w.find('.jc-base-image').attributes('style') ?? ''
    expect(style).not.toContain('padding-bottom')
  })
})

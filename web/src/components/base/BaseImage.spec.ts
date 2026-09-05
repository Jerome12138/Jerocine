import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseImage from './BaseImage.vue'

describe('BaseImage', () => {
  it('挂载成功 (div wrapper)', () => {
    const w = mount(BaseImage, { props: { src: 'http://x/a.jpg', alt: 'X' } })
    expect(w.find('.gf-base-image').exists()).toBe(true)
  })

  it('空 src 不崩, 走错误回退分支', () => {
    const w = mount(BaseImage, { props: { src: '', alt: 'X' } })
    expect(w.exists()).toBe(true)
  })

  it('ratio prop 设置 aspect-ratio style', () => {
    const w = mount(BaseImage, { props: { src: 'http://x/a.jpg', alt: 'X', ratio: '3/4' } })
    const style = w.find('.gf-base-image').attributes('style') ?? ''
    // aspect-ratio: 3 / 4 (style 序列化时可能加空格)
    expect(style).toMatch(/3\s*\/\s*4/)
  })
})

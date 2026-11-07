import { describe, it, expect, beforeEach } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { imgFallbackHosts } from '@/utils/imgHosts'
import BaseImage from './BaseImage.vue'

// 图床域名链走 utils/imgHosts 模块级单供体(不走 pinia), 测试直接写 ref
function mountImg(props: Record<string, unknown>) {
  return mount(BaseImage, { props })
}

describe('BaseImage', () => {
  beforeEach(() => {
    imgFallbackHosts.value = ''
  })

  it('挂载成功 (div wrapper)', () => {
    const w = mountImg({ src: 'http://x/a.jpg', alt: 'X' })
    expect(w.find('.jc-base-image').exists()).toBe(true)
  })

  it('空 src 不崩, 走错误回退分支', () => {
    const w = mountImg({ src: '', alt: 'X' })
    expect(w.exists()).toBe(true)
  })

  it('ratio prop 用 padding-bottom 撑比例(不用 aspect-ratio, 兼容 Chrome 66)', () => {
    const w = mountImg({ src: 'http://x/a.jpg', alt: 'X', ratio: '3/4' })
    const style = w.find('.jc-base-image').attributes('style') ?? ''
    // 3/4 → padding-bottom: 133.3333%; 且 height: 0(老内核盒子塌 0 的根因修复)
    expect(style).toContain('padding-bottom: 133.3333%')
    expect(style).not.toContain('aspect-ratio')
  })

  it('空 ratio 走父控高分支, 不注入 padding', () => {
    const w = mountImg({ src: 'http://x/a.jpg', alt: 'X', ratio: '' })
    const style = w.find('.jc-base-image').attributes('style') ?? ''
    expect(style).not.toContain('padding-bottom')
  })

  // ---- 图床域名链(000041): onerror 按链切域, 链尽走占位符 ----
  it('onerror 后在链内切下一个域(路径原样保留)', async () => {
    imgFallbackHosts.value = 'img.a.com,img.b.com,img.c.com'
    const w = mountImg({ src: 'https://img.a.com/x.jpg?w=400', alt: 'X', eager: true })
    await nextTick()
    expect((w.find('img').element as HTMLImageElement).src).toBe('https://img.a.com/x.jpg?w=400')
    await w.find('img').trigger('error')
    expect((w.find('img').element as HTMLImageElement).src).toBe('https://img.b.com/x.jpg?w=400')
    await w.find('img').trigger('error')
    expect((w.find('img').element as HTMLImageElement).src).toBe('https://img.c.com/x.jpg?w=400')
  })

  it('链尽走占位符, 不死循环', async () => {
    imgFallbackHosts.value = 'img.a.com,img.b.com'
    const w = mountImg({ src: 'https://img.a.com/x.jpg', alt: 'X', eager: true })
    await nextTick()
    await w.find('img').trigger('error') // a → b
    await w.find('img').trigger('error') // b 链尽 → 占位符
    expect(w.find('img').exists()).toBe(false)
    expect(w.find('.jc-base-image__fallback').exists()).toBe(true)
  })

  it('失败 host 不在链内(本站 blob/API 图)不切域, 直接占位符', async () => {
    imgFallbackHosts.value = 'img.a.com,img.b.com'
    const w = mountImg({ src: 'https://jerocine.art/api/upload/x.jpg', alt: 'X', eager: true })
    await nextTick()
    await w.find('img').trigger('error')
    expect(w.find('img').exists()).toBe(false)
    expect(w.find('.jc-base-image__fallback').exists()).toBe(true)
  })

  it('未配置域名链 = 现行为(失败即占位符)', async () => {
    const w = mountImg({ src: 'https://img.a.com/x.jpg', alt: 'X', eager: true })
    await nextTick()
    await w.find('img').trigger('error')
    expect(w.find('img').exists()).toBe(false)
  })
})

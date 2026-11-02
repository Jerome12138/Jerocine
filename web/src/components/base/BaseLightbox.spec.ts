import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseLightbox from './BaseLightbox.vue'

function clearBody(): void {
  // 清测试间 Teleport 残留 (不操作生产用户输入, 仅测试隔离)
  while (document.body.firstChild) {
    document.body.removeChild(document.body.firstChild)
  }
}

describe('BaseLightbox', () => {
  beforeEach(() => clearBody())

  it('src 非空 → 渲染图片与关闭按钮; src 空 → 不渲染', async () => {
    const w = mount(BaseLightbox, {
      props: { src: 'https://example.com/a.jpg', alt: '预览图' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.querySelector('.jc-lightbox__img')).not.toBeNull()
    expect(document.querySelector('.jc-lightbox__close')).not.toBeNull()
    w.unmount()

    const w2 = mount(BaseLightbox, { props: { src: '' }, attachTo: document.body })
    await w2.vm.$nextTick()
    expect(document.querySelector('.jc-lightbox')).toBeNull()
    w2.unmount()
  })

  it('点关闭按钮 → emit update:src("")', async () => {
    const w = mount(BaseLightbox, { props: { src: 'x.png' }, attachTo: document.body })
    await w.vm.$nextTick()
    ;(document.querySelector('.jc-lightbox__close') as HTMLButtonElement).click()
    await w.vm.$nextTick()
    expect(w.emitted('update:src')?.[0]).toEqual([''])
    w.unmount()
  })

  it('点遮罩空白 → emit update:src("")', async () => {
    const w = mount(BaseLightbox, { props: { src: 'x.png' }, attachTo: document.body })
    await w.vm.$nextTick()
    ;(document.querySelector('.jc-lightbox') as HTMLElement).dispatchEvent(
      new MouseEvent('click', { bubbles: true })
    )
    await w.vm.$nextTick()
    expect(w.emitted('update:src')?.[0]).toEqual([''])
    w.unmount()
  })

  it('单弹层 ESC → emit update:src("") (关闭行为回归)', async () => {
    const w = mount(BaseLightbox, { props: { src: 'x.png' }, attachTo: document.body })
    await w.vm.$nextTick()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await w.vm.$nextTick()
    expect(w.emitted('update:src')?.[0]).toEqual([''])
    w.unmount()
  })
})

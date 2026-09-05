import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import BackToTop from './BackToTop.vue'

describe('BackToTop', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'scrollY', { value: 0, configurable: true, writable: true })
  })

  it('scrollY=0 → 按钮不显示', async () => {
    const w = mount(BackToTop)
    await w.vm.$nextTick()
    expect(w.find('button').exists()).toBe(false)
  })

  it('scrollY>600 + scroll 事件 → 按钮显示', async () => {
    const w = mount(BackToTop)
    Object.defineProperty(window, 'scrollY', { value: 700, configurable: true })
    window.dispatchEvent(new Event('scroll'))
    await w.vm.$nextTick()
    expect(w.find('button').exists()).toBe(true)
  })

  it('点击按钮 → 调 window.scrollTo({top:0})', async () => {
    const scrollSpy = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    const w = mount(BackToTop)
    Object.defineProperty(window, 'scrollY', { value: 700, configurable: true })
    window.dispatchEvent(new Event('scroll'))
    await w.vm.$nextTick()
    await w.find('button').trigger('click')
    expect(scrollSpy).toHaveBeenCalled()
    const call = scrollSpy.mock.calls[0][0] as ScrollToOptions
    expect(call.top).toBe(0)
    scrollSpy.mockRestore()
  })

  it('button aria-label=回到顶部', async () => {
    Object.defineProperty(window, 'scrollY', { value: 700, configurable: true })
    const w = mount(BackToTop)
    window.dispatchEvent(new Event('scroll'))
    await w.vm.$nextTick()
    expect(w.find('button').attributes('aria-label')).toBe('回到顶部')
  })
})

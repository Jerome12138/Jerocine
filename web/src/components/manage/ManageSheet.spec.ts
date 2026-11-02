import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ManageSheet from './ManageSheet.vue'
import { useViewMode } from '@/composables/useViewMode'

function forceMode(m: 'mobile' | 'desktop'): void {
  useViewMode().setMode(m)
}

describe('ManageSheet', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = ''  // 清 Teleport 上轮残留
    forceMode('desktop')
  })

  it('desktop → 渲染 .jc-sheet--modal', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: true, title: 'T' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.querySelector('.jc-sheet--modal')).not.toBeNull()
    expect(document.querySelector('.jc-sheet--sheet')).toBeNull()
    w.unmount()
  })

  it('mobile + mobileMode=sheet → .jc-sheet--sheet', async () => {
    forceMode('mobile')
    const w = mount(ManageSheet, {
      props: { modelValue: true, title: 'T', mobileMode: 'sheet' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.querySelector('.jc-sheet--sheet')).not.toBeNull()
    expect(document.querySelector('.jc-sheet--modal')).toBeNull()
    w.unmount()
  })

  it('mobile + mobileMode=fullsheet → .jc-sheet--fullsheet', async () => {
    forceMode('mobile')
    const w = mount(ManageSheet, {
      props: { modelValue: true, title: 'T', mobileMode: 'fullsheet' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.querySelector('.jc-sheet--fullsheet')).not.toBeNull()
    w.unmount()
  })

  it('modelValue=false → 不渲染 overlay', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: false, title: 'T' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.querySelector('.jc-sheet__overlay')).toBeNull()
    w.unmount()
  })

  it('点击遮罩 emit update:modelValue=false + close', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: true, closeOnOverlay: true },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    const overlay = document.querySelector('.jc-sheet__overlay') as HTMLElement
    expect(overlay).not.toBeNull()
    overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await w.vm.$nextTick()
    expect(w.emitted('update:modelValue')?.[0]).toEqual([false])
    expect(w.emitted('close')).toBeTruthy()
    w.unmount()
  })

  it('closeOnOverlay=false 时点遮罩不关闭', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: true, closeOnOverlay: false },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    const overlay = document.querySelector('.jc-sheet__overlay') as HTMLElement
    overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await w.vm.$nextTick()
    expect(w.emitted('update:modelValue')).toBeUndefined()
    w.unmount()
  })

  it('单实例 ESC → emit update:modelValue=false', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: true, title: 'T' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await w.vm.$nextTick()
    expect(w.emitted('update:modelValue')?.[0]).toEqual([false])
    w.unmount()
  })

  it('挂载即开(modelValue 初始 true) → 滚动锁立即生效 body.overflow=hidden', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: true, title: 'T' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.body.style.overflow).toBe('hidden')
    w.unmount()
    expect(document.body.style.overflow).toBe('')
  })

  it('width="640px" → modal 容器 style maxWidth=640px; 默认 480px', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: true, title: 'T', width: '640px' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    const modal = document.querySelector('.jc-sheet--modal') as HTMLElement
    expect(modal.style.maxWidth).toBe('640px')
    w.unmount()

    const w2 = mount(ManageSheet, {
      props: { modelValue: true, title: 'T' },
      attachTo: document.body
    })
    await w2.vm.$nextTick()
    const modal2 = document.querySelector('.jc-sheet--modal') as HTMLElement
    expect(modal2.style.maxWidth).toBe('480px')
    w2.unmount()
  })

  it('无 title → modal 内仍渲染关闭按钮且可关', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: true },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    const modal = document.querySelector('.jc-sheet--modal')
    expect(modal).not.toBeNull()
    const closeBtn = modal!.querySelector('button') as HTMLButtonElement
    expect(closeBtn).not.toBeNull()
    closeBtn.click()
    await w.vm.$nextTick()
    expect(w.emitted('update:modelValue')?.[0]).toEqual([false])
    w.unmount()
  })

  it('双 sheet 叠开: ESC 只关最顶层; 顶层释放后 ESC 再关底层', async () => {
    forceMode('desktop')
    const wA = mount(ManageSheet, { props: { modelValue: true, title: 'A' }, attachTo: document.body })
    await wA.vm.$nextTick()
    const wB = mount(ManageSheet, { props: { modelValue: true, title: 'B' }, attachTo: document.body })
    await wB.vm.$nextTick()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wB.vm.$nextTick()
    expect(wB.emitted('update:modelValue')?.length).toBe(1)
    expect(wA.emitted('update:modelValue')).toBeUndefined()

    await wB.setProps({ modelValue: false })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wA.vm.$nextTick()
    expect(wA.emitted('update:modelValue')?.length).toBe(1)

    await wA.setProps({ modelValue: false })
    wA.unmount()
    wB.unmount()
  })
})

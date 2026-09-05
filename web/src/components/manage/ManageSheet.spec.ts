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

  it('desktop → 渲染 .gf-sheet--modal', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: true, title: 'T' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.querySelector('.gf-sheet--modal')).not.toBeNull()
    expect(document.querySelector('.gf-sheet--sheet')).toBeNull()
    w.unmount()
  })

  it('mobile + mobileMode=sheet → .gf-sheet--sheet', async () => {
    forceMode('mobile')
    const w = mount(ManageSheet, {
      props: { modelValue: true, title: 'T', mobileMode: 'sheet' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.querySelector('.gf-sheet--sheet')).not.toBeNull()
    expect(document.querySelector('.gf-sheet--modal')).toBeNull()
    w.unmount()
  })

  it('mobile + mobileMode=fullsheet → .gf-sheet--fullsheet', async () => {
    forceMode('mobile')
    const w = mount(ManageSheet, {
      props: { modelValue: true, title: 'T', mobileMode: 'fullsheet' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.querySelector('.gf-sheet--fullsheet')).not.toBeNull()
    w.unmount()
  })

  it('modelValue=false → 不渲染 overlay', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: false, title: 'T' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.querySelector('.gf-sheet__overlay')).toBeNull()
    w.unmount()
  })

  it('点击遮罩 emit update:modelValue=false + close', async () => {
    forceMode('desktop')
    const w = mount(ManageSheet, {
      props: { modelValue: true, closeOnOverlay: true },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    const overlay = document.querySelector('.gf-sheet__overlay') as HTMLElement
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
    const overlay = document.querySelector('.gf-sheet__overlay') as HTMLElement
    overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await w.vm.$nextTick()
    expect(w.emitted('update:modelValue')).toBeUndefined()
    w.unmount()
  })
})

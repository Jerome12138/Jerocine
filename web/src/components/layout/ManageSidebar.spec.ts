import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import ManageSidebar from './ManageSidebar.vue'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
})

function clearBody(): void {
  while (document.body.firstChild) document.body.removeChild(document.body.firstChild)
}

function mountSidebar(props: Record<string, unknown> = {}) {
  return mount(ManageSidebar, {
    props,
    global: { plugins: [router] },
    attachTo: document.body
  })
}

describe('ManageSidebar variant', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    clearBody()
  })
  afterEach(() => {
    clearBody()
  })

  it('variant=drawer + open=true → 渲染 drawer 全屏 root', () => {
    mountSidebar({ variant: 'drawer', open: true })
    expect(document.querySelector('.gf-drawer-root')).toBeTruthy()
    expect(document.querySelector('.gf-drawer-panel')).toBeTruthy()
    expect(document.querySelector('.gf-drawer-mask')).toBeTruthy()
  })

  it('variant=drawer + open=false → 不渲染 drawer', () => {
    mountSidebar({ variant: 'drawer', open: false })
    expect(document.querySelector('.gf-drawer-root')).toBeNull()
  })

  it('variant=icon-rail → aside style width 64px', () => {
    mountSidebar({ variant: 'icon-rail' })
    const aside = document.querySelector('aside') as HTMLElement
    expect(aside.style.width).toBe('64px')
  })

  it('variant=full + sidebarCollapsed 默认 false → aside style width 220px', () => {
    mountSidebar({ variant: 'full' })
    const aside = document.querySelector('aside') as HTMLElement
    expect(aside.style.width).toBe('220px')
  })

  it('drawer 模式 panel 宽度 260px', () => {
    mountSidebar({ variant: 'drawer', open: true })
    const panel = document.querySelector('.gf-drawer-panel') as HTMLElement
    expect(panel.classList.contains('w-[260px]')).toBe(true)
  })

  it('drawer 模式点击遮罩 emit close', async () => {
    const w = mountSidebar({ variant: 'drawer', open: true })
    const mask = document.querySelector('.gf-drawer-mask') as HTMLElement
    mask.dispatchEvent(new Event('click', { bubbles: true }))
    await w.vm.$nextTick()
    expect(w.emitted('close')).toBeTruthy()
  })

  it('drawer 模式点击菜单 item emit close', async () => {
    const w = mountSidebar({ variant: 'drawer', open: true })
    const link = document.querySelector('.gf-drawer-root a') as HTMLElement
    link.dispatchEvent(new Event('click', { bubbles: true }))
    await w.vm.$nextTick()
    expect(w.emitted('close')).toBeTruthy()
  })

  it('full 模式点击菜单 item 不 emit close', async () => {
    const w = mountSidebar({ variant: 'full' })
    const link = document.querySelector('aside a') as HTMLElement
    link.dispatchEvent(new Event('click', { bubbles: true }))
    await w.vm.$nextTick()
    expect(w.emitted('close')).toBeUndefined()
  })
})

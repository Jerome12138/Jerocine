import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import ManageHeader from './ManageHeader.vue'
import { useUIStore } from '@/stores/ui'
import { useUserStore } from '@/stores/user'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/', component: { template: '<div />' }, meta: { title: 'Home' } },
    { path: '/manage/index', component: { template: '<div />' }, meta: { title: 'Dashboard' } },
    { path: '/login', component: { template: '<div />' } },
    { path: '/:pathMatch(.*)*', component: { template: '<div />' } }
  ]
})

async function mountHeader(props: Record<string, unknown> = {}) {
  await router.push('/manage/index')
  await router.isReady()
  return mount(ManageHeader, {
    props,
    global: { plugins: [router] }
  })
}

describe('ManageHeader 汉堡按钮分支', () => {
  beforeEach(() => setActivePinia(createPinia()))

  // 汉堡按钮 = 模板里第一个 button (在 header 左侧 div 里)
  function hamburger(w: ReturnType<typeof mount>) {
    return w.findAll('button')[0]
  }

  it('showHamburger=true → 汉堡按钮 click 时 emit toggle-drawer (不触发 sidebar toggle)', async () => {
    const w = await mountHeader({ showHamburger: true })
    const uiStore = useUIStore()
    const spy = vi.spyOn(uiStore, 'toggleSidebar')

    await hamburger(w).trigger('click')

    expect(w.emitted('toggle-drawer')).toBeTruthy()
    expect(spy).not.toHaveBeenCalled()
  })

  it('showHamburger=false (default) → 汉堡按钮 click 时调 uiStore.toggleSidebar (不 emit toggle-drawer)', async () => {
    const w = await mountHeader({})
    const uiStore = useUIStore()
    const spy = vi.spyOn(uiStore, 'toggleSidebar')

    await hamburger(w).trigger('click')

    expect(spy).toHaveBeenCalledTimes(1)
    expect(w.emitted('toggle-drawer')).toBeUndefined()
  })

  it('showHamburger=true → sr-only 文案为「打开菜单」', async () => {
    const w = await mountHeader({ showHamburger: true })
    expect(hamburger(w).find('.sr-only').text()).toBe('打开菜单')
  })

  it('showHamburger=false → sr-only 文案为「切换侧栏」', async () => {
    const w = await mountHeader({})
    expect(hamburger(w).find('.sr-only').text()).toBe('切换侧栏')
  })

  it('汉堡按钮含 min-h-[44px] min-w-[44px] (WCAG 触摸目标)', async () => {
    const w = await mountHeader({ showHamburger: true })
    const cls = hamburger(w).classes()
    expect(cls).toContain('min-h-[44px]')
    expect(cls).toContain('min-w-[44px]')
  })

  it('显示路由 meta.title 作为标题', async () => {
    const w = await mountHeader({})
    expect(w.find('h3').text()).toBe('Dashboard')
  })

  it('未登录时 nickname/username 不存在 → 显示「管理员」兜底', async () => {
    const w = await mountHeader({})
    const userStore = useUserStore()
    // userStore.info 默认未设置
    expect(w.text()).toContain('管理员')
  })
})

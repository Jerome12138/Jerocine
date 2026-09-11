import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import ManageHeader from './ManageHeader.vue'
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

  it('showHamburger=true → 汉堡按钮 click 时 emit toggle-drawer', async () => {
    const w = await mountHeader({ showHamburger: true })

    await hamburger(w).trigger('click')

    expect(w.emitted('toggle-drawer')).toBeTruthy()
  })

  it('showHamburger=false (desktop) → 不渲染汉堡按钮(侧栏折叠已移除, 无开关)', async () => {
    const w = await mountHeader({})
    // 模板中不存在带 sr-only 的汉堡按钮
    const hasHamburger = w.findAll('button').some((b) => b.find('.sr-only').exists())
    expect(hasHamburger).toBe(false)
  })

  it('showHamburger=true → sr-only 文案为「打开菜单」', async () => {
    const w = await mountHeader({ showHamburger: true })
    expect(hamburger(w).find('.sr-only').text()).toBe('打开菜单')
  })

  it('汉堡按钮含 min-h-[44px] min-w-[44px] (WCAG 触摸目标)', async () => {
    const w = await mountHeader({ showHamburger: true })
    const cls = hamburger(w).classes()
    expect(cls).toContain('min-h-[44px]')
    expect(cls).toContain('min-w-[44px]')
  })

  it('头部不再重复页面标题(各页面内部已有标题)', async () => {
    const w = await mountHeader({})
    expect(w.find('h3').exists()).toBe(false)
  })

  it('未登录时 displayName 取 store 兜底「用户」, 不再显示「管理员」', async () => {
    const w = await mountHeader({})
    const userStore = useUserStore()
    // userStore.info 默认未设置 → displayName 兜底 '用户'
    expect(userStore.displayName).toBe('用户')
    expect(w.text()).toContain('用户')
    expect(w.text()).not.toContain('管理员')
  })

  it('已登录且 nickName 存在 → 按实际昵称展示', async () => {
    const userStore = useUserStore()
    userStore.info = { id: 1, nickName: '老王', avatar: 'empty' }
    const w = await mountHeader({})
    expect(w.text()).toContain('老王')
  })
})

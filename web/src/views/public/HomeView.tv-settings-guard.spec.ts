import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'

/**
 * HomeView TV 金刚区「设置」卡的宿主分支行为(评审后补)。
 *
 * 这个用例存在的唯一目的: 锁住 `@click.capture`。
 * 卡是 <RouterLink to="/settings">, 原生壳里要拦掉默认导航改开原生抽屉;
 * 若有人把它改回普通 `@click`(`useTvSettingsEntry.routerlink.spec.ts` 的机制测试
 * 察觉不到这种「用法回退」), 本用例会红。
 */

const mocks = vi.hoisted(() => ({
  openSettings: vi.fn(),
  native: { value: true }
}))

vi.mock('@/utils/jerocineNative', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/utils/jerocineNative')>()
  return {
    ...actual,
    isNative: () => mocks.native.value,
    jerocine: { ...actual.jerocine, openSettings: mocks.openSettings }
  }
})

vi.mock('@/api', async (importOriginal) => {
  const actual = (await importOriginal()) as Record<string, unknown>
  const filmApi = (actual.filmApi ?? {}) as Record<string, unknown>
  return {
    ...actual,
    filmApi: {
      ...filmApi,
      getHome: vi.fn().mockResolvedValue({ banner: [], content: [] }),
      getBanners: vi.fn().mockResolvedValue([])
    }
  }
})

vi.mock('@/api/history', () => ({
  listHistory: vi.fn().mockResolvedValue([]),
  upsertHistory: vi.fn().mockResolvedValue(undefined),
  removeHistory: vi.fn().mockResolvedValue(undefined),
  clearHistory: vi.fn().mockResolvedValue(undefined)
}))

vi.mock('@/api/auth', () => ({
  getUserInfo: vi.fn().mockResolvedValue({ userName: 'admin', role: 0 }),
  login: vi.fn(),
  logout: vi.fn(),
  changePassword: vi.fn()
}))

import HomeView from './HomeView.vue'
import { useViewMode } from '@/composables/useViewMode'

function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/settings', component: { template: '<div />' } },
      { path: '/login', component: { template: '<div />' } },
      // 金刚区里还有 分类/搜索/我的 等链接, 不逐一列出(只为消除 no-match 告警)
      { path: '/:pathMatch(.*)*', component: { template: '<div />' } }
    ]
  })
}

async function mountHome(router: Router) {
  const wrapper = mount(HomeView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

describe('HomeView TV「设置」卡 × 原生抽屉', () => {
  let router: Router

  beforeEach(async () => {
    setActivePinia(createPinia())
    localStorage.clear()
    mocks.native.value = true
    mocks.openSettings.mockClear()
    useViewMode().setMode('tv') // 只有 TV 模式才渲染金刚区
    router = makeRouter()
    await router.push('/')
    await router.isReady()
  })

  it('原生壳: 点设置卡 → 开原生抽屉, 且不跳 SPA 路由(靠 @click.capture 拦截)', async () => {
    const wrapper = await mountHome(router)
    const card = wrapper.find('.jc-tv-fc.fc-5')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('设备/账号') // 副标题(2026-10-10 起设置卡只在壳内渲染)

    const pushSpy = vi.spyOn(router, 'push')
    await card.trigger('click')
    await flushPromises()

    expect(mocks.openSettings).toHaveBeenCalledTimes(1)
    expect(pushSpy).not.toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/')
  })

  it('纯网页 TV 模式: 设置卡不渲染(SPA /settings 已删, 没有可去的设置页)', async () => {
    mocks.native.value = false
    const wrapper = await mountHome(router)
    expect(wrapper.find('.jc-tv-fc.fc-5').exists()).toBe(false)
    expect(mocks.openSettings).not.toHaveBeenCalled()
  })

  it('原生壳: 点「我的」卡 → 也开原生抽屉(账号行在抽屉里), 不跳路由', async () => {
    const wrapper = await mountHome(router)
    const card = wrapper.find('.jc-tv-fc.fc-1')
    expect(card.exists()).toBe(true)

    const pushSpy = vi.spyOn(router, 'push')
    await card.trigger('click')
    await flushPromises()

    expect(mocks.openSettings).toHaveBeenCalledTimes(1)
    expect(pushSpy).not.toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/')
  })

  it('纯网页 TV 模式: 点「我的」卡 → 兜底去 /login(未登录态; 登录页对已登录用户自动 redirect)', async () => {
    mocks.native.value = false
    const wrapper = await mountHome(router)
    const card = wrapper.find('.jc-tv-fc.fc-1')
    expect(card.exists()).toBe(true)

    await card.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/login')
    expect(mocks.openSettings).not.toHaveBeenCalled()
  })
})

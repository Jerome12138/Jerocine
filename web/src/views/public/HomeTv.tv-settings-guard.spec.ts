import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'

/**
 * HomeTv 金刚区「设置」卡的宿主分支行为(评审后补; 2026-11-06 随方案 B 拆分自
 * HomeView.tv-settings-guard.spec.ts 迁入 —— 守卫逻辑随组件下沉到了 HomeTv.vue)。
 *
 * 这个用例存在的唯一目的: 锁住 `@click.capture`。
 * 卡是 <RouterLink to="/login">, 原生壳里要拦掉默认导航改开原生抽屉;
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

import HomeTv from './HomeTv.vue'
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

/** 金刚区 props 基线: 1 张推荐图 + 有直播源(否则少卡破坏计数断言); 其余面板空 */
function baseProps() {
  return {
    heroSlides: [{ name: '片A', poster: 'a.jpg', cover: '', remarks: '' }],
    topRanking: [],
    rows: [],
    homeRows: [],
    historyList: [],
    isLoggedIn: false,
    hasLiveSources: true
  }
}

async function mountHomeTv(router: Router) {
  const wrapper = mount(HomeTv, {
    props: baseProps(),
    global: { plugins: [router] }
  })
  await flushPromises()
  return wrapper
}

describe('HomeTv TV「设置」卡 × 原生抽屉', () => {
  let router: Router

  beforeEach(async () => {
    setActivePinia(createPinia())
    localStorage.clear()
    mocks.native.value = true
    mocks.openSettings.mockClear()
    useViewMode().setMode('tv') // 语境对齐 TV 模式(门控在宿主 HomeView 的 v-if, 组件本身不再判 mode)
    router = makeRouter()
    await router.push('/')
    await router.isReady()
  })

  it('原生壳: 点设置卡 → 开原生抽屉, 且不跳 SPA 路由(靠 @click.capture 拦截)', async () => {
    const wrapper = await mountHomeTv(router)
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

  it('原生壳: 设置卡渲染 ⇒ 网格列数 = 卡数 6(--jc-tv-func-n, 恒一行铺满不折行)', async () => {
    const wrapper = await mountHomeTv(router)
    expect(wrapper.find('.jc-tv-fc.fc-5').exists()).toBe(true)
    expect(wrapper.findAll('.jc-tv-fc').length).toBe(6) // 收藏/分类/直播/搜索/我的/设置
    expect(wrapper.find('.jc-tv-funcs').attributes('style')).toMatch(/--jc-tv-func-n:\s*6/)
  })

  it('纯网页 TV 模式: 设置卡不渲染(SPA /settings 已删, 没有可去的设置页)', async () => {
    mocks.native.value = false
    const wrapper = await mountHomeTv(router)
    expect(wrapper.find('.jc-tv-fc.fc-5').exists()).toBe(false)
    expect(mocks.openSettings).not.toHaveBeenCalled()
  })

  it('纯网页 TV 模式: 无设置卡 ⇒ 网格列数 = 卡数 5(--jc-tv-func-n; 有直播源)', async () => {
    mocks.native.value = false
    const wrapper = await mountHomeTv(router)
    expect(wrapper.findAll('.jc-tv-fc').length).toBe(5) // 收藏/分类/直播/搜索/我的
    expect(wrapper.find('.jc-tv-funcs').attributes('style')).toMatch(/--jc-tv-func-n:\s*5/)
  })

  it('原生壳: 点「我的」卡 → 也开原生抽屉(账号行在抽屉里), 不跳路由', async () => {
    const wrapper = await mountHomeTv(router)
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
    const wrapper = await mountHomeTv(router)
    const card = wrapper.find('.jc-tv-fc.fc-1')
    expect(card.exists()).toBe(true)

    await card.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/login')
    expect(mocks.openSettings).not.toHaveBeenCalled()
  })
})

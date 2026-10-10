import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from "@vue/test-utils"
import { createMemoryHistory, createRouter } from 'vue-router'

const reloadToLatest = vi.fn()
vi.mock('@/utils/chunkReload', () => ({
  reloadToLatest: (...args: unknown[]) => reloadToLatest(...args)
}))

const openSettings = vi.fn()
let native = true
vi.mock('@/utils/jerocineNative', () => ({
  isNative: () => native,
  jerocine: { openSettings: () => openSettings() }
}))

import TvHeaderActions from './TvHeaderActions.vue'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/settings', name: 'settings', component: { template: '<div />' } },
    { path: '/:pathMatch(.*)*', component: { template: '<div />' } }
  ]
})

async function mountActions() {
  await router.push('/index')
  await router.isReady()
  return mount(TvHeaderActions, { global: { plugins: [router] } })
}

describe('TvHeaderActions (顶栏最右两个图标按钮)', () => {
  beforeEach(() => {
    reloadToLatest.mockClear()
    openSettings.mockClear()
    native = true
  })

  it('渲染 刷新 / 设置 两个**纯图标**按钮(无文字), 且都自带可聚焦属性(遥控器可达)', async () => {
    const w = await mountActions()
    const btns = w.findAll('button')
    expect(btns).toHaveLength(2)
    // 反馈修订: 只留图标 —— 文字退到 aria-label/title(TV 视距下文字既占宽又冗余)
    expect(btns[0].text()).toBe('')
    expect(btns[1].text()).toBe('')
    expect(btns[0].attributes('aria-label')).toBe('刷新到最新版本')
    expect(btns[0].attributes('title')).toBe('刷新到最新版本')
    expect(btns[1].attributes('aria-label')).toBe('打开设置')
    expect(btns[1].attributes('title')).toBe('打开设置')
    for (const b of btns) {
      expect(b.attributes('data-focusable')).toBe('true')
      expect(b.attributes('tabindex')).toBe('0')
      // 图标本身要画出来(否则按钮是空壳)
      expect(b.find('svg').exists()).toBe(true)
    }
  })

  it('点"刷新" → 调 reloadToLatest(而不是裸 location.reload)', async () => {
    const w = await mountActions()
    await w.findAll('button')[0].trigger('click')
    expect(reloadToLatest).toHaveBeenCalledTimes(1)
    const deps = reloadToLatest.mock.calls[0][0] as { guardStore: unknown; getRegistration: unknown }
    expect(deps.guardStore).toBe(sessionStorage)
    expect(typeof deps.getRegistration).toBe('function')
  })

  it('原生壳内点"设置" → 走 bridge openSettings, 不跳路由', async () => {
    native = true
    const w = await mountActions()
    await w.findAll('button')[1].trigger('click')
    expect(openSettings).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.path).toBe('/index')
  })

  it('纯网页 TV 模式: 「设置」按钮不渲染(SPA /settings 已删, 没有抽屉可开) —— 只剩「刷新」', async () => {
    native = false
    const w = await mountActions()
    const btns = w.findAll('button')
    expect(btns).toHaveLength(1)
    expect(btns[0].attributes('aria-label')).toBe('刷新到最新版本')
    expect(openSettings).not.toHaveBeenCalled()
  })
})

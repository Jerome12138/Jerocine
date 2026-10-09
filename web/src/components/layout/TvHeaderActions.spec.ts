import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
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

describe('TvHeaderActions (TV 胶囊行最右两个按钮)', () => {
  beforeEach(() => {
    reloadToLatest.mockClear()
    openSettings.mockClear()
    native = true
  })

  it('渲染 刷新 / 设置 两个按钮, 且都自带可聚焦属性(遥控器可达)', async () => {
    const w = await mountActions()
    const btns = w.findAll('button')
    expect(btns).toHaveLength(2)
    expect(btns[0].text()).toContain('刷新')
    expect(btns[1].text()).toContain('设置')
    for (const b of btns) {
      expect(b.attributes('data-focusable')).toBe('true')
      expect(b.attributes('tabindex')).toBe('0')
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

  it('纯网页 TV 模式(bridge 不在场)点"设置" → 回退到 SPA /settings', async () => {
    native = false
    const w = await mountActions()
    await w.findAll('button')[1].trigger('click')
    expect(openSettings).not.toHaveBeenCalled()
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/settings')
  })
})

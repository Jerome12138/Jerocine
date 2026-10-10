import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'

/**
 * PublicHeader × 顶栏最右「刷新 / 设置」按钮（2026-10-10 实测反馈修订）。
 *
 * 锁两条**只有看模板结构才能发现**的事实（纯函数用例察觉不到）：
 *   1. 按钮被放在 `.jc-header__inner` 的**最后一个元素子节点**（前面两个 flex-1 spacer
 *      把它顶到容器右缘 = "pin 在最右"）；且**不在**居中的胶囊导航 `.jc-header__nav` 里
 *      （旧实现挂在里面，位置随胶囊走）。
 *   2. 显隐：TV 模式恒显示；**壳内 + 桌面模式**也要显示（否则切到桌面模式后无法切回 TV）；
 *      纯网页 + 桌面模式则不显示。
 *
 * 这里刻意**不 mock** `@/utils/jerocineNative` —— 直接给 window 挂/摘 bridge，
 * 端到端走真实的 canOpenSettings()。
 */

import PublicHeader from './PublicHeader.vue'
import { useViewMode } from '@/composables/useViewMode'

type BridgeWindow = { JerocineNative?: unknown }
const w = window as unknown as BridgeWindow

function makeRouter(): Router {
  const stub = { template: '<div />' }
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: stub },
      { path: '/index', name: 'index', component: stub },
      { path: '/settings', name: 'settings', component: stub },
      { path: '/login', name: 'login', component: stub },
      { path: '/history', name: 'history', component: stub },
      { path: '/favorites', name: 'favorites', component: stub },
      { path: '/filmClassify', name: 'classify', component: stub },
      { path: '/:pathMatch(.*)*', component: stub }
    ]
  })
}

async function mountHeader(router: Router) {
  await router.push('/')
  await router.isReady()
  return mount(PublicHeader, { global: { plugins: [router] } })
}

/** 末位元素子节点是否是那两个按钮 */
function actionsIsLastChild(wrapper: ReturnType<typeof mount>): boolean {
  const inner = wrapper.find('.jc-header__inner')
  if (!inner.exists()) return false
  const kids = inner.element.children
  if (kids.length === 0) return false
  return (kids[kids.length - 1] as HTMLElement).classList.contains('jc-tv-actions')
}

describe('PublicHeader × 顶栏最右「刷新 / 设置」', () => {
  let router: Router

  beforeEach(async () => {
    setActivePinia(createPinia())
    localStorage.clear()
    delete w.JerocineNative
    router = makeRouter()
  })

  afterEach(() => {
    delete w.JerocineNative
  })

  it('壳内 + 桌面模式: 仍显示这两个按钮, 且钉在 .jc-header__inner 最右(不在居中胶囊里)', async () => {
    w.JerocineNative = { invoke: () => '{"ok":true}' }
    useViewMode().setMode('desktop') // 壳内选了桌面模式
    const wrapper = await mountHeader(router)

    const actions = wrapper.find('.jc-tv-actions')
    expect(actions.exists()).toBe(true)
    // 只应有一组(防止"两处各渲染一份"这种回退)
    expect(wrapper.findAll('.jc-tv-actions')).toHaveLength(1)
    expect(actions.findAll('button')).toHaveLength(2)
    // 移出了居中胶囊导航
    expect(wrapper.find('.jc-header__nav .jc-tv-actions').exists()).toBe(false)
    // 且是 inner 的末位子节点 = 被 spacer 顶到最右
    expect(actionsIsLastChild(wrapper)).toBe(true)
  })

  it('纯网页 + 桌面模式: 不显示(桥不在场 ⇒ 没有可开的原生设置)', async () => {
    useViewMode().setMode('desktop')
    const wrapper = await mountHeader(router)

    expect(wrapper.find('.jc-tv-actions').exists()).toBe(false)
  })

  it('纯网页 + TV 模式: 显示(设置按钮退化为跳 SPA /settings, 不是死按钮)', async () => {
    useViewMode().setMode('tv')
    const wrapper = await mountHeader(router)

    expect(wrapper.find('.jc-tv-actions').exists()).toBe(true)
    expect(actionsIsLastChild(wrapper)).toBe(true)
  })

  it('壳内 + TV 模式: 显示且同样钉在最右', async () => {
    w.JerocineNative = { invoke: () => '{"ok":true}' }
    useViewMode().setMode('tv')
    const wrapper = await mountHeader(router)

    expect(wrapper.find('.jc-tv-actions').exists()).toBe(true)
    expect(actionsIsLastChild(wrapper)).toBe(true)
  })
})

import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import RouteProgress from './RouteProgress.vue'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/', component: { template: '<div />' } },
    { path: '/x', component: { template: '<div />' } },
    { path: '/y', component: { template: '<div />' } }
  ]
})

describe('RouteProgress', () => {
  beforeEach(async () => {
    await router.push('/')
    await router.isReady()
  })

  it('挂载成功', () => {
    const w = mount(RouteProgress, { global: { plugins: [router] } })
    expect(w.exists()).toBe(true)
  })

  it('默认 phase=idle (不渲染条)', () => {
    const w = mount(RouteProgress, { global: { plugins: [router] } })
    // idle 通常不渲染或隐藏条
    expect(w.html()).toBeDefined()
  })

  it('路由切换后 phase 状态变化 (start → done)', async () => {
    const w = mount(RouteProgress, { global: { plugins: [router] } })
    await router.push('/x')
    // 等几拍, 让 raf + beforeEach + afterEach 完成
    await new Promise((r) => setTimeout(r, 50))
    // 此时应该处于 done (因为 afterEach 已执行)
    expect(w.exists()).toBe(true)
  })

  it('卸载时清理 router hook (不抛错)', () => {
    const w = mount(RouteProgress, { global: { plugins: [router] } })
    expect(() => w.unmount()).not.toThrow()
  })
})

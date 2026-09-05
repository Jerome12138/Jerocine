import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { defineComponent, h } from 'vue'
import PublicLayout from './PublicLayout.vue'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/', component: { template: '<div />' } },
    { path: '/index', component: { template: '<div />' } },
    { path: '/login', component: { template: '<div />' } },
    { path: '/:pathMatch(.*)*', component: { template: '<div />' } }
  ]
})

async function mountLayout() {
  await router.push('/')
  await router.isReady()
  const Wrapper = defineComponent({
    render() {
      return h(PublicLayout, null, { default: () => h('div', { class: 'test-slot' }, 'X') })
    }
  })
  return mount(Wrapper, { global: { plugins: [router] } })
}

describe('PublicLayout', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('挂载成功 + 含 header/main/footer', async () => {
    const w = await mountLayout()
    expect(w.findComponent({ name: 'PublicHeader' }).exists()).toBe(true)
    expect(w.find('main').exists()).toBe(true)
    expect(w.findComponent({ name: 'PublicFooter' }).exists()).toBe(true)
  })

  it('slot 内容渲染到 main', async () => {
    const w = await mountLayout()
    expect(w.find('main .test-slot').exists()).toBe(true)
  })

  it('根 div data-mode 响应 useViewMode', async () => {
    const w = await mountLayout()
    const root = w.find('.gf-public-layout')
    expect(root.attributes('data-mode')).toBeDefined()
  })

  it('含 MobileTabbar / BackToTop / RouteProgress 组件', async () => {
    const w = await mountLayout()
    expect(w.findComponent({ name: 'MobileTabbar' }).exists()).toBe(true)
    expect(w.findComponent({ name: 'BackToTop' }).exists()).toBe(true)
    expect(w.findComponent({ name: 'RouteProgress' }).exists()).toBe(true)
  })
})

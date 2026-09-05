import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { defineComponent, h } from 'vue'
import ManageLayout from './ManageLayout.vue'
import { useViewMode } from '@/composables/useViewMode'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/', component: { template: '<div />' }, meta: { title: 'Home' } },
    { path: '/manage/index', component: { template: '<div />' }, meta: { title: 'Dashboard' } },
    { path: '/login', component: { template: '<div />' } },
    { path: '/:pathMatch(.*)*', component: { template: '<div />' } }
  ]
})

function forceMode(m: 'mobile' | 'desktop'): void {
  useViewMode().setMode(m)
}

async function mountLayout() {
  await router.push('/manage/index')
  await router.isReady()
  const Wrapper = defineComponent({
    components: { ManageLayout },
    render() {
      return h(ManageLayout, null, { default: () => h('div', { class: 'test-slot' }, 'X') })
    }
  })
  return mount(Wrapper, {
    global: { plugins: [router] }
  })
}

describe('ManageLayout 响应式接入', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    forceMode('desktop')
  })

  it('根 div 有 gf-manage class (P4 触摸目标 CSS scope)', async () => {
    const w = await mountLayout()
    const root = w.find('div.gf-manage')
    expect(root.exists()).toBe(true)
  })

  it('desktop 模式 data-mode="desktop"', async () => {
    forceMode('desktop')
    const w = await mountLayout()
    expect(w.find('div.gf-manage').attributes('data-mode')).toBe('desktop')
  })

  it('mobile 模式 data-mode="mobile"', async () => {
    forceMode('mobile')
    const w = await mountLayout()
    expect(w.find('div.gf-manage').attributes('data-mode')).toBe('mobile')
  })

  it('main 区有三档 padding class (p-3 / md:p-4 / lg:p-6)', async () => {
    const w = await mountLayout()
    const main = w.find('main')
    const cls = main.classes()
    expect(cls).toContain('p-[var(--gf-space-3)]')
    expect(cls).toContain('md:p-[var(--gf-space-4)]')
    expect(cls).toContain('lg:p-[var(--gf-space-6)]')
  })

  it('slot 内容渲染到 main 区', async () => {
    const w = await mountLayout()
    expect(w.find('main .test-slot').exists()).toBe(true)
  })

  it('desktop 模式: ManageSidebar 收到 variant=full', async () => {
    forceMode('desktop')
    const w = await mountLayout()
    expect(w.findComponent({ name: 'ManageSidebar' }).props('variant')).toBe('full')
  })

  it('mobile 模式: ManageSidebar 收到 variant=drawer', async () => {
    forceMode('mobile')
    const w = await mountLayout()
    expect(w.findComponent({ name: 'ManageSidebar' }).props('variant')).toBe('drawer')
  })

  it('mobile 模式: ManageHeader 收到 showHamburger=true', async () => {
    forceMode('mobile')
    const w = await mountLayout()
    expect(w.findComponent({ name: 'ManageHeader' }).props('showHamburger')).toBe(true)
  })

  it('desktop 模式: ManageHeader 收到 showHamburger=false', async () => {
    forceMode('desktop')
    const w = await mountLayout()
    expect(w.findComponent({ name: 'ManageHeader' }).props('showHamburger')).toBe(false)
  })

  it('drawerOpen 默认 false, sidebar.open=false', async () => {
    forceMode('mobile')
    const w = await mountLayout()
    expect(w.findComponent({ name: 'ManageSidebar' }).props('open')).toBe(false)
  })

  it('ManageHeader emit toggle-drawer → sidebar.open 变 true', async () => {
    forceMode('mobile')
    const w = await mountLayout()
    await w.findComponent({ name: 'ManageHeader' }).vm.$emit('toggle-drawer')
    await w.vm.$nextTick()
    expect(w.findComponent({ name: 'ManageSidebar' }).props('open')).toBe(true)
  })

  it('ManageSidebar emit close → drawerOpen 重置 false', async () => {
    forceMode('mobile')
    const w = await mountLayout()
    // 先打开
    await w.findComponent({ name: 'ManageHeader' }).vm.$emit('toggle-drawer')
    await w.vm.$nextTick()
    expect(w.findComponent({ name: 'ManageSidebar' }).props('open')).toBe(true)
    // 触发 close
    await w.findComponent({ name: 'ManageSidebar' }).vm.$emit('close')
    await w.vm.$nextTick()
    expect(w.findComponent({ name: 'ManageSidebar' }).props('open')).toBe(false)
  })
})

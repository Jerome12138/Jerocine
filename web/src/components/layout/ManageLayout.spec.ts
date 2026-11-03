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

  it('根 div 有 jc-manage class (P4 触摸目标 CSS scope)', async () => {
    const w = await mountLayout()
    const root = w.find('div.jc-manage')
    expect(root.exists()).toBe(true)
  })

  it('desktop 模式 data-mode="desktop"', async () => {
    forceMode('desktop')
    const w = await mountLayout()
    expect(w.find('div.jc-manage').attributes('data-mode')).toBe('desktop')
  })

  it('mobile 模式 data-mode="mobile"', async () => {
    forceMode('mobile')
    const w = await mountLayout()
    expect(w.find('div.jc-manage').attributes('data-mode')).toBe('mobile')
  })

  it('main 区有三档 padding class (p-3 / md:p-4 / lg:p-6)', async () => {
    const w = await mountLayout()
    const main = w.find('main')
    const cls = main.classes()
    expect(cls).toContain('p-[var(--jc-space-3)]')
    expect(cls).toContain('md:p-[var(--jc-space-4)]')
    expect(cls).toContain('lg:p-[var(--jc-space-6)]')
  })

  it('main 含高度链类 flex flex-col min-h-0 (表格 fit 依赖)', async () => {
    const w = await mountLayout()
    const cls = w.find('main').classes()
    expect(cls).toContain('flex-col')
    expect(cls).toContain('min-h-0')
  })

  it('main 内存在 .jc-manage-main 内容包裹层: 居中封顶 1440px + 高度链', async () => {
    const w = await mountLayout()
    const inner = w.find('main .jc-manage-main')
    expect(inner.exists()).toBe(true)
    const cls = inner.classes()
    expect(cls).toContain('mx-auto')
    expect(cls).toContain('max-w-[var(--jc-content-max-w)]')
    expect(cls).toContain('min-h-0')
    expect(cls).toContain('flex-1')
    expect(cls).toContain('flex-col')
    // slot 内容落在包裹层内
    expect(inner.find('.test-slot').exists()).toBe(true)
  })

  it('slot 内容渲染到 main 区', async () => {
    const w = await mountLayout()
    expect(w.find('main .test-slot').exists()).toBe(true)
  })

  it('高度自适应: 根 h-screen+overflow-hidden, main overflow-hidden, 内容层 overflow-y-auto', async () => {
    const w = await mountLayout()
    const root = w.find('div.jc-manage').classes()
    expect(root).toContain('h-screen')
    expect(root).toContain('overflow-hidden')
    expect(w.find('main').classes()).toContain('overflow-hidden')
    expect(w.find('main .jc-manage-main').classes()).toContain('overflow-y-auto')
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

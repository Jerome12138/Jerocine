import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import PublicFooter from './PublicFooter.vue'
import { useSiteStore } from '@/stores/site'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
})

describe('PublicFooter', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('basic 未加载 → siteName 兜底 Jerocine', () => {
    const w = mount(PublicFooter, { global: { plugins: [router] } })
    expect(w.text()).toContain('Jerocine')
  })

  it('basic.siteName 显示', () => {
    const store = useSiteStore()
    store.basic = { siteName: 'MyFilm', domain: '', logo: '', keyword: '', describe: '', state: true, hint: '' } as never
    const w = mount(PublicFooter, { global: { plugins: [router] } })
    expect(w.text()).toContain('MyFilm')
  })

  it('basic.description 显示', () => {
    const store = useSiteStore()
    store.basic = { siteName: 'X', description: '在线影视', state: true } as never
    const w = mount(PublicFooter, { global: { plugins: [router] } })
    expect(w.text()).toContain('在线影视')
  })

  it('当前年份显示', () => {
    const w = mount(PublicFooter, { global: { plugins: [router] } })
    expect(w.text()).toContain(String(new Date().getFullYear()))
  })

  it('包含「站点」标题区', () => {
    const w = mount(PublicFooter, { global: { plugins: [router] } })
    expect(w.text()).toContain('站点')
  })
})

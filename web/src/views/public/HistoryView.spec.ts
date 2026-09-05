import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import HistoryView from './HistoryView.vue'
import { useHistoryStore } from '@/stores/history'

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

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
})

async function mountView() {
  await router.push('/history')
  await router.isReady()
  return mount(HistoryView, { global: { plugins: [router] } })
}

describe('HistoryView smoke', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    document.cookie.split(';').forEach((c) => {
      const eq = c.indexOf('=')
      const name = (eq > -1 ? c.slice(0, eq) : c).trim()
      document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`
    })
  })

  it('挂载成功 (无错误)', async () => {
    const w = await mountView()
    expect(w.exists()).toBe(true)
  })

  it('无历史记录 → 渲染 BaseEmpty', async () => {
    const w = await mountView()
    expect(w.findComponent({ name: 'BaseEmpty' }).exists()).toBe(true)
  })

  it('未登录 → 显示「本地历史 · 仅当前浏览器」', async () => {
    const w = await mountView()
    expect(w.text()).toContain('本地历史')
  })

  it('有历史记录 → 不渲染 BaseEmpty', async () => {
    setActivePinia(createPinia())
    const store = useHistoryStore()
    store.record({
      id: '1',
      name: '测试电影',
      link: '/play?id=1&source=x&episode=0',
      episode: '第 1 集'
    } as never)
    const w = await mountView()
    // 历史列表存在, BaseEmpty 不显示
    expect(w.findComponent({ name: 'BaseEmpty' }).exists()).toBe(false)
    expect(w.text()).toContain('测试电影')
  })

  it('清空按钮存在 (有记录时)', async () => {
    setActivePinia(createPinia())
    const store = useHistoryStore()
    store.record({
      id: '1',
      name: 'A',
      link: '/play?id=1&source=x&episode=0',
      episode: 'E'
    } as never)
    const w = await mountView()
    const buttons = w.findAll('button')
    const clearBtn = buttons.find(b => b.text().includes('清空'))
    expect(clearBtn).toBeTruthy()
  })
})

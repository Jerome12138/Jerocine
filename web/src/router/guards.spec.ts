import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { registerGuards } from './guards'

vi.mock('@/api/auth', () => ({
  login: vi.fn(),
  getUserInfo: vi.fn(),
  logout: vi.fn(),
  changePassword: vi.fn()
}))
vi.mock('@/api/http', () => ({
  toast: vi.fn()
}))

function setupRouter() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/index', component: { template: '<div />' } },
      { path: '/login', component: { template: '<div />' } },
      {
        path: '/manage/index',
        component: { template: '<div />' },
        meta: { requiresAuth: true, requiresAdmin: true, title: 'Dashboard' }
      },
      { path: '/account', component: { template: '<div />' }, meta: { requiresAuth: true } }
    ]
  })
  registerGuards(router)
  return router
}

describe('router guards', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    document.documentElement.removeAttribute('data-mode')
    vi.clearAllMocks()
  })

  it('未登录访问 requiresAuth → 重定向到 /login (带 redirect query)', async () => {
    const router = setupRouter()
    await router.push('/account')
    await router.isReady()
    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toBe('/account')
  })

  it('未登录访问 requiresAdmin → 重定向到 /login', async () => {
    const router = setupRouter()
    await router.push('/manage/index')
    expect(router.currentRoute.value.path).toBe('/login')
  })

  it('登录且是管理员 → 允许访问 /manage/index', async () => {
    localStorage.setItem('auth-token', 'tok')
    const { getUserInfo } = await import('@/api/auth')
    ;(getUserInfo as ReturnType<typeof vi.fn>).mockResolvedValue({ userName: 'admin', role: 1 })

    const router = setupRouter()
    await router.push('/manage/index')
    expect(router.currentRoute.value.path).toBe('/manage/index')
  })

  it('登录但非管理员访问 /manage → 重定向 /index + toast', async () => {
    localStorage.setItem('auth-token', 'tok')
    const { getUserInfo } = await import('@/api/auth')
    ;(getUserInfo as ReturnType<typeof vi.fn>).mockResolvedValue({ userName: 'jerry', role: 0 })
    const { toast } = await import('@/api/http')

    const router = setupRouter()
    await router.push('/manage/index')
    expect(router.currentRoute.value.path).toBe('/index')
    expect(toast).toHaveBeenCalledWith('error', expect.stringContaining('权限不足'))
  })

  it('TV 模式访问 /manage/* → 重定向 /index', async () => {
    document.documentElement.setAttribute('data-mode', 'tv')
    localStorage.setItem('auth-token', 'tok')
    const { getUserInfo } = await import('@/api/auth')
    ;(getUserInfo as ReturnType<typeof vi.fn>).mockResolvedValue({ userName: 'admin', role: 1 })

    const router = setupRouter()
    await router.push('/manage/index')
    expect(router.currentRoute.value.path).toBe('/index')
  })

  it('未声明 requiresAuth 的路由 → 直接放行', async () => {
    const router = setupRouter()
    await router.push('/')
    expect(router.currentRoute.value.path).toBe('/')
  })

  it('afterEach 设置 document.title (含 meta.title + APP_TITLE)', async () => {
    localStorage.setItem('auth-token', 'tok')
    const { getUserInfo } = await import('@/api/auth')
    ;(getUserInfo as ReturnType<typeof vi.fn>).mockResolvedValue({ userName: 'admin', role: 1 })

    const router = setupRouter()
    await router.push('/manage/index')
    await new Promise((r) => setTimeout(r, 0))
    expect(document.title).toContain('Dashboard')
  })
})

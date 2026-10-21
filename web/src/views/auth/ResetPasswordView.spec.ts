import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import ResetPasswordView from './ResetPasswordView.vue'
import { resetPassword } from '@/api/auth'
import { ApiError } from '@/types/api'

vi.mock('@/api/auth', () => ({
  login: vi.fn(),
  logout: vi.fn(),
  changePassword: vi.fn(),
  getUserInfo: vi.fn(),
  requestPasswordReset: vi.fn(),
  resetPassword: vi.fn(),
  listUsers: vi.fn()
}))

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/reset-password', component: ResetPasswordView },
    { path: '/forgot-password', component: { template: '<div />' } },
    { path: '/login', component: { template: '<div />' } }
  ]
})

async function mountView(query: Record<string, string> = { token: 'tok-abc' }) {
  await router.push({ path: '/reset-password', query })
  await router.isReady()
  const w = mount(ResetPasswordView, { global: { plugins: [router] } })
  await w.vm.$nextTick()
  return w
}

async function fillAndSubmit(w: Awaited<ReturnType<typeof mountView>>) {
  const pwds = w.findAll('input[type="password"]')
  await pwds[0].setValue('newsecret1')
  await pwds[1].setValue('newsecret1')
  await w.find('form').trigger('submit.prevent')
  await new Promise((r) => setTimeout(r, 10))
}

describe('ResetPasswordView 重置密码落地页', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(resetPassword).mockReset()
  })

  it('缺 token → 提示从邮件进入, 给出重新申请入口', async () => {
    const w = await mountView({})
    expect(w.text()).toContain('重置链接无效或已不完整')
    expect(w.find('form').exists()).toBe(false)
    const btn = w.findAll('button').find((b) => b.text() === '重新申请')
    expect(btn).toBeTruthy()
  })

  it('两次密码不一致 → 拦截, 不调端点', async () => {
    const w = await mountView()
    const pwds = w.findAll('input[type="password"]')
    await pwds[0].setValue('newsecret1')
    await pwds[1].setValue('newsecret2')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('两次输入的密码不一致')
    expect(resetPassword).not.toHaveBeenCalled()
  })

  it('密码超过 64 位 → 前端拦截(服务端按无效参数拒)', async () => {
    const w = await mountView()
    const pwds = w.findAll('input[type="password"]')
    const long = 'a'.repeat(65)
    await pwds[0].setValue(long)
    await pwds[1].setValue(long)
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('密码最多 64 位')
    expect(resetPassword).not.toHaveBeenCalled()
  })

  it('重置成功 → 带 token/password 调端点, 回登录页(带 reset=1)', async () => {
    vi.mocked(resetPassword).mockResolvedValue({ reset: true })
    const w = await mountView()
    await fillAndSubmit(w)

    expect(resetPassword).toHaveBeenCalledWith({ token: 'tok-abc', password: 'newsecret1' })
    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.reset).toBe('1')
  })

  it('token 无效/过期(400) → 提示重新申请', async () => {
    vi.mocked(resetPassword).mockRejectedValue(
      new ApiError(400, { message: '重置链接无效或已过期，请重新申请' })
    )
    const w = await mountView()
    await fillAndSubmit(w)
    expect(w.text()).toContain('重置链接无效或已过期，请重新申请')
    expect(router.currentRoute.value.path).toBe('/reset-password')
  })
})

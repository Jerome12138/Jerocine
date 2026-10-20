import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import ForgotPasswordView from './ForgotPasswordView.vue'
import { requestPasswordReset } from '@/api/auth'

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
    { path: '/forgot-password', component: ForgotPasswordView },
    { path: '/login', component: { template: '<div />' } },
    { path: '/index', component: { template: '<div />' } }
  ]
})

async function mountView() {
  await router.push('/forgot-password')
  await router.isReady()
  return mount(ForgotPasswordView, { global: { plugins: [router] } })
}

describe('ForgotPasswordView 忘记密码', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(requestPasswordReset).mockReset()
  })

  it('空邮箱提交 → 拦截', async () => {
    const w = await mountView()
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).toContain('请输入邮箱')
    expect(requestPasswordReset).not.toHaveBeenCalled()
  })

  it('提交成功 → 中性状态页(不透露邮箱是否注册)', async () => {
    vi.mocked(requestPasswordReset).mockResolvedValue({ sent: true })
    const w = await mountView()
    await w.find('input[type="email"]').setValue('jerry@x.com')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r, 10))

    expect(requestPasswordReset).toHaveBeenCalledWith('jerry@x.com')
    expect(w.text()).toContain('重置邮件已发送')
    expect(w.text()).toContain('若该邮箱已注册')
    expect(w.text()).toContain('垃圾箱')
    expect(w.find('form').exists()).toBe(false)
  })

  it('发送失败 → 错误原文, 停留表单', async () => {
    vi.mocked(requestPasswordReset).mockRejectedValue(new Error('验证码发送太频繁，请稍后再试'))
    const w = await mountView()
    await w.find('input[type="email"]').setValue('jerry@x.com')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('验证码发送太频繁，请稍后再试')
    expect(w.find('form').exists()).toBe(true)
  })
})

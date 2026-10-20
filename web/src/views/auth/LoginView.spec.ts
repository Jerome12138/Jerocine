import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import LoginView from './LoginView.vue'
import { useUserStore } from '@/stores/user'
import { getRegisterConfig, applyStatus } from '@/api/register'
import { ApiError } from '@/types/api'

vi.mock('@/api/register', () => ({
  getRegisterConfig: vi.fn().mockResolvedValue({ mode: 'off', smtpReady: false }),
  sendRegisterCode: vi.fn(),
  registerAccount: vi.fn(),
  applyRegister: vi.fn(),
  applyStatus: vi.fn(),
  resendActivation: vi.fn(),
  activateAccount: vi.fn()
}))

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/login', component: LoginView },
    { path: '/register', component: { template: '<div />' } },
    { path: '/forgot-password', component: { template: '<div />' } },
    { path: '/index', component: { template: '<div />' } },
    { path: '/manage/index', component: { template: '<div />' } },
    { path: '/:pathMatch(.*)*', component: { template: '<div />' } }
  ]
})

async function mountLogin() {
  await router.push('/login')
  await router.isReady()
  return mount(LoginView, { global: { plugins: [router] } })
}

describe('LoginView 校验 + autofill 兜底', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('空表单提交 → 弹「请输入用户名 / 邮箱」', async () => {
    const w = await mountLogin()
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).toContain('请输入用户名 / 邮箱')
  })

  it('用户名有, 密码空 → 弹「请输入密码」', async () => {
    const w = await mountLogin()
    await w.find('input[autocomplete="username"]').setValue('admin')
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).toContain('请输入密码')
  })

  it('autofill 兜底: input DOM 有 value 但 form.username 未通过 input event 同步, 提交时 syncAutofill 应能补上', async () => {
    const w = await mountLogin()
    const userStore = useUserStore()
    const loginSpy = vi.spyOn(userStore, 'login').mockResolvedValue({
      userName: 'admin',
      role: 1
    } as never)

    // 模拟 autofill: 直接设 DOM input value, 不 trigger input event
    const userInput = w.find('input[autocomplete="username"]').element as HTMLInputElement
    const pwdInput = w.find('input[autocomplete="current-password"]').element as HTMLInputElement
    userInput.value = 'admin'
    pwdInput.value = 'password123'

    // 提交 — syncAutofill 应从 DOM 兜底读 value
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()

    // 不弹「请输入...」错误
    expect(w.text()).not.toContain('请输入用户名 / 邮箱')
    expect(w.text()).not.toContain('请输入密码')
    // login 实际被调用且 username/password 正确
    expect(loginSpy).toHaveBeenCalledWith({
      username: 'admin',
      password: 'password123'
    })
  })

  it('@change 监听: input change 事件应同步到 form (autofill 完成后 focus/blur 触发 change)', async () => {
    const w = await mountLogin()
    const userInput = w.find('input[autocomplete="username"]')
    const el = userInput.element as HTMLInputElement
    // 模拟 autofill 后 change event (DOM value 改了, 触发 change)
    el.value = 'jerry'
    await userInput.trigger('change')

    // form.username 应该已经同步 (通过 @change handler)
    // 用提交验证: 不弹用户名错误
    const userStore = useUserStore()
    vi.spyOn(userStore, 'login').mockResolvedValue({ userName: 'jerry', role: 0 } as never)
    await w.find('input[autocomplete="current-password"]').setValue('xx')
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).not.toContain('请输入用户名 / 邮箱')
  })

  it('login 失败时 errorMsg 显示错误', async () => {
    const w = await mountLogin()
    const userStore = useUserStore()
    vi.spyOn(userStore, 'login').mockRejectedValue(new Error('用户名或密码错误'))

    await w.find('input[autocomplete="username"]').setValue('admin')
    await w.find('input[autocomplete="current-password"]').setValue('wrong')
    await w.find('form').trigger('submit.prevent')
    // 等异步 login reject
    await new Promise((r) => setTimeout(r, 10))

    expect(w.text()).toContain('用户名或密码错误')
  })

  it('密码可见性切换按钮', async () => {
    const w = await mountLogin()
    const pwdInput = w.find('input[autocomplete="current-password"]')
    expect(pwdInput.attributes('type')).toBe('password')

    // 找到 eye 按钮 (aria-label="切换密码可见性")
    const toggleBtn = w.find('button[aria-label="切换密码可见性"]')
    await toggleBtn.trigger('click')
    expect(pwdInput.attributes('type')).toBe('text')
    await toggleBtn.trigger('click')
    expect(pwdInput.attributes('type')).toBe('password')
  })

  it('register-config off(默认) → 保留「注册功能已暂时下线」指引, 不显示注册入口', async () => {
    const w = await mountLogin()
    await new Promise((r) => setTimeout(r, 10))
    await w.vm.$nextTick()
    expect(w.text()).toContain('注册功能已暂时下线')
    expect(w.text()).not.toContain('立即注册')
  })

  it('register-config open → 显示注册入口, 点击跳 /register', async () => {
    vi.mocked(getRegisterConfig).mockResolvedValue({ mode: 'open', smtpReady: true })
    const w = await mountLogin()
    await new Promise((r) => setTimeout(r, 10))
    await w.vm.$nextTick()

    expect(w.text()).toContain('还没有账号')
    const btn = w.findAll('button').find((b) => b.text() === '立即注册')
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(router.currentRoute.value.path).toBe('/register')
  })
})

describe('LoginView apply 模式: 登录申请态反馈 + 进度查询', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(applyStatus).mockReset()
  })

  async function mountApply() {
    vi.mocked(getRegisterConfig).mockResolvedValue({ mode: 'apply', smtpReady: true })
    const w = await mountLogin()
    await new Promise((r) => setTimeout(r, 10))
    await w.vm.$nextTick()
    return w
  }

  /** 用申请时的用户名尝试登录(账号实际不存在) → 服务端 403 申请态 */
  async function loginFailWith(err: Error) {
    const w = await mountApply()
    const userStore = useUserStore()
    vi.spyOn(userStore, 'login').mockRejectedValue(err)
    await w.find('input[autocomplete="username"]').setValue('jerry')
    await w.find('input[autocomplete="current-password"]').setValue('whatever')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r, 10))
    return w
  }

  it('pending → 登录失败展示「审核中」原文', async () => {
    const w = await loginFailWith(new ApiError(403, { message: '注册申请审核中，请等待邮件通知' }))
    expect(w.text()).toContain('注册申请审核中，请等待邮件通知')
    // pending 不误导用户重申
    expect(w.text()).not.toContain('可重新提交申请')
  })

  it('rejected → 展示驳回理由 + 补「可重新提交申请」入口', async () => {
    const w = await loginFailWith(new ApiError(403, { message: '注册申请被驳回：邀请人信息无法核实' }))
    expect(w.text()).toContain('注册申请被驳回：邀请人信息无法核实')
    expect(w.text()).toContain('可重新提交申请')
  })

  it('显示「查询注册申请进度」入口, 点击展开进度面板', async () => {
    vi.mocked(applyStatus).mockResolvedValue({ status: 'pending', reason: '', canResend: false })
    const w = await mountApply()
    const entry = w.findAll('button').find((b) => b.text() === '查询注册申请进度')
    expect(entry).toBeTruthy()
    await entry!.trigger('click')
    await w.vm.$nextTick()
    expect(w.find('input[type="email"]').exists()).toBe(true)
    expect(w.text()).toContain('查询进度')
  })

  it('显示「忘记密码？」入口, 点击跳 /forgot-password', async () => {
    vi.mocked(getRegisterConfig).mockResolvedValue({ mode: 'off', smtpReady: false })
    const w = await mountLogin()
    const btn = w.findAll('button').find((b) => b.text() === '忘记密码？')
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(router.currentRoute.value.path).toBe('/forgot-password')
  })

  it('?reset=1(重置成功跳回) → 提示用新密码登录', async () => {
    vi.mocked(getRegisterConfig).mockResolvedValue({ mode: 'off', smtpReady: false })
    await router.push('/login?reset=1')
    await router.isReady()
    const w = mount(LoginView, { global: { plugins: [router] } })
    await new Promise((r) => setTimeout(r, 10))
    await w.vm.$nextTick()
    expect(w.text()).toContain('密码已重置，请使用新密码登录')
  })
})

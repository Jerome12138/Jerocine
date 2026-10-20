import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import RegisterView from './RegisterView.vue'
import { useUserStore } from '@/stores/user'
import { getRegisterConfig, sendRegisterCode, registerAccount } from '@/api/register'

vi.mock('@/api/register', () => ({
  getRegisterConfig: vi.fn(),
  sendRegisterCode: vi.fn(),
  registerAccount: vi.fn()
}))

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/register', component: RegisterView },
    { path: '/login', component: { template: '<div />' } },
    { path: '/index', component: { template: '<div />' } }
  ]
})

async function mountRegister(mode: 'open' | 'apply' = 'open') {
  vi.mocked(getRegisterConfig).mockResolvedValue({ mode, smtpReady: true })
  await router.push('/register')
  await router.isReady()
  const w = mount(RegisterView, { global: { plugins: [router] } })
  // 等 onMounted 的 register-config 拉取落地
  await new Promise((r) => setTimeout(r, 10))
  await w.vm.$nextTick()
  return w
}

/** 打桩用户 store: setToken 触发的原生/telemetry 副作用与网络请求全部挡掉 */
function stubUserStore() {
  const userStore = useUserStore()
  vi.spyOn(userStore, 'setToken').mockImplementation(() => {})
  vi.spyOn(userStore, 'fetchInfo').mockResolvedValue({} as never)
  return userStore
}

describe('RegisterView 自注册(open)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(sendRegisterCode).mockReset()
    vi.mocked(registerAccount).mockReset()
  })

  it('mode=off → 回登录页(不应停留注册页)', async () => {
    vi.mocked(getRegisterConfig).mockResolvedValue({ mode: 'off', smtpReady: false })
    await router.push('/register')
    await router.isReady()
    mount(RegisterView, { global: { plugins: [router] } })
    await new Promise((r) => setTimeout(r, 10))
    expect(router.currentRoute.value.path).toBe('/login')
  })

  it('mode=apply(P1 占位) → 显示申请通道即将开放, 不渲染注册表单', async () => {
    const w = await mountRegister('apply')
    expect(w.text()).toContain('申请注册通道即将开放')
    expect(w.find('form').exists()).toBe(false)
  })

  it('open: 空表单提交 → 依次提示用户名/密码/邮箱/验证码', async () => {
    const w = await mountRegister()
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).toContain('请输入用户名')

    await w.find('input[autocomplete="username"]').setValue('jerry')
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).toContain('密码至少 6 位')

    await w.find('input[autocomplete="new-password"]').setValue('123456')
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).toContain('请输入邮箱')

    await w.find('input[autocomplete="email"]').setValue('a@b.com')
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).toContain('请输入验证码')
    expect(registerAccount).not.toHaveBeenCalled()
  })

  it('open: 发验证码成功 → 60s 冷却(重发按钮禁用并读秒)', async () => {
    vi.mocked(sendRegisterCode).mockResolvedValue({ sent: true })
    const w = await mountRegister()
    await w.find('input[autocomplete="email"]').setValue('a@b.com')
    const getCode = w.findAll('button').find((b) => b.text().includes('获取验证码'))
    await getCode!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(sendRegisterCode).toHaveBeenCalledWith('a@b.com')
    await w.vm.$nextTick()
    // 冷却按钮文案进入读秒, 且按钮禁用
    expect(w.text()).toMatch(/\d+s 后重发/)
    const resend = w.findAll('button').find((b) => b.text().includes('后重发'))
    expect(resend!.attributes('disabled')).toBeDefined()
  })

  it('open: 发验证码失败 → 错误原文上抛(如发送过快)', async () => {
    vi.mocked(sendRegisterCode).mockRejectedValue(new Error('发送过于频繁，请 1 分钟后再试'))
    const w = await mountRegister()
    await w.find('input[autocomplete="email"]').setValue('a@b.com')
    const resend = w.findAll('button').find((b) => b.text().includes('获取验证码'))
    await resend!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    await w.vm.$nextTick()
    expect(w.text()).toContain('发送过于频繁，请 1 分钟后再试')
  })

  it('open: 注册成功 → setToken + fetchInfo 后跳首页', async () => {
    vi.mocked(sendRegisterCode).mockResolvedValue({ sent: true })
    vi.mocked(registerAccount).mockResolvedValue({
      token: 'tok-1',
      expires: 123
    } as never)
    const userStore = stubUserStore()

    const w = await mountRegister()
    await w.find('input[autocomplete="username"]').setValue('jerry')
    await w.find('input[autocomplete="new-password"]').setValue('123456')
    await w.find('input[autocomplete="email"]').setValue('a@b.com')
    await w.find('input[autocomplete="one-time-code"]').setValue('654321')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r, 10))

    expect(registerAccount).toHaveBeenCalledWith({
      userName: 'jerry',
      password: '123456',
      email: 'a@b.com',
      code: '654321'
    })
    expect(userStore.setToken).toHaveBeenCalledWith('tok-1', 123)
    expect(userStore.fetchInfo).toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/index')
  })

  it('open: 注册失败 → 错误原文显示, 不写 token', async () => {
    vi.mocked(registerAccount).mockRejectedValue(new Error('邮箱已被注册'))
    const userStore = stubUserStore()

    const w = await mountRegister()
    await w.find('input[autocomplete="username"]').setValue('jerry')
    await w.find('input[autocomplete="new-password"]').setValue('123456')
    await w.find('input[autocomplete="email"]').setValue('a@b.com')
    await w.find('input[autocomplete="one-time-code"]').setValue('654321')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r, 10))

    expect(w.text()).toContain('邮箱已被注册')
    expect(userStore.setToken).not.toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/register')
  })

  it('open: smtpReady=false → 提示邮件通道未配置', async () => {
    vi.mocked(getRegisterConfig).mockResolvedValue({ mode: 'open', smtpReady: false })
    await router.push('/register')
    await router.isReady()
    const w = mount(RegisterView, { global: { plugins: [router] } })
    await new Promise((r) => setTimeout(r, 10))
    await w.vm.$nextTick()
    expect(w.text()).toContain('邮件通道未配置')
  })
})

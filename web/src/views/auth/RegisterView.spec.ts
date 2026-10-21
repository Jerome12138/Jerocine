import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import RegisterView from './RegisterView.vue'
import { useUserStore } from '@/stores/user'
import {
  getRegisterConfig,
  sendRegisterCode,
  registerAccount,
  applyRegister,
  applyStatus
} from '@/api/register'

vi.mock('@/api/register', () => ({
  getRegisterConfig: vi.fn(),
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

  it('邮件通道未就绪 → 只显示「暂不支持注册」(两种模式都如此, 不暴露技术细节、不渲染表单)', async () => {
    for (const mode of ['open', 'apply'] as const) {
      vi.mocked(getRegisterConfig).mockResolvedValue({ mode, smtpReady: false })
      await router.push('/register')
      await router.isReady()
      const w = mount(RegisterView, { global: { plugins: [router] } })
      await new Promise((r) => setTimeout(r, 10))
      await w.vm.$nextTick()
      expect(w.text(), `mode=${mode}`).toContain('暂不支持注册')
      expect(w.text(), `mode=${mode}`).not.toContain('邮件通道')
      expect(w.find('form').exists(), `mode=${mode}`).toBe(false)
      w.unmount()
    }
  })
})

describe('RegisterView 申请注册(apply)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(sendRegisterCode).mockReset()
    vi.mocked(applyRegister).mockReset()
    vi.mocked(applyStatus).mockReset()
  })

  it('apply → 渲染申请表单, 刻意无密码框', async () => {
    const w = await mountRegister('apply')
    expect(w.find('form').exists()).toBe(true)
    expect(w.find('input[autocomplete="new-password"]').exists()).toBe(false)
    expect(w.find('textarea').exists()).toBe(true)
    expect(w.text()).toContain('提交申请')
  })

  it('apply: 空表单提交 → 依次提示用户名/邮箱/验证码(无密码项)', async () => {
    const w = await mountRegister('apply')
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).toContain('请输入用户名')

    await w.find('input[autocomplete="username"]').setValue('jerry')
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).toContain('请输入邮箱')

    await w.find('input[autocomplete="email"]').setValue('a@b.com')
    await w.find('form').trigger('submit.prevent')
    await w.vm.$nextTick()
    expect(w.text()).toContain('请输入验证码')
    expect(applyRegister).not.toHaveBeenCalled()
  })

  it('apply: 提交成功 → 审核中状态页(三条预期) + payload 无 password + 自动查进度', async () => {
    vi.mocked(applyRegister).mockResolvedValue({ applied: true })
    vi.mocked(applyStatus).mockResolvedValue({ status: 'pending', reason: '', canResend: false })

    const w = await mountRegister('apply')
    await w.find('input[autocomplete="username"]').setValue('jerry')
    await w.find('input[autocomplete="email"]').setValue('a@b.com')
    await w.find('input[autocomplete="one-time-code"]').setValue('654321')
    await w.find('textarea').setValue('朋友邀请来的')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r, 10))

    expect(applyRegister).toHaveBeenCalledWith({
      userName: 'jerry',
      email: 'a@b.com',
      reason: '朋友邀请来的',
      code: '654321'
    })
    // 审核中状态页: 三条预期管理(含垃圾箱/数天内/回站查进度)
    expect(w.text()).toContain('申请已提交')
    expect(w.text()).toContain('垃圾箱')
    expect(w.text()).toContain('数天')
    // 进度面板预填邮箱自动查一次
    expect(applyStatus).toHaveBeenCalledWith('a@b.com')
    await w.vm.$nextTick()
    expect(w.text()).toContain('审核中')
  })

  it('apply: 同邮箱在途 → 403 原文(审核中提示), 停留表单', async () => {
    vi.mocked(applyRegister).mockRejectedValue(
      new Error('注册申请审核中，请等待邮件通知')
    )

    const w = await mountRegister('apply')
    await w.find('input[autocomplete="username"]').setValue('jerry')
    await w.find('input[autocomplete="email"]').setValue('a@b.com')
    await w.find('input[autocomplete="one-time-code"]').setValue('654321')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r, 10))

    expect(w.text()).toContain('注册申请审核中，请等待邮件通知')
    expect(w.find('form').exists()).toBe(true)
  })
})

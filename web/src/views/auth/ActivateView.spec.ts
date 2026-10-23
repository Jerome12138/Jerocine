import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import ActivateView from './ActivateView.vue'
import { useUserStore } from '@/stores/user'
import { activateAccount, checkUserName } from '@/api/register'
import { ApiError } from '@/types/api'

vi.mock('@/api/register', () => ({
  getRegisterConfig: vi.fn(),
  sendRegisterCode: vi.fn(),
  registerAccount: vi.fn(),
  applyRegister: vi.fn(),
  applyStatus: vi.fn(),
  resendActivation: vi.fn(),
  activateAccount: vi.fn(),
  checkUserName: vi.fn()
}))

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/activate', component: ActivateView },
    { path: '/login', component: { template: '<div />' } },
    { path: '/index', component: { template: '<div />' } }
  ]
})

function stubUserStore() {
  const userStore = useUserStore()
  vi.spyOn(userStore, 'setToken').mockImplementation(() => {})
  vi.spyOn(userStore, 'fetchInfo').mockResolvedValue({} as never)
  return userStore
}

async function mountActivate(query: Record<string, string> = { token: 'tok-abc' }) {
  await router.push({ path: '/activate', query })
  await router.isReady()
  const w = mount(ActivateView, { global: { plugins: [router] } })
  await w.vm.$nextTick()
  return w
}

async function fillAndSubmit(w: Awaited<ReturnType<typeof mountActivate>>) {
  await w.find('input[autocomplete="username"]').setValue('jerry')
  const pwds = w.findAll('input[type="password"]')
  await pwds[0].setValue('123456')
  await pwds[1].setValue('123456')
  await w.find('form').trigger('submit.prevent')
  await new Promise((r) => setTimeout(r, 10))
}

describe('ActivateView 激活页', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.mocked(activateAccount).mockReset()
    vi.mocked(checkUserName).mockReset()
  })

  it('缺 token → 提示从邮件链接进入, 不出表单', async () => {
    const w = await mountActivate({})
    expect(w.text()).toContain('激活链接无效或已不完整')
    expect(w.find('form').exists()).toBe(false)
  })

  it('两次密码不一致 → 拦截, 不调激活', async () => {
    const w = await mountActivate()
    await w.find('input[autocomplete="username"]').setValue('jerry')
    const pwds = w.findAll('input[type="password"]')
    await pwds[0].setValue('123456')
    await pwds[1].setValue('654321')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('两次输入的密码不一致')
    expect(activateAccount).not.toHaveBeenCalled()
  })

  it('激活成功 → 带 token/userName/password 调端点, setToken 后进首页', async () => {
    vi.mocked(activateAccount).mockResolvedValue({
      userName: 'jerry',
      token: 'tok-xyz',
      expires: 999,
      role: 0
    } as never)
    const userStore = stubUserStore()

    const w = await mountActivate()
    await fillAndSubmit(w)

    expect(activateAccount).toHaveBeenCalledWith({
      token: 'tok-abc',
      userName: 'jerry',
      password: '123456'
    })
    expect(userStore.setToken).toHaveBeenCalledWith('tok-xyz', 999)
    expect(router.currentRoute.value.path).toBe('/index')
  })

  it('409 用户名窗口期被占 → 提示改名重试(高亮), token 未被消费', async () => {
    vi.mocked(activateAccount).mockRejectedValue(
      new ApiError(409, { message: '用户名已被占用' })
    )
    stubUserStore()

    const w = await mountActivate()
    await fillAndSubmit(w)

    expect(w.text()).toContain('该用户名已被占用')
    expect(w.text()).toContain('激活链接仍然有效')
  })

  it('409 邮箱被 open 注册者占用 → 走管理员处理文案, 不误导改名', async () => {
    vi.mocked(activateAccount).mockRejectedValue(
      new ApiError(409, { message: '该邮箱已被注册' })
    )
    stubUserStore()

    const w = await mountActivate()
    await fillAndSubmit(w)

    expect(w.text()).toContain('该邮箱已被注册，请联系管理员处理')
    expect(w.text()).not.toContain('激活链接仍然有效')
  })

  it('token 过期(400) → 展示错误原文, 提供回登录页入口', async () => {
    vi.mocked(activateAccount).mockRejectedValue(new ApiError(400, { message: '激活链接已过期或已被使用' }))
    stubUserStore()

    const w = await mountActivate()
    await fillAndSubmit(w)

    expect(w.text()).toContain('激活链接已过期或已被使用')
    expect(w.text()).not.toContain('该用户名已被占用')
  })

  // ---- 用户名回填 + 失焦查重(与注册页同款) ----

  it('进页默认回填上次注册/申请填报的用户名(jc-last-username)', async () => {
    localStorage.setItem('jc-last-username', 'jerry-from-register')

    const w = await mountActivate()

    const input = w.find('input[autocomplete="username"]').element as HTMLInputElement
    expect(input.value).toBe('jerry-from-register')
  })

  it('无落盘用户名 → 用户名框为空', async () => {
    const w = await mountActivate()

    const input = w.find('input[autocomplete="username"]').element as HTMLInputElement
    expect(input.value).toBe('')
  })

  it('用户名失焦查重: 可用 → 绿色提示', async () => {
    vi.mocked(checkUserName).mockResolvedValue({ available: true })

    const w = await mountActivate()
    const input = w.find('input[autocomplete="username"]')
    await input.setValue('jerry')
    await input.trigger('blur')
    await new Promise((r) => setTimeout(r, 0))

    expect(checkUserName).toHaveBeenCalledWith('jerry')
    expect(w.text()).toContain('该用户名可用')
  })

  it('用户名失焦查重: 占用 → 红色提示', async () => {
    vi.mocked(checkUserName).mockResolvedValue({ available: false })

    const w = await mountActivate()
    const input = w.find('input[autocomplete="username"]')
    await input.setValue('jerry')
    await input.trigger('blur')
    await new Promise((r) => setTimeout(r, 0))

    expect(w.text()).toContain('该用户名已被占用，换一个试试')
  })

  it('查重后再次编辑 → 旧结论作废(提示消失)', async () => {
    vi.mocked(checkUserName).mockResolvedValue({ available: true })

    const w = await mountActivate()
    const input = w.find('input[autocomplete="username"]')
    await input.setValue('jerry')
    await input.trigger('blur')
    await new Promise((r) => setTimeout(r, 0))
    expect(w.text()).toContain('该用户名可用')

    await input.setValue('jerry2')
    await w.vm.$nextTick()
    expect(w.text()).not.toContain('该用户名可用')
  })

  it('查重接口异常 → 静默回 idle, 不打扰填写', async () => {
    vi.mocked(checkUserName).mockRejectedValue(new Error('network down'))

    const w = await mountActivate()
    const input = w.find('input[autocomplete="username"]')
    await input.setValue('jerry')
    await input.trigger('blur')
    await new Promise((r) => setTimeout(r, 0))

    expect(w.text()).not.toContain('正在检查用户名')
    expect(w.text()).not.toContain('该用户名可用')
  })
})

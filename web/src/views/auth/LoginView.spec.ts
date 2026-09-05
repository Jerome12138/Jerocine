import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import LoginView from './LoginView.vue'
import { useUserStore } from '@/stores/user'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/login', component: LoginView },
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
})

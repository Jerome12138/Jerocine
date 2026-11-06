import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseFormMessage from './BaseFormMessage.vue'

/**
 * BaseFormMessage(§C8#4) — 表单错误行原子: role="alert" 单点化。
 * 视觉 = 原 auth 五处手写段逐字收敛(text-xs + danger 色 + 居中);
 * message 空 = 不渲染(原 v-if="errorMsg" 语义)。
 */
describe('BaseFormMessage', () => {
  it('有 message: 渲染 p[role=alert], text-xs + danger + 居中类, 文案透传', () => {
    const w = mount(BaseFormMessage, { props: { message: '用户名或密码错误' } })
    const p = w.find('p')
    expect(p.exists()).toBe(true)
    expect(p.attributes('role')).toBe('alert')
    expect(p.text()).toBe('用户名或密码错误')
    expect(p.classes()).toContain('text-xs')
    expect(p.classes()).toContain('text-center')
    expect(p.classes()).toContain('text-[var(--jc-danger)]')
  })

  it('message 空串: 不渲染(原 v-if=errorMsg 语义)', () => {
    const w = mount(BaseFormMessage, { props: { message: '' } })
    expect(w.find('p').exists()).toBe(false)
  })

  it('message 缺省: 不渲染', () => {
    const w = mount(BaseFormMessage)
    expect(w.find('p').exists()).toBe(false)
  })

  it('class 透传合并: 宿主可追加类(TV 分支 jc-tv-login__err 原款由宿主类覆盖)', () => {
    const w = mount(BaseFormMessage, { props: { message: 'x' }, attrs: { class: 'jc-tv-login__err' } })
    expect(w.find('p').classes()).toContain('jc-tv-login__err')
    // 基础类仍在(TV scoped 规则以更高特异性接管, 见 LoginView TV 分支核对记录)
    expect(w.find('p').classes()).toContain('text-xs')
  })
})

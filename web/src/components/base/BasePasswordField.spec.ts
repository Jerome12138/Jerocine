import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BasePasswordField from './BasePasswordField.vue'

/**
 * W1b 密码输入框 — 锁图标 + 密码 input + 可见性眼睛(type 切换):
 * - 可见性归页面(v-model:visible): 注册/激活/重置页的确认密码框跟随同一 showPwd;
 * - 外壳(jc-tv-input / ManageFormField / 页面容器)由调用方保留;
 * - 其余 attrs(placeholder/autocomplete/maxlength/@keydown/@change/@focus/tabindex)透传 input;
 * - variant tv = LoginView TV 分支原样(jc-tv-login__wrap/__ic/__ic-r + jc-tv-input);
 *   variant desktop(默认) = 4 页桌面同款胶囊(uno 工具类)。
 */
describe('BasePasswordField', () => {
  const baseProps = { modelValue: '', visible: false }

  it('desktop(默认): password input + data-focusable + 眼睛按钮(aria-label 契约), attrs 透传', () => {
    const w = mount(BasePasswordField, {
      props: { ...baseProps, placeholder: '密码', autocomplete: 'current-password' }
    })
    const input = w.find('input')
    expect(input.attributes('type')).toBe('password')
    expect(input.attributes('placeholder')).toBe('密码')
    expect(input.attributes('autocomplete')).toBe('current-password')
    expect(input.attributes('data-focusable')).toBe('true')
    expect(w.find('button[aria-label="切换密码可见性"]').exists()).toBe(true)
  })

  it('visible=true → input type=text(眼睛切换显隐)', () => {
    const w = mount(BasePasswordField, { props: { ...baseProps, visible: true } })
    expect(w.find('input').attributes('type')).toBe('text')
  })

  it('点眼睛 → emit update:visible(翻转), 且 input 值不受影响', async () => {
    const w = mount(BasePasswordField, { props: { ...baseProps, modelValue: 'abc', visible: false } })
    await w.find('button[aria-label="切换密码可见性"]').trigger('click')
    expect(w.emitted('update:visible')![0]).toEqual([true])
    expect((w.find('input').element as HTMLInputElement).value).toBe('abc')
  })

  it('输入 → emit update:modelValue', async () => {
    const w = mount(BasePasswordField, { props: { ...baseProps } })
    await w.find('input').setValue('secret')
    expect(w.emitted('update:modelValue')![0]).toEqual(['secret'])
  })

  it('maxlength 经 attrs 透传', () => {
    const w = mount(BasePasswordField, { props: { ...baseProps, maxlength: '64' } })
    expect(w.find('input').attributes('maxlength')).toBe('64')
  })

  it('tv variant: wrap=jc-tv-login__wrap / input=jc-tv-input jc-tv-login__input / 眼睛=jc-tv-login__ic-r 且 tabindex=0', () => {
    const w = mount(BasePasswordField, {
      props: { ...baseProps, variant: 'tv', tabindex: '0' }
    })
    expect(w.find('.jc-tv-login__wrap').exists()).toBe(true)
    expect(w.find('input.jc-tv-input.jc-tv-login__input').exists()).toBe(true)
    const eye = w.find('button.jc-tv-login__ic-r')
    expect(eye.exists()).toBe(true)
    expect(eye.attributes('tabindex')).toBe('0')
    expect(eye.attributes('data-focusable')).toBe('true')
  })

  it('desktop variant: 胶囊 input(pr-10 留眼睛位) + 眼睛绝对定位右、无 tabindex', () => {
    const w = mount(BasePasswordField, { props: { ...baseProps } })
    const input = w.find('input')
    expect(input.classes()).toContain('rounded-[var(--jc-radius-full)]')
    expect(input.classes()).toContain('pr-[var(--jc-space-10)]')
    const eye = w.find('button')
    expect(eye.classes()).toContain('absolute')
    expect(eye.classes()).toContain('right-[var(--jc-space-3)]')
    expect(eye.attributes('tabindex')).toBeUndefined()
  })

  it('暴露内部 input 元素(登录页 autofill 兜底 syncAutofill 读取)', () => {
    const w = mount(BasePasswordField, { props: { ...baseProps } })
    const exposed = w.vm as unknown as { input: HTMLInputElement | null }
    expect(exposed.input).toBeInstanceOf(HTMLInputElement)
  })
})

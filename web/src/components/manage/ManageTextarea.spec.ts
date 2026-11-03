import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageTextarea from './ManageTextarea.vue'

describe('ManageTextarea', () => {
  it('显示 modelValue', () => {
    const w = mount(ManageTextarea, { props: { modelValue: 'hello' } })
    expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe('hello')
  })

  it('rows 默认 4', () => {
    const w = mount(ManageTextarea, { props: { modelValue: '' } })
    expect(w.find('textarea').attributes('rows')).toBe('4')
  })

  it('rows 自定义', () => {
    const w = mount(ManageTextarea, { props: { modelValue: '', rows: 8 } })
    expect(w.find('textarea').attributes('rows')).toBe('8')
  })

  it('input 事件 emit update:modelValue', async () => {
    const w = mount(ManageTextarea, { props: { modelValue: '' } })
    await w.find('textarea').setValue('multi\nline')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['multi\nline'])
  })

  it('class 含 w-full 和 控件高度令牌 (min-h calc)', () => {
    const w = mount(ManageTextarea, { props: { modelValue: '' } })
    const cls = w.find('textarea').classes()
    expect(cls).toContain('w-full')
    expect(cls).toContain('min-h-[calc(var(--jc-ctrl-h)*2.4)]')
  })

  it('maxlength 透传原生属性 (复核 #11)', () => {
    const w = mount(ManageTextarea, { props: { modelValue: '', maxlength: 255 } })
    expect(w.find('textarea').attributes('maxlength')).toBe('255')
  })

  it('maxlength 接受字符串值', () => {
    const w = mount(ManageTextarea, { props: { modelValue: '', maxlength: '500' } })
    expect(w.find('textarea').attributes('maxlength')).toBe('500')
  })

  it('disabled → 原生属性 + 置灰类', () => {
    const w = mount(ManageTextarea, { props: { modelValue: '', disabled: true } })
    expect(w.find('textarea').attributes('disabled')).toBeDefined()
    expect(w.find('textarea').classes()).toContain('disabled:opacity-50')
  })

  it('disabled 默认不渲染属性', () => {
    const w = mount(ManageTextarea, { props: { modelValue: '' } })
    expect(w.find('textarea').attributes('disabled')).toBeUndefined()
  })
})

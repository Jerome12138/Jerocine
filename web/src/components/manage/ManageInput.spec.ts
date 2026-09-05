import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageInput from './ManageInput.vue'

describe('ManageInput', () => {
  it('显示 modelValue', () => {
    const w = mount(ManageInput, { props: { modelValue: 'hello' } })
    expect((w.find('input').element as HTMLInputElement).value).toBe('hello')
  })

  it('placeholder 透传', () => {
    const w = mount(ManageInput, { props: { modelValue: '', placeholder: '请输入' } })
    expect(w.find('input').attributes('placeholder')).toBe('请输入')
  })

  it('type 默认 text', () => {
    const w = mount(ManageInput, { props: { modelValue: '' } })
    expect(w.find('input').attributes('type')).toBe('text')
  })

  it('type=password 透传', () => {
    const w = mount(ManageInput, { props: { modelValue: '', type: 'password' } })
    expect(w.find('input').attributes('type')).toBe('password')
  })

  it('disabled 禁用', () => {
    const w = mount(ManageInput, { props: { modelValue: '', disabled: true } })
    expect(w.find('input').attributes('disabled')).toBeDefined()
  })

  it('input 事件 emit update:modelValue', async () => {
    const w = mount(ManageInput, { props: { modelValue: '' } })
    await w.find('input').setValue('abc')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['abc'])
  })

  it('type=number 时输入数字 emit number 值', async () => {
    const w = mount(ManageInput, { props: { modelValue: 0, type: 'number' } })
    await w.find('input').setValue('42')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([42])
  })

  it('type=number 空串 emit 空串 (不强转 0)', async () => {
    const w = mount(ManageInput, { props: { modelValue: 1, type: 'number' } })
    await w.find('input').setValue('')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([''])
  })

  it('class 含 w-full 和 min-h-[44px] (P4 mobile)', () => {
    const w = mount(ManageInput, { props: { modelValue: '' } })
    const cls = w.find('input').classes()
    expect(cls).toContain('w-full')
    expect(cls).toContain('min-h-[44px]')
    expect(cls).toContain('md:min-h-[36px]')
  })
})

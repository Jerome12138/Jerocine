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

  it('class 含 w-full 和 min-h-[88px] (P4 mobile)', () => {
    const w = mount(ManageTextarea, { props: { modelValue: '' } })
    const cls = w.find('textarea').classes()
    expect(cls).toContain('w-full')
    expect(cls).toContain('min-h-[88px]')
  })
})

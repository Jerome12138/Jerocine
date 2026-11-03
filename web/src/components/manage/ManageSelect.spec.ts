import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageSelect from './ManageSelect.vue'

const OPTIONS = [
  { label: 'JSON', value: 0 },
  { label: 'XML', value: 1 },
  { label: '混合', value: 2 }
]

describe('ManageSelect 统一下拉', () => {
  it('验收1: options 3 项 → 渲染 3 个 option', () => {
    const w = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS, ariaLabel: '返回类型' } })
    expect(w.findAll('option')).toHaveLength(3)
  })

  it('验收2: modelValue=2 → value=2 的 option selected', () => {
    const w = mount(ManageSelect, { props: { modelValue: 2, options: OPTIONS, ariaLabel: '返回类型' } })
    const sel = w.find('select').element as HTMLSelectElement
    expect(sel.value).toBe('2')
    expect(w.findAll('option')[2]!.element.selected).toBe(true)
  })

  it('验收3: 选择数字 option → emit 原类型 number 而非字符串', async () => {
    const w = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS, ariaLabel: '返回类型' } })
    await w.find('select').setValue('1')
    expect(w.emitted('update:modelValue')![0]).toEqual([1])
  })

  it('验收4: disabled → select.disabled=true 且无 data-focusable', () => {
    const w = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS, disabled: true } })
    const sel = w.find('select')
    expect((sel.element as HTMLSelectElement).disabled).toBe(true)
    expect(sel.attributes('data-focusable')).toBeUndefined()
  })

  it('验收5: enabled 时恒有 data-focusable="true"', () => {
    const w = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS } })
    expect(w.find('select').attributes('data-focusable')).toBe('true')
  })

  it('验收6: modelValue=undefined + placeholder → disabled hidden option 且被选中', () => {
    const w = mount(ManageSelect, {
      props: { modelValue: undefined, options: OPTIONS, placeholder: '请选择' }
    })
    const ph = w.findAll('option')[0]!
    expect(ph.text()).toBe('请选择')
    expect(ph.attributes('disabled')).toBeDefined()
    expect(ph.attributes('hidden')).toBeDefined()
    expect(ph.element.selected).toBe(true)
    expect(w.findAll('option')).toHaveLength(4)
  })

  it('验收7: ariaLabel → select 的 aria-label 透传', () => {
    const w = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS, ariaLabel: '角色' } })
    expect(w.find('select').attributes('aria-label')).toBe('角色')
  })

  it('字符串 option: emit 字符串原类型', async () => {
    const w = mount(ManageSelect, {
      props: { modelValue: '', options: [{ label: '全部', value: '' }, { label: '失败', value: 'failed' }] }
    })
    await w.find('select').setValue('failed')
    expect(w.emitted('update:modelValue')![0]).toEqual(['failed'])
  })

  it('class 与 ManageInput 同族: bg-elevated + 控件高度令牌', () => {
    const w = mount(ManageSelect, { props: { modelValue: 0, options: OPTIONS } })
    const cls = w.find('select').classes()
    expect(cls).toContain('bg-elevated')
    expect(cls).toContain('min-h-[var(--jc-ctrl-h)]')
  })
})

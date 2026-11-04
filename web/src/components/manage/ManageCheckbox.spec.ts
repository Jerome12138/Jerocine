import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ManageCheckbox from './ManageCheckbox.vue'

/**
 * ManageCheckbox(规格 §3.4 / D③)用例: checked/indeterminate/disabled 渲染、
 * change emit(受控回写)、data-focusable、label/slot 文案。
 */

function input(w: ReturnType<typeof mount>) {
  return w.find('input[type="checkbox"]')
}

describe('ManageCheckbox', () => {
  it('modelValue=false → input.checked=false, 不渲染白勾/横杠', () => {
    const w = mount(ManageCheckbox, { props: { modelValue: false } })
    expect((input(w).element as HTMLInputElement).checked).toBe(false)
    expect(w.find('svg').exists()).toBe(false)
    expect(input(w).attributes('data-focusable')).toBe('true')
  })

  it('modelValue=true → input.checked=true, 渲染白勾(polyline)', () => {
    const w = mount(ManageCheckbox, { props: { modelValue: true } })
    expect((input(w).element as HTMLInputElement).checked).toBe(true)
    expect(w.find('svg polyline').exists()).toBe(true)
  })

  it('indeterminate=true → DOM indeterminate 置真 + 白横杠, checked 仍随 modelValue', () => {
    const w = mount(ManageCheckbox, { props: { modelValue: false, indeterminate: true } })
    const el = input(w).element as HTMLInputElement
    expect(el.indeterminate).toBe(true)
    expect(el.checked).toBe(false)
    expect(w.find('.jc-checkbox__bar').exists()).toBe(true)
  })

  it('checked 与 indeterminate 双真 → indeterminate 优先, 只渲染横杠不渲染白勾(取舍固化)', () => {
    // 现有调用点(ManageCascader.isPartialChecked)不会双真; 该组合规格未定义视觉契约,
    // 以组件实现为准固化: v-if indeterminate 分支优先, 横杠覆盖白勾
    const w = mount(ManageCheckbox, { props: { modelValue: true, indeterminate: true } })
    const el = input(w).element as HTMLInputElement
    expect(el.checked).toBe(true)
    expect(el.indeterminate).toBe(true)
    expect(w.find('.jc-checkbox__bar').exists()).toBe(true)
    expect(w.find('svg polyline').exists()).toBe(false)
  })

  it('indeterminate 翻 false → DOM indeterminate 复位(手控直写, 不被 Vue 重置)', async () => {
    const w = mount(ManageCheckbox, { props: { modelValue: false, indeterminate: true } })
    await w.setProps({ indeterminate: false })
    expect((input(w).element as HTMLInputElement).indeterminate).toBe(false)
  })

  it('change 时 emit update:modelValue=新值(setValue true/false 双向)', async () => {
    const w = mount(ManageCheckbox, { props: { modelValue: false } })
    await input(w).setValue(true)
    expect(w.emitted('update:modelValue')?.[0]).toEqual([true])

    await w.setProps({ modelValue: true })
    await input(w).setValue(false)
    expect(w.emitted('update:modelValue')?.[1]).toEqual([false])
  })

  it('受控回写: 父级未回写 modelValue 时 DOM 勾选态还原为 props 值', async () => {
    const w = mount(ManageCheckbox, { props: { modelValue: true } })
    await input(w).setValue(false) // 模拟"父级不更新"的受控场景
    expect(w.emitted('update:modelValue')?.[0]).toEqual([false])
    expect((input(w).element as HTMLInputElement).checked).toBe(true) // 还原
  })

  it('disabled: input 禁用 + 整体半透明样式, click 不 emit', async () => {
    const w = mount(ManageCheckbox, { props: { modelValue: false, disabled: true } })
    expect((input(w).element as HTMLInputElement).disabled).toBe(true)
    expect(w.find('label').classes()).toContain('opacity-50')
    await w.find('label').trigger('click')
    expect(w.emitted('update:modelValue')).toBeUndefined()
  })

  it('label prop 渲染兜底文案; 复杂内容走默认 slot', () => {
    const w = mount(ManageCheckbox, { props: { modelValue: false, label: '自动采集' } })
    expect(w.text()).toContain('自动采集')

    const w2 = mount(ManageCheckbox, { props: { modelValue: false }, slots: { default: '<b>哔哩哔哩</b>' } })
    expect(w2.text()).toContain('哔哩哔哩')
  })
})

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import ManageFormField from './ManageFormField.vue'

describe('ManageFormField', () => {
  it('显示 label', () => {
    const w = mount(ManageFormField, { props: { label: '用户名' } })
    expect(w.text()).toContain('用户名')
  })

  it('required=true 显示星号', () => {
    const w = mount(ManageFormField, { props: { label: 'X', required: true } })
    expect(w.find('span > span').text()).toContain('*')
  })

  it('required=false 不显示星号', () => {
    const w = mount(ManageFormField, { props: { label: 'X' } })
    expect(w.text()).not.toContain('*')
  })

  it('error 优先于 hint 显示', () => {
    const w = mount(ManageFormField, {
      props: { label: 'X', error: '错了', hint: '提示' }
    })
    expect(w.text()).toContain('错了')
    expect(w.text()).not.toContain('提示')
  })

  it('hint 在无 error 时显示', () => {
    const w = mount(ManageFormField, { props: { label: 'X', hint: '提示' } })
    expect(w.text()).toContain('提示')
  })

  it('默认 slot 渲染 input', () => {
    const w = mount(ManageFormField, {
      props: { label: 'X' },
      slots: { default: () => h('input', { class: 'my-input' }) }
    })
    expect(w.find('.my-input').exists()).toBe(true)
  })
})

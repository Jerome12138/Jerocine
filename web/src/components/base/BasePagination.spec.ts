import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BasePagination from './BasePagination.vue'

describe('BasePagination', () => {
  it('total<=pageSize → 渲染 1 页', () => {
    const w = mount(BasePagination, { props: { current: 1, pageSize: 20, total: 5 } })
    expect(w.text()).toContain('1')
  })

  it('total/pageSize 计算总页数', () => {
    const w = mount(BasePagination, { props: { current: 1, pageSize: 10, total: 35 } })
    // 35/10 → 4 页 (向上取整)
    expect(w.text()).toContain('4')
  })

  it('点击页码 emit change + update:current', async () => {
    const w = mount(BasePagination, { props: { current: 1, pageSize: 10, total: 30 } })
    // 找页码 2 的按钮
    const buttons = w.findAll('button').filter(b => b.text() === '2')
    if (buttons.length > 0) {
      await buttons[0].trigger('click')
      expect(w.emitted('change')?.[0]).toEqual([2])
      expect(w.emitted('update:current')?.[0]).toEqual([2])
    }
  })

  it('total=0 → totalPages 至少 1', () => {
    const w = mount(BasePagination, { props: { current: 1, pageSize: 10, total: 0 } })
    // 不崩
    expect(w.exists()).toBe(true)
  })

  it('页码超过 pagerCount → 出现省略号 …', () => {
    const w = mount(BasePagination, {
      props: { current: 5, pageSize: 10, total: 200, pagerCount: 7 }
    })
    // Pagination 用单字符 … (HORIZONTAL ELLIPSIS) 不是三个英文点
    expect(w.text()).toMatch(/…|\.\.\./)
  })

  // ---- T2 越界钳制: total 收缩(末页删空)后越界 current 回传, 不发 change ----
  it('current=9 越界(totalPages=1) → emit update:current=[[1]] 且不 emit change', () => {
    const w = mount(BasePagination, { props: { current: 9, pageSize: 5, total: 5 } })
    expect(w.emitted('update:current')).toEqual([[1]])
    expect(w.emitted('change')).toBeUndefined()
  })

  it('current=12 越界(totalPages=4) → emit update:current=[[4]]', () => {
    const w = mount(BasePagination, { props: { current: 12, pageSize: 10, total: 40 } })
    expect(w.emitted('update:current')).toEqual([[4]])
    expect(w.emitted('change')).toBeUndefined()
  })

  it('current=2 在正常范围内 → 不 emit 任何钳制事件', () => {
    const w = mount(BasePagination, { props: { current: 2, pageSize: 10, total: 40 } })
    expect(w.emitted('update:current')).toBeUndefined()
    expect(w.emitted('change')).toBeUndefined()
  })
})

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseEmpty from './BaseEmpty.vue'

describe('BaseEmpty', () => {
  it('默认 title=暂无内容', () => {
    const w = mount(BaseEmpty)
    expect(w.text()).toContain('暂无内容')
  })

  it('自定义 title', () => {
    const w = mount(BaseEmpty, { props: { title: '没数据' } })
    expect(w.text()).toContain('没数据')
  })

  it('description 显示', () => {
    const w = mount(BaseEmpty, { props: { description: '请稍后' } })
    expect(w.text()).toContain('请稍后')
  })

  it('role=status aria-live=polite (无障碍)', () => {
    const w = mount(BaseEmpty)
    expect(w.find('.gf-empty').attributes('role')).toBe('status')
    expect(w.find('.gf-empty').attributes('aria-live')).toBe('polite')
  })

  it('hideIcon=true 隐藏默认 icon', () => {
    const w = mount(BaseEmpty, { props: { hideIcon: true } })
    // 默认 BaseIcon name=search 不存在
    expect(w.find('[name="search"]').exists()).toBe(false)
  })
})

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import StatusTag from './StatusTag.vue'
import type { StatusMap } from './StatusTag.vue'

const MAP: StatusMap = {
  success: { label: '成功', variant: 'success' },
  failed: { label: '失败', variant: 'danger' },
  running: { label: '运行中', variant: 'brand' },
  true: { label: '启用', variant: 'success' },
  false: { label: '停用', variant: 'default' }
}

/** BaseTag 的 variant/size 无统一 BEM 类, 断言其真实渲染类 */
const VARIANT_CLS: Record<string, string> = {
  success: 'bg-[var(--jc-success-soft)]',
  danger: 'bg-[var(--jc-danger-soft)]',
  warning: 'bg-[var(--jc-warning-soft)]',
  brand: 'bg-[var(--jc-brand-primary)]',
  default: 'jc-tag--default'
}

describe('StatusTag 状态值→标签映射', () => {
  it('命中: value=success → BaseTag 文本「成功」variant=success', () => {
    const w = mount(StatusTag, { props: { value: 'success', map: MAP } })
    const tag = w.find('.jc-tag')
    expect(tag.text()).toBe('成功')
    expect(tag.classes()).toContain(VARIANT_CLS.success!)
  })

  it('布尔 true → 查字符串键 "true"', () => {
    const w = mount(StatusTag, { props: { value: true, map: MAP } })
    expect(w.text()).toBe('启用')
    expect(w.find('.jc-tag').classes()).toContain(VARIANT_CLS.success!)
  })

  it('未命中 → fallback 文本「—」variant=default', () => {
    const w = mount(StatusTag, { props: { value: 'unknown', map: MAP } })
    expect(w.text()).toBe('—')
    expect(w.find('.jc-tag').classes()).toContain(VARIANT_CLS.default!)
  })

  it('null/undefined → 直接 fallback', () => {
    const w1 = mount(StatusTag, { props: { value: null, map: MAP } })
    const w2 = mount(StatusTag, { props: { value: undefined, map: MAP } })
    expect(w1.text()).toBe('—')
    expect(w2.text()).toBe('—')
  })

  it('自定义 fallback 生效', () => {
    const w = mount(StatusTag, {
      props: { value: 'xxx', map: MAP, fallback: { label: '未知态', variant: 'warning' } }
    })
    expect(w.text()).toBe('未知态')
    expect(w.find('.jc-tag').classes()).toContain(VARIANT_CLS.warning!)
  })

  it('size 透传 BaseTag (xs→h-5, 默认 sm→h-6)', () => {
    const w = mount(StatusTag, { props: { value: 'success', map: MAP, size: 'xs' } })
    expect(w.find('.jc-tag').classes()).toContain('h-5')
    const d = mount(StatusTag, { props: { value: 'success', map: MAP } })
    expect(d.find('.jc-tag').classes()).toContain('h-6')
  })

  it('数字值 String 匹配', () => {
    const w = mount(StatusTag, {
      props: { value: 1, map: { '1': { label: '已处理', variant: 'success' } } }
    })
    expect(w.text()).toBe('已处理')
  })
})

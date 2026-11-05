import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import CardCornerRemove from './CardCornerRemove.vue'

/**
 * W2c 卡角删除钮(FavoritesView:174-181 z-2 / HistoryView:327-334 z-4 收编):
 * 海报右上角圆形 ✕ 小钮; 差异(aria-label 文案 / z 层级)经 props 承载,
 * 点击上抛原生事件(由调用方 preventDefault/stopPropagation + 移除)。
 */
describe('CardCornerRemove', () => {
  it('按钮结构与公共类逐字保留(24px 圆形半透明底 + close 图标 + 触达热区)', () => {
    const w = mount(CardCornerRemove, {
      props: { label: '取消收藏 X', zClass: 'z-2' }
    })
    const btn = w.find('button')
    expect(btn.attributes('type')).toBe('button')
    expect(btn.attributes('aria-label')).toBe('取消收藏 X')
    expect(btn.classes()).toEqual([
      'jc-hotarea', // 2026-11-05 三档统一: 视觉保持 24px(恰为移动标准), 命中面积靠热区补
      'absolute',
      'top-[var(--jc-space-2)]',
      'right-[var(--jc-space-2)]',
      'w-[24px]',
      'h-[24px]',
      'rounded-full',
      'bg-[rgba(0,0,0,0.6)]',
      'hover:bg-[rgba(0,0,0,0.85)]',
      'flex-center',
      'text-white',
      'transition-colors',
      'z-2'
    ])
    expect(w.find('.jc-icon').exists()).toBe(true)
  })

  it('zClass=z-4(历史页形态)与自定义 aria-label', () => {
    const w = mount(CardCornerRemove, {
      props: { label: '从历史中移除 Y', zClass: 'z-4' }
    })
    expect(w.find('button').classes().includes('z-4')).toBe(true)
    expect(w.find('button').attributes('aria-label')).toBe('从历史中移除 Y')
  })

  it('点击上抛原生 click 事件', async () => {
    const w = mount(CardCornerRemove, {
      props: { label: 'X', zClass: 'z-2' }
    })
    await w.find('button').trigger('click')
    expect(w.emitted('click')).toHaveLength(1)
    expect((w.emitted('click') as unknown[][])[0][0]).toBeInstanceOf(Event)
  })
})

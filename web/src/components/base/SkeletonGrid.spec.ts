import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SkeletonGrid from './SkeletonGrid.vue'

/**
 * W1c 骨架网格 — BaseSkeleton 的版式组合(props: count/layout, 内部仍用 BaseSkeleton):
 *   poster    = 桌面海报网格(jc-card-grid, theme.css 全局类)
 *   poster-tv = TV 网格(jc-tv-grid, tv-cards.css 全局类)
 *   items     = 裸条目 fragment(无 wrapper, 作页面既有网格容器的直接子元素)
 *   lines     = 单根 BaseSkeleton(count) 纵向行(Search 列表形态)
 * height/ratio/width/shape 原样透传 BaseSkeleton(调用点保持原几何, 不新增视觉)。
 */
describe('SkeletonGrid', () => {
  it('layout=poster(默认): jc-card-grid 包裹 + count 个骨架条目', () => {
    const w = mount(SkeletonGrid, { props: { count: 12, height: 'auto', ratio: '3/4' } })
    expect(w.find('.jc-card-grid').exists()).toBe(true)
    expect(w.findAll('.jc-skeleton').length).toBe(12)
  })

  it('count 默认 12', () => {
    const w = mount(SkeletonGrid)
    expect(w.findAll('.jc-skeleton').length).toBe(12)
  })

  it('layout=poster-tv: jc-tv-grid 包裹', () => {
    const w = mount(SkeletonGrid, {
      props: { count: 6, layout: 'poster-tv', height: '280px', ratio: '3/4' }
    })
    expect(w.find('.jc-tv-grid').exists()).toBe(true)
    expect(w.findAll('.jc-skeleton').length).toBe(6)
    expect(w.find('.jc-card-grid').exists()).toBe(false)
  })

  it('layout=items: 无网格 wrapper(条目=逐个 BaseSkeleton, 与原 v-for DOM 同构)', () => {
    const w = mount(SkeletonGrid, {
      props: { count: 7, layout: 'items', shape: 'rect', ratio: '3/4', width: '100%' }
    })
    expect(w.find('.jc-card-grid').exists()).toBe(false)
    expect(w.find('.jc-tv-grid').exists()).toBe(false)
    // 与原实现一致: N 个独立 BaseSkeleton(各带 1 个 jc-skeleton-wrap), 作页面网格直接子元素
    expect(w.findAll('.jc-skeleton-wrap').length).toBe(7)
    expect(w.findAll('.jc-skeleton').length).toBe(7)
  })

  it('layout=lines: 单根 BaseSkeleton(count=N) — Search 列表形态', () => {
    const w = mount(SkeletonGrid, { props: { count: 5, layout: 'lines', height: '180px' } })
    expect(w.findAll('.jc-skeleton-wrap').length).toBe(1)
    expect(w.findAll('.jc-skeleton').length).toBe(5)
  })

  it('几何透传: ratio=3/4 → padding-bottom 133.3333%(padding hack, chrome61 兼容路径不变)', () => {
    const w = mount(SkeletonGrid, {
      props: { count: 1, layout: 'lines', shape: 'rect', ratio: '3/4', width: '100%' }
    })
    const style = w.find('.jc-skeleton').attributes('style') ?? ''
    expect(style).toContain('133.3333%')
  })

  it('几何透传: height 原样落到条目 style', () => {
    const w = mount(SkeletonGrid, { props: { count: 2, layout: 'lines', height: '180px' } })
    const style = w.find('.jc-skeleton').attributes('style') ?? ''
    expect(style).toContain('180px')
  })
})

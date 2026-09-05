import { describe, it, expect } from 'vitest'
import type { RouteLocationNormalized } from 'vue-router'
import { scrollBehavior } from './scrollBehavior'

/**
 * 回归: 之前 scrollBehavior 用 savedPosition 还原, TV 上从详情 back 回首页
 * 滚动停在旧位置, 用户要求永远 {top:0} (含 back), 仅 hash 锚点除外.
 */
describe('router scrollBehavior', () => {
  const dummy = (to: Partial<RouteLocationNormalized>): RouteLocationNormalized =>
    ({ hash: '', path: '/', ...to } as RouteLocationNormalized)

  it('普通切页 → 回顶 (即使有 savedPosition 也不还原)', () => {
    const r = scrollBehavior(dummy({ path: '/index' }), dummy({ path: '/' }), { left: 0, top: 500 })
    expect(r).toEqual({ top: 0 })
  })

  it('back 回上一页 → 仍然回顶 (savedPosition 被忽略)', () => {
    const r = scrollBehavior(
      dummy({ path: '/filmDetail' }),
      dummy({ path: '/play' }),
      { left: 0, top: 1200 }
    )
    expect(r).toEqual({ top: 0 })
  })

  it('带 hash 锚点 → 平滑滚到目标元素', () => {
    const r = scrollBehavior(dummy({ hash: '#section-3', path: '/index' }), dummy({}), null)
    expect(r).toEqual({ el: '#section-3', behavior: 'smooth' })
  })
})

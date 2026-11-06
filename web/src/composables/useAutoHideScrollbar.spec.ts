import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { effectScope } from 'vue'
import { useAutoHideScrollbar } from './useAutoHideScrollbar'

/**
 * useAutoHideScrollbar：滚动中给容器加 `.is-scrolling`，停止 delay 后移除。
 * 只做 class 加减（显隐由 CSS 负责），故这里断言 class 与定时器行为。
 */
function makeEl(): HTMLElement {
  return document.createElement('div')
}

function scrollEventOn(el: HTMLElement): Event {
  const e = new Event('scroll')
  Object.defineProperty(e, 'currentTarget', { value: el, configurable: true })
  return e
}

describe('useAutoHideScrollbar', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('滚动时加 is-scrolling，delay 后移除', () => {
    const scope = effectScope()
    const { onScroll } = scope.run(() => useAutoHideScrollbar(700))!
    const el = makeEl()

    onScroll(scrollEventOn(el))
    expect(el.classList.contains('is-scrolling')).toBe(true)

    vi.advanceTimersByTime(699)
    expect(el.classList.contains('is-scrolling')).toBe(true)

    vi.advanceTimersByTime(1)
    expect(el.classList.contains('is-scrolling')).toBe(false)
    scope.stop()
  })

  it('连续滚动只保留最后一次计时（不会中途提前收起）', () => {
    const scope = effectScope()
    const { onScroll } = scope.run(() => useAutoHideScrollbar(700))!
    const el = makeEl()

    onScroll(scrollEventOn(el))
    vi.advanceTimersByTime(500)
    onScroll(scrollEventOn(el)) // 重新计时
    vi.advanceTimersByTime(500)
    expect(el.classList.contains('is-scrolling')).toBe(true)

    vi.advanceTimersByTime(200)
    expect(el.classList.contains('is-scrolling')).toBe(false)
    scope.stop()
  })

  it('无 currentTarget 时静默返回（不抛错）', () => {
    const scope = effectScope()
    const { onScroll } = scope.run(() => useAutoHideScrollbar())!
    expect(() => onScroll(new Event('scroll'))).not.toThrow()
    scope.stop()
  })

  it('卸载时清理未触发的定时器', () => {
    const scope = effectScope()
    const { onScroll } = scope.run(() => useAutoHideScrollbar(700))!
    const el = makeEl()
    onScroll(scrollEventOn(el))
    scope.stop()
    // 卸载后推进时间，不应再操作节点（class 保持进入卸载时的状态由 CSS 兜底）
    expect(() => vi.advanceTimersByTime(1000)).not.toThrow()
  })
})

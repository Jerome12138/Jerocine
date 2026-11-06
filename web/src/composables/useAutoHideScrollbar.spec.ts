import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { effectScope } from 'vue'
import { useAutoHideScrollbar } from './useAutoHideScrollbar'

/**
 * useAutoHideScrollbar：
 *   1. 滚动中给容器加 `.is-scrolling`，停止 delay 后移除（显隐交给 CSS）；
 *   2. 按实际溢出维护 `.is-scrollable`（供 CSS 决定是否阻断滚动链 —— 无可滚时不能上 contain，
 *      否则滚轮会被吃掉）。
 * 只做 class 加减，故这里断言 class 与定时器行为。
 */
function makeEl(overflow = false): HTMLElement {
  const el = document.createElement('div')
  Object.defineProperty(el, 'scrollHeight', { value: overflow ? 200 : 100, configurable: true })
  Object.defineProperty(el, 'clientHeight', { value: 100, configurable: true })
  Object.defineProperty(el, 'scrollWidth', { value: 100, configurable: true })
  Object.defineProperty(el, 'clientWidth', { value: 100, configurable: true })
  return el
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
    const { onScroll } = scope.run(() => useAutoHideScrollbar(undefined, 700))!
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
    const { onScroll } = scope.run(() => useAutoHideScrollbar(undefined, 700))!
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

  it('sync() 按实际溢出维护 is-scrollable（可滚加、不可滚去）', () => {
    const scope = effectScope()
    const overflow = makeEl(true)
    const plain = makeEl(false)
    let target: HTMLElement = overflow
    const { sync } = scope.run(() => useAutoHideScrollbar(() => target))!

    sync()
    expect(overflow.classList.contains('is-scrollable')).toBe(true)

    target = plain
    sync()
    expect(plain.classList.contains('is-scrollable')).toBe(false)
    scope.stop()
  })

  it('无可滚内容时滚动不会误加 is-scrollable（contain 才不会吃掉滚轮）', () => {
    const scope = effectScope()
    const el = makeEl(false)
    const { onScroll } = scope.run(() => useAutoHideScrollbar(() => el))!

    onScroll(scrollEventOn(el))
    expect(el.classList.contains('is-scrolling')).toBe(true)
    expect(el.classList.contains('is-scrollable')).toBe(false)
    scope.stop()
  })

  it('无 currentTarget 时静默返回（不抛错）', () => {
    const scope = effectScope()
    const { onScroll } = scope.run(() => useAutoHideScrollbar(() => null))!
    expect(() => onScroll(new Event('scroll'))).not.toThrow()
    scope.stop()
  })

  it('卸载时清理未触发的定时器', () => {
    const scope = effectScope()
    const { onScroll } = scope.run(() => useAutoHideScrollbar(() => null, 700))!
    const el = makeEl()
    onScroll(scrollEventOn(el))
    scope.stop()
    expect(() => vi.advanceTimersByTime(1000)).not.toThrow()
  })
})

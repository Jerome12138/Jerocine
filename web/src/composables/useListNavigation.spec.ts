import { describe, it, expect, vi } from 'vitest'
import { useListNavigation } from './useListNavigation'

/**
 * useListNavigation(规格 §3.2)用例清单:
 *   循环移动(末→首/首→末) / Enter 确认带索引 / Esc 关闭 / Home / End /
 *   空列表不动作 / open() 重置为 initial() / 已处理方向键 preventDefault。
 */
function key(keyStr: string): KeyboardEvent {
  return new KeyboardEvent('keydown', { key: keyStr, cancelable: true })
}

describe('useListNavigation', () => {
  it('ArrowDown/ArrowUp 循环移动: 首→末、末→首', () => {
    const nav = useListNavigation({ itemCount: () => 3, onConfirm: vi.fn(), onClose: vi.fn() })
    expect(nav.activeIndex.value).toBe(0)
    nav.onKeydown(key('ArrowUp')) // 首→末(循环)
    expect(nav.activeIndex.value).toBe(2)
    nav.onKeydown(key('ArrowDown')) // 末→首(循环)
    expect(nav.activeIndex.value).toBe(0)
    nav.onKeydown(key('ArrowDown'))
    expect(nav.activeIndex.value).toBe(1)
  })

  it('Enter / Space 确认回调带当前 activeIndex', () => {
    const onConfirm = vi.fn()
    const nav = useListNavigation({ itemCount: () => 3, onConfirm, onClose: vi.fn() })
    nav.onKeydown(key('ArrowDown')) // active=1
    nav.onKeydown(key('Enter'))
    expect(onConfirm).toHaveBeenCalledTimes(1)
    expect(onConfirm).toHaveBeenCalledWith(1)
    nav.onKeydown(key(' '))
    expect(onConfirm).toHaveBeenCalledTimes(2)
    expect(onConfirm).toHaveBeenLastCalledWith(1)
  })

  it('Esc 触发关闭回调', () => {
    const onClose = vi.fn()
    const nav = useListNavigation({ itemCount: () => 3, onConfirm: vi.fn(), onClose })
    nav.onKeydown(key('Escape'))
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(nav.activeIndex.value).toBe(0) // 关闭不改 active
  })

  it('Home 跳首 / End 跳末', () => {
    const nav = useListNavigation({ itemCount: () => 3, onConfirm: vi.fn(), onClose: vi.fn() })
    nav.onKeydown(key('End'))
    expect(nav.activeIndex.value).toBe(2)
    nav.onKeydown(key('Home'))
    expect(nav.activeIndex.value).toBe(0)
  })

  it('itemCount=0: 方向键/Home/End/Enter/Space 不动作; Esc 仍可关闭(空浮层也要能退出)', () => {
    const onConfirm = vi.fn()
    const onClose = vi.fn()
    const nav = useListNavigation({ itemCount: () => 0, onConfirm, onClose })
    nav.onKeydown(key('ArrowDown'))
    nav.onKeydown(key('ArrowUp'))
    nav.onKeydown(key('Home'))
    nav.onKeydown(key('End'))
    expect(nav.activeIndex.value).toBe(0)
    nav.onKeydown(key('Enter'))
    nav.onKeydown(key(' '))
    expect(onConfirm).not.toHaveBeenCalled()
    nav.onKeydown(key('Escape'))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('open() 重置 activeIndex = initial(); 未传 initial 时回 0', () => {
    const nav = useListNavigation({
      itemCount: () => 4,
      initial: () => 2,
      onConfirm: vi.fn(),
      onClose: vi.fn()
    })
    nav.onKeydown(key('ArrowDown')) // 未 open 前从 0 起步 → 1
    expect(nav.activeIndex.value).toBe(1)
    nav.open()
    expect(nav.activeIndex.value).toBe(2) // 重置为 initial()

    const nav2 = useListNavigation({ itemCount: () => 4, onConfirm: vi.fn(), onClose: vi.fn() })
    nav2.open()
    expect(nav2.activeIndex.value).toBe(0) // 默认 0
  })

  it('已处理的方向键(ArrowUp/Down/Home/End) preventDefault; Enter/Esc 不拦截默认行为', () => {
    const nav = useListNavigation({ itemCount: () => 3, onConfirm: vi.fn(), onClose: vi.fn() })
    for (const k of ['ArrowDown', 'ArrowUp', 'Home', 'End']) {
      const e = key(k)
      nav.onKeydown(e)
      expect(e.defaultPrevented, k).toBe(true)
    }
    const enter = key('Enter')
    nav.onKeydown(enter)
    expect(enter.defaultPrevented).toBe(false)
    const esc = key('Escape')
    nav.onKeydown(esc)
    expect(esc.defaultPrevented).toBe(false)
  })
})

import { describe, it, expect, beforeEach } from 'vitest'
import { acquireOverlay, releaseOverlay, overlayCount, isTopOverlay } from './useOverlayStack'

function clearBodyFlags(): void {
  document.body.removeAttribute('data-jc-modal-open')
  document.body.style.overflow = ''
}

describe('useOverlayStack', () => {
  beforeEach(() => clearBodyFlags())

  it('acquire×2 → count=2 且 body 加锁; release 首个 → 锁仍在; release 第二个 → 锁清除', () => {
    const s1 = acquireOverlay()
    const s2 = acquireOverlay()

    expect(overlayCount()).toBe(2)
    expect(document.body.getAttribute('data-jc-modal-open')).toBe('1')
    expect(document.body.style.overflow).toBe('hidden')

    releaseOverlay(s1)
    expect(overlayCount()).toBe(1)
    expect(document.body.getAttribute('data-jc-modal-open')).toBe('1')
    expect(document.body.style.overflow).toBe('hidden')

    releaseOverlay(s2)
    expect(overlayCount()).toBe(0)
    expect(document.body.hasAttribute('data-jc-modal-open')).toBe(false)
    expect(document.body.style.overflow).toBe('')
  })

  it('isTopOverlay: 后申请者 true / 先申请者 false; 顶层释放后前者回升为 true', () => {
    const s1 = acquireOverlay()
    const s2 = acquireOverlay()
    expect(isTopOverlay(s2)).toBe(true)
    expect(isTopOverlay(s1)).toBe(false)

    releaseOverlay(s2)
    expect(isTopOverlay(s1)).toBe(true)
    releaseOverlay(s1)
  })

  it('乱序 release: 先放底层再放顶层, 计数与归零清理仍正确', () => {
    const s1 = acquireOverlay()
    const s2 = acquireOverlay()

    releaseOverlay(s1) // 先放底层
    expect(overlayCount()).toBe(1)
    expect(document.body.getAttribute('data-jc-modal-open')).toBe('1')
    expect(document.body.style.overflow).toBe('hidden')

    releaseOverlay(s2)
    expect(overlayCount()).toBe(0)
    expect(document.body.hasAttribute('data-jc-modal-open')).toBe(false)
    expect(document.body.style.overflow).toBe('')
  })

  it('已锁时重复 acquire 不重设锁; 未持有的 seq release 为幂等 no-op', () => {
    const s1 = acquireOverlay()
    // 人为破坏标志: 若实现重复 acquire 时无条件重设, 会被覆盖回 '1' 而暴露
    document.body.setAttribute('data-jc-modal-open', '0')
    const s2 = acquireOverlay()
    expect(document.body.getAttribute('data-jc-modal-open')).toBe('0')
    document.body.setAttribute('data-jc-modal-open', '1')

    releaseOverlay(s2)
    releaseOverlay(s1)
    releaseOverlay(s1) // 幂等: 再放已释放的 seq 不影响计数与清理
    expect(overlayCount()).toBe(0)
    expect(document.body.hasAttribute('data-jc-modal-open')).toBe(false)
    expect(document.body.style.overflow).toBe('')
  })
})

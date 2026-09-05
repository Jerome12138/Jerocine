import { describe, it, expect, afterEach } from 'vitest'
import { DPAD_KEY_MAP, normalizeDpadKey, installDpadBridge } from './dpad'

describe('DPAD_KEY_MAP', () => {
  it('方向键 19/20/21/22 映射到 Arrow*', () => {
    expect(DPAD_KEY_MAP[19]).toBe('ArrowUp')
    expect(DPAD_KEY_MAP[20]).toBe('ArrowDown')
    expect(DPAD_KEY_MAP[21]).toBe('ArrowLeft')
    expect(DPAD_KEY_MAP[22]).toBe('ArrowRight')
  })

  it('DPAD center 23 + Enter 66 → Enter', () => {
    expect(DPAD_KEY_MAP[23]).toBe('Enter')
    expect(DPAD_KEY_MAP[66]).toBe('Enter')
  })

  it('Back 4 → Escape', () => expect(DPAD_KEY_MAP[4]).toBe('Escape'))

  it('Media 键映射', () => {
    expect(DPAD_KEY_MAP[85]).toBe('MediaPlayPause')
    expect(DPAD_KEY_MAP[87]).toBe('MediaTrackNext')
    expect(DPAD_KEY_MAP[88]).toBe('MediaTrackPrevious')
  })

  it('未知 keyCode 返回 undefined', () => {
    expect(DPAD_KEY_MAP[999]).toBeUndefined()
  })
})

describe('normalizeDpadKey', () => {
  it('keyCode 在 map 内 → 取 map (优先于 e.key)', () => {
    const e = new KeyboardEvent('keydown', { keyCode: 21, key: 'foo' })
    expect(normalizeDpadKey(e)).toBe('ArrowLeft')
  })

  it('keyCode 不在 map 内 → 回退 e.key', () => {
    const e = new KeyboardEvent('keydown', { key: 'a' })
    expect(normalizeDpadKey(e)).toBe('a')
  })

  // 回归：keyCode 与 D-pad/媒体键撞码的字母键，必须按 e.key 原样返回，不得被改写
  it.each([
    ['r', 82], // KEYCODE_MENU
    ['b', 66], // KEYCODE_ENTER
    ['u', 85], // MediaPlayPause
    ['w', 87], // MediaTrackNext
    ['x', 88], // MediaTrackPrevious
    ['y', 89], // MediaRewind
    ['z', 90] // MediaFastForward
  ])('物理键盘字母 "%s"(keyCode %i 撞 D-pad 码) 原样返回', (ch, code) => {
    const e = new KeyboardEvent('keydown', { key: ch, keyCode: code })
    expect(normalizeDpadKey(e)).toBe(ch)
  })
})

describe('installDpadBridge', () => {
  let uninstall: (() => void) | null = null
  afterEach(() => {
    uninstall?.()
    uninstall = null
  })

  // 回归：物理键盘按下与 D-pad 撞码的字母键，桥接不得 preventDefault（否则吞字符）
  it.each([
    ['r', 82],
    ['b', 66],
    ['u', 85],
    ['w', 87],
    ['x', 88],
    ['y', 89],
    ['z', 90]
  ])('字母 "%s"(keyCode %i) 不被 preventDefault', (ch, code) => {
    uninstall = installDpadBridge()
    const e = new KeyboardEvent('keydown', {
      key: ch,
      keyCode: code,
      bubbles: true,
      cancelable: true
    })
    document.dispatchEvent(e)
    expect(e.defaultPrevented).toBe(false)
  })

  it('真实 D-pad keyCode（无 key 字段）仍被桥接 preventDefault', () => {
    uninstall = installDpadBridge()
    const e = new KeyboardEvent('keydown', { keyCode: 21, bubbles: true, cancelable: true })
    document.dispatchEvent(e)
    expect(e.defaultPrevented).toBe(true)
  })
})

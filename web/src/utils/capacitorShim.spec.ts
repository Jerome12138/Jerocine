import { describe, it, expect, vi } from 'vitest'
import { installCapacitorShim } from './capacitorShim'

/**
 * Capacitor 兜底垫片单测。
 *
 * 背景：APK 远程加载 https://jerocine.art 时, Capacitor 的 native-bridge.js
 * 不会注入 window.Capacitor。但壳(Capacitor Bridge / Cordova mock)在 App
 * pause/resume 生命周期会 evaluateJavascript("window.Capacitor.triggerEvent('pause','document')"),
 * 远程页无此对象 → Uncaught TypeError: Cannot read properties of undefined (reading 'triggerEvent')
 * → 被 MainActivity.onConsoleMessage 抓出弹 Toast 打扰用户。
 *
 * 垫片职责：保证 window.Capacitor.triggerEvent 存在且为 no-op, 且绝不覆盖真实 Capacitor。
 */

type WinLike = {
  Capacitor?: { triggerEvent?: (...a: unknown[]) => unknown; [k: string]: unknown }
}

describe('installCapacitorShim', () => {
  it('window.Capacitor 不存在时, 创建带 no-op triggerEvent 的对象', () => {
    const win: WinLike = {}
    installCapacitorShim(win as unknown as Window & typeof globalThis)
    expect(win.Capacitor).toBeDefined()
    expect(typeof win.Capacitor?.triggerEvent).toBe('function')
  })

  it('triggerEvent("pause","document") 不抛错且返回 undefined', () => {
    const win: WinLike = {}
    installCapacitorShim(win as unknown as Window & typeof globalThis)
    expect(() => win.Capacitor?.triggerEvent?.('pause', 'document')).not.toThrow()
    expect(win.Capacitor?.triggerEvent?.('pause', 'document')).toBeUndefined()
  })

  it('已存在真实 Capacitor.triggerEvent 时绝不覆盖', () => {
    const real = vi.fn(() => 'real')
    const win: WinLike = { Capacitor: { triggerEvent: real, isNativePlatform: () => true } }
    installCapacitorShim(win as unknown as Window & typeof globalThis)
    expect(win.Capacitor?.triggerEvent).toBe(real)
    // 真实对象其它字段保持
    expect((win.Capacitor as Record<string, unknown>).isNativePlatform).toBeTypeOf('function')
  })

  it('Capacitor 存在但缺 triggerEvent 时, 仅补 triggerEvent 不动其它', () => {
    const win: WinLike = { Capacitor: { getPlatform: () => 'web' } }
    installCapacitorShim(win as unknown as Window & typeof globalThis)
    expect(typeof win.Capacitor?.triggerEvent).toBe('function')
    expect((win.Capacitor as Record<string, unknown>).getPlatform).toBeTypeOf('function')
  })

  it('重复调用幂等, 不报错', () => {
    const win: WinLike = {}
    installCapacitorShim(win as unknown as Window & typeof globalThis)
    const first = win.Capacitor?.triggerEvent
    expect(() => installCapacitorShim(win as unknown as Window & typeof globalThis)).not.toThrow()
    // 幂等: 第二次不替换已装的 no-op
    expect(win.Capacitor?.triggerEvent).toBe(first)
  })

  it('win 为 undefined(SSR) 时安全返回, 不抛', () => {
    expect(() => installCapacitorShim(undefined)).not.toThrow()
  })
})

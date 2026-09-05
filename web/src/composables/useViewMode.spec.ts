import { describe, it, expect, beforeEach, vi } from 'vitest'

describe('useViewMode 四档检测', () => {
  beforeEach(() => {
    localStorage.removeItem('gf-mode')
    document.documentElement.removeAttribute('data-mode')
    vi.resetModules()
  })

  it('视口 600px → mobile', async () => {
    Object.defineProperty(window, 'innerWidth', { value: 600, configurable: true })
    const m = await import('@/composables/useViewMode')
    m.installViewMode()
    const v = m.useViewMode()
    expect(v.mode.value).toBe('mobile')
    expect(v.isMobile.value).toBe(true)
    expect(v.isTablet.value).toBe(false)
    expect(v.isNarrow.value).toBe(true)
  })

  it('视口 900px → tablet', async () => {
    Object.defineProperty(window, 'innerWidth', { value: 900, configurable: true })
    const m = await import('@/composables/useViewMode')
    m.installViewMode()
    const v = m.useViewMode()
    expect(v.mode.value).toBe('tablet')
    expect(v.isTablet.value).toBe(true)
    expect(v.isNarrow.value).toBe(true)
  })

  it('视口 1280px → desktop', async () => {
    Object.defineProperty(window, 'innerWidth', { value: 1280, configurable: true })
    const m = await import('@/composables/useViewMode')
    m.installViewMode()
    const v = m.useViewMode()
    expect(v.mode.value).toBe('desktop')
    expect(v.isNarrow.value).toBe(false)
  })

  /**
   * Native APK 强制 TV — 回归之前的 bug: 用户在 drawer 里点过"切桌面模式",
   * localStorage 留下 gf-mode='desktop', 物理 TV 就锁死在 desktop 模式, 页面
   * 横向溢出. 修复后 detectCapacitorAndroid=true 时持久化被无视, 永远 'tv'.
   */
  it('JerocineNative 注入时强制 TV 模式 (忽略 persisted desktop)', async () => {
    localStorage.setItem('gf-mode', 'desktop')
    Object.defineProperty(window, 'innerWidth', { value: 960, configurable: true })
    ;(window as unknown as { JerocineNative: { invoke: () => string } }).JerocineNative = {
      invoke: () => '{"ok":true}'
    }
    const m = await import('@/composables/useViewMode')
    m.installViewMode()
    const v = m.useViewMode()
    expect(v.mode.value).toBe('tv')
    expect(v.isTV.value).toBe(true)
    // 清理 — 不污染后续测试
    delete (window as unknown as { JerocineNative?: unknown }).JerocineNative
  })

  /** URL ?mode=desktop 仍可在 native 壳里覆盖, 用于本地调试 */
  it('Native APK 下 URL ?mode=desktop 可临时覆盖 TV', async () => {
    ;(window as unknown as { JerocineNative: { invoke: () => string } }).JerocineNative = {
      invoke: () => '{"ok":true}'
    }
    const orig = window.location.search
    Object.defineProperty(window, 'location', {
      value: { ...window.location, search: '?mode=desktop' },
      configurable: true
    })
    const m = await import('@/composables/useViewMode')
    m.installViewMode()
    const v = m.useViewMode()
    expect(v.mode.value).toBe('desktop')
    delete (window as unknown as { JerocineNative?: unknown }).JerocineNative
    Object.defineProperty(window, 'location', {
      value: { ...window.location, search: orig },
      configurable: true
    })
  })
})

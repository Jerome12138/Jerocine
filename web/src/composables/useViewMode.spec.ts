import { describe, it, expect, beforeEach, vi } from 'vitest'

/** 原生壳桥 + 抽屉钩子的类型收窄(测试里频繁用) */
type ModeWin = { __jcSetMode?: (v: unknown) => boolean; JerocineNative?: unknown }

function withNative(): void {
  ;(window as unknown as ModeWin).JerocineNative = { invoke: () => '{"ok":true}' }
}
function clearNative(): void {
  delete (window as unknown as ModeWin).JerocineNative
}

describe('useViewMode 四档检测', () => {
  beforeEach(() => {
    localStorage.removeItem('jc-mode')
    localStorage.removeItem('jc-native-mode')
    delete (window as unknown as ModeWin).__jcSetMode
    delete (window as unknown as ModeWin).JerocineNative
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
   * localStorage 留下 jc-mode='desktop', 物理 TV 就锁死在 desktop 模式, 页面
   * 横向溢出. 修复后 detectCapacitorAndroid=true 时持久化被无视, 永远 'tv'.
   */
  it('JerocineNative 注入时强制 TV 模式 (忽略 persisted desktop)', async () => {
    localStorage.setItem('jc-mode', 'desktop')
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

/**
 * 原生壳「显示模式」—— 抽屉经 window.__jcSetMode 下发(方案 §6 / 抽屉重设计 §4.2)。
 *
 * 之前抽屉是"写 jc-mode + location.reload", 而壳内 detectMode 忽略 jc-mode ⇒ 三个
 * 状态切哪个都一样(空转)。现在走壳专用键 jc-native-mode, 且**不 reload**(mode 是
 * 响应式 ref, 即时重排), 于是"切了没反应"和"切完刷新又回去了"两个坑都要有测试兜住。
 */
describe('useViewMode 原生壳显示模式钩子', () => {
  beforeEach(() => {
    localStorage.removeItem('jc-mode')
    localStorage.removeItem('jc-native-mode')
    delete (window as unknown as ModeWin).__jcSetMode
    delete (window as unknown as ModeWin).JerocineNative
    document.documentElement.removeAttribute('data-mode')
    Object.defineProperty(window, 'innerWidth', { value: 960, configurable: true })
    vi.resetModules()
  })

  it('归一化: 只认 tv / desktop, 其余一律当"清除覆盖"', async () => {
    const m = await import('@/composables/useViewMode')
    expect(m.normalizeNativeMode('tv')).toBe('tv')
    expect(m.normalizeNativeMode('desktop')).toBe('desktop')
    expect(m.normalizeNativeMode('auto')).toBeNull()
    expect(m.normalizeNativeMode(null)).toBeNull()
    expect(m.normalizeNativeMode(undefined)).toBeNull()
    expect(m.normalizeNativeMode('')).toBeNull()
    expect(m.normalizeNativeMode('TV')).toBeNull()
    expect(m.normalizeNativeMode('mobile')).toBeNull()
  })

  it('原生壳装 window.__jcSetMode; 浏览器(无桥)不装', async () => {
    const web = await import('@/composables/useViewMode')
    web.installViewMode()
    expect((window as unknown as ModeWin).__jcSetMode).toBeUndefined()

    withNative()
    vi.resetModules()
    const native = await import('@/composables/useViewMode')
    native.installViewMode()
    expect(typeof (window as unknown as ModeWin).__jcSetMode).toBe('function')
  })

  it('切 desktop: 写壳专用键(不写 jc-mode)、即时改 data-mode、无需 reload', async () => {
    withNative()
    const m = await import('@/composables/useViewMode')
    m.installViewMode()
    const v = m.useViewMode()
    expect(v.mode.value).toBe('tv')

    const ok = (window as unknown as ModeWin).__jcSetMode?.('desktop')
    expect(ok).toBe(true)
    expect(v.mode.value).toBe('desktop')
    expect(v.isTV.value).toBe(false)
    expect(document.documentElement.getAttribute('data-mode')).toBe('desktop')
    expect(localStorage.getItem('jc-native-mode')).toBe('desktop')
    // 关键: 绝不写网页用的 jc-mode —— 那个键在壳内被忽略, 写了会造成"两套真相"
    expect(localStorage.getItem('jc-mode')).toBeNull()
  })

  it('切 auto / 脏值: 清掉覆盖并回到壳内默认 tv', async () => {
    withNative()
    localStorage.setItem('jc-native-mode', 'desktop')
    const m = await import('@/composables/useViewMode')
    m.installViewMode()
    const v = m.useViewMode()
    expect(v.mode.value).toBe('desktop')

    ;(window as unknown as ModeWin).__jcSetMode?.('auto')
    expect(v.mode.value).toBe('tv')
    expect(document.documentElement.getAttribute('data-mode')).toBe('tv')
    expect(localStorage.getItem('jc-native-mode')).toBeNull()

    // 前端自己也要做归一化, 不依赖 Java 一定传对
    ;(window as unknown as ModeWin).__jcSetMode?.('桌面')
    expect(localStorage.getItem('jc-native-mode')).toBeNull()
    expect(v.mode.value).toBe('tv')
  })

  it('切完 desktop 再"重载"(刷新页面 / 抽屉的刷新按钮): 仍保持 desktop', async () => {
    withNative()
    let m = await import('@/composables/useViewMode')
    m.installViewMode()
    ;(window as unknown as ModeWin).__jcSetMode?.('desktop')

    // 模拟整页 reload: 模块状态重置(installed/mode 归零), localStorage 保留
    vi.resetModules()
    m = await import('@/composables/useViewMode')
    m.installViewMode()
    expect(m.useViewMode().mode.value).toBe('desktop')
  })

  it('resize 不会把抽屉选的 desktop 自动改回 tv', async () => {
    withNative()
    const m = await import('@/composables/useViewMode')
    m.installViewMode()
    const v = m.useViewMode()
    ;(window as unknown as ModeWin).__jcSetMode?.('desktop')

    window.dispatchEvent(new Event('resize'))
    expect(v.mode.value).toBe('desktop')
  })

  it('壳内 setMode 也走壳专用通道(jc-mode 在壳内不生效, 不能"看起来切了其实没切")', async () => {
    withNative()
    const m = await import('@/composables/useViewMode')
    m.installViewMode()
    const v = m.useViewMode()

    v.setMode('desktop')
    expect(v.mode.value).toBe('desktop')
    expect(localStorage.getItem('jc-native-mode')).toBe('desktop')
    expect(localStorage.getItem('jc-mode')).toBeNull()

    v.setMode(null)
    expect(v.mode.value).toBe('tv')
    expect(localStorage.getItem('jc-native-mode')).toBeNull()
  })
})

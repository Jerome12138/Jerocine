import { describe, it, expect, vi, beforeEach } from 'vitest'

const openSettings = vi.fn()
let native = true
vi.mock('@/utils/jerocineNative', () => ({
  isNative: () => native,
  jerocine: { openSettings: () => openSettings() }
}))

const push = vi.fn(() => Promise.resolve())
vi.mock('vue-router', () => ({
  useRouter: () => ({ push })
}))

import { useTvSettingsEntry, shouldShowHeaderActions } from './useTvSettingsEntry'

describe('useTvSettingsEntry (TV 设置入口语义)', () => {
  beforeEach(() => {
    openSettings.mockClear()
    push.mockClear()
    native = true
  })

  it('原生壳: open() → bridge openSettings, 不跳路由', () => {
    const entry = useTvSettingsEntry()
    entry.open()
    expect(openSettings).toHaveBeenCalledTimes(1)
    expect(push).not.toHaveBeenCalled()
  })

  it('原生壳: guard() 拦截链接默认跳转并开抽屉', () => {
    const entry = useTvSettingsEntry()
    const e = { preventDefault: vi.fn() }
    entry.guard(e)
    expect(e.preventDefault).toHaveBeenCalledTimes(1)
    expect(openSettings).toHaveBeenCalledTimes(1)
    expect(entry.isIntercepted()).toBe(true)
  })

  it('纯网页 TV 模式: open() → SPA /settings(无桥可调)', () => {
    native = false
    const entry = useTvSettingsEntry()
    entry.open()
    expect(openSettings).not.toHaveBeenCalled()
    expect(push).toHaveBeenCalledWith('/settings')
    expect(entry.isIntercepted()).toBe(false)
  })

  it('纯网页 TV 模式: guard() 不拦截, 让 <a> 正常导航', () => {
    native = false
    const entry = useTvSettingsEntry()
    const e = { preventDefault: vi.fn() }
    entry.guard(e)
    expect(e.preventDefault).not.toHaveBeenCalled()
    expect(openSettings).not.toHaveBeenCalled()
  })

  it('无事件对象时 guard() 也不炸(原生壳)', () => {
    const entry = useTvSettingsEntry()
    expect(() => entry.guard()).not.toThrow()
    expect(openSettings).toHaveBeenCalledTimes(1)
  })
})

describe('shouldShowHeaderActions (顶栏「刷新/设置」显隐)', () => {
  it('TV 模式: 无论能不能开抽屉都显示 —— 纯网页 TV 模式也得有, 设置会退化成跳 /settings', () => {
    expect(shouldShowHeaderActions(true, false)).toBe(true)
    expect(shouldShowHeaderActions(true, true)).toBe(true)
  })

  it('非 TV 模式 + 壳内可调起设置: 显示 —— 否则用户切到桌面模式后没有入口切回 TV', () => {
    expect(shouldShowHeaderActions(false, true)).toBe(true)
  })

  it('非 TV 模式 + 无桥/调不动设置: 不显示 —— 纯网页桌面布局不该冒出原生设置入口', () => {
    expect(shouldShowHeaderActions(false, false)).toBe(false)
  })
})

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

import { useTvSettingsEntry } from './useTvSettingsEntry'

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

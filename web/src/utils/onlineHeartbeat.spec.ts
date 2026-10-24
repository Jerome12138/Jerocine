import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import type * as HbMod from './onlineHeartbeat'

function setHidden(hidden: boolean): void {
  Object.defineProperty(document, 'hidden', { value: hidden, configurable: true })
}

async function loadModule(): Promise<typeof HbMod> {
  vi.resetModules()
  return import('./onlineHeartbeat')
}

/** 取最后一次心跳 payload */
function lastBody(): { sid: string; watching: boolean; path: string; pv: boolean; webdriver: boolean } {
  const calls = (globalThis.fetch as unknown as { mock: { calls: unknown[][] } }).mock.calls
  const last = calls[calls.length - 1] as [string, RequestInit]
  return JSON.parse(String(last[1].body))
}

function fetchMock(): { mockClear: () => void } {
  return globalThis.fetch as unknown as { mockClear: () => void }
}

describe('onlineHeartbeat', () => {
  // 每个用例持有的模块实例(失败时也要在 afterEach 里停掉, 防止定时器泄漏到下一个用例)
  let m: typeof HbMod | undefined

  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({ ok: true })))
    localStorage.clear()
    setHidden(false)
    m = undefined
  })

  afterEach(() => {
    m?.stopOnlineHeartbeat()
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('启动即上报, 且首跳带 pv=true(首次载入=一次页面访问)', async () => {
    m = await loadModule()
    m.startOnlineHeartbeat()
    expect(globalThis.fetch).toHaveBeenCalledTimes(1)
    expect(lastBody().pv).toBe(true)
    expect(lastBody().path).toBe(window.location.pathname)
  })

  it('trackPageview 打点: 该跳 pv=true, 之后常规心跳 pv=false', async () => {
    m = await loadModule()
    m.startOnlineHeartbeat()
    fetchMock().mockClear()
    m.trackPageview()
    expect(lastBody().pv).toBe(true)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(lastBody().pv).toBe(false)
  })

  it('页面隐藏且非观看中: 不发心跳(后台标签页不算在线)', async () => {
    m = await loadModule()
    m.startOnlineHeartbeat()
    fetchMock().mockClear()
    setHidden(true)
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(120_000)
    expect(globalThis.fetch).not.toHaveBeenCalled()
  })

  // 回归: 安卓原生播放是独立 Activity 盖在 WebView 上, document.hidden 会变 true;
  // 旧版"隐藏即停表"导致播放期间一发心跳都没有("观看中"永远计不上)
  it('页面隐藏但观看中: 照常上报(原生播放, playerProgress 5s tick 续窗口)', async () => {
    m = await loadModule()
    m.startOnlineHeartbeat()
    fetchMock().mockClear()
    setHidden(true)
    document.dispatchEvent(new Event('visibilitychange'))
    // 原生播放: 每 5s 一次 playerProgress → touchNativeWatching 续 15s 窗口
    for (let i = 0; i < 6; i++) {
      await vi.advanceTimersByTimeAsync(5_000)
      m.touchNativeWatching()
    }
    expect(globalThis.fetch).toHaveBeenCalled()
    expect(lastBody().watching).toBe(true)
  })

  // web 浏览器口径: 切后台标签页但仍在播放 → 继续发心跳; 后台且非播放中 → 停表
  it('web 后台标签页仍在播放: 照常上报(watching=true); 暂停后停发', async () => {
    m = await loadModule()
    m.startOnlineHeartbeat()
    fetchMock().mockClear()
    m.setWebWatching(true) // video.js 播放中
    setHidden(true) // 切到后台标签页
    document.dispatchEvent(new Event('visibilitychange'))
    // 后台标签页定时器会被浏览器节流(最慢约 1/min), 推进 60s 覆盖一个节流周期
    await vi.advanceTimersByTimeAsync(60_000)
    expect(globalThis.fetch).toHaveBeenCalled()
    expect(lastBody().watching).toBe(true)
    // 暂停/切走播放页: 后台且非观看中 → 不再上报
    m.setWebWatching(false)
    fetchMock().mockClear()
    await vi.advanceTimersByTimeAsync(120_000)
    expect(globalThis.fetch).not.toHaveBeenCalled()
  })

  it('恢复可见立即续报', async () => {
    m = await loadModule()
    m.startOnlineHeartbeat()
    setHidden(true)
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(30_000)
    fetchMock().mockClear()
    setHidden(false)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(globalThis.fetch).toHaveBeenCalledTimes(1)
  })

  it('web 播放器(video.js)播放状态驱动 watching', async () => {
    m = await loadModule()
    m.startOnlineHeartbeat()
    fetchMock().mockClear()
    m.setWebWatching(true)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(lastBody().watching).toBe(true)
    m.setWebWatching(false)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(lastBody().watching).toBe(false)
  })

  it('native watching 15s 窗口: 过期后不算观看中', async () => {
    m = await loadModule()
    m.startOnlineHeartbeat()
    fetchMock().mockClear()
    m.touchNativeWatching()
    m.trackPageview() // 立即上报: 窗口内 watching=true
    expect(lastBody().watching).toBe(true)
    await vi.advanceTimersByTimeAsync(20_000) // 超过 15s 窗口
    m.trackPageview()
    expect(lastBody().watching).toBe(false)
  })
})

import { describe, it, expect, afterEach } from 'vitest'
import { isNative, canOpenSettings } from './jerocineNative'

/**
 * 壳判定的两个层级（2026-10-10 实测反馈新增 canOpenSettings）。
 *
 * 差异点：`isNative()` 只要求"桥在场"；`canOpenSettings()` 还要求"桥暴露了调用入口"。
 * 顶栏「设置」按钮在非 TV 模式下的显隐必须用后者 —— 否则在老 APK 上会给出一个
 * 点了没反应的假入口（见 useTvSettingsEntry.shouldShowHeaderActions）。
 */

type BridgeWindow = { JerocineNative?: unknown; JerocinePlayer?: unknown }
const w = window as unknown as BridgeWindow

afterEach(() => {
  delete w.JerocineNative
  delete w.JerocinePlayer
})

describe('isNative / canOpenSettings (原生壳判定)', () => {
  it('无桥(纯浏览器访问) ⇒ 都 false', () => {
    expect(isNative()).toBe(false)
    expect(canOpenSettings()).toBe(false)
  })

  it('现版 bridge(只有 invoke, openSettings 是 invoke 的一个 case) ⇒ 都 true', () => {
    w.JerocineNative = { invoke: () => '{"ok":true}' }
    expect(isNative()).toBe(true)
    expect(canOpenSettings()).toBe(true)
  })

  it('旧 v1 bridge(只有 playVideo, 没有 invoke) ⇒ isNative true 但 canOpenSettings false', () => {
    w.JerocineNative = { playVideo: () => {} }
    expect(isNative()).toBe(true)
    expect(canOpenSettings()).toBe(false)
  })

  it('bridge 显式暴露 openSettings 方法也认(若将来提成独立 @JavascriptInterface)', () => {
    w.JerocineNative = { invoke: () => '{"ok":true}', openSettings: () => {} }
    expect(canOpenSettings()).toBe(true)
  })

  it('旧全局名 JerocinePlayer 同样识别', () => {
    w.JerocinePlayer = { invoke: () => '{"ok":true}' }
    expect(isNative()).toBe(true)
    expect(canOpenSettings()).toBe(true)
  })
})

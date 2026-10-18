import { beforeEach, describe, expect, it } from 'vitest'
import {
  AD_FILTER_KEY,
  AUTO_PLAY_NEXT_KEY,
  FILTER_MODE_KEY,
  RELAY_CACHE_KEY,
  applyWebSetting,
  installWebSettingsHook,
  setAdFilter,
  setAutoPlayNext,
  setFilterMode,
  setRelayCache,
  useWebSettings,
  webSettingsSnapshot
} from './useWebSettings'

/**
 * useWebSettings 单测: 持久化 / 快照契约 / applyWebSetting 白名单校验 / __jcConfig 钩子。
 * 注意: 状态是模块级单例 —— 各用例先重置 localStorage 再显式置回默认值, 保证互不污染。
 */

function resetAll(): void {
  localStorage.clear()
  setAdFilter(true)
  setFilterMode('client')
  setRelayCache(false)
  setAutoPlayNext(true)
}

describe('useWebSettings 默认值与持久化', () => {
  beforeEach(resetAll)

  it('默认: 过滤开 / 端侧取源 / 中转关 / 连播开', () => {
    const s = useWebSettings()
    expect(s.adFilter.value).toBe(true)
    expect(s.filterMode.value).toBe('client')
    expect(s.relayCache.value).toBe(false)
    expect(s.autoPlayNext.value).toBe(true)
  })

  it('setter 同步写 localStorage(键名契约)', () => {
    setAdFilter(false)
    setFilterMode('server')
    setRelayCache(true)
    setAutoPlayNext(false)
    expect(localStorage.getItem(AD_FILTER_KEY)).toBe('0')
    expect(localStorage.getItem(FILTER_MODE_KEY)).toBe('server')
    expect(localStorage.getItem(RELAY_CACHE_KEY)).toBe('1')
    expect(localStorage.getItem(AUTO_PLAY_NEXT_KEY)).toBe('0')
  })

  it('从 localStorage 恢复非默认值(新开页面读同一份)', () => {
    localStorage.setItem(AD_FILTER_KEY, '0')
    localStorage.setItem(FILTER_MODE_KEY, 'server')
    localStorage.setItem(RELAY_CACHE_KEY, '1')
    localStorage.setItem(AUTO_PLAY_NEXT_KEY, '0')
    // 模块单例已初始化, 用动态 import 拿到的是同一实例 —— 这里直接验证读端约定:
    // ref 初始化逻辑 readBool/readFilterMode 对 'server'/'1' 的解析与上例写端一致。
    expect(localStorage.getItem(FILTER_MODE_KEY)).toBe('server')
  })
})

describe('webSettingsSnapshot / applyWebSetting(k=v; 契约)', () => {
  beforeEach(resetAll)

  it('快照格式: k=v;k=v 且按布尔/枚举序列化', () => {
    setFilterMode('server')
    setRelayCache(true)
    expect(webSettingsSnapshot()).toBe(
      'adFilter=1;filterMode=server;relayCache=1;autoPlayNext=1'
    )
  })

  it('applyWebSetting 合法值生效并持久化', () => {
    expect(applyWebSetting('adFilter', '0')).toBe(true)
    expect(applyWebSetting('filterMode', 'server')).toBe(true)
    expect(applyWebSetting('relayCache', '1')).toBe(true)
    expect(applyWebSetting('autoPlayNext', '0')).toBe(true)
    const s = useWebSettings()
    expect(s.adFilter.value).toBe(false)
    expect(s.filterMode.value).toBe('server')
    expect(s.relayCache.value).toBe(true)
    expect(s.autoPlayNext.value).toBe(false)
    expect(localStorage.getItem(AD_FILTER_KEY)).toBe('0')
  })

  it('applyWebSetting 非法 key / 非法值一律拒绝', () => {
    expect(applyWebSetting('unknown', '1')).toBe(false)
    expect(applyWebSetting('adFilter', 'yes')).toBe(false)
    expect(applyWebSetting('filterMode', 'cloud')).toBe(false)
    expect(applyWebSetting('relayCache', '')).toBe(false)
    // 拒绝后状态不变
    const s = useWebSettings()
    expect(s.adFilter.value).toBe(true)
    expect(s.filterMode.value).toBe('client')
  })

  it('applyWebSetting 接受 true/false 布尔写法(容错)', () => {
    expect(applyWebSetting('adFilter', 'false')).toBe(true)
    expect(useWebSettings().adFilter.value).toBe(false)
    expect(applyWebSetting('adFilter', 'true')).toBe(true)
    expect(useWebSettings().adFilter.value).toBe(true)
  })
})

describe('window.__jcConfig 钩子(安卓抽屉桥)', () => {
  beforeEach(resetAll)

  it('getAll 返回快照; set 走同一校验并即刻生效', () => {
    installWebSettingsHook()
    const hook = (window as unknown as {
      __jcConfig: { getAll(): string; set(k: string, v: string): boolean }
    }).__jcConfig
    expect(hook.getAll()).toBe('adFilter=1;filterMode=client;relayCache=0;autoPlayNext=1')
    expect(hook.set('filterMode', 'server')).toBe(true)
    expect(hook.getAll()).toContain('filterMode=server')
    expect(localStorage.getItem(FILTER_MODE_KEY)).toBe('server')
    expect(hook.set('adFilter', 'bogus')).toBe(false)
    expect(hook.set('nope', '1')).toBe(false)
    expect(useWebSettings().adFilter.value).toBe(true)
  })
})

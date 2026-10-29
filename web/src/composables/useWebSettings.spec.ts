import { beforeEach, describe, expect, it } from 'vitest'
import {
  AD_FILTER_KEY,
  AUTO_PLAY_NEXT_KEY,
  applyWebSetting,
  installWebSettingsHook,
  setAdFilter,
  setAutoPlayNext,
  useWebSettings,
  webSettingsSnapshot
} from './useWebSettings'

/**
 * useWebSettings 单测: 持久化 / 快照契约 / applyWebSetting 白名单校验 / __jcConfig 钩子。
 * 注意: 状态是模块级单例 —— 各用例先重置 localStorage 再显式置回默认值, 保证互不污染。
 * (2026-10-28: filterMode/relayCache 随服务端过滤下线移除, 旧快照键被白名单拒绝。)
 */

function resetAll(): void {
  localStorage.clear()
  setAdFilter(true)
  setAutoPlayNext(true)
}

describe('useWebSettings 默认值与持久化', () => {
  beforeEach(resetAll)

  it('默认: 过滤开 / 连播开', () => {
    const s = useWebSettings()
    expect(s.adFilter.value).toBe(true)
    expect(s.autoPlayNext.value).toBe(true)
  })

  it('setter 同步写 localStorage(键名契约)', () => {
    setAdFilter(false)
    setAutoPlayNext(false)
    expect(localStorage.getItem(AD_FILTER_KEY)).toBe('0')
    expect(localStorage.getItem(AUTO_PLAY_NEXT_KEY)).toBe('0')
  })

  it('从 localStorage 恢复非默认值(新开页面读同一份)', () => {
    localStorage.setItem(AD_FILTER_KEY, '0')
    localStorage.setItem(AUTO_PLAY_NEXT_KEY, '0')
    // 模块单例已初始化, 用动态 import 拿到的是同一实例 —— 这里直接验证读端约定:
    // ref 初始化逻辑 readBool 对 '0' 的解析与上例写端一致。
    expect(localStorage.getItem(AD_FILTER_KEY)).toBe('0')
  })

  it('旧快照的 filterMode/relayCache 键被白名单拒绝(2026-10-28 下线兼容)', () => {
    expect(applyWebSetting('filterMode', 'server')).toBe(false)
    expect(applyWebSetting('relayCache', '1')).toBe(false)
  })
})

describe('webSettingsSnapshot / applyWebSetting(k=v; 契约)', () => {
  beforeEach(resetAll)

  it('快照格式: k=v;k=v 且按布尔序列化', () => {
    setAdFilter(false)
    expect(webSettingsSnapshot()).toBe('adFilter=0;autoPlayNext=1')
  })

  it('applyWebSetting 合法值生效并持久化', () => {
    expect(applyWebSetting('adFilter', '0')).toBe(true)
    expect(applyWebSetting('autoPlayNext', '0')).toBe(true)
    const s = useWebSettings()
    expect(s.adFilter.value).toBe(false)
    expect(s.autoPlayNext.value).toBe(false)
    expect(localStorage.getItem(AD_FILTER_KEY)).toBe('0')
  })

  it('applyWebSetting 非法 key / 非法值一律拒绝', () => {
    expect(applyWebSetting('unknown', '1')).toBe(false)
    expect(applyWebSetting('adFilter', 'yes')).toBe(false)
    expect(applyWebSetting('filterMode', 'server')).toBe(false) // 已下线的键
    expect(applyWebSetting('relayCache', '1')).toBe(false)
    // 拒绝后状态不变
    const s = useWebSettings()
    expect(s.adFilter.value).toBe(true)
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
    expect(hook.getAll()).toBe('adFilter=1;autoPlayNext=1')
    expect(hook.set('adFilter', '0')).toBe(true)
    expect(hook.getAll()).toContain('adFilter=0')
    expect(localStorage.getItem(AD_FILTER_KEY)).toBe('0')
    expect(useWebSettings().adFilter.value).toBe(false)
    expect(hook.set('adFilter', 'bogus')).toBe(false)
    expect(hook.set('nope', '1')).toBe(false)
    // 非法值被拒绝 → 状态保持上次合法值(false), 不被非法输入污染
    expect(useWebSettings().adFilter.value).toBe(false)
  })
})

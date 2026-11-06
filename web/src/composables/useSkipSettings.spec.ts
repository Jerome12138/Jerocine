import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import * as skipApi from '@/api/skip'
import {
  useSkipSettings,
  DEFAULT_SKIP_KEY,
  normalizeSkipSec,
  flushSkipSaves,
  SKIP_SAVE_DEBOUNCE_MS
} from './useSkipSettings'

/** getToken 做成可变：默认匿名（既有用例口径不变），防抖回写用例里置为已登录 */
const auth = vi.hoisted(() => ({ token: '' }))
vi.mock('@/utils/token', () => ({ getToken: () => auth.token }))
vi.mock('@/api/skip', () => ({
  skipSave: vi.fn(() => Promise.resolve()),
  skipReset: vi.fn(() => Promise.resolve()),
  skipList: vi.fn(() => Promise.resolve([]))
}))

/**
 * useSkipSettings —— 2026-10-26 新增用户级全局默认层:
 * 取值回退链 = 单影片(jc-skip-{filmId}) ?? 用户级默认(jc-skip-default) ?? 内置(90/60)。
 * 单影片配置优先于全局默认; 全局默认仅本地(未登录即本地级)。
 */
describe('useSkipSettings', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  // 排干挂起的防抖定时器，避免跨用例残留（匿名态下发不出去，纯清理）
  afterEach(() => {
    flushSkipSaves()
  })

  it('save 回写账号防抖 2s：连改只发最后一次；reset 取消挂起；flush 立即发', () => {
    const saveMock = vi.mocked(skipApi.skipSave)
    saveMock.mockClear()
    auth.token = 't'
    vi.useFakeTimers()
    try {
      const s = useSkipSettings()
      s.save(1, { intro: 10, outro: 5, enabled: true })
      s.save(1, { intro: 20, outro: 5, enabled: true })
      s.save(1, { intro: 30, outro: 5, enabled: true })
      expect(saveMock).not.toHaveBeenCalled() // 防抖窗口内一次都不发
      vi.advanceTimersByTime(SKIP_SAVE_DEBOUNCE_MS)
      expect(saveMock).toHaveBeenCalledTimes(1) // 只发最后一次
      expect(saveMock).toHaveBeenLastCalledWith(1, 30, 5, true)

      // reset 必须取消挂起的回写，否则 2s 后会把刚删掉的配置又 PUT 回来
      s.save(2, { intro: 40, outro: 5, enabled: true })
      s.reset(2)
      vi.advanceTimersByTime(SKIP_SAVE_DEBOUNCE_MS)
      expect(saveMock).toHaveBeenCalledTimes(1)

      // flush 立即发（防抖窗口内离开页面不丢最后一次改动）
      s.save(3, { intro: 50, outro: 5, enabled: true })
      flushSkipSaves()
      expect(saveMock).toHaveBeenCalledTimes(2)
      expect(saveMock).toHaveBeenLastCalledWith(3, 50, 5, true)
    } finally {
      auth.token = ''
      vi.useRealTimers()
    }
  })

  it('normalizeSkipSec: 非负整数归一化（负数不得翻正 —— 「−10s」按钮回归防呆）', () => {
    // 回归：播放器设置浮窗的减号按钮传「当前值 − 10」进来，曾经的字符串归一化会把 -10 剥成 10（负数变正数）
    expect(normalizeSkipSec(-10)).toBe(0)
    expect(normalizeSkipSec(-5)).toBe(0)
    expect(normalizeSkipSec(0)).toBe(0)
    expect(normalizeSkipSec(90)).toBe(90)
    expect(normalizeSkipSec(12.7)).toBe(12)
    expect(normalizeSkipSec(Number.NaN)).toBe(0)
    expect(normalizeSkipSec(Number.POSITIVE_INFINITY)).toBe(0)
  })

  it('无任何配置 ⇒ 内置默认 90/60 且启用', () => {
    const s = useSkipSettings()
    expect(s.get(1)).toEqual({ intro: 90, outro: 60, enabled: true })
    expect(s.effective(1)).toEqual({ intro: 90, outro: 60 })
  })

  it('单影片配置优先于全局默认', () => {
    const s = useSkipSettings()
    s.saveDefault({ intro: 60, outro: 30, enabled: true })
    s.save(1, { intro: 10, outro: 5, enabled: true })
    expect(s.get(1)).toEqual({ intro: 10, outro: 5, enabled: true })
    // 未设置单影片的影片回退全局默认
    expect(s.get(2)).toEqual({ intro: 60, outro: 30, enabled: true })
    expect(s.effective(2)).toEqual({ intro: 60, outro: 30 })
  })

  it('全局默认 enabled=false ⇒ 未覆盖影片不跳, 单影片可单独开', () => {
    const s = useSkipSettings()
    s.saveDefault({ intro: 60, outro: 30, enabled: false })
    expect(s.effective(2)).toEqual({ intro: 0, outro: 0 })
    s.save(2, { intro: 60, outro: 30, enabled: true })
    expect(s.effective(2)).toEqual({ intro: 60, outro: 30 })
  })

  it('saveDefault/resetDefault 持久化与回退', () => {
    const s = useSkipSettings()
    expect(s.isDefaultGlobal()).toBe(true)
    s.saveDefault({ intro: 45, outro: 20, enabled: false })
    expect(s.isDefaultGlobal()).toBe(false)
    expect(s.getDefault()).toEqual({ intro: 45, outro: 20, enabled: false })
    s.resetDefault()
    expect(s.isDefaultGlobal()).toBe(true)
    expect(s.getDefault()).toEqual({ intro: 90, outro: 60, enabled: true })
  })

  it('reset(单影片) 后回退到全局默认而非内置', () => {
    const s = useSkipSettings()
    s.saveDefault({ intro: 50, outro: 25, enabled: true })
    s.save(1, { intro: 999, outro: 888, enabled: true })
    s.reset(1)
    expect(s.get(1)).toEqual({ intro: 50, outro: 25, enabled: true })
  })

  it('DEFAULT_SKIP_KEY 键名正确（与设置页/播放页共用契约）', () => {
    expect(DEFAULT_SKIP_KEY).toBe('jc-skip-default')
  })
})

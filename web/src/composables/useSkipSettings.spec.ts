import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useSkipSettings, DEFAULT_SKIP_KEY } from './useSkipSettings'

vi.mock('@/utils/token', () => ({ getToken: () => '' }))
vi.mock('@/api/skip', () => ({ skipSave: vi.fn(), skipReset: vi.fn(), skipList: vi.fn() }))

/**
 * useSkipSettings —— 2026-10-26 新增用户级全局默认层:
 * 取值回退链 = 单影片(jc-skip-{filmId}) ?? 用户级默认(jc-skip-default) ?? 内置(90/60)。
 * 单影片配置优先于全局默认; 全局默认仅本地(未登录即本地级)。
 */
describe('useSkipSettings', () => {
  beforeEach(() => {
    localStorage.clear()
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

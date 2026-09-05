import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from './user'

vi.mock('@/api/auth', () => ({
  login: vi.fn(),
  getUserInfo: vi.fn(),
  changePassword: vi.fn(),
  logout: vi.fn()
}))

import { login as apiLogin, getUserInfo as apiGetInfo } from '@/api/auth'

describe('useUserStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('未登录初始: isLoggedIn=false, isAdmin=false', () => {
    const u = useUserStore()
    expect(u.isLoggedIn).toBe(false)
    expect(u.isAdmin).toBe(false)
  })

  it('info.role=1 → isAdmin=true', () => {
    const u = useUserStore()
    u.info = { userName: 'admin', role: 1 } as never
    expect(u.isAdmin).toBe(true)
  })

  it('info.role=0 → isAdmin=false', () => {
    const u = useUserStore()
    u.info = { userName: 'jerry', role: 0 } as never
    expect(u.isAdmin).toBe(false)
  })

  it('displayName 优先 nickName > userName > username > 默认', () => {
    const u = useUserStore()
    u.info = { nickName: 'Zero', userName: 'admin', role: 1 } as never
    expect(u.displayName).toBe('Zero')
    u.info = { userName: 'admin', role: 1 } as never
    expect(u.displayName).toBe('admin')
    u.info = { role: 0 } as never
    expect(u.displayName).toBe('用户')
  })

  it('login 成功 → 写 token + 调 fetchInfo', async () => {
    ;(apiLogin as ReturnType<typeof vi.fn>).mockResolvedValue({
      userName: 'admin',
      token: 'tok-123',
      expires: 9999999999,
      role: 1
    })
    ;(apiGetInfo as ReturnType<typeof vi.fn>).mockResolvedValue({
      userName: 'admin',
      nickName: 'Zero',
      role: 1
    })
    const u = useUserStore()
    const info = await u.login({ username: 'admin', password: 'pwd' })
    expect(u.token).toBe('tok-123')
    expect(u.isLoggedIn).toBe(true)
    expect(info.userName).toBe('admin')
    expect(u.info?.nickName).toBe('Zero')
  })

  it('login 无 token 字段 → throw', async () => {
    ;(apiLogin as ReturnType<typeof vi.fn>).mockResolvedValue({
      userName: 'x',
      token: '',
      role: 0
    })
    const u = useUserStore()
    await expect(u.login({ username: 'x', password: 'p' })).rejects.toThrow(/缺少 token/)
  })

  it('login: fetchInfo 失败 → fallback 用 LoginResult 字段', async () => {
    ;(apiLogin as ReturnType<typeof vi.fn>).mockResolvedValue({
      userName: 'admin',
      token: 'tok',
      role: 1
    })
    ;(apiGetInfo as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('404'))
    const u = useUserStore()
    const info = await u.login({ username: 'admin', password: 'p' })
    expect(info.userName).toBe('admin')
    expect(info.role).toBe(1)
    expect(u.info).toEqual(info)
  })

  it('login 兼容 userName 字段 (camelCase)', async () => {
    ;(apiLogin as ReturnType<typeof vi.fn>).mockResolvedValue({
      userName: 'x',
      token: 't',
      role: 0
    })
    ;(apiGetInfo as ReturnType<typeof vi.fn>).mockResolvedValue({ userName: 'x', role: 0 })
    const u = useUserStore()
    await u.login({ userName: 'x', password: 'p' } as never)
    expect(apiLogin).toHaveBeenCalledWith({ userName: 'x', password: 'p' })
  })
})

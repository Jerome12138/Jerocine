import { describe, it, expect, beforeEach } from 'vitest'
import { setToken, getToken, clearToken } from './token'

describe('token utils', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('setToken("x") + getToken → "x"', () => {
    setToken('xyz')
    expect(getToken()).toBe('xyz')
  })

  it('setToken("") → 清空', () => {
    setToken('xyz')
    setToken('')
    expect(getToken()).toBe('')
  })

  it('setToken 带 expires → 存 auth-expires', () => {
    setToken('xyz', 1999999999)
    expect(localStorage.getItem('auth-expires')).toBe('1999999999')
  })

  it('过期的 token getToken → 空 + 主动清理', () => {
    const expiredSec = Math.floor(Date.now() / 1000) - 60
    setToken('expired-tok', expiredSec)
    expect(getToken()).toBe('')
    expect(localStorage.getItem('auth-token')).toBeNull()
    expect(localStorage.getItem('auth-expires')).toBeNull()
  })

  it('未过期的 token 正常返回', () => {
    const futureSec = Math.floor(Date.now() / 1000) + 3600
    setToken('valid-tok', futureSec)
    expect(getToken()).toBe('valid-tok')
  })

  it('clearToken() 移除 token + expires', () => {
    setToken('x', 9999999999)
    clearToken()
    expect(localStorage.getItem('auth-token')).toBeNull()
    expect(localStorage.getItem('auth-expires')).toBeNull()
  })

  it('legacy "auth" key 残留 → getToken 时自动清理', () => {
    localStorage.setItem('auth', '{"key":"old","value":"old-tok"}')
    setToken('new-tok')
    getToken() // 触发清理
    expect(localStorage.getItem('auth')).toBeNull()
    expect(getToken()).toBe('new-tok')
  })

  it('没 expires 不会因过期被清', () => {
    setToken('no-exp')
    expect(getToken()).toBe('no-exp')
  })
})

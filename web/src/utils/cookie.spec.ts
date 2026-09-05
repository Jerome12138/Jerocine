import { describe, it, expect, beforeEach } from 'vitest'
import { setCookie, getCookie, clearCookie, COOKIE_KEYS } from './cookie'

function clearAllCookies(): void {
  // jsdom 没原生 cookie 清理 helper, 手动遍历 document.cookie
  document.cookie.split(';').forEach((c) => {
    const eq = c.indexOf('=')
    const name = (eq > -1 ? c.slice(0, eq) : c).trim()
    document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`
  })
}

describe('cookie utils', () => {
  beforeEach(() => clearAllCookies())

  it('setCookie + getCookie 往返', () => {
    setCookie('foo', 'bar')
    expect(getCookie('foo')).toBe('bar')
  })

  it('不存在的 cookie → 空串', () => {
    expect(getCookie('nope')).toBe('')
  })

  it('clearCookie 后 getCookie 返回空', () => {
    setCookie('foo', 'bar')
    clearCookie('foo')
    expect(getCookie('foo')).toBe('')
  })

  it('特殊字符 URL encode', () => {
    setCookie('k', 'a b&c=d')
    expect(getCookie('k')).toBe('a b&c=d')
  })

  it('COOKIE_KEYS.FILM_HISTORY 常量', () => {
    expect(COOKIE_KEYS.FILM_HISTORY).toBe('filmHistory')
  })
})

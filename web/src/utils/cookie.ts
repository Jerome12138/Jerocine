/**
 * Cookie 工具，兼容旧站 cookieUtil
 * 旧站约定：观看历史 key = 'filmHistory'
 */

export const COOKIE_KEYS = {
  FILM_HISTORY: 'filmHistory'
} as const

/** 设置 cookie，expire 单位天 */
export function setCookie(name: string, value: string, expireDays = 30): void {
  if (typeof document === 'undefined') {
    return
  }
  const d = new Date()
  d.setTime(d.getTime() + expireDays * 24 * 60 * 60 * 1000)
  const expires = `expires=${d.toUTCString()}`
  const secure = location.protocol === 'https:' ? '; Secure' : ''
  document.cookie = `${name}=${encodeURIComponent(value)}; ${expires}; path=/; SameSite=Lax${secure}`
}

/** 读取 cookie；不存在返回空字符串 */
export function getCookie(name: string): string {
  if (typeof document === 'undefined') {
    return ''
  }
  const cookies = document.cookie.split('; ')
  for (const c of cookies) {
    const eq = c.indexOf('=')
    if (eq < 0) {
      continue
    }
    const k = c.slice(0, eq)
    const v = c.slice(eq + 1)
    if (k === name) {
      return decodeURIComponent(v)
    }
  }
  return ''
}

/** 清 cookie */
export function clearCookie(name: string): void {
  if (typeof document === 'undefined') {
    return
  }
  document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`
}

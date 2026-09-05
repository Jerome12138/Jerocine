/**
 * Token 持久化.
 *
 * 协议:
 *  - 服务端登录返回 body.token (LoginResult.token), 前端写入 localStorage
 *  - 请求时 http 拦截器以 `Authorization: Bearer <token>` 注入
 *  - 已废弃: 旧版 { key: 'auth-token', value }+`auth-token` 头方案
 */

const STORAGE_KEY = 'auth-token'
const EXPIRES_KEY = 'auth-expires'
const LEGACY_KEY = 'auth' // 旧版 { key, value } 结构残留, 首次清理掉

/** 写入 token + 过期时间 (unix 秒). 传空串视为清除. */
export function setToken(value: string, expires?: number): void {
  try {
    if (value) {
      localStorage.setItem(STORAGE_KEY, value)
      if (typeof expires === 'number' && expires > 0) {
        localStorage.setItem(EXPIRES_KEY, String(expires))
      }
    } else {
      localStorage.removeItem(STORAGE_KEY)
      localStorage.removeItem(EXPIRES_KEY)
    }
  } catch {
    /* 隐私模式 / 配额满, 忽略 */
  }
}

/** 读取 token; 不存在/已过期返回空串. 顺手清理老版本 'auth' key. */
export function getToken(): string {
  try {
    // 一次性清掉旧 client 留下的残留, 避免开发者工具里看到迷惑的"鉴权数据"
    if (localStorage.getItem(LEGACY_KEY) !== null) {
      localStorage.removeItem(LEGACY_KEY)
    }
    const tok = localStorage.getItem(STORAGE_KEY) ?? ''
    if (!tok) return ''
    const expRaw = localStorage.getItem(EXPIRES_KEY)
    if (expRaw) {
      const exp = Number(expRaw)
      if (Number.isFinite(exp) && exp > 0 && exp <= nowSec()) {
        // 已过期, 主动清掉, 避免拿着过期 token 撞 401
        clearToken()
        return ''
      }
    }
    return tok
  } catch {
    return ''
  }
}

/** 清除 token + 过期时间. */
export function clearToken(): void {
  try {
    localStorage.removeItem(STORAGE_KEY)
    localStorage.removeItem(EXPIRES_KEY)
  } catch {
    /* ignore */
  }
}

function nowSec(): number {
  return Math.floor(Date.now() / 1000)
}

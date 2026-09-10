/** URL / Query 工具 */

export type QueryValue = string | number | boolean | null | undefined

/**
 * 把对象拼成 query 字符串
 * - 自动跳过 undefined / null / ''
 * - 不做 URL 编码以外的转义
 */
export function stringifyQuery(params: Record<string, QueryValue>): string {
  const parts: string[] = []
  for (const k of Object.keys(params)) {
    const v = params[k]
    if (v === undefined || v === null || v === '') {
      continue
    }
    parts.push(`${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
  }
  return parts.join('&')
}

/** 把 query 字符串解析为 Record<string,string> */
export function parseQuery(query: string): Record<string, string> {
  const out: Record<string, string> = {}
  const s = query.startsWith('?') ? query.slice(1) : query
  if (!s) {
    return out
  }
  for (const pair of s.split('&')) {
    if (!pair) {
      continue
    }
    const idx = pair.indexOf('=')
    const k = idx < 0 ? pair : pair.slice(0, idx)
    const v = idx < 0 ? '' : pair.slice(idx + 1)
    out[decodeURIComponent(k)] = decodeURIComponent(v)
  }
  return out
}

/**
 * 是否为站外链接(http/https)。
 *
 * 轮播跳转约定: 站外链接开新窗口, 其余一律按站内路径走 router
 * (与后端 validBannerLink 白名单口径一致 —— 非 / 开头又非 http(s) 的伪协议
 * 根本进不了库, 这里只需区分"新窗口"与"路由跳转"两种走向)。
 * 桌面/移动/TV 三端共用本判断, 不要在组件里各自内联正则。
 */
export function isExternalLink(link: string): boolean {
  return /^https?:\/\//i.test(link.trim())
}

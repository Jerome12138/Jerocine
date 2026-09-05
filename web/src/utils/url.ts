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

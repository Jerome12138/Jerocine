/**
 * 端侧播放测速(浏览器端视频加载速度)—— PlayView 线路测速 与 后台采集页「测播放」共用。
 *
 * 链路: 原始 m3u8 → 服务端代理(同源, 规避跨域)解析首片 → 浏览器直连 CDN 计时。
 * 盗版 CDN 无 CORS → 用 no-cors 不透明请求, 计时到响应可用(首字节往返), 拿到即 abort 不下整片。
 * 注: no-cors 看不到状态码, 测的是"可达 + 首字节延时", 不保证 200。
 */
import { absApiBase } from '@/utils/jerocineNative'

const reM3u8 = /\.m3u8(\?|#|$)/i

/** 强制走 m3u8 代理(同源, 规避跨域), 用于测真实播放链路的解析入口 */
export function proxyM3u8Url(link: string): string {
  return `${absApiBase()}/v1/m3u8/proxy?src=${encodeURIComponent(link)}`
}

function originAbs(path: string): string {
  return (typeof window !== 'undefined' ? window.location.origin : '') + path
}

/** 限时读取(代理后的) m3u8 文本(同源代理可读) */
async function fetchText(url: string, timeoutMs = 8000): Promise<string> {
  const ctrl = new AbortController()
  const timer = window.setTimeout(() => ctrl.abort(), timeoutMs)
  try {
    const resp = await fetch(url, { signal: ctrl.signal, cache: 'no-store' })
    if (!resp.ok) return ''
    return await resp.text()
  } catch {
    return ''
  } finally {
    window.clearTimeout(timer)
  }
}

/**
 * 从(代理后的) m3u8 解析出第一条真实分片的绝对 URL.
 * 代理重写后: 子播放列表是 `/api/v1/m3u8/proxy?src=...`(同源可读), 分片是 CDN 绝对直链.
 * 遇子播放列表下钻一层(最多 2 层)直到拿到分片.
 */
export async function resolveFirstSegment(playlistUrl: string, depth = 0): Promise<string> {
  if (depth > 2) return ''
  const text = await fetchText(playlistUrl)
  if (!text) return ''
  const uris = text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith('#'))
  const first = uris[0]
  if (!first) return ''
  const isProxy = first.includes('/m3u8/proxy?')
  if (isProxy || reM3u8.test(first)) {
    const next = isProxy ? originAbs(first) : proxyM3u8Url(first)
    return resolveFirstSegment(next, depth + 1)
  }
  return first // 绝对 CDN 分片 URL
}

/** 端侧抓"第一片"计时: 直连 CDN 拉首个分片(真实播放的瓶颈链路), 拿到响应即 abort 不下整片 */
export async function measureSegment(segUrl: string, timeoutMs = 8000): Promise<number> {
  const ctrl = new AbortController()
  const timer = window.setTimeout(() => ctrl.abort(), timeoutMs)
  const t0 = performance.now()
  try {
    await fetch(segUrl, { mode: 'no-cors', cache: 'no-store', signal: ctrl.signal })
    const ms = Math.round(performance.now() - t0)
    ctrl.abort() // 拿到响应即止, 不下整片
    return ms
  } catch {
    return -1
  } finally {
    window.clearTimeout(timer)
  }
}

/** 单线路测速: 解析首片 → 直连 CDN 计时; 失败/非 m3u8 返回 -1 */
export async function measureLine(link: string): Promise<number> {
  if (!reM3u8.test(link)) return -1
  const seg = await resolveFirstSegment(proxyM3u8Url(link))
  if (!seg) return -1
  return measureSegment(seg)
}

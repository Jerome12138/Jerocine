/**
 * 端侧播放测速(浏览器端视频加载速度)—— PlayView 线路测速 与 后台采集页「测播放」共用。
 *
 * 链路: 浏览器直连抓清单(需源带 CORS 头, 与端侧本地过滤同前提) → 解析首片 → 直连 CDN 计时。
 * 盗版 CDN 无 CORS → 分片计时用 no-cors 不透明请求, 计时到响应可用(首字节往返),
 * 拿到即 abort 不下整片。注: no-cors 看不到状态码, 测的是"可达 + 首字节延时", 不保证 200。
 * 2026-10-28: 服务端 m3u8 代理已下线, 清单解析改为浏览器直连(无 CORS 的源测速返回 -1,
 * 与端侧本地过滤的前提一致 —— 过滤不了的源本来也不可测)。
 */
const reM3u8 = /\.m3u8(\?|#|$)/i

/** 限时读取 m3u8 文本(需源带 CORS 头) */
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
 * 从 m3u8 解析出第一条真实分片的绝对 URL.
 * 遇子播放列表下钻一层(最多 2 层)直到拿到分片; 子表地址端侧自行绝对化。
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
  if (reM3u8.test(first)) {
    return resolveFirstSegment(new URL(first, playlistUrl).href, depth + 1)
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
  const seg = await resolveFirstSegment(link)
  if (!seg) return -1
  return measureSegment(seg)
}

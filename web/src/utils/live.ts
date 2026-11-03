/**
 * 直播纯函数（LiveView 的可测内核）：频道分组聚合 + 端侧探源。
 */
import type { LiveChannel, LiveChannelGroup } from '@/types/live'

/** 未标注分组的频道归入该组（与服务端 DefaultGroup 同值）。 */
export const LIVE_DEFAULT_GROUP = '其他'

/**
 * 频道按分组聚合（保持首次出现顺序；组内保序）。
 * 服务端已按源文件顺序返回频道，这里只做分组归拢，不排序不打乱。
 */
export function groupChannels(channels: readonly LiveChannel[]): LiveChannelGroup[] {
  const groups: LiveChannelGroup[] = []
  const index = new Map<string, number>()
  for (const ch of channels) {
    const name = ch.group?.trim() ? ch.group.trim() : LIVE_DEFAULT_GROUP
    const i = index.get(name)
    if (i !== undefined) {
      groups[i]!.channels.push(ch)
    } else {
      index.set(name, groups.length)
      groups.push({ name, channels: [ch] })
    }
  }
  return groups
}

/** 端侧探源结果：alive = 本浏览器可达；cors = 带 ACAO（hls.js 可直接拉流）。 */
export interface LiveProbeResult {
  url: string
  alive: boolean
  cors: boolean
}

/**
 * 单频道探源（两段式）：
 *   ① cors 模式 fetch：成功 = 可达且带 ACAO → { alive: true, cors: true }；
 *      失败（TypeError = 无 ACAO 或不可达）→ ② no-cors 模式复测：
 *      成功 = 可达但跨域受限 → { alive: true, cors: false }；仍失败/超时 → { alive: false, cors: false }。
 * 注：no-cors 请求浏览器不做 CORS 校验，能拿到不透明响应即网络层可达；
 *     hls.js 拉流走的是 cors 模式 XHR，故 cors=false 时播放会失败（角标提示跨域受限）。
 */
export async function probeLiveChannel(
  url: string,
  timeoutMs: number,
  fetchImpl: typeof fetch = fetch,
): Promise<LiveProbeResult> {
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), timeoutMs)
  try {
    await fetchImpl(url, { mode: 'cors', signal: ctrl.signal, redirect: 'follow' })
    return { url, alive: true, cors: true }
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      return { url, alive: false, cors: false }
    }
    try {
      await fetchImpl(url, { mode: 'no-cors', signal: ctrl.signal, redirect: 'follow' })
      return { url, alive: true, cors: false }
    } catch {
      return { url, alive: false, cors: false }
    }
  } finally {
    clearTimeout(timer)
  }
}

/**
 * 限流并发探源：固定工作池上限（同一 IPTV 源的瞬间连接数可控，防被源站封禁），
 * 每个结果经 onResult 随到随回调（频道角标渐进点亮），全部完成后返回结果数组。
 * delayMs > 0 时，每个请求之间再插入间隔（pid 级节流），用于后台批量探测进一步降低
 * 被源站限流/封禁的风险。
 */
export async function probeLiveChannelsLimited(
  urls: readonly string[],
  concurrency: number,
  timeoutMs: number,
  fetchImpl: typeof fetch = fetch,
  onResult?: (r: LiveProbeResult) => void,
  delayMs = 0,
): Promise<LiveProbeResult[]> {
  const out: LiveProbeResult[] = []
  let idx = 0
  const sleep = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms))
  const workers = Array.from(
    { length: Math.max(1, Math.min(concurrency, urls.length)) },
    async () => {
      while (idx < urls.length) {
        const url = urls[idx++]!
        const r = await probeLiveChannel(url, timeoutMs, fetchImpl).catch(() => null)
        if (r) {
          out.push(r)
          onResult?.(r)
        }
        if (delayMs > 0) await sleep(delayMs)
      }
    },
  )
  await Promise.allSettled(workers)
  return out
}

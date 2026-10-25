/**
 * adFilter - 广告过滤的成功提示 / 失败埋点纯逻辑.
 *
 * 抽成纯函数便于单测; PlayView.vue 负责副作用(fetch / toast / telemetry).
 *  - 成功: GET /m3u8/stats?src=... 返回 {filtered,total}(dto.OK 直返, 无 data 外层),
 *          filtered>0 时提示"已过滤 N 段广告".
 *  - 失败: 播放代理 URL 报错回退原片时, 上报 category=adfilter-fail 埋点, 存原始 + 代理链接备查.
 */

/** 后端 /m3u8/stats 契约 (server M3u8Stats), dto.OK 直返顶层字段. */
export interface AdFilterStats {
  filtered: number
  total: number
}

/** 宽松解析 stats 响应: 非法/缺字段一律归 0, 不抛错. */
export function parseAdFilterStats(body: unknown): AdFilterStats {
  const o = (body ?? {}) as Record<string, unknown>
  const filtered = typeof o.filtered === 'number' && o.filtered > 0 ? Math.floor(o.filtered) : 0
  const total = typeof o.total === 'number' && o.total > 0 ? Math.floor(o.total) : 0
  return { filtered, total }
}

/** 成功提示文案. */
export function adFilterSuccessMessage(filtered: number): string {
  return `已过滤 ${filtered} 段广告`
}

/**
 * loading 遮罩文案三档（2026-10-25 用户报"已过滤提示已出, 遮罩还显示正在准备视频"后重排）。
 *
 * 背景(线上实测 iPhone 12 + Edge): 打开播放页到起播可切三段——
 *   1. 取数据/等片源(静态资源 + playInfo + 过滤抓清单+POST), 遮罩「正在准备视频」;
 *   2. 过滤落地(blob/直链已 commit, "已过滤 N 段"toast 已出) → **分片下载+起播缓冲**
 *      (国际链路实测 1s~数十秒), 此前文案仍停在"正在准备视频", 与真实阶段错位;
 *   3. canplay → 遮罩收。
 * srcReady 即"片源已就绪等播放器就绪"标志(commit 成功置位, 切集/切源复位)。
 */
export function loadingHintText(opts: { srcReady: boolean; filtering: boolean }): string {
  if (opts.srcReady) return '正在缓冲视频…'
  return opts.filtering ? '正在准备视频 · 广告过滤中…' : '正在准备视频…'
}

/** 取 URL host, 解析失败返回空串. 用于埋点 label 按来源 CDN 聚合. */
export function adFilterHost(url: string): string {
  try {
    return new URL(url).hostname
  } catch {
    return ''
  }
}

export interface AdFilterFailureCtx {
  /** 'web' = video.js 报错; 'native' = 原生 ExoPlayer 报错回调 */
  channel: 'web' | 'native'
  /** 原始片源 URL(未包代理) */
  originalUrl: string
  /** 实际喂播放器的代理 URL */
  proxyUrl: string
  sourceId?: string
  sourceName?: string
  episodeIndex?: number
  filmId?: string
  filmName?: string
}

/** telemetry.track('error', ...) 的入参; 把链接存进 extra 便于后台按 host 聚合 + 复现. */
export function buildAdFilterFailureEvent(ctx: AdFilterFailureCtx): {
  category: string
  action: string
  label: string
  extra: Record<string, unknown>
} {
  return {
    category: 'adfilter-fail',
    action: ctx.channel,
    label: adFilterHost(ctx.originalUrl) || 'unknown',
    extra: {
      src: ctx.originalUrl,
      proxyUrl: ctx.proxyUrl,
      sourceId: ctx.sourceId ?? '',
      sourceName: ctx.sourceName ?? '',
      episode: ctx.episodeIndex ?? -1,
      filmId: ctx.filmId ?? '',
      filmName: ctx.filmName ?? '',
      channel: ctx.channel
    }
  }
}

/* ============ 清单判据 (与安卓 player-core PlayerUrls 同款口径) ============ */

/**
 * 判断 m3u8 文本是否为**媒体列表**(含分片, 有 #EXTINF) —— 区别于 master 主表。
 *
 * 与安卓 {@code PlayerUrls.hasMediaSegments} 同款内容判据: 按 HLS 规范媒体列表
 * 每个分片必有 #EXTINF, master 主表只有变体声明(#EXT-X-STREAM-INF)没有分片。
 * 不用 URL 扩展名猜 —— 变体地址不带 .m3u8 后缀的源(token 化 CDN 常见)按扩展名
 * 会被误判成媒体表, master 层被当媒体层过滤, 带广告的子表被播放器直连绕过。
 */
export function hasMediaSegments(text: string): boolean {
  for (const line of text.split('\n')) {
    if (line.trim().toUpperCase().startsWith('#EXTINF')) return true
  }
  return false
}

/**
 * master 主表下钻: 取第一个非注释 URI 作为子清单地址(**不看扩展名**, master/媒体
 * 的区分只看内容判据 hasMediaSegments), 供过滤链路逐层跟进。
 * 媒体列表(要就地过滤)或没有任何 URI 的文本返回 null —— 调用方对 null 一律按
 * "媒体列表 → 就地 POST 过滤"处理, 病态输入退化为与旧实现一致的服务端过滤。
 */
export function firstChildPlaylistUrl(text: string): string | null {
  if (hasMediaSegments(text)) return null
  for (const line of text.split('\n')) {
    const t = line.trim()
    if (!t || t.startsWith('#')) continue
    return t
  }
  return null
}

/* ============ 角标文案 (与安卓 AdFilterStatus 逐字对齐) ============ */

/** 广告过滤角标状态 — 与安卓 AdFilterStatus 同名同文案(文案在 adFilterBadgeText). */
export type AdFilterBadgeKind =
  | 'off'
  | 'unsupported'
  | 'proxy'
  | 'filtering'
  | 'clean'
  | 'filtered'
  | 'failed'

/** 角标文案。色调约定: off/unsupported/failed=灰, proxy/filtering=蓝, clean/filtered=绿。 */
export function adFilterBadgeText(kind: AdFilterBadgeKind, count: number): string {
  switch (kind) {
    case 'off':
      return '过滤未开启'
    case 'unsupported':
      return '该源无需过滤'
    case 'proxy':
      return '服务端过滤中'
    case 'filtering':
      return '广告过滤中'
    case 'clean':
      return '未检出广告'
    case 'filtered':
      return `已过滤 ${count} 段广告`
    case 'failed':
      return '过滤失败'
  }
}

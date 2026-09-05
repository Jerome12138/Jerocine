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

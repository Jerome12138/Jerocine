/**
 * adFilter - 广告过滤的成功提示 / 角标纯逻辑.
 *
 * 抽成纯函数便于单测; PlayView.vue 负责副作用(fetch / toast / telemetry).
 *  - 成功: 本地内核(m3u8Filter.rewriteM3u8)返回 filtered>0 时提示"已过滤 N 段广告".
 *  - 服务端过滤已下线(2026-10-28): 端侧本地过滤是唯一路径, 无 MSE 设备走原始直链.
 */

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

/** 广告过滤角标状态 — 与安卓 AdFilterStatus 同名同文案(文案在 adFilterBadgeText)。
 *  nosupport 为 web 端新增: 设备走原生 HLS(无 MSE), 端侧 blob 清单不可播。
 *  2026-10-28: 'proxy'(服务端过滤中) 随服务端过滤下线移除。 */
export type AdFilterBadgeKind =
  | 'off'
  | 'unsupported'
  | 'filtering'
  | 'clean'
  | 'filtered'
  | 'failed'
  | 'nosupport'

/** 角标文案。色调约定: off/unsupported/failed=灰, filtering=蓝, clean/filtered=绿。 */
export function adFilterBadgeText(kind: AdFilterBadgeKind, count: number): string {
  switch (kind) {
    case 'off':
      return '过滤未开启'
    case 'unsupported':
      return '该源无需过滤'
    case 'filtering':
      return '广告过滤中'
    case 'clean':
      return '未检出广告'
    case 'filtered':
      return `已过滤 ${count} 段广告`
    case 'failed':
      return '过滤失败'
    case 'nosupport':
      return '该设备不支持端侧过滤'
  }
}

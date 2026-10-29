/**
 * m3u8 去广告过滤内核 —— TS 移植（Go: server/internal/service/m3u8_service.go rewriteM3u8Core）。
 *
 * 可迁移纯函数：零 DOM/window 依赖（仅标准全局 URL），Node/浏览器/worker 通用。
 * 算法规范见 docs/m3u8-filter-ALGORITHM.md；三端(Go/TS/Kotlin)一致性由
 * fixtures/m3u8-filter/cases.json 锁死，改规则必须三端同步重跑夹具。
 *
 * client 语义：仅绝对化 + 过滤（可选），子播放列表只绝对化、不代理回环。
 */

/** 阈值常量（三端一致，见 ALGORITHM.md §8） */
export const AD_NUM_MIN_SEGS = 10
export const AD_NUM_MIN_COVERAGE = 0.8
export const AD_NUM_MIN_CONTENT = 0.6
export const AD_NUM_MAX_DROP_FRAC = 0.3
export const AD_CROSS_MIN_SEGS = 10
export const AD_CROSS_MAX_DROP_FRAC = 0.3
export const AD_SHORT_MAX_DUR = 1.0
export const AD_SHORT_SKIP_HEAD = 4
export const AD_SHORT_BURST_MIN_DUR = 3.0
export const M3U8_SAMPLE_MAX = 5

export interface M3u8FilterOptions {
  /** 是否启用广告过滤；false 时仅绝对化，filtered 恒 0 */
  filterAds: boolean
}

export interface M3u8FilterResult {
  /** 重写后的清单文本（逐行处理，末尾空行保留） */
  text: string
  /** 被判为广告剔除的分片数 */
  filtered: number
  /** 本列表内真实分片总数 */
  total: number
  /** 跨域/异目录命中剔除数（排查用） */
  cross: number
  /** 编号跳脱命中剔除数（排查用） */
  num: number
  /** 极短簇命中剔除数（排查用） */
  short: number
  /** 是否含 #EXT-X-DISCONTINUITY（排查用） */
  hasDisc: boolean
}

/** RFC3986 相对解析：http(s) 绝对原样；其余用 base 解析；失败原样返回。 */
export function absolutize(ref: string, base: URL): string {
  if (ref.startsWith('http://') || ref.startsWith('https://')) return ref
  try {
    return new URL(ref, base).href
  } catch {
    return ref
  }
}

/** 忽略 query/fragment，path 以 .m3u8 结尾（大小写不敏感）。 */
export function isPlaylistURI(rawURL: string): boolean {
  try {
    const u = new URL(rawURL)
    return u.pathname.toLowerCase().endsWith('.m3u8')
  } catch {
    return false
  }
}

/** 解析 #EXTINF 时长（秒），兼容 "#EXTINF:4.0," / "#EXTINF:3"；失败或负值返回 null。 */
export function parseExtinfDuration(line: string): number | null {
  if (!line.startsWith('#EXTINF')) return null
  const i = line.indexOf(':')
  if (i < 0) return null
  let rest = line.slice(i + 1).trim()
  const j = rest.indexOf(',')
  if (j >= 0) rest = rest.slice(0, j).trim()
  if (rest === '') return null // Go ParseFloat("") 报错
  const d = Number(rest)
  if (!Number.isFinite(d) || d < 0) return null
  return d
}

/** 取分片文件名（去 query/扩展名）末尾连续数字；无则 -1。 */
export function segNumber(rawURL: string): number {
  let p = rawURL
  try {
    p = new URL(rawURL).pathname
  } catch {
    /* 非 URL：按原串处理 */
  }
  const slash = p.lastIndexOf('/')
  if (slash >= 0) p = p.slice(slash + 1)
  const dot = p.lastIndexOf('.')
  if (dot >= 0) p = p.slice(0, dot)
  let j = p.length
  while (j > 0 && p.charCodeAt(j - 1)! >= 48 && p.charCodeAt(j - 1)! <= 57) j -= 1
  if (j === p.length) return -1
  let digits = p.slice(j)
  if (digits.length > 15) digits = digits.slice(digits.length - 15)
  const v = Number(digits)
  return Number.isSafeInteger(v) ? v : -1
}

/** 排序数列相邻差值的中位数（用于估计正片编号步长）。 */
export function medianGap(sorted: readonly number[]): number {
  if (sorted.length < 2) return 0
  const gaps: number[] = []
  for (let i = 1; i < sorted.length; i += 1) gaps.push(sorted[i]! - sorted[i - 1]!)
  gaps.sort((a, b) => a - b)
  return gaps[Math.floor(gaps.length / 2)] ?? 0
}

/**
 * 分片归属键 = host + 内容目录（剥文件名后取前两段目录）。
 * 必须先剥文件名再取目录段：目录只有一层时旧实现第二段取到文件名，
 * 每个分片各自成键互为异键 → 正常分片被整批当跨域广告删掉（segKey 回归用例）。
 */
export function segKey(abs: string): string {
  let u: URL
  try {
    u = new URL(abs)
  } catch {
    return ''
  }
  if (!u.hostname) return ''
  let dir = u.pathname
  const slash = dir.lastIndexOf('/')
  if (slash >= 0) dir = dir.slice(0, slash)
  else dir = ''
  const parts = dir.replace(/^\/+|\/+$/g, '').split('/').filter((s) => s !== '')
  if (parts.length >= 2) return `${u.host}/${parts[0]}/${parts[1]}`
  if (parts.length === 1) return `${u.host}/${parts[0]}`
  return u.host
}

/** 统计本播放列表内分片（非子列表）的归属键，返回出现最多者（真实内容）。 */
export function dominantSegmentKey(lines: readonly string[], base: URL): string {
  const counts = new Map<string, number>()
  for (const ln of lines) {
    const t = ln.trim()
    if (t === '' || t.startsWith('#')) continue
    const abs = absolutize(t, base)
    if (isPlaylistURI(abs)) continue
    const k = segKey(abs)
    if (k !== '') counts.set(k, (counts.get(k) ?? 0) + 1)
  }
  let best = ''
  let bestN = 0
  for (const [k, n] of counts) {
    if (n > bestN) {
      best = k
      bestN = n
    }
  }
  return best
}

/** 跨域维度：标记归属键 ≠ 主导键的分片；异键占比超上限（长表）→ 整表不剔，返回 null。 */
export function crossDomainAds(segURIs: readonly string[], dominant: string): Map<number, boolean> | null {
  if (dominant === '' || segURIs.length === 0) return null
  const ads = new Map<number, boolean>()
  for (let i = 0; i < segURIs.length; i += 1) {
    const k = segKey(segURIs[i]!)
    if (k !== '' && k !== dominant) ads.set(i, true)
  }
  if (ads.size === 0) return null
  if (segURIs.length >= AD_CROSS_MIN_SEGS && ads.size > segURIs.length * AD_CROSS_MAX_DROP_FRAC) return null
  return ads
}

/**
 * 编号跳脱：正片尾号在紧凑连续区间（主内容簇），广告编号远离该区间。
 * 不满足条件（命名非顺序/无主簇/超剔除上限）→ null（不剔）。
 */
export function numberOutlierAds(segURIs: readonly string[]): Map<number, boolean> | null {
  const n = segURIs.length
  if (n < AD_NUM_MIN_SEGS) return null
  const nums: number[] = []
  let numbered = 0
  for (const u of segURIs) {
    const v = segNumber(u)
    nums.push(v)
    if (v >= 0) numbered += 1
  }
  if (numbered < n * AD_NUM_MIN_COVERAGE) return null
  const seen = new Set<number>()
  const distinct: number[] = []
  for (const x of nums) {
    if (x >= 0 && !seen.has(x)) {
      seen.add(x)
      distinct.push(x)
    }
  }
  if (distinct.length < 2) return null
  distinct.sort((a, b) => a - b)
  const thr = Math.max(1000, 1000 * medianGap(distinct))
  let lo = distinct[0]!
  let hi = distinct[0]!
  let bestLo = lo
  let bestHi = hi
  let bestCount = 0
  const flush = (l: number, h: number): void => {
    let cnt = 0
    for (const x of nums) {
      if (x >= l && x <= h) cnt += 1
    }
    if (cnt > bestCount) {
      bestLo = l
      bestHi = h
      bestCount = cnt
    }
  }
  for (let i = 1; i < distinct.length; i += 1) {
    const cur = distinct[i]!
    if (cur - distinct[i - 1]! > thr) {
      flush(lo, hi)
      lo = cur
    }
    hi = cur
  }
  flush(lo, hi)
  if (bestCount < n * AD_NUM_MIN_CONTENT) return null
  const ads = new Map<number, boolean>()
  for (let i = 0; i < nums.length; i += 1) {
    const x = nums[i]!
    if (x >= 0 && (x < bestLo || x > bestHi)) ads.set(i, true)
  }
  if (ads.size > n * AD_NUM_MAX_DROP_FRAC) return null
  return ads
}

/** 连续极短分片簇（广告密集切片）；开头 adShortSkipHead 段豁免。 */
export function shortBurstAds(segDurs: readonly number[]): Map<number, boolean> {
  const out = new Map<number, boolean>()
  const n = segDurs.length
  let i = 0
  while (i < n) {
    const di = segDurs[i]!
    if (di >= 0 && di < AD_SHORT_MAX_DUR && i > AD_SHORT_SKIP_HEAD) {
      let j = i
      let burst = 0
      while (j < n) {
        const dj = segDurs[j]!
        if (!(dj >= 0 && dj < AD_SHORT_MAX_DUR && j > AD_SHORT_SKIP_HEAD)) break
        burst += dj
        j += 1
      }
      if (burst >= AD_SHORT_BURST_MIN_DUR) {
        for (let k = i; k < j; k += 1) out.set(k, true)
      }
      i = j
    } else {
      i += 1
    }
  }
  return out
}

/** 分片时长均值与标准差（总体标准差；输入须已剔除未知时长）。 */
export function segDurationStats(known: readonly number[]): { avg: number; std: number } {
  const n = known.length
  if (n === 0) return { avg: 0, std: 0 }
  let s = 0
  for (const d of known) s += d
  const avg = s / n
  let sq = 0
  for (const d of known) sq += (d - avg) * (d - avg)
  return { avg, std: Math.sqrt(sq / n) }
}

/** 时长异常分片（z-score>3）索引集与样本 URI（仅预警不剔除）。 */
export function durationOutliers(
  segDurs: readonly number[],
  segURIs: readonly string[]
): { outliers: Map<number, boolean>; samples: string[] } {
  const outliers = new Map<number, boolean>()
  const known: number[] = []
  for (const d of segDurs) {
    if (d >= 0) known.push(d)
  }
  if (known.length < AD_NUM_MIN_SEGS) return { outliers, samples: [] }
  const { avg, std } = segDurationStats(known)
  if (std <= 0) return { outliers, samples: [] }
  const samples: string[] = []
  for (let i = 0; i < segDurs.length; i += 1) {
    const d = segDurs[i]!
    if (d < 0) continue
    if (Math.abs(d - avg) > 3 * std) {
      outliers.set(i, true)
      if (samples.length < M3U8_SAMPLE_MAX && i < segURIs.length) samples.push(segURIs[i]!)
    }
  }
  return { outliers, samples }
}

const HLS_URI_ATTR = /URI="([^"]+)"/

/** 改写标签行内 URI="..." 属性（client 语义：仅绝对化）。 */
export function rewriteTagURI(line: string, base: URL): string {
  const m = HLS_URI_ATTR.exec(line)
  if (!m) return line
  const abs = absolutize(m[1]!, base)
  return line.slice(0, m.index + 'URI="'.length) + abs + line.slice(m.index + 'URI="'.length + m[1]!.length)
}

/** 主入口：逐行绝对化（可选过滤），返回重写文本与统计。 */
export function rewriteM3u8(body: string, base: string | URL, opts: M3u8FilterOptions): M3u8FilterResult {
  const baseURL = typeof base === 'string' ? new URL(base) : base
  const lines = body.split('\n')
  const out: string[] = []
  const dominant = dominantSegmentKey(lines, baseURL)

  let adByIdx: Map<number, boolean> | null = null
  let shortByIdx: Map<number, boolean> | null = null
  let crossByIdx: Map<number, boolean> | null = null
  if (opts.filterAds) {
    const segURIs: string[] = []
    const segDurs: number[] = []
    let pendingDur = -1
    for (const ln of lines) {
      const t = ln.trim()
      if (t === '') continue
      if (t.startsWith('#EXTINF')) {
        const d = parseExtinfDuration(t)
        if (d !== null) pendingDur = d
        continue
      }
      if (t.startsWith('#')) continue
      const abs = absolutize(t, baseURL)
      if (isPlaylistURI(abs)) continue
      segURIs.push(abs)
      segDurs.push(pendingDur)
      pendingDur = -1
    }
    adByIdx = numberOutlierAds(segURIs)
    shortByIdx = shortBurstAds(segDurs)
    crossByIdx = crossDomainAds(segURIs, dominant)
    // 时长异常(z-score)预警仅服务端日志用(Go stats.DurOutlier/DurSample);
    // 客户端 Result 不暴露该统计 → 不在本地内核中计算(避免白跑一趟 O(n))
  }

  let pendingExtinf = ''
  let filtered = 0
  let total = 0
  let cross = 0
  let num = 0
  let short = 0
  let hasDisc = false
  let segIdx = -1

  for (const ln of lines) {
    const t = ln.trim()
    if (t === '') {
      out.push(ln)
      continue
    }
    if (t.startsWith('#EXTINF')) {
      pendingExtinf = ln
      continue
    }
    if (t.startsWith('#')) {
      if (t.startsWith('#EXT-X-DISCONTINUITY')) {
        hasDisc = true
      }
      out.push(rewriteTagURI(ln, baseURL))
      continue
    }
    const abs = absolutize(t, baseURL)
    if (isPlaylistURI(abs)) {
      if (pendingExtinf !== '') {
        out.push(pendingExtinf)
        pendingExtinf = ''
      }
      out.push(abs)
      continue
    }
    total += 1
    segIdx += 1
    if (opts.filterAds && pendingExtinf !== '') {
      const crossDrop = crossByIdx?.get(segIdx) ?? false
      const numDrop = !crossDrop && (adByIdx?.get(segIdx) ?? false)
      const shortDrop = !crossDrop && !numDrop && (shortByIdx?.get(segIdx) ?? false)
      if (crossDrop || numDrop || shortDrop) {
        pendingExtinf = ''
        filtered += 1
        if (crossDrop) cross += 1
        else if (numDrop) num += 1
        else short += 1
        continue
      }
    }
    if (pendingExtinf !== '') {
      out.push(pendingExtinf)
      pendingExtinf = ''
    }
    out.push(abs)
  }

  return {
    text: out.join('\n'),
    filtered,
    total,
    cross,
    num,
    short,
    hasDisc
  }
}

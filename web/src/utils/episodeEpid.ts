/**
 * 集身份 `epid` 与跨源集对齐（纯函数，无依赖）。
 * 方案与规则见 `JerocineHarness/docs/方案-epid集身份与跨源对齐-2026-11-06.md`。
 *
 * 语义与 Go 侧 `server/internal/danmu/episode_key.go` 对齐（同一组黄金用例钉住）：
 *  - `epid` 只含**绝对身份**：s(栏目) / v(变体) / p(主次部分合并) / n(期号) / d(日期)；
 *    **不放"期序"** —— 序号是集合内的相对量，跨源分集不一致时会污染身份。
 *  - 期号优先于日期（两侧都有期号 ⇒ 只比期号，不看日期）。
 *  - 主次部分用**复合单段**（上/中/下/中上/中下）+「前缀相容」。
 */

const RE_VARIANT = /加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外/g
/** 无意义后缀：解析前剔除（与括号/标点同等地位） */
const RE_VARIANT_NOISE = /会员|未公开|未播|直拍|完整版|未删减/g
const RE_BRACKET = /【[^】]*】|（[^）]*）|\([^)]*\)|《[^》]*》|「[^」]*」|『[^』]*』|\[[^\]]*\]/g
const RE_DATE8 = /(?:19|20)\d{6}/
const RE_DI_N = /第\s*(\d{1,4})\s*[期集话回季部]/
const RE_SERIES_STOP = /^(正片|国语|粤语|原声|普通话|双语)$/
const PARTS = '上中下'
const LEFT_BRACKETS = '（(【[《「『'

export interface EpisodeEpid {
  /** s：栏目/系列名（正片为空），如「心动的周末」 */
  series: string
  /** v：有意义的变体集合，字典序 `|` 连接（加更/陪看/纯享/先导/超前/彩蛋/特辑/花絮/预告/番外） */
  variant: string
  /** p：上/中/下/中上/中下（取最先出现的，紧邻的第二个合并） */
  part: string
  /** n：期号（0 = 无） */
  num: number
  /** d：YYYYMMDD（'' = 无） */
  date: string
}

const EMPTY: EpisodeEpid = { series: '', variant: '', part: '', num: 0, date: '' }

/** 集名 → epid。 */
export function parseEpid(raw: string | undefined | null): EpisodeEpid {
  const out: EpisodeEpid = { ...EMPTY }
  let s = (raw ?? '').trim()
  if (!s) return out
  s = s.replace(RE_VARIANT_NOISE, '')
  const dm = s.match(RE_DATE8)
  if (dm) out.date = dm[0]
  const nm = s.match(RE_DI_N)
  if (nm) out.num = Number(nm[1]) || 0
  const pi = firstPartIndex(s)
  if (pi >= 0) {
    const first = s[pi] ?? ''
    let j = pi + 1
    if (j < s.length && LEFT_BRACKETS.includes(s[j]!)) j++
    const second = j < s.length ? (s[j] ?? '') : ''
    out.part = PARTS.includes(second) ? first + second : first
  }
  const vs = s.match(RE_VARIANT)
  if (vs && vs.length) out.variant = [...new Set(vs)].sort().join('|')
  out.series = seriesOf(s)
  return out
}

/** 序列化成统一串：`s=..|v=..|p=..|n=..|d=..`（缺值填 `-`）。 */
export function epidOf(raw: string | undefined | null): string {
  const e = parseEpid(raw)
  return [
    `s=${e.series || '-'}`,
    `v=${e.variant || '-'}`,
    `p=${e.part || '-'}`,
    `n=${e.num || '-'}`,
    `d=${e.date || '-'}`
  ].join('|')
}

/** 是否「综艺形态」：只有它才值得走 epid 对齐（剧/动漫的「第N集」形态天然不触发，零回归）。 */
export function isVarietyShaped(e: EpisodeEpid): boolean {
  return e.num > 0 || e.date !== '' || e.part !== '' || e.variant !== ''
}

function firstPartIndex(s: string): number {
  let best = -1
  for (const ch of PARTS) {
    const i = s.indexOf(ch)
    if (i >= 0 && (best < 0 || i < best)) best = i
  }
  return best
}

/** 栏目/系列名：截断到第一个「：」→ 去括号内容 → 去日期/期号/变体 → 只留中文 → 去上中下；≥2 字才算。 */
function seriesOf(s: string): string {
  let t = s
  const ci = t.search(/[：:]/)
  if (ci >= 0) t = t.slice(0, ci)
  t = t.replace(RE_BRACKET, '')
  t = t.replace(new RegExp(RE_DATE8.source, 'g'), '')
  t = t.replace(new RegExp(RE_DI_N.source, 'g'), '')
  t = t.replace(RE_VARIANT, '').replace(RE_VARIANT_NOISE, '')
  const han = t.replace(/[^\u4e00-\u9fa5]/g, '').replace(/[上中下]/g, '')
  if (han.length < 2 || RE_SERIES_STOP.test(han)) return ''
  return han
}

/** 主/次部分相容：一方缺失视为通配（源里只有一个「中」、库里是「中（上）/（下）」）；都非空则须相等或前缀相容。 */
function partCompatible(a: string, b: string): boolean {
  if (!a || !b) return true
  return a === b || a.startsWith(b) || b.startsWith(a)
}

/** 身份相容（不含期号/日期）：栏目 + 变体 + 主次部分。 */
function identityCompatible(a: EpisodeEpid, b: EpisodeEpid): boolean {
  return a.variant === b.variant && a.series === b.series && partCompatible(a.part, b.part)
}

function sameEpid(a: EpisodeEpid, b: EpisodeEpid): boolean {
  return (
    a.series === b.series && a.variant === b.variant && a.part === b.part && a.num === b.num && a.date === b.date
  )
}

/** 「期槽」：期号优先、无期号用日期（无则空串 = 不参与期序）。 */
function periodKey(e: EpisodeEpid): string {
  if (e.num > 0) return `n${e.num}`
  return e.date ? `d${e.date}` : ''
}

/** 期序：同 (series, variant) 分组内，按「期槽」去重排序后的序号（1 起；无期槽 = 0）。 */
function periodOrdinals(list: EpisodeEpid[]): number[] {
  const out = list.map(() => 0)
  const groups = new Map<string, number[]>()
  list.forEach((e, i) => {
    const g = `${e.series}\u0001${e.variant}`
    const arr = groups.get(g) ?? []
    arr.push(i)
    groups.set(g, arr)
  })
  for (const idxs of groups.values()) {
    const keys = [
      ...new Set(idxs.map((i) => periodKey(list[i]!)).filter((k) => k !== ''))
    ].sort(comparePeriodKey)
    const rank = new Map(keys.map((k, n) => [k, n + 1]))
    for (const i of idxs) out[i] = rank.get(periodKey(list[i]!)) ?? 0
  }
  return out
}

function comparePeriodKey(a: string, b: string): number {
  const na = Number(a.slice(1))
  const nb = Number(b.slice(1))
  if (a[0] === 'n' && b[0] === 'n') return na - nb
  return a < b ? -1 : a > b ? 1 : 0
}

/** 对齐档位：exact=身份全等；samePeriod=同期号；sameDate=同日期；byOrdinal=同期序；position=按位置。 */
export type AlignTier = 'exact' | 'samePeriod' | 'sameDate' | 'byOrdinal' | 'position' | 'failed'

export interface AlignResult {
  /** 目标源里的下标；-1 = 完全对不上 */
  index: number
  tier: AlignTier
}

/** 能否据此**自动**换源：只有精确档（exact/samePeriod/sameDate）才放行；
 *  byOrdinal / position 是启发式兜底 ⇒ 只提示、不自动切（宁可不切，不乱切）。 */
export function canAutoSwitch(tier: AlignTier): boolean {
  return tier === 'exact' || tier === 'samePeriod' || tier === 'sameDate'
}

/**
 * 跨源集对齐（5 档降级）。`curEps` / `targetEps` 为两侧完整集名列表（下标即播放用的集号）。
 * 全部对不上时返回 `{ index: -1, tier: 'failed' }`。
 */
export function alignEpisode(curIndex: number, curEps: string[], targetEps: string[]): AlignResult {
  const cur = curEps.map(parseEpid)
  const tgt = targetEps.map(parseEpid)
  const c = cur[curIndex]
  if (!c || tgt.length === 0) return { index: -1, tier: 'failed' }

  // ① epid 精确相等
  for (let i = 0; i < tgt.length; i++) {
    if (sameEpid(c, tgt[i]!)) return { index: i, tier: 'exact' }
  }
  // ② 同身份 + 期号相同（两侧都有期号）—— 期号优先，此时不看日期
  if (c.num > 0) {
    for (let i = 0; i < tgt.length; i++) {
      const t = tgt[i]!
      if (t.num > 0 && t.num === c.num && identityCompatible(c, t)) {
        return { index: i, tier: 'samePeriod' }
      }
    }
  }
  // ③ 同身份 + 日期相同（两侧都没期号、都有日期）
  if (c.num === 0 && c.date !== '') {
    for (let i = 0; i < tgt.length; i++) {
      const t = tgt[i]!
      if (t.num === 0 && t.date === c.date && identityCompatible(c, t)) {
        return { index: i, tier: 'sameDate' }
      }
    }
  }
  // ④ 同身份 + 期序相同（跨命名体系：源 A 用日期、源 B 用期号）
  const curOrd = periodOrdinals(cur)[curIndex] ?? 0
  if (curOrd > 0) {
    const tgtOrd = periodOrdinals(tgt)
    for (let i = 0; i < tgt.length; i++) {
      if (tgtOrd[i] === curOrd && identityCompatible(c, tgt[i]!)) {
        return { index: i, tier: 'byOrdinal' }
      }
    }
  }
  // ⑤ 整体位置（原有兜底：第 i 个 ↔ 第 i 个，超出取最后一集）
  return { index: Math.min(curIndex, tgt.length - 1), tier: 'position' }
}

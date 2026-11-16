/**
 * 集名身份键（与 `server/internal/danmu/episode_key.go` v2 同规, 2026-11-07；黄金用例与 Go 测试同一组）。
 *
 * 用途（方案-epid §5/§8）:
 *  1. 弹幕池缓存键 `mid:epid` —— 触发的集 100% 有 epid（5 段定长、缺值填 `-`；期号经桥接归一写入 n 段），
 *     未触发（裸「第N集」/E圆号）没有 → 调用方回退旧键；
 *  2. A2 现算期号（经他源桥接）传给聚合接口 `epNum`；
 *  3. （后续）切源对齐 sameEpisode。
 */

export interface EpisodeKey {
  /** YYYYMMDD（三格式归一后） */
  date: string
  /** 第N期/集/话/回/季/部（无则 0） */
  num: number
  /** E01/EP01（无则 0） */
  eNum: number
  /**
   * **主部分（复合单段）**：上/中/下；若紧跟（最多隔一个左括号）还有 上/中/下 ⇒ 复合为「中上」「中下」「上中」等。
   * P0-1c（2026-11-10）：由 `part` + `sub` 两字段合并为**一个复合字段** —— 少一个字段、少一处三端同步面，
   * 且让 `epid` 串自足（`p=中上`）。比较规则见 `partCompatible`（**前缀相容**）。
   */
  part: string
  /** 有意义变体，字典序 `|` 连接 */
  variant: string
  /**
   * **变体序号**（无则 0）：变体词后**紧跟**的数字 —— 「特辑2」⇒ (variant=特辑, variantNum=2)。
   * 必须与变体复合为身份：否则「特辑1..5」坍缩成同一 variant，L3a 组内不唯一 ⇒ 整组配不上
   * （桃花坞6 特辑 4~5 集丢弹幕的根因；P0-1a，2026-11-10 三端同规）。纯日期/长数字不取（见 RE_VARIANT_NUM）。
   */
  variantNum: number
  /** 栏目/系列名（正片为空；连续片段提取，绝不跨段拼字） */
  series: string
}

const RE_DASH_DATE = /((?:19|20)\d{2})[-.](\d{2})[-.](\d{2})/g
const RE_DATE8 = /(?:19|20)\d{6}/
const RE_DATE8_G = /(?:19|20)\d{6}/g
const RE_DI_N = /第\s*(\d{1,4})\s*[期集话回季部]/
const RE_E_NUM = /(?:^|[^A-Za-z])(?:E|EP)\s*0*(\d{1,3})(?:\D|$)/i
const RE_E_NUM_G = /(?:^|[^A-Za-z])(?:E|EP)\s*0*(\d{1,3})(?:\D|$)/gi
const RE_VARIANT_G = /加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外/g
/**
 * 变体序号（P0-1a）：变体词后**紧跟**的数字（最多隔一个左括号/空白）。
 * - `特辑2` / `特辑（2）` → 2
 * - `加更版1` → 不取（隔着「版」⇒ 不算紧跟）
 * - `特辑2026` / `特辑12345` → 不取（避免吞掉日期）
 * 注：用 `(?:[^0-9]|$)` 消费一个后继字符做「后面不再跟数字」的等价断言 —— 与 Go RE2（不支持 lookahead）同写法。
 */
const RE_VARIANT_NUM = /(?:加更|陪看|纯享|先导|超前|彩蛋|特辑|花絮|预告|番外)[\s（(【[《「『]*(\d{1,3})(?:[^0-9]|$)/
const RE_VARIANT_NOISE_G = /会员|未公开|未播|直拍|完整版|未删减/g
const RE_BRACKET_G = /【[^】]*】|（[^）]*）|\([^)]*\)|《[^》]*》|「[^」]*」|『[^』]*』|\[[^\]]*\]/g
const RE_SERIES_STOP = /^正片$|^国语$|^粤语$|^原声$|^普通话$|^双语$/
// 期号宽跨度: 第 + 0~8位数字 + 单位字（吃掉「第20260731期」日期形态, 防 第/期 残渣污染栏目名）
const RE_DI_N_WIDE_G = /第\s*\d{0,8}\s*[期集话回季部]/g
// 季号跨度: 第 + 阿拉伯/中文数字 + 季（分季区分位, 不是栏目名成分）
const RE_SEASON_SPAN_G = /第\s*[\d一二三四五六七八九十零两]+\s*季/g
const SEP = '\x00'

/**
 * 解析**主部分（复合）**：最先出现的 上/中/下；若紧跟（最多隔一个左括号）还有 上/中/下 ⇒ 拼成复合值。
 * 次部分只在紧邻时才算，避免把摘要正文当成分支标记。
 */
function parsePart(s: string): string {
  const rs = [...s]
  const isP = (r: string) => r === '上' || r === '中' || r === '下'
  const pi = rs.findIndex(isP)
  if (pi < 0) return ''
  let out = rs[pi] as string
  let j = pi + 1
  if (j < rs.length && '（(【[《「『'.includes(rs[j] as string)) j++
  if (j < rs.length && isP(rs[j] as string)) out += rs[j] as string
  return out
}

/** 栏目名 v2: 跨度删除 + 最长纯汉字连续段 —— 连续性由构造保证（绝不满段拼字） */
function seriesOf(s: string): string {
  const ci = s.search(/：|:/)
  if (ci >= 0) s = s.slice(0, ci)
  s = s
    .replace(RE_BRACKET_G, SEP)
    // ⚠️ 顺序要紧：必须**先删「第N期/集/话…」再删日期**。
    // 反过来的话，日期会先被替换成占位符（SEP），而 RE_DI_N_WIDE_G 的 `\s*\d{0,8}\s*` 跨不过占位符
    // ⇒ 「第20261005期心动的旅行上」这类**日期样式的期号**会残留一个「期」⇒ 栏目名变成「期心动的旅行」，
    // 与库侧「心动的旅行」对不上（线上真实形态，2026-11-09 实测）。
    .replace(RE_DI_N_WIDE_G, SEP)
    .replace(RE_DATE8_G, SEP)
    .replace(RE_SEASON_SPAN_G, SEP)
    .replace(RE_E_NUM_G, SEP)
    .replace(RE_VARIANT_G, SEP)
    .replace(RE_VARIANT_NOISE_G, SEP)
  let best = ''
  for (const frag of s.split(/[^一-鿿]+/)) {
    if (!frag) continue
    let rs = [...frag]
    // 尾部 part 标记截断（上/中/下总在栏目名之后）
    while (rs.length && (rs[rs.length - 1] === '上' || rs[rs.length - 1] === '中' || rs[rs.length - 1] === '下')) {
      rs.pop()
    }
    // 「季」边界截断
    const ji = rs.indexOf('季')
    if (ji >= 0) rs = rs.slice(0, ji)
    if (rs.length < 2 || rs.length > 8) continue
    const f = rs.join('')
    if (RE_SERIES_STOP.test(f)) continue
    if (rs.length > best.length) best = f
  }
  return best
}

/** 解析集名（与 Go ParseEpisodeKey v2 语义一致） */
export function parseEpisodeKey(title: string): EpisodeKey {
  const k: EpisodeKey = { date: '', num: 0, eNum: 0, part: '', variant: '', variantNum: 0, series: '' }
  let s = (title ?? '').trim()
  if (!s) return k
  // 日期格式归一(先于一切提取): 破折/点分 → 8 位
  s = s.replace(RE_DASH_DATE, '$1$2$3')
  s = s.replace(RE_VARIANT_NOISE_G, '')
  const dm = RE_DATE8.exec(s)
  let dateTail = ''
  if (dm) {
    k.date = dm[0]
    // P2（2026-11-12 评审）: 尾随 1~2 位「当日集序」（不透明长编号 ≥3 位不拆）
    const tailM = /^[0-9]{1,2}(?![0-9])/.exec(s.slice((dm.index ?? 0) + dm[0].length))
    if (tailM) dateTail = tailM[0]
  }
  const nm = s.match(RE_DI_N)
  if (nm) k.num = Number.parseInt(nm[1] as string, 10) || 0
  const em = s.match(RE_E_NUM)
  if (em) k.eNum = Number.parseInt(em[1] as string, 10) || 0
  k.part = parsePart(s)
  // P2（2026-11-12 评审）: 日期尾随「当日集序」进 p 段(parsePart 命中 上/中/下 时不叠加 ——
  // 复合值过不了服务端 epk 的 isPartMark 校验); 与 Go dateSeqTail 同规
  if (dateTail && !k.part) k.part = dateTail
  const vs = [...s.matchAll(RE_VARIANT_G)].map((m) => m[0]).sort()
  // ⚠️ M1（2026-11-11 评审）：变体集合用 `,` 连接而**不是 `|`** —— `|` 是 epid/epk 的段分隔符，
  // 多变体会把串写成 7 段 ⇒ 服务端拒收 + `parseEpid` 恒 null（续播②档失效）+ 落库坏串 ✗
  if (vs.length) k.variant = vs.join(',')
  const vnm = s.match(RE_VARIANT_NUM)
  if (vnm) k.variantNum = Number.parseInt(vnm[1] as string, 10) || 0
  k.series = seriesOf(s)
  return k
}

/**
 * 主部分是否指向同一支 —— **前缀相容**（P0-1c 2026-11-10，替代旧「part 严格 + sub 通配」两段式）：
 * - 一方为空 ⇒ 通配（App 常只有「中」而库里是「中（上）/（下）」）；
 * - 短的是长的前缀 ⇒ 相容：`中` ⊂ `中上`/`中下`；
 * - 互不为前缀 ⇒ 不相容：`中上` ≠ `中下`（**区分仍在**，不会把上下半集混为一集）。
 *
 * 与旧实现逐例等价；收益是"一条规则同时表达宽容与区分"，且不再需要"epid 串 + sameEpisode 必须一起用"的隐式契约。
 */
export function partCompatible(a: EpisodeKey, b: EpisodeKey): boolean {
  if (!a.part || !b.part) return true
  // P2（2026-11-12 评审）: 数字序位（日期尾随「当日集序」）**不做前缀相容** —— 前缀规则是
  // 为「中」⊂「中上」设计的，"1" 与 "12" 是不同集 ⇒ 任一侧为纯数字时要求严格相等。
  if (/^[0-9]+$/.test(a.part) || /^[0-9]+$/.test(b.part)) return a.part === b.part
  return a.part.startsWith(b.part) || b.part.startsWith(a.part)
}

/** SameEpisode′（与 Go SameEpisode v2 同语义）: 日期双有不等 → 否决（硬证据） */
export function sameEpisode(a: EpisodeKey, b: EpisodeKey): boolean {
  if (a.variant !== b.variant || a.variantNum !== b.variantNum || a.series !== b.series) return false
  if (a.date && b.date) {
    if (a.date !== b.date) return false
    return partCompatible(a, b)
  }
  if (a.num > 0 && b.num > 0 && a.num === b.num) return partCompatible(a, b)
  if (a.eNum > 0 && b.eNum > 0 && a.eNum === b.eNum) return true
  return false
}

/** 综艺/衍生形态判定（epid 只给触发的集; 裸「第N集/期」与 E圆号不触发） */
/**
 * 集名是否"可产身份"：**有任何可区分本集的信息**即可（2026-11-11 放宽，与 Go/Java 同规）。
 *
 * 放宽点：**序号 `n`** —— 「第01集」这类纯序号集名在**片内**是明确唯一的（同剧第 3 集 ↔ 另一个源
 * 的第 3 集）⇒ 必须产 `n=3`（旧判据漏了序号，导致连续剧/动漫几乎全空）。
 * 旧设计误加了"必须**全局**唯一"的前提 ✗：实际所有比较都在**片内**（续播在同一部片的源里找、
 * 切源对齐是同片两源、弹幕池键本就带 `mid:`）⇒ 跨片的 `n` 重复不会被比较 ✓
 * 仍不触发：E/EP 圆号（epid 串不含 `e` 段）与无任何信息的裸集名。
 */
export function episodeTriggered(k: EpisodeKey): boolean {
  return !!k.date || !!k.part || !!k.variant || !!k.series || k.num > 0
}

/**
 * 5 段身份串: s|v|p|n|d, 缺值填 `-`（触发的集必有）。
 * `v` 段含**变体序号**：`v=特辑#2`（P0-1a）—— 否则「特辑」与「特辑2」串相同会撞缓存键。
 */
export function epidOf(k: EpisodeKey): string {
  const variant = k.variant && k.variantNum > 0 ? `${k.variant}#${k.variantNum}` : k.variant
  return [
    `s=${k.series || '-'}`,
    `v=${variant || '-'}`,
    `p=${k.part || '-'}`,
    `n=${k.num > 0 ? String(k.num) : '-'}`,
    `d=${k.date || '-'}`
  ].join('|')
}

/**
 * P0-1c（2026-11-10）：**身份串相容**（对齐 ①档用）。与"epid 串全等"只差一处 —— `p` 段按**前缀相容**比较
 * （`中` ⊂ `中上`/`中下`），其余 `s` / `v`（含变体序号）/ `n` / `d` 四段仍要求**全等**。
 *
 * 为什么需要：主部分合并为复合值后，`中` 与 `中（上）` 的 `p` 段不再相等（这正是要的精确性），
 * 但"库侧分了上/下、源侧只有中"的宽容必须保留 ⇒ 用前缀相容还原。
 * 逐例等价于合并前的「`epidOf` 全等 && `sameEpisode`」——旧实现 `p` 段只取主部分，二者串相等，
 * 由 `sameEpisode` 的通配规则放行；现在前移到串比较层，语义一致。
 */
export function epidCompatible(a: EpisodeKey, b: EpisodeKey): boolean {
  if (a.series !== b.series || a.variant !== b.variant || a.variantNum !== b.variantNum) return false
  if (a.num !== b.num || a.date !== b.date) return false
  return partCompatible(a, b)
}

/** P0-3b: 把 epid 串拆成 5 段（缺值 `-` 归一为空串）。空串 / 段数不对 / 含未知键 ⇒ null（当作"无 epid"）。 */
export function parseEpid(epid: string): { s: string; v: string; p: string; n: string; d: string } | null {
  const raw = (epid ?? '').trim()
  if (!raw) return null
  const seg = raw.split('|')
  if (seg.length !== 5) return null
  const out = { s: '', v: '', p: '', n: '', d: '' }
  for (const kv of seg) {
    const i = kv.indexOf('=')
    if (i <= 0) return null
    const key = kv.slice(0, i)
    const val = kv.slice(i + 1) === '-' ? '' : kv.slice(i + 1)
    if (key === 's') out.s = val
    else if (key === 'v') out.v = val
    else if (key === 'p') out.p = val
    else if (key === 'n') out.n = val
    else if (key === 'd') out.d = val
    else return null
  }
  return out
}

/**
 * P0-3b / **方案2 两档身份**（2026-11-10）：在**目标源的集名列表**里按 `epid` 定位集下标；
 * 定位不到返回 -1（调用方回退 `episodeIndex` 下标 —— 方案 §3.1.9 的两级回退）。
 *
 * **① 强等**：`epidOf(该集)` 与存储 epid **串全等** —— 同源集序变化（重排/插集/补集）也能对上。
 *
 * **② 弱等（相容）**：`s` / `v`（含变体序号）必须**相等**；`n` / `p` / `d` 三段**单边缺失视为通配、
 * 双有必须满足各段规则**（`n`/`d` 相等，`p` 前缀相容 `中` ⊂ `中上`）；且**候选唯一**才采纳
 * （多个 ⇒ -1，宁缺勿错）。
 *
 * 为什么需要②档：`bridgeNum` 只补 `n`，而 `p` 是「相容」非「相等」、`d` 可能一侧缺失
 * ⇒ 借到期号后**串仍可能不等** ⇒ 必须由②档兜住这一层。
 *
 * ⚠️ **不采用「把 `d`/`p` 填过来」**：它们是**原始事实**（不是可对齐的序号），填了就成编造，
 * 且会污染 `d` 这条硬证据（"用日期相等判同集、又把日期填过去当证据"＝循环论证）—— 方案 §4「禁止填事实」。
 */
export function indexByEpid(epid: string, titles: string[]): number {
  const want = parseEpid(epid)
  if (!want || !titles || titles.length === 0) return -1
  // F6（2026-11-11 评审；**同日放宽，与 Go/Java 同规**）：只拒绝"**任何段都空**"的串
  //（`s/v/p/d/n` 全空 = 裸集名，没有任何可区分信息 ⇒ 命中只会造成误配）。
  // ⚠️ 不再要求"s/v/p/d 至少一段非空" —— 序号型（只有 `n`）在**片内**明确唯一（连续剧/动漫的
  // 「第01集」），旧守卫会把它们全挡在门外 ✗；"必须全局唯一"本就是错误前提（比较永在片内）。
  if (!want.s && !want.v && !want.p && !want.d && !want.n) return -1
  // ① 强等：整串全等
  for (let i = 0; i < titles.length; i++) {
    if (epidOf(parseEpisodeKey(titles[i] ?? '')) === epid) return i
  }
  // ② 弱等：s/v 相等；n/p/d 单边通配（双有须过各自规则）；候选唯一才采纳
  const wantN = want.n ? Number.parseInt(want.n, 10) || 0 : 0
  let only = -1
  for (let i = 0; i < titles.length; i++) {
    const k = parseEpisodeKey(titles[i] ?? '')
    const kv = k.variant && k.variantNum > 0 ? `${k.variant}#${k.variantNum}` : k.variant
    if (k.series !== want.s || kv !== want.v) continue
    // 期号：**双有必须相等**（不得整体忽略 —— 否则「第3期」会被「第2期」顶上）；单边缺 ⇒ 通配
    if (wantN > 0 && k.num > 0 && wantN !== k.num) continue
    // 日期：双有必须相等（硬证据）；单边缺 ⇒ 通配
    if (want.d && k.date && k.date !== want.d) continue
    // 主次：双有必须前缀相容；单边缺 ⇒ 通配
    if (want.p && k.part && !(k.part.startsWith(want.p) || want.p.startsWith(k.part))) continue
    if (only >= 0) return -1
    only = i
  }
  return only
}

/** P0-1b: 结构化身份传输串的版本前缀（与 Go `EpKeyVersion` 一致；格式变更时同步递增）。 */
export const EPKEY_VERSION = 'v1'
/**
 * P0-1b（2026-11-10）：**结构化身份字段传输串**（HTTP 参数 `epk`）—— 与 Go `ParseEpisodeKeyString` 同格式。
 *
 * 形如 `v1|d=20260803|n=1|e=0|p=中上|v=特辑|vn=2|s=|o=0|from=none`（数值缺省 0、字符串缺省空；
 * P0-1c 起**无独立 `u=` 段**，`p` 即复合主部分；末段 `from=` 为期号来源，P1-3）。
 * 服务端**优先采用**并与 `epTitle` 交叉校验（date/num 冲突以 `epTitle` 为准）⇒ 三端解析若漂移可被检出。
 * `num` 传入 A2 现算期号（桥接/锚点结果，0=未提供）；`ordinal` 为可选序位兜底（本批仅透传）。
 */
export function serializeEpisodeKey(k: EpisodeKey, ordinal = 0, from: EpNumSource = 'none'): string {
  return [
    EPKEY_VERSION,
    `d=${k.date}`,
    `n=${k.num > 0 ? k.num : 0}`,
    `e=${k.eNum > 0 ? k.eNum : 0}`,
    `p=${k.part}`,
    `v=${k.variant}`,
    `vn=${k.variantNum > 0 ? k.variantNum : 0}`,
    `s=${k.series}`,
    `o=${ordinal > 0 ? ordinal : 0}`,
    // P1-3（2026-11-10）：期号来源（self|bridge|anchor|none）—— 供 P1-1 落库统计，**不参与身份判定**
    `from=${from}`
  ].join('|')
}

/**
 * 当前集的传输串。**只有触发的集才发**（未触发返回 `''`，与 `epidFor` 同口径）——
 * 否则裸「第N集」也会多带一份冗余身份，无谓扩大改动面。
 * `from` 传 {@link derivedNumSourceFor} 的结果（需 allTitles，故由调用方算好传入）。
 */
export function epKeyString(title: string, num = 0, ordinal = 0, from: EpNumSource = 'none'): string {
  const k = parseEpisodeKey(title ?? '')
  if (!episodeTriggered(k)) return ''
  if (num > 0) k.num = num
  return serializeEpisodeKey(k, ordinal, from)
}

/**
 * 期号桥接（§5.1 归一）: 在其余源的集名键里找「期号+日期并存、日期相等、栏目/变体一致、part 相容」
 * 的集读其期号; 多个候选期号不一致（错标/缺期漂移）→ 放弃（宁缺勿错）。返回 0 = 桥接失败。
 */
export function bridgeNum(cur: EpisodeKey, otherKeys: EpisodeKey[]): number {
  const seen = new Set<number>()
  for (const k of otherKeys) {
    if (k.num === 0 || !k.date || k.date !== cur.date) continue
    if (k.variant !== cur.variant || k.series !== cur.series) continue
    if (!partCompatible(cur, k)) continue
    seen.add(k.num)
  }
  return seen.size === 1 ? [...seen][0] ?? 0 : 0
}

/** 当前集的 epid（触发的集必有; 期号缺失经桥接归一写入 n 段）。未触发返回 ''（调用方回退旧池键） */
export function epidFor(title: string, otherTitles: string[]): string {
  const k = parseEpisodeKey(title)
  if (!episodeTriggered(k)) return ''
  if (k.num === 0 && k.date) {
    const n = bridgeNum(k, otherTitles.map((t) => parseEpisodeKey(t)))
    if (n > 0) k.num = n
  }
  return epidOf(k)
}

/**
 * 锚点期号（**单源片**专用兜底）—— 期号 = 本片「正片『上』」日期集合中**不晚于本集日期**的个数。
 *
 * <p>为什么需要：`bridgeNum`（他源桥接）要从**别的源**借期号，单源片没有源可借 ⇒ 期号恒 0 ⇒
 * 这类综艺（App 侧只有日期、库侧只有期号）永远查不到弹幕。实测（心动9，2026-11-09）：
 * 现规则 0% → 锚点法 34/133 = 25.6%，且**推算期号与库侧期号 34/34 完全一致**。
 *
 * <p>为什么不用"7 天一期"：跨季/断更/非周更都不成立；「每期必有『上』、『上』即该期首播日」
 * 是综艺的普遍规律，按日期数「上」即可（断更/跨季/一天多期都自洽）。
 *
 * <p>口径（用户 2026-11-09 拍板）：**只数正片** —— 排除衍生栏目（`series` 非空）与变体（`variant` 非空），
 * 否则「心动的周末/旅行」会把正片序带偏。保守条件：正片「上」不足 2 个、或本集非正片/无日期 → 返回 0（不猜）。
 *
 * <p>注：入参 `allTitles` 允许是"本片所有源的所有集名" —— 内部按**日期去重**，同一期在多个源里只算一次。
 */
export function anchorNumFor(title: string, allTitles: string[]): number {
  const k = parseEpisodeKey(title ?? '')
  if (!k.date || k.variant) return 0
  const dates: string[] = []
  for (const t of allTitles ?? []) {
    const o = parseEpisodeKey(t ?? '')
    // ⚠️ 只数**同一栏目组**的「上」：正片组 series=''；衍生栏目（心动的周末/线上集结篇…）各数各的。
    // 2026-11-09 由"只数正片"扩展为"按 series 分组" —— 否则「心动的周末上」永远拿不到期号 1/2/3。
    if (o.series !== k.series || o.variant) continue
    if (!o.part.startsWith('上')) continue
    if (o.date) dates.push(o.date)
  }
  const uniq = [...new Set(dates)].sort()
  if (uniq.length < 2) return 0
  let n = 0
  for (const d of uniq) {
    if (d <= k.date) n++
  }
  return n
}

/** A2 现算期号（传给聚合接口 epNum）: 自带期号直接用 → 他源桥接 → **锚点兜底**；都不行返回 0 */
export function derivedNumFor(title: string, otherTitles: string[]): number {
  const k = parseEpisodeKey(title)
  if (!episodeTriggered(k)) return 0
  if (k.num > 0) return k.num
  if (!k.date) return 0
  const bridged = bridgeNum(k, otherTitles.map((t) => parseEpisodeKey(t)))
  if (bridged > 0) return bridged
  return anchorNumFor(title, otherTitles)
}

/** 期号来源（P1-3）：自带 / 他源桥接 / 锚点推算 / 无 */
export type EpNumSource = 'self' | 'bridge' | 'anchor' | 'none'

/**
 * P1-3（2026-11-10）：**期号的来源** —— 判据与 {@link derivedNumFor} **严格同一降级顺序**
 * （自带 → 他源桥接 → 锚点），只做**标注**、不参与身份判定。
 *
 * 用途：写进 `epk` 的 `from=` 段 ⇒ 服务端 **P1-1 落库时一并记录** ⇒ 来源占比与准确性统计
 * 由落库结果直接得到（**不必另行做在线打点**）。**不进 `epid`** ⇒ 缓存键不变、零格式破坏。
 */
export function derivedNumSourceFor(title: string, allTitles: string[]): EpNumSource {
  const k = parseEpisodeKey(title ?? '')
  if (!episodeTriggered(k)) return 'none'
  if (k.num > 0) return 'self'
  if (!k.date) return 'none'
  if (bridgeNum(k, (allTitles ?? []).map((t) => parseEpisodeKey(t ?? ''))) > 0) return 'bridge'
  if (anchorNumFor(title ?? '', allTitles ?? []) > 0) return 'anchor'
  return 'none'
}

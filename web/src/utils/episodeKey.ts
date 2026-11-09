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
  /** 主部分 上/中/下 */
  part: string
  /** 次部分（紧邻主部分） */
  sub: string
  /** 有意义变体，字典序 `|` 连接 */
  variant: string
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
const RE_VARIANT_NOISE_G = /会员|未公开|未播|直拍|完整版|未删减/g
const RE_BRACKET_G = /【[^】]*】|（[^）]*）|\([^)]*\)|《[^》]*》|「[^」]*」|『[^』]*』|\[[^\]]*\]/g
const RE_SERIES_STOP = /^正片$|^国语$|^粤语$|^原声$|^普通话$|^双语$/
// 期号宽跨度: 第 + 0~8位数字 + 单位字（吃掉「第20260731期」日期形态, 防 第/期 残渣污染栏目名）
const RE_DI_N_WIDE_G = /第\s*\d{0,8}\s*[期集话回季部]/g
// 季号跨度: 第 + 阿拉伯/中文数字 + 季（分季区分位, 不是栏目名成分）
const RE_SEASON_SPAN_G = /第\s*[\d一二三四五六七八九十零两]+\s*季/g
const SEP = '\x00'

/** 主/次部分: 最先出现的 上/中/下 + 紧邻（最多隔一个左括号）的第二个 */
function partAndSub(s: string): [string, string] {
  const rs = [...s]
  const isP = (r: string) => r === '上' || r === '中' || r === '下'
  const pi = rs.findIndex(isP)
  if (pi < 0) return ['', '']
  const part = rs[pi] as string
  let j = pi + 1
  if (j < rs.length && '（(【[《「『'.includes(rs[j] as string)) j++
  const sub = j < rs.length && isP(rs[j] as string) ? (rs[j] as string) : ''
  return [part, sub]
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
  const k: EpisodeKey = { date: '', num: 0, eNum: 0, part: '', sub: '', variant: '', series: '' }
  let s = (title ?? '').trim()
  if (!s) return k
  // 日期格式归一(先于一切提取): 破折/点分 → 8 位
  s = s.replace(RE_DASH_DATE, '$1$2$3')
  s = s.replace(RE_VARIANT_NOISE_G, '')
  const dm = s.match(RE_DATE8)
  if (dm) k.date = dm[0]
  const nm = s.match(RE_DI_N)
  if (nm) k.num = Number.parseInt(nm[1] as string, 10) || 0
  const em = s.match(RE_E_NUM)
  if (em) k.eNum = Number.parseInt(em[1] as string, 10) || 0
  const [part, sub] = partAndSub(s)
  k.part = part
  k.sub = sub
  const vs = [...s.matchAll(RE_VARIANT_G)].map((m) => m[0]).sort()
  if (vs.length) k.variant = vs.join('|')
  k.series = seriesOf(s)
  return k
}

/** 主/次部分相容: 一方缺失视为通配（App 常只有「中」而库里是「中（上）/（下）」） */
export function partCompatible(a: EpisodeKey, b: EpisodeKey): boolean {
  if (a.part && b.part && a.part !== b.part) return false
  if (a.sub && b.sub && a.sub !== b.sub) return false
  return true
}

/** SameEpisode′（与 Go SameEpisode v2 同语义）: 日期双有不等 → 否决（硬证据） */
export function sameEpisode(a: EpisodeKey, b: EpisodeKey): boolean {
  if (a.variant !== b.variant || a.series !== b.series) return false
  if (a.date && b.date) {
    if (a.date !== b.date) return false
    return partCompatible(a, b)
  }
  if (a.num > 0 && b.num > 0 && a.num === b.num) return partCompatible(a, b)
  if (a.eNum > 0 && b.eNum > 0 && a.eNum === b.eNum) return true
  return false
}

/** 综艺/衍生形态判定（epid 只给触发的集; 裸「第N集/期」与 E圆号不触发） */
export function episodeTriggered(k: EpisodeKey): boolean {
  return !!k.date || !!k.part || !!k.variant || !!k.series
}

/** 5 段身份串: s|v|p|n|d, 缺值填 `-`（触发的集必有） */
export function epidOf(k: EpisodeKey): string {
  return [
    `s=${k.series || '-'}`,
    `v=${k.variant || '-'}`,
    `p=${k.part || '-'}`,
    `n=${k.num > 0 ? String(k.num) : '-'}`,
    `d=${k.date || '-'}`
  ].join('|')
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

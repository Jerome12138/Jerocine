import { describe, expect, it } from 'vitest'
import {
  bridgeNum,
  derivedNumFor,
  derivedNumSourceFor,
  epKeyString,
  EPKEY_VERSION,
  epidCompatible,
  epidFor,
  epidOf,
  indexByEpid,
  indexByServerEpid,
  epidHash,
  parseEpid,
  partCompatible,
  parseEpisodeKey,
  sameEpisode,
  serializeEpisodeKey
} from './episodeKey'

// 黄金用例 —— 与 server/internal/danmu/episode_key_test.go 同一组(三端同规, 任一侧改动必须同组通过)
describe('parseEpisodeKey 现网真实集名', () => {
  it('基础形态', () => {
    expect(parseEpisodeKey('20260803(第1期上)')).toEqual({
      date: '20260803', num: 1, eNum: 0, part: '上', variant: '', variantNum: 0, series: ''
    })
    expect(parseEpisodeKey('20260818中（下）')).toMatchObject({ date: '20260818', part: '中下' })
    expect(parseEpisodeKey('第3期中（上）')).toMatchObject({ num: 3, part: '中上' })
    expect(parseEpisodeKey('20260128未公开特辑')).toMatchObject({ date: '20260128', variant: '特辑' })
    expect(parseEpisodeKey('E01.220705')).toMatchObject({ eNum: 1 })
    expect(parseEpisodeKey('第1集')).toMatchObject({ num: 1 })
    expect(parseEpisodeKey('正片')).toEqual(parseEpisodeKey(''))
  })

  it('v2 日期三格式归一(8位/破折/点分)', () => {
    expect(parseEpisodeKey('2026-10-10期')).toMatchObject({ date: '20261010' })
    expect(parseEpisodeKey('第20261010期')).toMatchObject({ date: '20261010' })
    expect(parseEpisodeKey('2022.05.01期')).toMatchObject({ date: '20220501' })
    expect(
      parseEpisodeKey('【qq】 第1期上：出发团变落汤鸡，沈腾被迫洗头 2026-10-10期')
    ).toMatchObject({ num: 1, part: '上', date: '20261010' })
  })

  it('v2 栏目名连续片段(残渣不污染/季跨度删除/描述拒识)', () => {
    // 「先导片」是变体: 变体身份优先, 栏目名为空
    expect(parseEpisodeKey('第20260731期先导片上')).toMatchObject({
      date: '20260731', part: '上', variant: '先导', series: ''
    })
    expect(parseEpisodeKey('王牌对王牌第七季2022.05.01期-会员版')).toMatchObject({
      date: '20220501', series: '王牌对王牌'
    })
    // 13 字自由描述 → 拒识
    expect(parseEpisodeKey('檀健次带北江小分队回好六了！ 2025-01-04期')).toMatchObject({
      date: '20250104', series: ''
    })
    expect(parseEpisodeKey('20261027心动的周末第3期下')).toMatchObject({
      date: '20261027', num: 3, part: '下', series: '心动的周末'
    })
    expect(parseEpisodeKey('20261012心动的周末上')).toMatchObject({
      date: '20261012', part: '上', series: '心动的周末'
    })
    expect(parseEpisodeKey('【qq】 第1期上：又争又抢，心动了就要打明牌！')).toMatchObject({ series: '' })
  })
})

describe('sameEpisode(SameEpisode′)', () => {
  it('期号/变体/part 通配规则', () => {
    const app = parseEpisodeKey('20260803(第1期上)')
    expect(sameEpisode(app, parseEpisodeKey('第1期上'))).toBe(true)
    expect(sameEpisode(app, parseEpisodeKey('第1期'))).toBe(true)
    expect(sameEpisode(parseEpisodeKey('20260806(第1期加更上)'), parseEpisodeKey('第1期上'))).toBe(false)
    expect(sameEpisode(parseEpisodeKey('20260803上'), parseEpisodeKey('20260803中'))).toBe(false)
    expect(sameEpisode(parseEpisodeKey('20260818中（下）'), parseEpisodeKey('20260818中（上）'))).toBe(false)
    expect(
      sameEpisode(parseEpisodeKey('心动的周末第1期上：高甜游戏'), parseEpisodeKey('第1期上：又争又抢'))
    ).toBe(false)
    expect(sameEpisode(parseEpisodeKey('正片'), parseEpisodeKey('正片'))).toBe(false)
  })

  it('v2 日期三格式归一 + 日期硬否决', () => {
    // 心动9 真实场景: App 纯日期 vs 库「期号+破折日期」→ 归一后直等
    expect(
      sameEpisode(parseEpisodeKey('20261010上'), parseEpisodeKey('【qq】 第1期上：出发团变落汤鸡 2026-10-10期'))
    ).toBe(true)
    expect(sameEpisode(parseEpisodeKey('2022.05.01期'), parseEpisodeKey('20220501'))).toBe(true)
    // 日期双有不等 → 否决(即使期号相等)
    expect(sameEpisode(parseEpisodeKey('20260817第3期上'), parseEpisodeKey('20260810第3期上'))).toBe(false)
    // 一方只有期号 → 不触发否决, 走期号规则
    expect(sameEpisode(parseEpisodeKey('20260817第3期上'), parseEpisodeKey('第3期上'))).toBe(true)
  })
})

describe('epid', () => {
  it('5 段定长, 缺值填 -', () => {
    expect(epidOf(parseEpisodeKey('正片第1期上'))).toBe('s=-|v=-|p=上|n=1|d=-')
    expect(epidOf(parseEpisodeKey('20260803上'))).toBe('s=-|v=-|p=上|n=-|d=20260803')
    expect(epidOf(parseEpisodeKey('20260806加更上'))).toBe('s=-|v=加更|p=上|n=-|d=20260806')
  })

  it('触发判定: 序号型触发（片内唯一即够）；E圆号不触发', () => {
    // 2026-11-11 放宽：纯序号「第3集」在**片内**明确唯一 ⇒ 必须产 `n=3`
    expect(epidFor('第3集', [])).toBe('s=-|v=-|p=-|n=3|d=-')
    // E 圆号仍不触发（epid 串不含 e 段；且不同源 E 编号常漂）
    expect(epidFor('E01.220705', [])).toBe('')
    expect(epidFor('20260803上', [])).toBe('s=-|v=-|p=上|n=-|d=20260803')
  })
})

describe('P0-1a 变体序号（变体词后紧跟的数字必须复合为身份）', () => {
  it('解析: 特辑2/特辑（2）/特辑12 取序号; 无序号与长数字不取', () => {
    expect(parseEpisodeKey('特辑2')).toMatchObject({ variant: '特辑', variantNum: 2, series: '' })
    expect(parseEpisodeKey('20260128特辑2')).toMatchObject({
      date: '20260128', variant: '特辑', variantNum: 2
    })
    expect(parseEpisodeKey('第1期特辑5上')).toMatchObject({
      num: 1, part: '上', variant: '特辑', variantNum: 5
    })
    expect(parseEpisodeKey('特辑（2）')).toMatchObject({ variant: '特辑', variantNum: 2 })
    expect(parseEpisodeKey('特辑12')).toMatchObject({ variant: '特辑', variantNum: 12 })
    // 序号不紧跟（隔着「版」）→ 不取
    expect(parseEpisodeKey('第1期加更版1')).toMatchObject({ num: 1, variant: '加更', variantNum: 0 })
    // 纯日期/长数字 → 不取（避免吞日期）
    expect(parseEpisodeKey('特辑2026')).toMatchObject({ variant: '特辑', variantNum: 0 })
    expect(parseEpisodeKey('20260128特辑')).toMatchObject({
      date: '20260128', variant: '特辑', variantNum: 0
    })
    // 无序号变体 + 栏目名
    expect(parseEpisodeKey('夏日清凉特辑')).toMatchObject({
      variant: '特辑', variantNum: 0, series: '夏日清凉'
    })
  })

  it('sameEpisode: 序号纳入身份', () => {
    expect(sameEpisode(parseEpisodeKey('特辑2'), parseEpisodeKey('特辑3'))).toBe(false)
    expect(sameEpisode(parseEpisodeKey('特辑2'), parseEpisodeKey('特辑'))).toBe(false)
    // 两侧都无期号/日期 ⇒ 不判同（交 L3a 组内唯一兜底）
    expect(sameEpisode(parseEpisodeKey('特辑2'), parseEpisodeKey('特辑2'))).toBe(false)
    expect(sameEpisode(parseEpisodeKey('第1期特辑2'), parseEpisodeKey('第1期特辑2'))).toBe(true)
    expect(sameEpisode(parseEpisodeKey('第1期特辑2'), parseEpisodeKey('第1期特辑3'))).toBe(false)
  })

  it('epid: v 段含序号（特辑 vs 特辑2 不得撞键）', () => {
    expect(epidOf(parseEpisodeKey('特辑2'))).toBe('s=-|v=特辑#2|p=-|n=-|d=-')
    expect(epidOf(parseEpisodeKey('20260128特辑2'))).toBe('s=-|v=特辑#2|p=-|n=-|d=20260128')
    expect(epidOf(parseEpisodeKey('特辑'))).toBe('s=-|v=特辑|p=-|n=-|d=-')
    expect(epidOf(parseEpisodeKey('夏日清凉特辑'))).toBe('s=夏日清凉|v=特辑|p=-|n=-|d=-')
  })
})

describe('P0-1b/P0-1c 结构化身份传输串（epk）', () => {
  it('序列化：与 Go EpisodeKey.Serialize 同格式（数值缺省 0 / 字符串缺省空；p 段为复合主部分）', () => {
    expect(
      serializeEpisodeKey({
        date: '20260803', num: 1, eNum: 2, part: '中上',
        variant: '特辑', variantNum: 2, series: '心动的周末'
      }, 5, 'self')
    ).toBe('v1|d=20260803|n=1|e=2|p=中上|v=特辑|vn=2|s=心动的周末|o=5|from=self')
    expect(EPKEY_VERSION).toBe('v1')
    // 零键（from 缺省 = none）
    expect(
      serializeEpisodeKey({ date: '', num: 0, eNum: 0, part: '', variant: '', variantNum: 0, series: '' })
    ).toBe('v1|d=|n=0|e=0|p=|v=|vn=0|s=|o=0|from=none')
  })

  it('epKeyString：只有触发的集才发（未触发为空串，与 epidFor 同口径）', () => {
    // 2026-11-11 放宽：序号型也算触发 ⇒ 会带上 `n=3`（不再要求"有栏目/日期"）
    expect(epKeyString('第3集')).toContain('n=3')
    expect(epKeyString('E01.220705')).toBe('')
    expect(epKeyString('')).toBe('')
    // 触发的集：s/v/vn/p/n/d 全在串里（P0-1c 后 p 段即复合主部分，不再有独立 u 段）
    expect(epKeyString('20260803(第1期上)')).toBe('v1|d=20260803|n=1|e=0|p=上|v=|vn=0|s=|o=0|from=none')
    expect(epKeyString('第1期特辑2上')).toBe('v1|d=|n=1|e=0|p=上|v=特辑|vn=2|s=|o=0|from=none')
    expect(epKeyString('20260818中（下）')).toBe('v1|d=20260818|n=0|e=0|p=中下|v=|vn=0|s=|o=0|from=none')
  })

  it('epKeyString：A2 现算期号写入 n 段（桥接/锚点结果不得丢）', () => {
    // 纯日期形态自身无期号，桥接得 10 ⇒ n=10（与 epidFor 同口径）
    expect(epKeyString('20261005上', 10)).toBe('v1|d=20261005|n=10|e=0|p=上|v=|vn=0|s=|o=0|from=none')
    // num 参数与服务端 appNum 同语义：非 0 即覆盖（客户端传的就是 derivedNumFor 结果 ⇒ 实践中与自带期号一致）
    expect(epKeyString('20260803(第1期上)', 9)).toBe('v1|d=20260803|n=9|e=0|p=上|v=|vn=0|s=|o=0|from=none')
  })
})

describe('M1 多变体分隔符（不得撞 epid/epk 的段分隔符 |）', () => {
  it('变体集合用「,」连接 ⇒ epid 恒 5 段、epk 段数固定、parseEpid 不 null', () => {
    const k = parseEpisodeKey('20260803(第1期上)特辑花絮')
    expect(k.variant).toContain(',')
    expect(k.variant).not.toContain('|')
    expect(k.variant).toContain('特辑')
    expect(k.variant).toContain('花絮')

    const ep = epidOf(k)
    expect(ep.split('|')).toHaveLength(5) // 段数恒 5 —— 多变体也不撞
    expect(ep).toContain(`v=${k.variant}`)
    expect(parseEpid(ep)).not.toBeNull() // web 侧解析不得 null（否则续播②档失效）

    // epk：版本段 + 9 个字段段 = 10 段（多变体不得撑破）
    expect(serializeEpisodeKey(k).split('|')).toHaveLength(10)
    // 单变体回归
    const one = parseEpisodeKey('20260803(第1期上)特辑')
    expect(one.variant).toBe('特辑')
    expect(epidOf(one).split('|')).toHaveLength(5)
  })
})

describe('P1-3 期号来源标注（epk 的 from= 段；不进 epid）', () => {
  it('derivedNumSourceFor：与 derivedNumFor 同一降级顺序（self → bridge → anchor → none）', () => {
    const others = ['20260803第1期上', '20260810第2期上']
    // 自带期号 ⇒ self
    expect(derivedNumSourceFor('20260803第1期上', others)).toBe('self')
    // 无期号但有日期、他源可桥接（日期相等 + 栏目/变体一致 + 主次相容）⇒ bridge
    expect(derivedNumSourceFor('20260803上', others)).toBe('bridge')
    // 桥上不了（others 里没有同日期带期号的集）⇒ 锚点（同栏目组「上」日期 ≥2）
    expect(derivedNumSourceFor('20260803上', ['20260803上', '20260810上', '20260817上'])).toBe('anchor')
    // 无日期 ⇒ 既不能桥接也不能锚点 ⇒ none
    expect(derivedNumSourceFor('特辑', others)).toBe('none')
    // 纯序号「第N集」自带序号 ⇒ 来源=self（2026-11-11 放宽后它也算触发）
    expect(derivedNumSourceFor('第3集', others)).toBe('self')
  })

  it('from 只在 epk 里；epid 不受影响（缓存键不变）', () => {
    expect(epKeyString('20260803上', 1, 0, 'anchor')).toMatch(/\|from=anchor$/)
    expect(epKeyString('20260803上', 1)).toMatch(/\|from=none$/)
    expect(epidOf(parseEpisodeKey('20260803上'))).toBe('s=-|v=-|p=上|n=-|d=20260803')
  })
})

describe('P0-1c 主部分合并 + 前缀相容（epid 自足）', () => {
  it('epid：p 段为复合主部分（中上 ≠ 中；中上 ≠ 中下）', () => {
    expect(epidOf(parseEpisodeKey('第3期中（上）'))).toBe('s=-|v=-|p=中上|n=3|d=-')
    expect(epidOf(parseEpisodeKey('第3期中（下）'))).toBe('s=-|v=-|p=中下|n=3|d=-')
    expect(epidOf(parseEpisodeKey('第3期中'))).toBe('s=-|v=-|p=中|n=3|d=-')
  })

  it('epidCompatible：p 段前缀相容，其余段要求全等', () => {
    const zhong = parseEpisodeKey('第3期中')
    const shang = parseEpisodeKey('第3期中（上）')
    const xia = parseEpisodeKey('第3期中（下）')
    expect(epidCompatible(zhong, shang)).toBe(true) // 中 ⊂ 中上
    expect(epidCompatible(zhong, xia)).toBe(true) // 中 ⊂ 中下
    expect(epidCompatible(shang, xia)).toBe(false) // 中上 ≠ 中下（区分仍在）
    expect(epidCompatible(shang, shang)).toBe(true)
    // 其余段必须全等
    expect(epidCompatible(parseEpisodeKey('第1期中'), parseEpisodeKey('第3期中'))).toBe(false)
    expect(epidCompatible(parseEpisodeKey('20260803中'), parseEpisodeKey('第3期中'))).toBe(false)
  })

  it('partCompatible：前缀相容（互不为前缀 ⇒ 不相容；一方为空 ⇒ 通配）', () => {
    expect(partCompatible(parseEpisodeKey('第1期上'), parseEpisodeKey('第1期中'))).toBe(false)
    expect(partCompatible(parseEpisodeKey('EP8上'), parseEpisodeKey('EP8上中'))).toBe(true)
    expect(partCompatible(parseEpisodeKey('正片'), parseEpisodeKey('第1期中'))).toBe(true)
  })
})

describe('P0-3b 续播定位（parseEpid / indexByEpid）', () => {
  it('parseEpid：5 段解析；缺值 - 归空；空串/段数不对/未知键 ⇒ null', () => {
    expect(parseEpid('s=-|v=-|p=中上|n=3|d=-')).toEqual({ s: '', v: '', p: '中上', n: '3', d: '' })
    expect(parseEpid('s=心动的周末|v=特辑#2|p=-|n=-|d=20260930')).toEqual({
      s: '心动的周末', v: '特辑#2', p: '', n: '', d: '20260930'
    })
    expect(parseEpid('')).toBeNull()
    expect(parseEpid('   ')).toBeNull()
    expect(parseEpid('s=-|v=-')).toBeNull()
    expect(parseEpid('s=-|v=-|p=-|n=-|zz=-')).toBeNull()
  })

  it('indexByEpid ①严格：同源集序变化（重排/插集）也能定位回原来那一集', () => {
    const ep = epidOf(parseEpisodeKey('20260803上'))
    expect(indexByEpid(ep, ['第1集', '第2集', '20260803上', '20260803中'])).toBe(2)
    // 目标源插了「第9集」并重排 ⇒ 老下标 2 已失效，epid 仍命中新下标 0
    expect(indexByEpid(ep, ['20260803上', '第1集', '第9集', '第2集', '20260803中'])).toBe(0)
  })

  it('indexByEpid ②弱等（相容）：s/v 相等；n/p/d 单边通配、双有须过各自规则；候选唯一才采纳', () => {
    const titles = ['20260803上', '20260803中', '20260804下']
    // 心动9 真实形态：App 纯日期，记录时 epid 的 n=1 是桥接值，本地 epidOf 得到 n=- ⇒ 单边缺 ⇒ 通配
    expect(indexByEpid('s=-|v=-|p=上|n=1|d=20260803', titles)).toBe(0)
    // p 段为空 ⇒ 同日期两条候选 ⇒ 不唯一 ⇒ 放弃（宁缺勿错，交回退）
    expect(indexByEpid('s=-|v=-|p=-|n=1|d=20260803', titles)).toBe(-1)
    // ⚠️ 期号**双有不等** ⇒ 拒绝（不得整体忽略 n —— 否则「第3期」会被「第2期」顶上，配错集）
    expect(indexByEpid('s=-|v=-|p=上|n=3|d=-', ['第2期上', '第4期上'])).toBe(-1)
    expect(indexByEpid('s=-|v=-|p=上|n=2|d=-', ['第2期上'])).toBe(0)
    // 日期**单边缺失** ⇒ 通配 ⇒ 仍可命中（桥接只补 n，d 一侧可能没有）
    expect(indexByEpid('s=-|v=-|p=上|n=1|d=-', ['20260803第1期上'])).toBe(0)
    // F6（2026-11-11 评审；**同日放宽**）：只拒绝"**任何段都空**"的串（`s/v/p/d/n` 全空）。
    // 序号型（只有 `n`）在**片内**是明确唯一的 ⇒ 必须放行 —— 旧守卫要求 s/v/p/d 至少一段非空，
    // 会把连续剧/动漫（集名=「第01集」）全挡在门外 ✗；而"必须全局唯一"本就是错误前提
    //（比较永在片内：续播同片源内、切源同片两源、弹幕池键带 mid:）。
    expect(indexByEpid('s=-|v=-|p=-|n=1|d=-', ['20260803第1期'])).toBe(0)
    expect(indexByEpid('s=-|v=-|p=-|n=3|d=-', ['第3集', '第4集'])).toBe(0)
    // 真·裸串（五段全空）仍拒绝：没有任何可区分信息，命中只会造成误配
    expect(indexByEpid('s=-|v=-|p=-|n=-|d=-', ['第3集', '第4集'])).toBe(-1)
    // 日期**双有不等** ⇒ 硬证据 ⇒ 拒绝
    expect(indexByEpid('s=-|v=-|p=上|n=-|d=20260810', ['20260803上'])).toBe(-1)
  })

  it('indexByEpid：对不上 / 空列表 / 坏串 ⇒ -1（调用方回退 episode 下标）', () => {
    expect(indexByEpid('s=-|v=-|p=下|n=-|d=20261231', ['20260803上'])).toBe(-1)
    expect(indexByEpid('s=-|v=-|p=上|n=-|d=20260803', [])).toBe(-1)
    expect(indexByEpid('坏串', ['20260803上'])).toBe(-1)
    expect(indexByEpid('', ['20260803上'])).toBe(-1)
  })
})

describe('bridgeNum / A2 现算期号(心动9 真实场景)', () => {
  // lz 纯日期形态; bf/sn 日期+期号形态 —— 与实测一致
  const lzTitles = ['20260731先导片上', '20260803上', '20260803中', '20261005上', '20261012心动的周末上']
  const otherTitles = [
    '20260803第1期上',
    '20260803第1期中',
    '20260810第2期中',
    '20261005第10期上',
    '20261012心动的周末第1期上',
    '20260731第0期先导片上'
  ]

  it('桥接: 日期相等 + 栏目/变体一致 + part 相容 → 读期号', () => {
    expect(derivedNumFor('20260803上', otherTitles)).toBe(1)
    expect(derivedNumFor('20261005上', otherTitles)).toBe(10)
    expect(derivedNumFor('20261012心动的周末上', otherTitles)).toBe(1)
  })

  it('自集无期号且无桥接 → 0；纯序号自带序号 → 取自身', () => {
    expect(derivedNumFor('20260731先导片上', otherTitles)).toBe(0) // 变体不同不桥接
    // 2026-11-11 放宽：序号型也算触发 ⇒ 现算期号取**自带序号** 3
    expect(derivedNumFor('第3集', otherTitles)).toBe(3)
  })

  it('epidFor: 桥接期号写入 n 段 → 跨源同集 epid 串相等', () => {
    const lzEpid = epidFor('20261005上', [...otherTitles, ...lzTitles])
    const bfEpid = epidOf(parseEpisodeKey('20261005第10期上'))
    expect(lzEpid).toBe('s=-|v=-|p=上|n=10|d=20261005')
    expect(lzEpid).toBe(bfEpid)
  })

  it('桥接候选期号冲突 → 放弃(0)', () => {
    expect(bridgeNum(parseEpisodeKey('20260803上'), [parseEpisodeKey('20260803第1期上'), parseEpisodeKey('20260803第2期上')])).toBe(0)
  })
})

describe('P2 日期尾随集序（2026-11-12 评审）', () => {
  it('date+1位集序进 p 段, 两集 epid 区分', () => {
    const k1 = parseEpisodeKey('202401011')
    expect(k1.date).toBe('20240101')
    expect(k1.part).toBe('1')
    const k2 = parseEpisodeKey('202401012')
    expect(k2.part).toBe('2')
    expect(epidOf(k1)).not.toBe(epidOf(k2))
    // 数字序不做前缀相容: p=1 与 p=2 不同集; 同名重解析判同
    expect(sameEpisode(k1, k2)).toBe(false)
    expect(sameEpisode(k1, parseEpisodeKey('202401011'))).toBe(true)
  })

  it('续播定位: 各自 epid 强等命中各自下标(旧行为两串相同错配到第 1 集)', () => {
    const titles = ['202401011', '202401012']
    expect(indexByEpid('s=-|v=-|p=1|n=-|d=20240101', titles)).toBe(0)
    expect(indexByEpid('s=-|v=-|p=2|n=-|d=20240101', titles)).toBe(1)
  })

  it('回归: 纯日期不带序号行为不变; 不透明长编号(≥3 位尾随)不拆序号', () => {
    expect(parseEpisodeKey('20240101').part).toBe('')
    const k3 = parseEpisodeKey('20240101113')
    expect(k3.date).toBe('20240101')
    expect(k3.part).toBe('')
  })
})

// ===== 2026-11-16 端侧判等简化 + epid 短码（hash 形态）=====
//
// `epidHash` 必须与 Go `danmu.EpidHash` / android `EpisodeKey.epidHash` **同算法**
// （FNV-1a 64 → base36 小写、按 UTF-8 字节哈希）—— 下面的"黄金值"由 Go 侧实测产出，改算法即红。
describe('epidHash（三端同规锚点）', () => {
  it('黄金值（Go 实测）', () => {
    expect(epidHash('s=-|v=-|p=-|n=1|d=-')).toBe('2y60kj4pkp7rs')
    expect(epidHash('s=王牌对王牌|v=-|p=-|n=5|d=-')).toBe('1vo45avj8hymg')
    expect(epidHash('s=-|v=特辑|p=中|n=12|d=20260803')).toBe('tk06iynry8kt')
  })
  // 纯 JS 32 位半字实现 vs BigInt 参考实现（2026-11-20 重写后的等价性锚定）
  it('纯JS实现与 BigInt 参考实现随机交叉一致', () => {
    const ref = (s: string): string => {
      const bytes = new TextEncoder().encode(s)
      let h = 0xcbf29ce484222325n
      const prime = 0x100000001b3n
      for (let i = 0; i < bytes.length; i++) { h ^= BigInt(bytes[i] as number); h = (h * prime) & 0xffffffffffffffffn }
      return h.toString(36)
    }
    for (let i = 0; i < 300; i++) {
      const s = 'x=' + Math.random().toString(36).slice(2) + String(i)
      expect(epidHash(s)).toBe(ref(s))
    }
  })
  it('确定性 / 纯 ASCII / 长度 ≤13 / 异串异码', () => {
    const a = epidHash('s=-|v=-|p=-|n=1|d=-')
    expect(epidHash('s=-|v=-|p=-|n=1|d=-')).toBe(a)
    expect(a).toMatch(/^[0-9a-z]{1,13}$/)
    expect(epidHash('s=-|v=-|p=-|n=2|d=-')).not.toBe(a)
  })
})

describe('indexByServerEpid（端侧只判等，不再宽容）', () => {
  const epids = ['s=-|v=-|p=-|n=1|d=-', 's=-|v=-|p=-|n=2|d=-', 's=-|v=-|p=-|n=3|d=-']
  it('全等命中 ⇒ 下标', () => {
    expect(indexByServerEpid('s=-|v=-|p=-|n=1|d=-', epids)).toBe(0)
    expect(indexByServerEpid('s=-|v=-|p=-|n=3|d=-', epids)).toBe(2)
  })
  it('对不上 ⇒ -1（调用方退化 episodeIndex）', () => {
    expect(indexByServerEpid('s=-|v=-|p=-|n=9|d=-', epids)).toBe(-1)
    // ⚠️ 不再有单边通配 / p 前缀相容：`p=中` 对 `p=中上` 判**不中**（旧 indexByEpid 的②档行为已废弃）
    expect(indexByServerEpid('s=-|v=-|p=中|n=1|d=-', ['s=-|v=-|p=中上|n=1|d=-'])).toBe(-1)
  })
  it('空值/空列表/坏串 ⇒ -1', () => {
    expect(indexByServerEpid('', epids)).toBe(-1)
    expect(indexByServerEpid('s=-|v=-|p=-|n=1|d=-', [])).toBe(-1)
    expect(indexByServerEpid('s=-|v=-|p=-|n=1|d=-', ['', null, undefined])).toBe(-1)
  })
  it('短码/原串两条路都能对上（形态切换期双读）', () => {
    expect(indexByServerEpid('s=-|v=-|p=-|n=2|d=-', epids)).toBe(1)
  })
})
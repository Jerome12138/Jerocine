import { describe, expect, it } from 'vitest'
import { anchorNumFor, derivedNumFor, parseEpisodeKey } from './episodeKey'

/** 锚点期号（单源片兜底）—— 口径：只数「正片『上』」，排除衍生栏目与变体。 */
const XD9 = [
  // 正片（series/variant 皆空）；每周「上」= 一期首播
  '20260731先导片上',
  '20260803上',
  '20260803中',
  '20260804下',
  '20260810上',
  '20260810中',
  '20260811下',
  '20260817上',
  '20260818中（下）',
  '20260818下',
  '20260928上',
  '20260928中',
  '20260929下',
  // 衍生栏目（series=心动的旅行/心动的周末）—— 必须被排除
  '20261005心动的旅行上',
  '20261005心动的旅行中',
  '20261006心动的旅行下',
  '20261012心动的周末上',
  '20261013心动的周末下',
  // 变体（variant 非空）—— 必须被排除
  '20260803上纯享',
  '20260806加更上'
]

describe('anchorNumFor · 锚点期号', () => {
  it('正片：期号 = 不晚于本集日期的「上」个数', () => {
    expect(anchorNumFor('20260803上', XD9)).toBe(1)
    expect(anchorNumFor('20260803中', XD9)).toBe(1)
    expect(anchorNumFor('20260804下', XD9)).toBe(1)
    expect(anchorNumFor('20260810上', XD9)).toBe(2)
    expect(anchorNumFor('20260811下', XD9)).toBe(2)
    expect(anchorNumFor('20260817上', XD9)).toBe(3)
    expect(anchorNumFor('20260929下', XD9)).toBe(4) // 「上」:0803/0810/0817/0928 ⇒ 4
  })

  it('衍生栏目：不参与锚点计数（也可作为被推算的集？→ 不给期号）', () => {
    // 衍生栏目自身：series 非空 ⇒ 直接返回 0（库里那条本来就没有期号）
    expect(anchorNumFor('20261005心动的旅行上', XD9)).toBe(0)
    expect(anchorNumFor('20261012心动的周末上', XD9)).toBe(0)
    // 且它们不会把正片序带偏：正片 20260929下 仍为 4（若把旅行/周末的「上」算进去会更大）
    expect(anchorNumFor('20260929下', XD9)).toBe(4)
  })

  it('变体（纯享/加更）不参与计数', () => {
    expect(anchorNumFor('20260803上纯享', XD9)).toBe(0) // 变体 ⇒ 不给期号
    expect(anchorNumFor('20260810中', XD9)).toBe(2) // 上纯享/加更 未把序抬高
  })

  it('保守条件：正片「上」不足 2 个 ⇒ 0（不猜）', () => {
    expect(anchorNumFor('20260803上', ['20260803上', '20260803中'])).toBe(0)
    expect(anchorNumFor('20260803上', [])).toBe(0)
  })

  it('非正片/无日期 ⇒ 0', () => {
    expect(anchorNumFor('第3期上', XD9)).toBe(0) // 无日期
    expect(anchorNumFor('正片', XD9)).toBe(0)
  })

  it('多源并集按日期去重：同一期在多个源只算一次', () => {
    const twoSources = [...XD9, ...XD9] // 同一批集名出现两次
    expect(anchorNumFor('20260810上', twoSources)).toBe(2)
  })

  it('解析器抽查：第20261005期心动的旅行上（线上真实形态）', () => {
    const k = parseEpisodeKey('第20261005期心动的旅行上')
    expect(k.date).toBe('20261005') // 「第…期」里的 8 位数字被当日期（歪打正着，就是首播日）
    expect(k.num).toBe(0)
    expect(k.part).toBe('上')
    expect(k.series).toBe('心动的旅行')
  })
})

describe('derivedNumFor · 期号来源优先级', () => {
  it('自带期号 > 桥接 > 锚点；单源片落到锚点', () => {
    expect(derivedNumFor('第5期上', XD9)).toBe(5) // 自带
    expect(derivedNumFor('20260810上', XD9)).toBe(2) // 桥接失败（无人带期号）⇒ 锚点
    expect(derivedNumFor('20261005心动的旅行上', XD9)).toBe(0) // 衍生栏目 ⇒ 不给（库里本来也没有期号）
  })

  it('有他源可桥接时优先桥接（而不是锚点）', () => {
    const withBridge = [...XD9, '20260810上（第2期）'] // 另一源的集名带期号 + 同日期
    expect(derivedNumFor('20260810上', withBridge)).toBe(2)
  })
})

const WEEKEND = [
  ...XD9,
  '20261019心动的周末上',
  '20261019心动的周末下',
  '20261026心动的周末上',
  '20261026心动的周末下'
]

describe('anchorNumFor · 衍生栏目按栏目组计数（2026-11-09 扩展）', () => {
  it('心动的周末：组内「上」日期序 = 期号', () => {
    expect(anchorNumFor('20261012心动的周末上', WEEKEND)).toBe(1)
    expect(anchorNumFor('20261019心动的周末上', WEEKEND)).toBe(2)
    expect(anchorNumFor('20261026心动的周末上', WEEKEND)).toBe(3)
  })

  it('正片仍按正片组计数（不被衍生栏目带偏）', () => {
    expect(anchorNumFor('20260929下', WEEKEND)).toBe(4)
  })

  it('心动的旅行：组内只有 1 个「上」⇒ 保守返回 0（库侧该栏目本就无期号）', () => {
    expect(anchorNumFor('20261005心动的旅行上', WEEKEND)).toBe(0)
  })
})

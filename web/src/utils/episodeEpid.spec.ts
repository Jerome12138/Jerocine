import { describe, expect, it } from 'vitest'
import {
  alignEpisode,
  canAutoSwitch,
  epidOf,
  isVarietyShaped,
  parseEpid,
  type AlignTier
} from './episodeEpid'

/** 黄金用例：与 Go 侧 episode_key_test.go 同语义（两侧改动必须同时通过）。 */
describe('episodeEpid · 解析与序列化', () => {
  it('基础形态', () => {
    expect(epidOf('第1集')).toBe('s=-|v=-|p=-|n=1|d=-')
    expect(epidOf('第3话')).toBe('s=-|v=-|p=-|n=3|d=-')
    expect(epidOf('20260803上')).toBe('s=-|v=-|p=上|n=-|d=20260803')
    expect(epidOf('第3期中（上）')).toBe('s=-|v=-|p=中上|n=3|d=-')
    expect(epidOf('第3期中（下）')).toBe('s=-|v=-|p=中下|n=3|d=-')
    expect(epidOf('20260831中上')).toBe('s=-|v=-|p=中上|n=-|d=20260831')
  })

  it('无意义后缀剔除（会员/未公开/未播/直拍/完整版/未删减）', () => {
    expect(parseEpid('20260128未公开特辑')).toEqual({
      series: '',
      variant: '特辑',
      part: '',
      num: 0,
      date: '20260128'
    })
    expect(epidOf('20260803完整版')).toBe(epidOf('20260803'))
    expect(epidOf('20260805超前彩蛋未播')).toBe(epidOf('20260805超前彩蛋'))
  })

  it('栏目名（Series）：正片为空，衍生栏目单独成身份', () => {
    expect(parseEpid('【qq】 第1期上：又争又抢，心动了就要打明牌！').series).toBe('')
    expect(parseEpid('心动的周末第1期上：高甜游戏').series).toBe('心动的周末')
    expect(parseEpid('20261012心动的周末上').series).toBe('心动的周末')
    expect(parseEpid('心动的旅行上：草原团建').series).toBe('心动的旅行')
    // 语言/类型标签不算栏目名
    expect(parseEpid('正片').series).toBe('')
    expect(parseEpid('国语').series).toBe('')
  })

  it('综艺形态判定：剧/动漫的「第N集」不触发', () => {
    expect(isVarietyShaped(parseEpid('20260803上'))).toBe(true)
    expect(isVarietyShaped(parseEpid('第3期中（上）'))).toBe(true)
    expect(isVarietyShaped(parseEpid('20260806加更上'))).toBe(true)
    expect(isVarietyShaped(parseEpid('第12集'))).toBe(true) // 「第N集」也算身份，但走②/①档即等价于同下标
    expect(isVarietyShaped(parseEpid('无特征名'))).toBe(false)
  })
})

describe('episodeEpid · 跨源对齐 5 档', () => {
  it('① 精确：剧集同序号', () => {
    const r = alignEpisode(4, ['第1集', '第2集', '第3集', '第4集', '第5集'], ['第1集', '第2集', '第3集', '第4集', '第5集'])
    expect(r).toEqual({ index: 4, tier: 'exact' })
  })

  it('① 精确：空格差异不影响（第 3 集 vs 第3集）', () => {
    const r = alignEpisode(2, ['第1集', '第2集', '第 3 集'], ['第1集', '第2集', '第3集'])
    expect(r).toEqual({ index: 2, tier: 'exact' })
  })

  it('② 同期号：两侧都用期号', () => {
    const r = alignEpisode(0, ['第1期上', '第1期中'], ['第1期上', '第1期中', '第1期下', '第2期上'])
    expect(r).toEqual({ index: 0, tier: 'exact' })
  })

  it('③ 同日期：两侧都没期号、用日期', () => {
    const r = alignEpisode(1, ['20260803上', '20260804下'], ['20260803上', '20260804下'])
    expect(r).toEqual({ index: 1, tier: 'exact' })
  })

  it('④ 期序：源 A 用日期、源 B 用期号（跨命名体系）', () => {
    // A 的第 2 集（20260804下）↔ B 的第 2 期下
    const r = alignEpisode(1, ['20260803上', '20260804下'], ['第1期上', '第2期下'])
    expect(r.tier).toBe('byOrdinal')
    expect(r.index).toBe(1)
    expect(canAutoSwitch(r.tier)).toBe(false) // 兜底档不自动换源
  })

  it('⑤ 位置兜底：完全对不上时按原有顺序对', () => {
    const r = alignEpisode(1, ['先导片上', '先导片下'], ['第1集', '第2集', '第3集'])
    expect(r.tier).toBe('position')
    expect(r.index).toBe(1)
    expect(canAutoSwitch(r.tier)).toBe(false)
  })

  it('⑤ 位置兜底夹取：目标源更短时取最后一集', () => {
    const cur = Array.from({ length: 10 }, (_, i) => `第${i + 1}集`)
    const r = alignEpisode(9, cur, ['第1期'])
    expect(r.tier).toBe('position')
    expect(r.index).toBe(0)
  })

  it('当前下标越界 ⇒ failed（不猜）', () => {
    expect(alignEpisode(9, ['第1集', '第2集'], ['第1期'])).toEqual({ index: -1, tier: 'failed' })
  })

  it('同一期内 上/中/下 共享期序（心动的周末）', () => {
    const cur = ['20261012心动的周末上', '20261013心动的周末下']
    const tgt = ['心动的周末第1期上', '心动的周末第1期下']
    const r0 = alignEpisode(0, cur, tgt)
    const r1 = alignEpisode(1, cur, tgt)
    expect(r0.index).toBe(0)
    expect(r1.index).toBe(1)
  })

  it('栏目不同不误配：正片 ↔ 心动的周末 不互相对齐', () => {
    const r = alignEpisode(0, ['正片第1期上'], ['心动的周末第1期上'])
    expect(r.tier).toBe('position') // 身份不兼容 ⇒ 落到位置兜底（不自动换源）
    expect(canAutoSwitch(r.tier)).toBe(false)
  })

  it('目标集列表为空 ⇒ failed', () => {
    expect(alignEpisode(0, ['第1集'], [])).toEqual({ index: -1, tier: 'failed' })
  })

  it('canAutoSwitch：只有精确档放行', () => {
    expect(canAutoSwitch('exact')).toBe(true)
    expect(canAutoSwitch('samePeriod')).toBe(true)
    expect(canAutoSwitch('sameDate')).toBe(true)
    expect(canAutoSwitch('byOrdinal')).toBe(false)
    expect(canAutoSwitch('position')).toBe(false)
    expect(canAutoSwitch('failed')).toBe(false)
  })
})

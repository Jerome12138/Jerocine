import { describe, expect, it } from 'vitest'
import {
  bridgeNum,
  derivedNumFor,
  epidFor,
  epidOf,
  parseEpisodeKey,
  sameEpisode
} from './episodeKey'

// 黄金用例 —— 与 server/internal/danmu/episode_key_test.go 同一组(三端同规, 任一侧改动必须同组通过)
describe('parseEpisodeKey 现网真实集名', () => {
  it('基础形态', () => {
    expect(parseEpisodeKey('20260803(第1期上)')).toEqual({
      date: '20260803', num: 1, eNum: 0, part: '上', sub: '', variant: '', series: ''
    })
    expect(parseEpisodeKey('20260818中（下）')).toMatchObject({ date: '20260818', part: '中', sub: '下' })
    expect(parseEpisodeKey('第3期中（上）')).toMatchObject({ num: 3, part: '中', sub: '上' })
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

  it('触发判定: 裸序号/E圆号不触发', () => {
    expect(epidFor('第3集', [])).toBe('')
    expect(epidFor('E01.220705', [])).toBe('')
    expect(epidFor('20260803上', [])).toBe('s=-|v=-|p=上|n=-|d=20260803')
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

  it('自集无期号且无桥接 → 0; 未触发 → 0', () => {
    expect(derivedNumFor('20260731先导片上', otherTitles)).toBe(0) // 变体不同不桥接
    expect(derivedNumFor('第3集', otherTitles)).toBe(0)
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

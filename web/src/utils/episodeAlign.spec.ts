import { describe, expect, it } from 'vitest'
import { alignEpisode, canAutoSwitch } from './episodeAlign'
import { parseEpisodeKey } from './episodeKey'

/** 黄金用例：与 Go(server/internal/danmu/episode_key_test.go)、Java(EpisodeAlignTest) 同语义。 */
describe('episodeAlign · 跨源集对齐 5 档', () => {
  it('① 精确：剧集同序号（第N集 两侧一致）', () => {
    const cur = ['第1集', '第2集', '第3集', '第4集', '第5集']
    expect(alignEpisode(4, cur, [...cur])).toEqual({ index: 4, tier: 'exact' })
  })

  it('① 精确：集名空格差异不影响（第 3 集 vs 第3集）', () => {
    expect(alignEpisode(2, ['第1集', '第2集', '第 3 集'], ['第1集', '第2集', '第3集'])).toEqual({
      index: 2,
      tier: 'exact'
    })
  })

  it('① 精确：期 + 主次部分一致', () => {
    const r = alignEpisode(0, ['第3期中（上）'], ['第3期中（上）', '第3期中（下）'])
    expect(r).toEqual({ index: 0, tier: 'exact' })
  })

  it('② 同期号：序号错位但期号相同', () => {
    // A 的第 3 个视频是「第2期上」，B 里它排第 1
    const r = alignEpisode(0, ['第2期上'], ['第2期上', '第1期上'])
    expect(r.tier).toBe('exact')
    expect(r.index).toBe(0)
  })

  it('② 同期号：集数不同也能对上（源 A 尾部多出"纯享"）', () => {
    const cur = ['第1期上', '第1期上纯享', '第1期下']
    const tgt = ['第1期下', '第1期上']
    const r = alignEpisode(2, cur, tgt)
    expect(r.tier).toBe('exact')
    expect(r.index).toBe(0)
  })

  it('③ 同日期：两侧都没期号', () => {
    expect(alignEpisode(1, ['20260803上', '20260804下'], ['20260803上', '20260804下'])).toEqual({
      index: 1,
      tier: 'exact'
    })
  })

  it('④ 期序：源 A 用日期、源 B 用期号（跨命名体系 ⇒ 兜底档，不自动换）', () => {
    const r = alignEpisode(1, ['20260803上', '20260804下'], ['第1期上', '第2期下'])
    expect(r.tier).toBe('byOrdinal')
    expect(r.index).toBe(1)
    expect(canAutoSwitch(r.tier)).toBe(false)
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

  it('当前下标越界 / 目标为空 ⇒ failed（不猜）', () => {
    expect(alignEpisode(9, ['第1集', '第2集'], ['第1期'])).toEqual({ index: -1, tier: 'failed' })
    expect(alignEpisode(0, ['第1集'], [])).toEqual({ index: -1, tier: 'failed' })
  })

  it('栏目不同不误配：正片 ↔ 心动的周末（落位置兜底、不自动换）', () => {
    const r = alignEpisode(0, ['正片第1期上'], ['心动的周末第1期上'])
    expect(r.tier).toBe('position')
    expect(canAutoSwitch(r.tier)).toBe(false)
  })

  it('同一期内 上/中/下 共享期序（心动的周末）', () => {
    const cur = ['20261012心动的周末上', '20261013心动的周末下']
    const tgt = ['心动的周末第1期上', '心动的周末第1期下']
    expect(alignEpisode(0, cur, tgt).index).toBe(0)
    expect(alignEpisode(1, cur, tgt).index).toBe(1)
  })

  it('主/次部分前缀相容：库里「中（上）/（下）」、源里只有「中」', () => {
    const r = alignEpisode(0, ['第3期中'], ['第3期中（上）', '第3期中（下）'])
    expect(r.index).toBe(0) // 取第一条
    expect(canAutoSwitch(r.tier)).toBe(true)
  })

  it('canAutoSwitch：只有精确档放行', () => {
    expect(canAutoSwitch('exact')).toBe(true)
    expect(canAutoSwitch('samePeriod')).toBe(true)
    expect(canAutoSwitch('sameDate')).toBe(true)
    expect(canAutoSwitch('byOrdinal')).toBe(false)
    expect(canAutoSwitch('position')).toBe(false)
    expect(canAutoSwitch('failed')).toBe(false)
  })

  it('解析器（复用 episodeKey）仍可用 —— 抽查 epid 触发', () => {
    expect(parseEpisodeKey('20260803上').date).toBe('20260803')
    expect(parseEpisodeKey('第3期中（上）').part).toBe('中')
  })
})

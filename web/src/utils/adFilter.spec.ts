import { describe, it, expect } from 'vitest'
import {
  parseAdFilterStats,
  adFilterSuccessMessage,
  adFilterHost,
  buildAdFilterFailureEvent
} from './adFilter'

describe('parseAdFilterStats', () => {
  it('解析顶层 filtered/total (dto.OK 直返, 无 data 外层)', () => {
    expect(parseAdFilterStats({ filtered: 3, total: 120 })).toEqual({ filtered: 3, total: 120 })
  })

  it('缺字段 / 非法输入一律归 0, 不抛错', () => {
    expect(parseAdFilterStats(null)).toEqual({ filtered: 0, total: 0 })
    expect(parseAdFilterStats('oops')).toEqual({ filtered: 0, total: 0 })
    expect(parseAdFilterStats({ filtered: '3' })).toEqual({ filtered: 0, total: 0 })
    expect(parseAdFilterStats({ filtered: -1, total: -5 })).toEqual({ filtered: 0, total: 0 })
  })

  it('小数向下取整', () => {
    expect(parseAdFilterStats({ filtered: 2.9, total: 10.1 })).toEqual({ filtered: 2, total: 10 })
  })
})

describe('adFilterSuccessMessage', () => {
  it('拼出 N 段广告文案', () => {
    expect(adFilterSuccessMessage(5)).toBe('已过滤 5 段广告')
  })
})

describe('adFilterHost', () => {
  it('取 host', () => {
    expect(adFilterHost('https://ads.evil.com/a.ts?x=1')).toBe('ads.evil.com')
  })
  it('非法 URL 返回空串', () => {
    expect(adFilterHost('not a url')).toBe('')
  })
})

describe('buildAdFilterFailureEvent', () => {
  it('category=adfilter-fail, label=host, extra 存原始+代理链接', () => {
    const ev = buildAdFilterFailureEvent({
      channel: 'web',
      originalUrl: 'https://cdn.x.com/play/index.m3u8',
      proxyUrl: 'https://site/api/v1/m3u8/proxy?src=enc',
      sourceId: 's1',
      sourceName: 'HD(lz)',
      episodeIndex: 2,
      filmId: '100',
      filmName: '片名'
    })
    expect(ev.category).toBe('adfilter-fail')
    expect(ev.action).toBe('web')
    expect(ev.label).toBe('cdn.x.com')
    expect(ev.extra.src).toBe('https://cdn.x.com/play/index.m3u8')
    expect(ev.extra.proxyUrl).toBe('https://site/api/v1/m3u8/proxy?src=enc')
    expect(ev.extra.sourceName).toBe('HD(lz)')
    expect(ev.extra.episode).toBe(2)
    expect(ev.extra.channel).toBe('web')
  })

  it('缺省字段有兜底, label 退化为 unknown', () => {
    const ev = buildAdFilterFailureEvent({
      channel: 'native',
      originalUrl: 'bad-url',
      proxyUrl: ''
    })
    expect(ev.label).toBe('unknown')
    expect(ev.action).toBe('native')
    expect(ev.extra.episode).toBe(-1)
    expect(ev.extra.filmId).toBe('')
  })
})

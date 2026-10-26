import { describe, it, expect } from 'vitest'
import {
  parseAdFilterStats,
  adFilterSuccessMessage,
  adFilterHost,
  buildAdFilterFailureEvent,
  hasMediaSegments,
  firstChildPlaylistUrl,
  adFilterBadgeText,
  loadingHintText,
  resolveClientFilterGate
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

describe('resolveClientFilterGate', () => {
  it('有 MSE 一律走端侧 client(blob 清单 VHS 自己抓, 能播)', () => {
    expect(resolveClientFilterGate({ hasMse: true, adFilterOk: true })).toBe('client')
    expect(resolveClientFilterGate({ hasMse: true, adFilterOk: false })).toBe('client')
    expect(resolveClientFilterGate({ hasMse: true, adFilterOk: undefined })).toBe('client')
  })
  it('无 MSE(iPhone 原生 HLS): 服务器抓得到该源 → 降级服务端代理过滤', () => {
    expect(resolveClientFilterGate({ hasMse: false, adFilterOk: true })).toBe('proxy')
    expect(resolveClientFilterGate({ hasMse: false, adFilterOk: undefined })).toBe('proxy')
  })
  it('无 MSE 且服务器明确抓不到(adFilterOk=false): 代理必失败, 直接原始直链', () => {
    expect(resolveClientFilterGate({ hasMse: false, adFilterOk: false })).toBe('direct')
  })
})

describe('adFilterBadgeText · nosupport', () => {
  it('无 MSE 设备直链回退的角标文案', () => {
    expect(adFilterBadgeText('nosupport', 0)).toBe('该设备不支持端侧过滤')
  })
})

describe('adFilterSuccessMessage', () => {
  it('拼出 N 段广告文案', () => {
    expect(adFilterSuccessMessage(5)).toBe('已过滤 5 段广告')
  })
})

describe('loadingHintText', () => {
  // 2026-10-25 用户报"已过滤 N 段"toast 已出但遮罩仍显示"正在准备视频"(线上实测
  // iPhone+Edge: 过滤落地到 canplay 还有 1s~数十秒分片下载窗口)。三档逐阶段:
  it('片源未就绪 + 过滤在途 ⇒ 广告过滤中文案', () => {
    expect(loadingHintText({ srcReady: false, filtering: true })).toBe('正在准备视频 · 广告过滤中…')
  })

  it('片源未就绪 + 无过滤 ⇒ 通用准备文案', () => {
    expect(loadingHintText({ srcReady: false, filtering: false })).toBe('正在准备视频…')
  })

  it('片源已就绪(过滤落地/直链) ⇒ 降档为缓冲文案, 不再谎称"正在准备"', () => {
    expect(loadingHintText({ srcReady: true, filtering: true })).toBe('正在缓冲视频…')
    expect(loadingHintText({ srcReady: true, filtering: false })).toBe('正在缓冲视频…')
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

describe('hasMediaSegments (#EXTINF 内容判据, 与安卓 PlayerUrls 同款)', () => {
  const master = [
    '#EXTM3U',
    '#EXT-X-STREAM-INF:BANDWIDTH=1280000',
    'playlist_b392800.m3u8',
    '#EXT-X-STREAM-INF:BANDWIDTH=4285000',
    'playlist_b4285000.m3u8'
  ].join('\n')

  it('媒体列表(#EXTINF) → true', () => {
    expect(hasMediaSegments('#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXTINF:9.0,\nseg001.ts')).toBe(true)
  })

  it('master 主表只有变体声明 → false', () => {
    expect(hasMediaSegments(master)).toBe(false)
  })

  it('大小写 / CRLF 不敏感', () => {
    expect(hasMediaSegments('#EXTM3U\r\n#extinf:6.0,\r\nseg001.ts')).toBe(true)
  })

  it('空文本 → false', () => {
    expect(hasMediaSegments('')).toBe(false)
    expect(hasMediaSegments('#EXTM3U\n')).toBe(false)
  })
})

describe('firstChildPlaylistUrl (master 下钻, 判据只看内容不看扩展名)', () => {
  it('master: 首个 URI 即子表 —— **无 .m3u8 后缀的变体地址也跟进**(修: 按扩展名会误判成媒体表, 子表广告被绕过)', () => {
    const master = [
      '#EXTM3U',
      '#EXT-X-STREAM-INF:BANDWIDTH=1280000',
      'https://cdn.x.com/hls/720/list?token=abc',
      '#EXT-X-STREAM-INF:BANDWIDTH=4285000',
      'https://cdn.x.com/hls/1080/list?token=abc'
    ].join('\n')
    expect(firstChildPlaylistUrl(master)).toBe('https://cdn.x.com/hls/720/list?token=abc')
  })

  it('master: 带 .m3u8 后缀的变体照常返回', () => {
    const master = '#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nchunklist.m3u8\n'
    expect(firstChildPlaylistUrl(master)).toBe('chunklist.m3u8')
  })

  it('媒体列表 → null(即便首个分片 URL 的 query 恰好以 .m3u8 结尾)', () => {
    const media = '#EXTM3U\n#EXTINF:9.0,\nseg001.ts?next=https://cdn.x.com/other.m3u8\n'
    expect(firstChildPlaylistUrl(media)).toBeNull()
  })

  it('空文本 / 全注释(无 URI) → null(调用方退化为就地服务端过滤)', () => {
    expect(firstChildPlaylistUrl('')).toBeNull()
    expect(firstChildPlaylistUrl('#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n')).toBeNull()
  })
})

describe('adFilterBadgeText (与安卓 AdFilterStatus 逐字对齐)', () => {
  it('七态文案', () => {
    expect(adFilterBadgeText('off', 0)).toBe('过滤未开启')
    expect(adFilterBadgeText('unsupported', 0)).toBe('该源无需过滤')
    expect(adFilterBadgeText('proxy', 0)).toBe('服务端过滤中')
    expect(adFilterBadgeText('filtering', 0)).toBe('广告过滤中')
    expect(adFilterBadgeText('clean', 0)).toBe('未检出广告')
    expect(adFilterBadgeText('filtered', 3)).toBe('已过滤 3 段广告')
    expect(adFilterBadgeText('failed', 0)).toBe('过滤失败')
  })
})

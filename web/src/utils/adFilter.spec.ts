import { describe, it, expect } from 'vitest'
import {
  adFilterSuccessMessage,
  hasMediaSegments,
  firstChildPlaylistUrl,
  adFilterBadgeText,
  loadingHintText
} from './adFilter'

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

  it('空文本 / 全注释(无 URI) → null(调用方按无过滤形态处理)', () => {
    expect(firstChildPlaylistUrl('')).toBeNull()
    expect(firstChildPlaylistUrl('#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n')).toBeNull()
  })
})

describe('adFilterBadgeText (与安卓 AdFilterStatus 逐字对齐)', () => {
  it('六态文案(2026-10-28: proxy 态随服务端过滤下线移除)', () => {
    expect(adFilterBadgeText('off', 0)).toBe('过滤未开启')
    expect(adFilterBadgeText('unsupported', 0)).toBe('该源无需过滤')
    expect(adFilterBadgeText('filtering', 0)).toBe('广告过滤中')
    expect(adFilterBadgeText('clean', 0)).toBe('未检出广告')
    expect(adFilterBadgeText('filtered', 3)).toBe('已过滤 3 段广告')
    expect(adFilterBadgeText('failed', 0)).toBe('过滤失败')
  })
})

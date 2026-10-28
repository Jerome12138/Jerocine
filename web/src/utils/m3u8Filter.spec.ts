import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import {
  absolutize,
  crossDomainAds,
  durationOutliers,
  isPlaylistURI,
  medianGap,
  numberOutlierAds,
  parseExtinfDuration,
  rewriteM3u8,
  rewriteTagURI,
  segKey,
  segNumber,
  shortBurstAds
} from './m3u8Filter'

/** golden 夹具路径：fixtures/m3u8-filter/cases.json（仓库根，web/src/utils 上两级） */
const FIXTURES = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'fixtures', 'm3u8-filter', 'cases.json')

interface FixtureCase {
  name: string
  base: string
  m3u8: string
  filterAds: boolean
  expected: { text: string; filtered: number; total: number }
}

function loadCases(): FixtureCase[] {
  const doc = JSON.parse(readFileSync(FIXTURES, 'utf8')) as { cases: FixtureCase[] }
  return doc.cases
}

describe('rewriteM3u8：golden 夹具全语料（与 Go 内核逐例对拍）', () => {
  const cases = loadCases()
  it.each(cases.map((c) => [c.name, c] as const))('%s', (_name, c) => {
    const got = rewriteM3u8(c.m3u8, c.base, { filterAds: c.filterAds })
    expect(got.text).toBe(c.expected.text)
    expect(got.filtered).toBe(c.expected.filtered)
    expect(got.total).toBe(c.expected.total)
  })
})

describe('absolutize', () => {
  const base = new URL('https://cdn.example.com/play/index.m3u8')
  it('相对 → 同目录绝对', () => {
    expect(absolutize('seg0.ts', base)).toBe('https://cdn.example.com/play/seg0.ts')
  })
  it('根相对 → host 根', () => {
    expect(absolutize('/abs/seg1.ts', base)).toBe('https://cdn.example.com/abs/seg1.ts')
  })
  it('绝对 URL 原样', () => {
    expect(absolutize('https://ads.evil.com/ad.ts', base)).toBe('https://ads.evil.com/ad.ts')
  })
  it('协议相对 → 继承协议', () => {
    expect(absolutize('//cdn.example.com/x/seg.ts', base)).toBe('https://cdn.example.com/x/seg.ts')
  })
})

describe('isPlaylistURI', () => {
  it('按 path 后缀判定，忽略 query；大小写不敏感', () => {
    expect(isPlaylistURI('https://h/a/index.m3u8')).toBe(true)
    expect(isPlaylistURI('https://h/a/index.m3u8?token=x')).toBe(true)
    expect(isPlaylistURI('https://h/a/index.M3U8')).toBe(true)
    expect(isPlaylistURI('https://h/a/seg1.ts')).toBe(false)
    expect(isPlaylistURI('https://h/a/seg1.ts?x=1')).toBe(false)
    expect(isPlaylistURI('not a url')).toBe(false)
  })
})

describe('parseExtinfDuration', () => {
  it.each([
    ['#EXTINF:4.0,', 4.0],
    ['#EXTINF:3', 3],
    ['#EXTINF:2.5, next', 2.5],
    ['#EXTINF:0.8,', 0.8]
  ])('%s → %s', (line, want) => {
    expect(parseExtinfDuration(line)).toBe(want)
  })
  it('失败/负值/空值 → null', () => {
    expect(parseExtinfDuration('#EXTINF:-1,')).toBeNull()
    expect(parseExtinfDuration('#EXTINF:abc,')).toBeNull()
    expect(parseExtinfDuration('#EXT-X-TARGETDURATION:10')).toBeNull()
    expect(parseExtinfDuration('#EXTINF:,')).toBeNull()
  })
})

describe('segNumber', () => {
  it('取文件名末尾数字，忽略 query', () => {
    expect(segNumber('https://h/x/seg123.ts?token=abc')).toBe(123)
    expect(segNumber('https://h/x/seg5000000.ts')).toBe(5000000)
    // 20 位 → 截末 15 位（Go 同规则）
    expect(segNumber('/video/adjump/time/17766952435470000000.ts')).toBe(952435470000000)
  })
  it('无数字 → -1；超 15 位截末 15 位', () => {
    expect(segNumber('https://h/x/seg_aa.ts')).toBe(-1)
    expect(segNumber('https://h/x/seg12345678901234567890.ts')).toBe(678901234567890)
  })
})

describe('segKey：先剥文件名再取目录段', () => {
  it('目录单层 → host/段', () => {
    expect(segKey('https://cdn.example.com/play/seg0.ts')).toBe('cdn.example.com/play')
    expect(segKey('https://cdn.example.com/play/seg1.ts')).toBe('cdn.example.com/play')
  })
  it('目录多层 → 前两段', () => {
    expect(segKey('https://v.fengbao8.com/video/liurendiliuji/ecca40a6bdd2/0000001.ts')).toBe(
      'v.fengbao8.com/video/liurendiliuji'
    )
  })
  it('host 根 → 仅 host（URL host 小写规范化，Go u.Host 不规范化——组内比较一致即可）', () => {
    expect(segKey('https://cdnA.com/s0.ts')).toBe('cdna.com')
  })
  it('同 host 异内容目录 → 不同键（广告形态）', () => {
    expect(segKey('https://x.com/video/Dy0OgKn2/0001.ts')).not.toBe(segKey('https://x.com/video/oVx7Nl6K/ad.ts'))
  })
})

describe('medianGap', () => {
  it('相邻差值排序中位', () => {
    expect(medianGap([1, 2, 3, 4, 5])).toBe(1)
    expect(medianGap([1, 2, 3, 100])).toBe(1) // gaps 排序后 [1,1,97]，len/2=1 → 1
  })
  it('<2 个 → 0', () => {
    expect(medianGap([])).toBe(0)
    expect(medianGap([5])).toBe(0)
  })
})

describe('crossDomainAds / numberOutlierAds / shortBurstAds / durationOutliers：规则直测', () => {
  it('cross：异键标广告；长表超 30% 保护整表不剔', () => {
    const mixed = crossDomainAds(
      [
        'https://cdnA.com/s0.ts',
        'https://cdnA.com/s1.ts',
        'https://ads.evil.com/ad.ts',
        'https://cdnA.com/s2.ts'
      ],
      'cdna.com'
    )
    expect([...mixed!.keys()]).toEqual([2])
    // 保护仅对长表(≥10)生效：12 段中异键 4(33%>30%) → 整表不剔
    const protectedCase = crossDomainAds(
      [
        ...Array.from({ length: 8 }, (_, i) => `https://c1.com/hls/p${i}.ts`),
        ...Array.from({ length: 4 }, (_, i) => `https://c2.com/hls/ad${i}.ts`)
      ],
      'c1.com/hls'
    )
    expect(protectedCase).toBeNull()
  })

  it('num：编号跳脱命中；占比超上限回 null', () => {
    const segs: string[] = []
    for (let i = 100; i <= 109; i += 1) segs.push(`https://h/x/seg${i}.ts`)
    segs.push('https://h/x/seg900000.ts')
    const ads = numberOutlierAds(segs)
    expect([...ads!.keys()]).toEqual([10])
    const over = numberOutlierAds([
      ...Array.from({ length: 10 }, (_, i) => `https://h/x/seg${100 + i}.ts`),
      ...Array.from({ length: 6 }, (_, i) => `https://h/x/seg${900000 + i}.ts`)
    ])
    expect(over).toBeNull()
  })

  it('short：连续极短簇累计≥3s 剔整簇；开头 4 段豁免；不足阈值保留', () => {
    const durs = [0.8, 0.8, 0.8, 0.8, 4, 4, 0.8, 0.8, 0.8, 0.8, 4, 4]
    const ads = shortBurstAds(durs)
    expect([...ads.keys()]).toEqual([6, 7, 8, 9])
    expect(shortBurstAds([4, 4, 0.5, 0.5, 4, 4]).size).toBe(0)
  })

  it('durOutlier：z-score>3 仅预警不剔除（返回集供日志）', () => {
    const durs = Array.from({ length: 10 }, () => 4.0)
    durs.push(25.0)
    const { outliers, samples } = durationOutliers(durs, durs.map((_, i) => `seg${i}.ts`))
    expect(outliers.has(10)).toBe(true)
    expect(samples[0]).toBe('seg10.ts')
  })
})

describe('rewriteTagURI：URI 属性绝对化（KEY/MAP）', () => {
  const base = new URL('https://cdn.example.com/play/index.m3u8')
  it('相对 URI 绝对化，标签其余不变', () => {
    expect(rewriteTagURI('#EXT-X-KEY:METHOD=AES-128,URI="key.bin"', base)).toBe(
      '#EXT-X-KEY:METHOD=AES-128,URI="https://cdn.example.com/play/key.bin"'
    )
  })
  it('绝对 URI 原样', () => {
    expect(rewriteTagURI('#EXT-X-MAP:URI="https://cdn.example.com/init.mp4"', base)).toBe(
      '#EXT-X-MAP:URI="https://cdn.example.com/init.mp4"'
    )
  })
  it('无 URI 属性 → 原样', () => {
    expect(rewriteTagURI('#EXT-X-VERSION:3', base)).toBe('#EXT-X-VERSION:3')
  })
})

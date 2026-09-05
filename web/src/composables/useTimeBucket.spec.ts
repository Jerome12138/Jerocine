import { describe, it, expect } from 'vitest'
import {
  bucketOf,
  groupByTimeBucket,
  progressPercent,
  episodeLabel,
  BUCKET_LABEL
} from './useTimeBucket'

// 固定参考时刻: 2026-05-16 14:00 (UTC+8)
const NOW = new Date('2026-05-16T06:00:00Z').getTime()

describe('bucketOf', () => {
  it('今天 0:00 之后归 today', () => {
    const today0 = new Date(NOW)
    today0.setHours(0, 0, 0, 0)
    expect(bucketOf(today0.getTime(), NOW)).toBe('today')
    expect(bucketOf(NOW - 60_000, NOW)).toBe('today')
  })

  it('昨天 (24h 前) 归 week (因为不在今天 0 点之后, 但在 7 天内)', () => {
    const day = 24 * 60 * 60 * 1000
    expect(bucketOf(NOW - day, NOW)).toBe('week')
    expect(bucketOf(NOW - 5 * day, NOW)).toBe('week')
  })

  it('7-30 天前归 month', () => {
    const day = 24 * 60 * 60 * 1000
    expect(bucketOf(NOW - 10 * day, NOW)).toBe('month')
    expect(bucketOf(NOW - 29 * day, NOW)).toBe('month')
  })

  it('>= 30 天前归 earlier', () => {
    const day = 24 * 60 * 60 * 1000
    expect(bucketOf(NOW - 31 * day, NOW)).toBe('earlier')
    expect(bucketOf(NOW - 365 * day, NOW)).toBe('earlier')
  })

  it('ts=0 或负值归 earlier (兼容缺失字段)', () => {
    expect(bucketOf(0, NOW)).toBe('earlier')
    expect(bucketOf(-1, NOW)).toBe('earlier')
  })
})

describe('groupByTimeBucket', () => {
  const day = 24 * 60 * 60 * 1000

  it('空数组返回空数组', () => {
    expect(groupByTimeBucket([], NOW)).toEqual([])
  })

  it('按 today → week → month → earlier 顺序分组, 空桶不出现', () => {
    const recs = [
      { id: 'a', timeStamp: NOW - 60_000 }, // today
      { id: 'b', timeStamp: NOW - 2 * day }, // week
      { id: 'c', timeStamp: NOW - 15 * day }, // month
      { id: 'd', timeStamp: NOW - 50 * day }, // earlier
      { id: 'e', timeStamp: NOW - 3 * day } // week
    ]
    const groups = groupByTimeBucket(recs, NOW)
    expect(groups.map((g) => g.bucket)).toEqual([
      'today',
      'week',
      'month',
      'earlier'
    ])
    expect(groups[0]!.items.map((r) => r.id)).toEqual(['a'])
    expect(groups[1]!.items.map((r) => r.id)).toEqual(['b', 'e'])
    expect(groups[2]!.items.map((r) => r.id)).toEqual(['c'])
    expect(groups[3]!.items.map((r) => r.id)).toEqual(['d'])
  })

  it('单桶: 只有 today 的记录, 不出现其它空桶', () => {
    const groups = groupByTimeBucket(
      [{ id: 'a', timeStamp: NOW - 30_000 }],
      NOW
    )
    expect(groups).toHaveLength(1)
    expect(groups[0]!.bucket).toBe('today')
    expect(groups[0]!.label).toBe(BUCKET_LABEL.today)
  })

  it('保持桶内原顺序 (不重排), 调用方负责按时间倒序排好', () => {
    const recs = [
      { id: '1', timeStamp: NOW - 60_000 },
      { id: '2', timeStamp: NOW - 120_000 },
      { id: '3', timeStamp: NOW - 30_000 }
    ]
    const groups = groupByTimeBucket(recs, NOW)
    expect(groups[0]!.items.map((r) => r.id)).toEqual(['1', '2', '3'])
  })
})

describe('progressPercent', () => {
  it('字段缺失返回 0', () => {
    expect(progressPercent(undefined, undefined)).toBe(0)
    expect(progressPercent(100, 0)).toBe(0)
    expect(progressPercent(100, -1)).toBe(0)
    expect(progressPercent(0, 7200)).toBe(0)
  })

  it('正常区间返回 0-100', () => {
    expect(progressPercent(60, 7200)).toBe(1)
    expect(progressPercent(3600, 7200)).toBe(50)
  })

  it('已看完 (距结束 < 30s) 返回 100', () => {
    expect(progressPercent(7190, 7200)).toBe(100)
    expect(progressPercent(7200, 7200)).toBe(100)
  })

  it('结果在 [0, 100] 内 clamp', () => {
    expect(progressPercent(8000, 7200)).toBe(100) // 越界
  })
})

describe('episodeLabel', () => {
  it('非纯数字集名优先原样返回', () => {
    expect(episodeLabel('第01集', 0)).toBe('第01集')
    expect(episodeLabel('HD', 3)).toBe('HD')
  })
  it('纯数字集名(remote 存的 index 字符串)→ 用 episodeIndex 算第N集', () => {
    expect(episodeLabel('5', 5)).toBe('第 6 集')
  })
  it('无集名 → 用 episodeIndex 第N集(0-based→+1)', () => {
    expect(episodeLabel('', 0)).toBe('第 1 集')
    expect(episodeLabel(undefined, 4)).toBe('第 5 集')
  })
  it('都缺 → 空串', () => {
    expect(episodeLabel('', undefined)).toBe('')
    expect(episodeLabel(undefined, undefined)).toBe('')
  })
  it('遗留 "第0集" / 纯 "0" 串 → 归一为第1集(杜绝第0集)', () => {
    expect(episodeLabel('第0集', 0)).toBe('第 1 集')
    expect(episodeLabel('第0集', undefined)).toBe('第 1 集')
    expect(episodeLabel('0', undefined)).toBe('第 1 集')
  })
})

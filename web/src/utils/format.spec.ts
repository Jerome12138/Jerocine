import { describe, it, expect } from 'vitest'
import { formatDuration, formatBytes, formatDateTime, truncate } from './format'

describe('formatDuration', () => {
  it('0 秒 → 00:00', () => expect(formatDuration(0)).toBe('00:00'))
  it('59 秒 → 00:59', () => expect(formatDuration(59)).toBe('00:59'))
  it('60 秒 → 01:00', () => expect(formatDuration(60)).toBe('01:00'))
  it('小于 1h 不出 HH', () => expect(formatDuration(125)).toBe('02:05'))
  it('>=1h 出 HH:MM:SS', () => expect(formatDuration(3661)).toBe('01:01:01'))
  it('负数 → 00:00 (clamp)', () => expect(formatDuration(-5)).toBe('00:00'))
  it('浮点向下取整', () => expect(formatDuration(59.9)).toBe('00:59'))
})

describe('formatBytes', () => {
  it('0 → 0.00 B', () => expect(formatBytes(0)).toBe('0.00 B'))
  it('1023 → 1023.0 B (>=10 走 1 位小数)', () => expect(formatBytes(1023)).toBe('1023.0 B'))
  it('1024 → 1.00 KB', () => expect(formatBytes(1024)).toBe('1.00 KB'))
  it('1MB', () => expect(formatBytes(1024 * 1024)).toBe('1.00 MB'))
  it('>=10 显示 1 位小数', () => expect(formatBytes(1024 * 10)).toBe('10.0 KB'))
  it('负数 → -', () => expect(formatBytes(-1)).toBe('-'))
  it('NaN/Infinity → -', () => {
    expect(formatBytes(NaN)).toBe('-')
    expect(formatBytes(Infinity)).toBe('-')
  })
})

describe('formatDateTime', () => {
  it('合法时间戳 → YYYY-MM-DD HH:mm', () => {
    const d = new Date(2026, 4, 23, 14, 30) // 2026-05-23 14:30 (local)
    expect(formatDateTime(d.getTime())).toBe('2026-05-23 14:30')
  })

  it('接受 Date 对象', () => {
    const d = new Date(2026, 0, 1, 0, 0)
    expect(formatDateTime(d)).toBe('2026-01-01 00:00')
  })

  it('非法值 → -', () => {
    expect(formatDateTime('not a date')).toBe('-')
    expect(formatDateTime(NaN)).toBe('-')
  })

  it('月日时分都补零', () => {
    const d = new Date(2026, 0, 5, 3, 7)
    expect(formatDateTime(d)).toBe('2026-01-05 03:07')
  })
})

describe('truncate', () => {
  it('短于 max → 原样', () => expect(truncate('hi', 10)).toBe('hi'))
  it('等于 max → 原样', () => expect(truncate('hello', 5)).toBe('hello'))
  it('超过 max → 截断 + …', () => expect(truncate('helloworld', 5)).toBe('hello…'))
  it('空串 → 空串', () => expect(truncate('', 5)).toBe(''))
})

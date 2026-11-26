import { describe, it, expect } from 'vitest'
import {
  scheduleError, scheduleValid, describeSchedule,
  parseSchedule, composeSchedule, DEFAULT_SCHEDULE_PARTS
} from './schedule'

describe('schedule 友好文本周期校验（与后端 taskcenter.ValidateSchedule 同规则）', () => {
  it('空串合法（不定时 / 仅手动）', () => {
    expect(scheduleValid('')).toBe(true)
    expect(scheduleValid('   ')).toBe(true)
    expect(scheduleError('')).toBe('')
  })

  it('every:Nm|h', () => {
    expect(scheduleValid('every:30m')).toBe(true)
    expect(scheduleValid('every:6h')).toBe(true)
    expect(scheduleValid('every:59m')).toBe(true)
    expect(scheduleValid('every:0m')).toBe(false)
    expect(scheduleValid('every:60m')).toBe(false)
    expect(scheduleError('every:60m')).toContain('分钟间隔最大 59')
    expect(scheduleValid('every:30')).toBe(false)
    expect(scheduleValid('every:30s')).toBe(false)
  })

  it('daily:HH:MM', () => {
    expect(scheduleValid('daily:03:00')).toBe(true)
    expect(scheduleValid('daily:00:00')).toBe(true)
    expect(scheduleValid('daily:23:59')).toBe(true)
    expect(scheduleValid('daily:24:00')).toBe(false)
    expect(scheduleValid('daily:23:60')).toBe(false)
    expect(scheduleValid('daily:3:00')).toBe(false)
    expect(scheduleError('daily:24:00')).toContain('小时 00~23')
  })

  it('weekly:D HH:MM（1=周一 … 7=周日）', () => {
    expect(scheduleValid('weekly:1 03:00')).toBe(true)
    expect(scheduleValid('weekly:7 23:59')).toBe(true)
    expect(scheduleValid('weekly:0 03:00')).toBe(false)
    expect(scheduleValid('weekly:8 03:00')).toBe(false)
    expect(scheduleValid('weekly:1-03:00')).toBe(false)
  })

  it('monthly:D HH:MM（1~28）', () => {
    expect(scheduleValid('monthly:1 04:00')).toBe(true)
    expect(scheduleValid('monthly:28 04:00')).toBe(true)
    expect(scheduleValid('monthly:29 04:00')).toBe(false)
    expect(scheduleError('monthly:29 04:00')).toContain('1~28')
  })

  it('旧 cron 表达式与其它脏值一律不接受', () => {
    for (const bad of ['0 0 3 * * *', '*/20 * * * * ?', 'daily', 'daily:', 'foo:1 03:00', '每天 04:00']) {
      expect(scheduleValid(bad)).toBe(false)
      expect(scheduleError(bad)).toContain('周期格式不支持')
    }
  })

  it('describeSchedule 与后端 DescribeSchedule 同口径', () => {
    expect(describeSchedule('')).toBe('仅手动')
    expect(describeSchedule('every:30m')).toBe('每 30 分钟')
    expect(describeSchedule('every:6h')).toBe('每 6 小时')
    expect(describeSchedule('daily:03:00')).toBe('每天 03:00')
    expect(describeSchedule('weekly:7 04:00')).toBe('每周日 04:00')
    expect(describeSchedule('monthly:1 04:00')).toBe('每月 1 日 04:00')
  })

  it('守卫：源码里不得出现"冒号紧跟左括号"（UnoCSS variantGroup 会吃掉括号）', async () => {
    // 该陷阱实测代价极高（校验恒失败且无报错），这条用例把"写法约束"钉死：
    // 源码中一旦出现 `kind:(` 形式，transformerVariantGroup 会把它改写掉。
    const fs = await import('node:fs')
    const path = await import('node:path')
    const src = fs.readFileSync(path.resolve('src/utils/schedule.ts'), 'utf8')
    // 剥掉注释：文件头注释里正举着这个陷阱的反例，不剥会自己撞自己
    const code = src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/.*$/gm, '')
    expect(/[A-Za-z]+:\(/.test(code)).toBe(false)
  })
})

describe('parseSchedule / composeSchedule（选择器前后端同一套取值）', () => {
  it('四种形态往返一致', () => {
    for (const text of ['every:30m', 'every:6h', 'daily:03:00', 'weekly:1 03:00', 'monthly:28 23:59']) {
      const parts = parseSchedule(text)
      expect(parts, text).not.toBeNull()
      expect(composeSchedule(parts!)).toBe(text)
    }
  })

  it('解析不了的一律 null（让选择器回退"自定义"而不是编一个值）', () => {
    for (const bad of ['', '   ', '0 0 3 * *', 'daily', 'daily:3:00', 'foo:1m', 'every:0m', 'monthly:29 04:00']) {
      expect(parseSchedule(bad), bad).toBeNull()
    }
  })

  it('默认值本身合法（防止选择器一进来就吐非法值）', () => {
    expect(scheduleValid(composeSchedule(DEFAULT_SCHEDULE_PARTS))).toBe(true)
  })
})

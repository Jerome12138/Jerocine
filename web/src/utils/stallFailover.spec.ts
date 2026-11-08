import { describe, it, expect } from 'vitest'
import {
  shouldStallFailover,
  pickStallTarget,
  speedOf,
  STALL_THRESHOLD_MS,
  STALL_COOLDOWN_MS,
  MAX_STALL_FAILOVERS_PER_EPISODE,
  FASTER_RATIO
} from './stallFailover'

/** 合法基线: 缓冲超阈值、无暂停/seek/互斥、有候选、没触发过 */
function baseCtx(overrides: Partial<Parameters<typeof shouldStallFailover>[0]> = {}) {
  return {
    bufferingMs: STALL_THRESHOLD_MS,
    msSinceLastFailover: Infinity,
    failoverCount: 0,
    paused: false,
    seeking: false,
    switching: false,
    otherSourceCount: 2,
    ...overrides
  }
}

describe('shouldStallFailover', () => {
  it('缓冲达到阈值且无干扰时触发', () => {
    expect(shouldStallFailover(baseCtx())).toBe(true)
    expect(shouldStallFailover(baseCtx({ bufferingMs: STALL_THRESHOLD_MS - 1 }))).toBe(false)
  })

  it('暂停/拖动进度/切集互斥窗口内不触发', () => {
    expect(shouldStallFailover(baseCtx({ paused: true }))).toBe(false)
    expect(shouldStallFailover(baseCtx({ seeking: true }))).toBe(false)
    expect(shouldStallFailover(baseCtx({ switching: true }))).toBe(false)
  })

  it('没有可换的源时不触发(单源影片)', () => {
    expect(shouldStallFailover(baseCtx({ otherSourceCount: 0 }))).toBe(false)
  })

  it('本集内达到换源上限后不再触发', () => {
    expect(
      shouldStallFailover(baseCtx({ failoverCount: MAX_STALL_FAILOVERS_PER_EPISODE }))
    ).toBe(false)
    expect(
      shouldStallFailover(baseCtx({ failoverCount: MAX_STALL_FAILOVERS_PER_EPISODE - 1 }))
    ).toBe(true)
  })

  it('冷却期内不触发, 冷却过后恢复', () => {
    expect(shouldStallFailover(baseCtx({ msSinceLastFailover: STALL_COOLDOWN_MS - 1 }))).toBe(false)
    expect(shouldStallFailover(baseCtx({ msSinceLastFailover: STALL_COOLDOWN_MS }))).toBe(true)
  })
})

describe('speedOf', () => {
  it('有效值透传; 未测/失败/无此线路一律 Infinity', () => {
    const speeds = { a: 800, b: -1 }
    expect(speedOf(speeds, 'a')).toBe(800)
    expect(speedOf(speeds, 'b')).toBe(Infinity) // 测失败(-1)不可信
    expect(speedOf(speeds, 'c')).toBe(Infinity) // 未测
    expect(speedOf(speeds, undefined)).toBe(Infinity)
  })
})

describe('pickStallTarget', () => {
  it('存在显著更快(省 40%+)的线路时返回它', () => {
    const speeds = { cur: 1000, fast: 500, slow: 900 }
    // 500 < 1000*0.6=600 ✓
    expect(pickStallTarget(speeds, 'cur', ['fast', 'slow', 'cur'])).toBe('fast')
  })

  it('只快一点点(不足 FASTER_RATIO)不值得换, 返回 null', () => {
    const speeds = { cur: 1000, other: 800 } // 800 > 600
    expect(pickStallTarget(speeds, 'cur', ['other'])).toBe(null)
  })

  it('当前线路没测过/测挂时, 任一有效候选都算更优(死线路正是 stall 常见原因)', () => {
    expect(pickStallTarget({ a: 1500 }, 'cur', ['a'])).toBe('a')
    expect(pickStallTarget({ a: 1500, cur: -1 }, 'cur', ['a'])).toBe('a')
  })

  it('所有候选都不可用时返回 null(换无可换)', () => {
    expect(pickStallTarget({ a: -1, b: -1 }, 'cur', ['a', 'b'])).toBe(null)
    expect(pickStallTarget({}, 'cur', [])).toBe(null)
  })

  it('并列时取更快的那个, 不会挑到当前线路自己', () => {
    const speeds = { cur: 400, a: 500 }
    // a=500 不满足 < 400*0.6 → null(自己最快, 不折腾)
    expect(pickStallTarget(speeds, 'cur', ['a'])).toBe(null)
  })
})

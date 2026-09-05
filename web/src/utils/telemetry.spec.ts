import { describe, it, expect, vi, beforeEach } from 'vitest'
import { telemetry, toIngestEvent, ENDPOINT } from './telemetry'

/**
 * telemetry 单测 — 重点覆盖:
 *   - track API 不抛错
 *   - trackError 处理 Error / string / unknown
 *   - trackApi status >= 400 自动产生 error 事件
 *   - setUserId 影响后续事件 userId 字段
 *
 * 不测网络发送 (依赖 fetch + setTimeout), 只验证 queue 行为.
 */
describe('telemetry', () => {
  beforeEach(() => {
    // 重置内部队列 + errorIndex (后者跟 queue 配对, 不清会导致 dedupe 把后续测的事件吞掉)
    const internal = telemetry as unknown as { queue: unknown[]; errorIndex: Map<string, unknown> }
    internal.queue = []
    internal.errorIndex = new Map()
    telemetry.setUserId(0)
    vi.useFakeTimers()
  })

  it('track 不抛错且入队', () => {
    expect(() => telemetry.track('action', { action: 'click' })).not.toThrow()
    const q = (telemetry as unknown as { queue: unknown[] }).queue
    expect(q.length).toBe(1)
  })

  it('trackError 接受 Error 对象', () => {
    telemetry.trackError(new Error('boom'), 'js-error')
    const q = (telemetry as unknown as { queue: Array<{ category?: string; label?: string }> }).queue
    expect(q[0].category).toBe('js-error')
    expect(q[0].label).toContain('boom')
  })

  it('trackError 接受字符串', () => {
    telemetry.trackError('plain message', 'custom')
    const q = (telemetry as unknown as { queue: Array<{ label?: string }> }).queue
    expect(q[0].label).toBe('plain message')
  })

  it('chunk 加载失败(部署自愈)不上报 error', () => {
    const q = (telemetry as unknown as { queue: unknown[] }).queue
    telemetry.trackError(
      new Error('Failed to fetch dynamically imported module: https://x/assets/a.js'),
      'vue-error'
    )
    telemetry.trackError('Importing a module script failed.', 'unhandled-rejection')
    telemetry.trackError('Unable to preload CSS for /assets/RouteSpinner-x.css', 'vue-error')
    telemetry.trackError(
      new Error("Expected a JavaScript module but got 'text/html' is not a valid JavaScript MIME type"),
      'js-error-opaque'
    )
    expect(q.length).toBe(0) // 全部被过滤, 不入队
    // 普通错误仍正常上报
    telemetry.trackError(new Error('real boom'), 'js-error')
    expect(q.length).toBe(1)
  })

  it('trackApi status>=400 只产生 api(不再额外造 HTTP error 刷屏错误列表)', () => {
    telemetry.trackApi('/api/x', 100, 500)
    const q = (telemetry as unknown as { queue: Array<{ type?: string; label?: string }> }).queue
    expect(q.length).toBe(1)
    expect(q[0].type).toBe('api')
    expect(q[0].label).toBe('500') // 状态码仍记录在 api 事件
  })

  it('trackApi status<400 只产生 api', () => {
    telemetry.trackApi('/api/y', 50, 200)
    const q = (telemetry as unknown as { queue: Array<{ type?: string }> }).queue
    expect(q.length).toBe(1)
    expect(q[0].type).toBe('api')
  })

  it('setUserId 影响后续事件', () => {
    telemetry.setUserId(42)
    telemetry.track('pv', { action: 'route' })
    const q = (telemetry as unknown as { queue: Array<{ userId?: number }> }).queue
    expect(q[0].userId).toBe(42)
  })

  /** 同 (category|path|label) 的 error 在 queue 内合并 — value 累加, 行数不增 */
  it('error 去重: 同 key 多次 track 只 1 行, value 累计', () => {
    telemetry.trackError(new Error('boom'), 'js-error')
    telemetry.trackError(new Error('boom'), 'js-error')
    telemetry.trackError(new Error('boom'), 'js-error')
    const q = (telemetry as unknown as { queue: Array<{ value?: number }> }).queue
    expect(q.length).toBe(1)
    expect(q[0].value).toBe(3)
  })

  /** 不同 label 的 error 不合并 */
  it('error 不同 label 各占 1 行', () => {
    telemetry.trackError(new Error('aaa'), 'js-error')
    telemetry.trackError(new Error('bbb'), 'js-error')
    const q = (telemetry as unknown as { queue: Array<unknown> }).queue
    expect(q.length).toBe(2)
  })

  /** trackError 应自动 synth 一个 stack 当输入是字符串 (没自带 stack) */
  it('trackError 非 Error 输入自动 synth stack', () => {
    telemetry.trackError('plain string err', 'js-error')
    const q = (telemetry as unknown as { queue: Array<{ extra?: string }> }).queue
    expect(q[0].extra).toBeDefined()
    const parsed = JSON.parse(q[0].extra ?? '{}') as { stack?: string }
    expect(parsed.stack).toContain('synthetic')
  })

  /** trackError 自动把视口快照塞 extra (innerW/mode/dpr ...) */
  it('trackError extra 自动带 viewport 信息', () => {
    telemetry.trackError(new Error('boom'), 'js-error')
    const q = (telemetry as unknown as { queue: Array<{ extra?: string }> }).queue
    const parsed = JSON.parse(q[0].extra ?? '{}') as Record<string, unknown>
    expect(parsed).toHaveProperty('innerW')
    expect(parsed).toHaveProperty('mode')
  })

  /** ES2022 Error.cause 链应被展开到 stack */
  it('trackError 展开 Error.cause 链', () => {
    const root = new Error('root cause')
    // @ts-expect-error - ES2022 cause 标准属性
    const wrap = new Error('wrap', { cause: root })
    telemetry.trackError(wrap, 'js-error')
    const q = (telemetry as unknown as { queue: Array<{ extra?: string }> }).queue
    const parsed = JSON.parse(q[0].extra ?? '{}') as { stack?: string }
    expect(parsed.stack).toContain('Caused by')
    expect(parsed.stack).toContain('root cause')
  })
})

// 上报端点 + 后端 IngestEvent 形状映射(本次修复"埋点 batch 接口异常"的回归点)
const ingestBase = { ts: 1700000000000, sessionId: 's1', platform: 'web', appVersion: 'prod', userId: 0 }

describe('telemetry 上报端点', () => {
  it('ENDPOINT 指向 /api/v1/telemetry/events(而非旧的 /api/telemetry/batch)', () => {
    expect(ENDPOINT).toBe('/api/v1/telemetry/events')
  })
})

describe('toIngestEvent → 后端 IngestEvent 形状', () => {
  it('pv: category 回退 type, clientTs=ts, extra 带 type/platform', () => {
    const out = toIngestEvent({ ...ingestBase, type: 'pv', action: 'route', label: '/x' } as never)
    expect(out.category).toBe('pv')
    expect(out.clientTs).toBe(1700000000000)
    expect(out.label).toBe('/x')
    expect(out.duration).toBe(0)
    expect(out.extra).toMatchObject({ type: 'pv', action: 'route', platform: 'web', appVersion: 'prod' })
  })

  it('api: 耗时(value) 同时写入 duration', () => {
    const out = toIngestEvent({ ...ingestBase, type: 'api', action: '/films', label: '200', value: 123.6 } as never)
    expect(out.category).toBe('api')
    expect(out.value).toBe(123.6)
    expect(out.duration).toBe(124)
  })

  it('error: 显式 category 优先于 type; extra 字符串被解析为对象并合并', () => {
    const out = toIngestEvent({
      ...ingestBase,
      type: 'error',
      category: 'js-error',
      label: 'boom',
      extra: JSON.stringify({ stack: 'at foo', innerW: 390 })
    } as never)
    expect(out.category).toBe('js-error')
    expect(typeof out.extra).toBe('object')
    expect(out.extra).toMatchObject({ type: 'error', stack: 'at foo', innerW: 390 })
  })

  it('extra 非法 JSON → 退化为 { raw }, 不抛错', () => {
    const out = toIngestEvent({ ...ingestBase, type: 'action', extra: '{不是json' } as never)
    expect(out.extra).toMatchObject({ type: 'action', raw: '{不是json' })
  })

  it('匿名(userId=0) 不写入 extra.userId', () => {
    const out = toIngestEvent({ ...ingestBase, type: 'pv', userId: 0 } as never)
    expect('userId' in out.extra).toBe(false)
  })
})

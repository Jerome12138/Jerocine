import { describe, it, expect, afterEach, vi } from 'vitest'
import { CanceledError, type AxiosAdapter } from 'axios'
import { http } from './http'

vi.mock('@/utils/telemetry', () => ({ telemetry: { trackApi: vi.fn() } }))

/**
 * 回归: 主动取消的请求必须原样抛出 CanceledError。
 *
 * 背景: 响应拦截器原先把所有失败统一包装成 ApiError(name='ApiError'),
 * 于是各页面 `e.name === 'CanceledError'` 的守卫永远不成立 —— 被新请求取代的
 * 旧请求(cancel)被当成"加载失败"。典型表现: 片库点筛选/翻页后偶发
 * "加载失败, 请检查链接中的 Pid 参数", 刷新(只发一次请求)又正常。
 */
describe('http 响应拦截器', () => {
  const origin = http.defaults.adapter

  afterEach(() => {
    http.defaults.adapter = origin
  })

  it('取消请求 → 原样抛出 CanceledError(不包成 ApiError)', async () => {
    http.defaults.adapter = (() => Promise.reject(new CanceledError('canceled'))) as AxiosAdapter
    await expect(http.get('/films')).rejects.toMatchObject({ name: 'CanceledError' })
  })

  it('普通失败 → 仍包成 ApiError 且带 HTTP status', async () => {
    http.defaults.adapter = (() =>
      Promise.reject(
        Object.assign(new Error('boom'), {
          isAxiosError: true,
          config: {},
          response: { status: 500, data: { code: 500, message: '服务器错误' } }
        })
      )) as AxiosAdapter
    const err = (await http.get('/films').catch((e: unknown) => e)) as {
      name: string
      status: number
      message: string
    }
    expect(err.name).toBe('ApiError')
    expect(err.status).toBe(500)
    expect(err.message).toBe('服务器错误')
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const API_ORIGIN = 'https://api.jerocine.art'
const BLOB = '/api/upload/backdrop/158815.jpg'

describe('resolveBlobUrl', () => {
  beforeEach(() => {
    vi.resetModules()
    // util 在模块顶层读 VITE_API_BASE —— 每例重置模块后动态 import, 保证取到当前值
    import.meta.env.VITE_API_BASE = `${API_ORIGIN}/api`
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    import.meta.env.VITE_API_BASE = '/api' // 还原, 防污染同 worker 其它 spec
  })

  it('非 /api/upload 相对路径原样返回', async () => {
    vi.stubGlobal('window', { location: { origin: 'https://web.jerocine.art' } })
    const m = await import('./blobImg')
    expect(m.resolveBlobUrl('/assets/x.js')).toBe('/assets/x.js')
    expect(m.resolveBlobUrl('')).toBe('')
  })

  it('跨域静态部署: blob 相对路径 → API 域绝对地址', async () => {
    vi.stubGlobal('window', { location: { origin: 'https://web.jerocine.art' } })
    const m = await import('./blobImg')
    expect(m.resolveBlobUrl(BLOB)).toBe(`${API_ORIGIN}${BLOB}`)
  })

  it('同源部署(主站): 原样返回, 行为不变', async () => {
    vi.stubGlobal('window', { location: { origin: API_ORIGIN } })
    const m = await import('./blobImg')
    expect(m.resolveBlobUrl(BLOB)).toBe(BLOB)
  })

  it('无 window(SSR/node)原样返回', async () => {
    vi.stubGlobal('window', undefined)
    const m = await import('./blobImg')
    expect(m.resolveBlobUrl(BLOB)).toBe(BLOB)
  })

  it('绝对 URL(外链海报)原样返回', async () => {
    vi.stubGlobal('window', { location: { origin: 'https://web.jerocine.art' } })
    const m = await import('./blobImg')
    expect(m.resolveBlobUrl('https://img.lzipic.com/a.webp')).toBe('https://img.lzipic.com/a.webp')
  })
})

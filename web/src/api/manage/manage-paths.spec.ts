import { describe, it, expect, beforeEach, vi } from 'vitest'

// 记录所有 http 调用; vi.hoisted 保证可在被提升的 vi.mock 工厂内引用
const h = vi.hoisted(() => ({ calls: [] as Array<{ method: string; url: string; body?: unknown }> }))

vi.mock('../http', () => {
  const mockData = (url: string): unknown => {
    if (url.includes('site-config')) return { state: 0 }
    if (url.endsWith('/files')) return { list: [], page: { current: 1, size: 39, total: 0, pageCount: 0 } }
    if (/\/collect-sources\/[^/]+$/.test(url)) return { state: 0 }
    return []
  }
  return {
    http: {
      get: (url: string) => {
        h.calls.push({ method: 'GET', url })
        return Promise.resolve(mockData(url))
      },
      delete: (url: string) => {
        h.calls.push({ method: 'DELETE', url })
        return Promise.resolve(undefined)
      },
      post: (url: string, body?: unknown) => {
        h.calls.push({ method: 'POST', url, body })
        return Promise.resolve({ link: 'http://x/y.jpg' })
      },
      put: (url: string, body?: unknown) => {
        h.calls.push({ method: 'PUT', url, body })
        return Promise.resolve(undefined)
      },
      patch: (url: string, body?: unknown) => {
        h.calls.push({ method: 'PATCH', url, body })
        return Promise.resolve(undefined)
      }
    },
    toast: () => {}
  }
})

import * as collect from './collect'
import * as cron from './cron'
import * as file from './file'
import * as system from './system'

beforeEach(() => {
  h.calls.length = 0
})

describe('manage collect api → 新 REST 路由', () => {
  it('list → GET /manage/collect-sources', async () => {
    await collect.list()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/collect-sources' })
  })

  it('find → GET /manage/collect-sources/:id', async () => {
    await collect.find('lzi')
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/collect-sources/lzi' })
  })

  it('add → POST /manage/collect-sources, state bool→0/interval→intervalMs', async () => {
    await collect.add({
      id: 'lzi', name: 'n', uri: 'u', resultModel: 0, grade: 0,
      syncPictures: false, collectType: 0, state: true, interval: 5
    })
    expect(h.calls[0]?.method).toBe('POST')
    expect(h.calls[0]?.url).toBe('/manage/collect-sources')
    expect(h.calls[0]?.body).toMatchObject({ state: 0, intervalMs: 5 })
  })

  it('update → PUT /manage/collect-sources/:id', async () => {
    await collect.update({
      id: 'lzi', name: 'n', uri: 'u', resultModel: 0, grade: 0,
      syncPictures: false, collectType: 0, state: false, interval: 0
    })
    expect(h.calls[0]).toMatchObject({ method: 'PUT', url: '/manage/collect-sources/lzi' })
    expect(h.calls[0]?.body).toMatchObject({ state: 1 })
  })

  it('remove → DELETE /manage/collect-sources/:id', async () => {
    await collect.remove('lzi')
    expect(h.calls[0]).toMatchObject({ method: 'DELETE', url: '/manage/collect-sources/lzi' })
  })

  it('test → POST /manage/collect-sources/:id/test', async () => {
    await collect.test('lzi')
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/collect-sources/lzi/test' })
  })

  it('testAll → POST /manage/collect-sources/test', async () => {
    await collect.testAll()
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/collect-sources/test' })
  })

  it('health → GET /manage/collect-sources/health', async () => {
    await collect.health()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/collect-sources/health' })
  })

  it('startSpider → POST /manage/spider/jobs {sourceId,duration}', async () => {
    await collect.startSpider({ id: 'lzi', ids: [], time: -1, batch: false })
    expect(h.calls[0]).toMatchObject({
      method: 'POST',
      url: '/manage/spider/jobs',
      body: { sourceId: 'lzi', duration: -1 }
    })
  })

  it('resetSpider → POST /manage/spider/reset {confirm}', async () => {
    await collect.resetSpider('Re0')
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/spider/reset', body: { confirm: 'Re0' } })
  })

  it('spiderJobs → GET /manage/spider/jobs', async () => {
    await collect.spiderJobs()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/spider/jobs' })
  })

  it('spiderJobPause/Resume/Cancel → POST /manage/spider/jobs/:id/*', async () => {
    await collect.spiderJobPause('lzi')
    await collect.spiderJobResume('lzi')
    await collect.spiderJobCancel('lzi')
    expect(h.calls.map((c) => c.url)).toEqual([
      '/manage/spider/jobs/lzi/pause',
      '/manage/spider/jobs/lzi/resume',
      '/manage/spider/jobs/lzi/cancel'
    ])
  })
})

describe('manage cron api → 新 REST 路由', () => {
  it('list → GET /manage/cron-tasks', async () => {
    await cron.list()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/cron-tasks' })
  })

  it('update → PUT /manage/cron-tasks/:id, ids→sourceIds/id→number/state bool→int', async () => {
    await cron.update({ id: '7', ids: ['a', 'b'], time: 24, spec: '* * * * * *', model: 1, state: false, remark: '' })
    expect(h.calls[0]).toMatchObject({ method: 'PUT', url: '/manage/cron-tasks/7' })
    expect(h.calls[0]?.body).toMatchObject({ sourceIds: ['a', 'b'], id: 7, state: 1 })
  })

  it('remove → DELETE /manage/cron-tasks/:id', async () => {
    await cron.remove('7')
    expect(h.calls[0]).toMatchObject({ method: 'DELETE', url: '/manage/cron-tasks/7' })
  })
})

describe('manage file api → 新 REST 路由', () => {
  it('list → GET /manage/files', async () => {
    await file.list({ current: 2 })
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/files' })
  })

  it('remove → DELETE /manage/files/:id', async () => {
    await file.remove(5)
    expect(h.calls[0]).toMatchObject({ method: 'DELETE', url: '/manage/files/5' })
  })

  it('upload → POST /manage/files, 返回 link 字符串', async () => {
    const url = await file.upload(new FormData())
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/files' })
    expect(url).toBe('http://x/y.jpg')
  })
})

describe('manage system api → 新 REST 路由', () => {
  it('dashboard → GET /manage/dashboard', async () => {
    await system.dashboard()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/dashboard' })
  })

  it('getBasic → GET /manage/site-config, description→describe/state→bool', async () => {
    const r = await system.getBasic()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/site-config' })
    expect(r.state).toBe(true)
  })

  it('updateBasic → POST /manage/site-config, describe→description/state→int', async () => {
    await system.updateBasic({ siteName: 's', logo: '', keyword: '', describe: 'D', domain: '', state: true, hint: '' })
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/site-config' })
    expect(h.calls[0]?.body).toMatchObject({ description: 'D', state: 0 })
  })
})

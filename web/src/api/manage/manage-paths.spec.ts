import { describe, it, expect, beforeEach, vi } from 'vitest'

// 记录所有 http 调用; vi.hoisted 保证可在被提升的 vi.mock 工厂内引用
const h = vi.hoisted(() => ({
  calls: [] as Array<{ method: string; url: string; body?: unknown; params?: unknown; config?: unknown }>,
  // POST 的返回值可被单测覆写(默认 { link: 'http://x/y.jpg' }), 用于空 link 场景
  postResult: undefined as unknown
}))

vi.mock('../http', () => {
  const mockData = (url: string): unknown => {
    if (url.includes('site-config')) return { state: 0, tmdbKeyMasked: '0c06…676b', tmdbKeySet: true }
    if (url.endsWith('/files'))
      return {
        list: [
          // 后端原样 DTO(entity.FileInfo): fileType 存扩展名, 无 size 字段
          { id: 5, link: 'http://x/y/gallery/1.jpg', objectKey: 'gallery/1.jpg', type: 1, fileType: 'jpg', createdAt: 123 }
        ],
        page: { current: 1, size: 39, total: 1, pageCount: 1 }
      }
    if (/\/collect-sources\/[^/]+$/.test(url)) return { state: 0 }
    if (url === '/manage/tasks/runs') return { list: [], page: { current: 1, size: 20, total: 0, pageCount: 0 } }
    if (url === '/manage/tasks/overview') return { running: 0, todaySuccess: 0, todayFailed: 0, pendingFailures: 0 }
    // cron 列表返回一条后端原样 DTO, 供 toView 方向映射断言
    if (url === '/manage/cron-tasks')
      return [
        {
          id: 7,
          sourceIds: ['a', 'b'],
          spec: '0 0 3 * * *',
          time: 24,
          model: 1,
          state: 1,
          remark: 'r',
          lastRunAt: 123,
          lastStatus: 'success'
        },
        { id: 9, sourceIds: undefined, spec: '* * * * * *', time: 1, model: 5, state: 0, remark: '' }
      ]
    return []
  }
  return {
    http: {
      get: (url: string, config?: { params?: unknown }) => {
        h.calls.push({ method: 'GET', url, params: config?.params })
        return Promise.resolve(mockData(url))
      },
      delete: (url: string) => {
        h.calls.push({ method: 'DELETE', url })
        return Promise.resolve(undefined)
      },
      post: (url: string, body?: unknown, config?: unknown) => {
        h.calls.push({ method: 'POST', url, body, config })
        return Promise.resolve(h.postResult ?? { link: 'http://x/y.jpg' })
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

import * as banner from './banner'
import * as collect from './collect'
import * as cron from './cron'
import * as file from './file'
import * as register from './register'
import * as system from './system'
import * as tasks from './tasks'
import * as telemetry from './telemetry'

beforeEach(() => {
  h.calls.length = 0
  h.postResult = undefined
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
  it('list → GET /manage/cron-tasks, DTO→视图映射(id 数字化/state 反转/model 归一)', async () => {
    const rows = await cron.list()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/cron-tasks' })
    expect(rows[0]).toMatchObject({
      id: '7',
      ids: ['a', 'b'],
      model: 1,
      state: false, // state:1 停用
      lastRunAt: 123
    })
    // 兜底: model 越界归 0, state:0 启用, 缺省 lastRunAt→0, sourceIds 缺省→[]
    expect(rows[1]).toMatchObject({ id: '9', ids: [], model: 0, state: true, lastRunAt: 0 })
  })

  it('add → POST /manage/cron-tasks, ids→sourceIds/id→number/state bool→int', async () => {
    await cron.add({ id: '', ids: ['a'], time: 12, spec: '0 0 3 * * *', model: 1, state: true, remark: 'x' })
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/cron-tasks' })
    expect(h.calls[0]?.body).toMatchObject({ id: 0, sourceIds: ['a'], time: 12, state: 0 })
  })

  it('update → PUT /manage/cron-tasks/:id, ids→sourceIds/id→number/state bool→int', async () => {
    await cron.update({ id: '7', ids: ['a', 'b'], time: 24, spec: '* * * * * *', model: 1, state: false, remark: '' })
    expect(h.calls[0]).toMatchObject({ method: 'PUT', url: '/manage/cron-tasks/7' })
    expect(h.calls[0]?.body).toMatchObject({ sourceIds: ['a', 'b'], id: 7, state: 1 })
  })

  it('change → PUT /manage/cron-tasks/:id (与 update 同端点, state bool→int)', async () => {
    await cron.change({ id: '3', ids: [], time: 24, spec: '* * * * * *', model: 0, state: false, remark: '' })
    expect(h.calls[0]).toMatchObject({ method: 'PUT', url: '/manage/cron-tasks/3' })
    expect(h.calls[0]?.body).toMatchObject({ state: 1 })
  })

  it('runNow → POST /manage/cron-tasks/:id/run', async () => {
    await cron.runNow('7')
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/cron-tasks/7/run' })
  })

  it('remove → DELETE /manage/cron-tasks/:id', async () => {
    await cron.remove('7')
    expect(h.calls[0]).toMatchObject({ method: 'DELETE', url: '/manage/cron-tasks/7' })
  })
})

describe('manage file api → 新 REST 路由', () => {
  it('list → GET /manage/files, current 映射为 page 查询参数', async () => {
    await file.list({ current: 2 })
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/files' })
    expect(h.calls[0].params).toEqual({ page: 2 })
  })

  it('list DTO→视图映射: objectKey→name(取文件名), link→url, fileType→type', async () => {
    const resp = await file.list({ current: 1 })
    expect(resp.list[0]).toEqual({
      id: 5,
      name: '1.jpg',
      url: 'http://x/y/gallery/1.jpg',
      type: 'jpg',
      createdAt: 123
    })
    expect(resp.page).toEqual({ total: 1, current: 1, size: 39, pageCount: 1 })
  })

  it('remove → DELETE /manage/files/:id', async () => {
    await file.remove(5)
    expect(h.calls[0]).toMatchObject({ method: 'DELETE', url: '/manage/files/5' })
  })

  it('upload → POST /manage/files(silent, 不触发全局 toast), 返回 link 字符串', async () => {
    const url = await file.upload(new FormData())
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/files' })
    // silent: 多文件上传时逐个弹全局 toast 会刷屏, 反馈交给页面行内状态
    expect((h.calls[0].config as { silent?: boolean }).silent).toBe(true)
    expect(url).toBe('http://x/y.jpg')
  })

  it('upload 空 link → 视为失败抛错, 不静默吞掉', async () => {
    h.postResult = { link: '' }
    await expect(file.upload(new FormData())).rejects.toThrow('上传成功但未返回文件地址')
  })
})

describe('manage banner api → 新 REST 路由', () => {
  const draft = { id: 0, title: 't', subtitle: '', image: '', poster: '', mid: 0, link: '', sort: 0, state: 0, startAt: 0, endAt: 0 }

  it('board → GET /manage/banners/effective', async () => {
    await banner.board()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/banners/effective' })
  })

  it('move → POST /manage/banners/move 带 slot/dir', async () => {
    await banner.move(2, 'up')
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/banners/move', body: { slot: 2, dir: 'up' } })
  })

  it('save 无 id → POST /manage/banners, 带 slot 透传', async () => {
    await banner.save({ ...draft })
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/banners' })
    await banner.save({ ...draft }, 3)
    expect(h.calls[1]).toMatchObject({ method: 'POST', url: '/manage/banners', body: { slot: 3 } })
  })

  it('save 带 id → PUT /manage/banners/:id', async () => {
    await banner.save({ ...draft, id: 7 })
    expect(h.calls[0]).toMatchObject({ method: 'PUT', url: '/manage/banners/7' })
  })

  it('remove → DELETE /manage/banners/:id', async () => {
    await banner.remove(7)
    expect(h.calls[0]).toMatchObject({ method: 'DELETE', url: '/manage/banners/7' })
  })

  it('refreshHotBoard → POST /manage/hot/refresh, 覆盖默认 timeout 为 600s(刷新实测 1-2 分钟)且 silent', async () => {
    await banner.refreshHotBoard()
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/hot/refresh' })
    expect(h.calls[0]?.config).toEqual({ timeout: 600_000, silent: true })
  })

  it('disableByMid → POST /manage/banners/disable 带 mid', async () => {
    await banner.disableByMid(9)
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/banners/disable', body: { mid: 9 } })
  })

  it('fillBackdrop → POST /manage/backdrop/fill 带 mid, silent(结果由调用方按 status 提示)', async () => {
    await banner.fillBackdrop(5)
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/backdrop/fill', body: { mid: 5 } })
    expect(h.calls[0]?.config).toEqual({ silent: true })
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

  it('getTMDBKey → GET /manage/site-config, 掩码/set 提取', async () => {
    const r = await system.getTMDBKey()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/site-config' })
    expect(r).toEqual({ masked: '0c06…676b', set: true })
  })

  it('setTMDBKey → POST /manage/tmdb-key {key}', async () => {
    await system.setTMDBKey('k'.repeat(32))
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/tmdb-key', body: { key: 'k'.repeat(32) } })
  })

  it('clearTMDBKey → DELETE /manage/tmdb-key', async () => {
    await system.clearTMDBKey()
    expect(h.calls[0]).toMatchObject({ method: 'DELETE', url: '/manage/tmdb-key' })
  })
})

describe('manage register(注册审核) api → 新 REST 路由', () => {
  it('list → GET /manage/register/applies, status 缺省 -1(不限)', async () => {
    await register.list()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/register/applies' })
  })

  it('approve → POST /manage/register/applies/:id/approve', async () => {
    await register.approve(7)
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/register/applies/7/approve' })
  })

  it('reject → POST /manage/register/applies/:id/reject {reason}', async () => {
    await register.reject(7, '信息不实')
    expect(h.calls[0]).toMatchObject({
      method: 'POST',
      url: '/manage/register/applies/7/reject',
      body: { reason: '信息不实' }
    })
  })

  it('resendActivation → POST /manage/register/applies/:id/resend-activation', async () => {
    await register.resendActivation(7)
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/register/applies/7/resend-activation' })
  })
})

describe('manage tasks api → 新 REST 路由', () => {
  it('runs → GET /manage/tasks/runs 带 status/type/page/size 查询参数', async () => {
    await tasks.runs({ status: 'failed', type: 'hot_refresh', page: 2, size: 20 })
    expect(h.calls[0]).toMatchObject({
      method: 'GET',
      url: '/manage/tasks/runs',
      params: { status: 'failed', type: 'hot_refresh', page: 2, size: 20 }
    })
    // 无筛选时 undefined 参数原样传给 axios(请求拦截器会清理空值)
    await tasks.runs({ page: 1 })
    expect(h.calls[1]).toMatchObject({ method: 'GET', url: '/manage/tasks/runs', params: { page: 1 } })
  })

  it('overview → GET /manage/tasks/overview', async () => {
    await tasks.overview()
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/tasks/overview' })
  })

  it('rerun → POST /manage/tasks/:id/rerun', async () => {
    await tasks.rerun(7)
    expect(h.calls[0]).toMatchObject({ method: 'POST', url: '/manage/tasks/7/rerun' })
  })
})

describe('manage telemetry api → 新 REST 路由', () => {
  it('overview → GET /manage/telemetry/overview?days=', async () => {
    await telemetry.overview(3)
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/telemetry/overview', params: { days: 3 } })
  })

  it('listEvents → GET /manage/telemetry/events (type/days/limit/offset/status 透传)', async () => {
    await telemetry.listEvents({ type: 'error', days: 7, limit: 100, offset: 100, status: 'expired' })
    expect(h.calls[0]).toMatchObject({
      method: 'GET',
      url: '/manage/telemetry/events',
      params: { type: 'error', days: 7, limit: 100, offset: 100, status: 'expired' }
    })
  })

  it('resolveIssue → POST /manage/telemetry/resolve, body 含 category/path/label/commitId/note', async () => {
    await telemetry.resolveIssue({ category: 'js-error', path: '/play', label: 'boom', commitId: 'abc123', note: '修好了' })
    expect(h.calls[0]).toMatchObject({
      method: 'POST',
      url: '/manage/telemetry/resolve',
      body: { category: 'js-error', path: '/play', label: 'boom', commitId: 'abc123', note: '修好了' }
    })
  })

  it('unresolveIssue → POST /manage/telemetry/unresolve, body 为 issue 三元组', async () => {
    await telemetry.unresolveIssue({ category: 'js-error', path: '/play', label: 'boom' })
    expect(h.calls[0]).toMatchObject({
      method: 'POST',
      url: '/manage/telemetry/unresolve',
      body: { category: 'js-error', path: '/play', label: 'boom' }
    })
  })

  it('hotFilms → GET /manage/telemetry/hot-films, hours = days * 24 换算(默认 7 天=168h)', async () => {
    await telemetry.hotFilms(2, 5)
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/telemetry/hot-films', params: { hours: 48, limit: 5 } })
    await telemetry.hotFilms()
    expect(h.calls[1]).toMatchObject({ method: 'GET', url: '/manage/telemetry/hot-films', params: { hours: 168, limit: 10 } })
  })

  it('mostFavorited → GET /manage/telemetry/most-favorited?limit=', async () => {
    await telemetry.mostFavorited(5)
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/telemetry/most-favorited', params: { limit: 5 } })
  })

  it('apiPerf → GET /manage/telemetry/api-perf?days=&limit=', async () => {
    await telemetry.apiPerf(3, 5)
    expect(h.calls[0]).toMatchObject({ method: 'GET', url: '/manage/telemetry/api-perf', params: { days: 3, limit: 5 } })
  })
})

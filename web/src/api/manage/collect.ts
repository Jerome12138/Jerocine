import type { AxiosRequestConfig } from 'axios'
import { http } from '../http'
import type { BackendPage, CollectFailure, CollectParams, CollectSource } from '@/types/manage'

const enc = encodeURIComponent

// ---- 后端契约 DTO (entity.CollectSource 子集) ----
interface SourceDTO {
  id: string
  name: string
  uri: string
  /** 站点网址(可选) */
  siteUrl?: string
  resultModel: number
  grade: number
  syncPictures: boolean
  collectType: number
  intervalMs: number
  state: number // 0 启用 / 1 停用
}

const toView = (d: SourceDTO): CollectSource => ({
  id: d.id,
  name: d.name,
  uri: d.uri,
  siteUrl: d.siteUrl || '',
  resultModel: d.resultModel as CollectSource['resultModel'],
  grade: d.grade as CollectSource['grade'],
  syncPictures: d.syncPictures,
  collectType: d.collectType as CollectSource['collectType'],
  state: d.state === 0,
  interval: d.intervalMs
})

const toDTO = (v: CollectSource): SourceDTO => ({
  id: v.id,
  name: v.name,
  uri: v.uri,
  siteUrl: (v.siteUrl ?? '').trim(),
  resultModel: v.resultModel,
  grade: v.grade,
  syncPictures: v.syncPictures,
  collectType: v.collectType,
  intervalMs: Number(v.interval) || 0,
  state: v.state ? 0 : 1
})

/** GET /manage/collect-sources 采集源列表 */
export const list = async (): Promise<CollectSource[]> => {
  const data = await http.get<unknown, SourceDTO[]>('/manage/collect-sources')
  return (data ?? []).map(toView)
}

/** GET /manage/collect-sources/:id 采集源详情 */
export const find = async (id: string): Promise<CollectSource> =>
  toView(await http.get<unknown, SourceDTO>(`/manage/collect-sources/${enc(id)}`))

/** POST /manage/collect-sources 新增 */
export const add = (data: CollectSource): Promise<void> =>
  http.post<unknown, void>('/manage/collect-sources', toDTO(data))

/** PUT /manage/collect-sources/:id 修改 */
export const update = (data: CollectSource): Promise<void> =>
  http.put<unknown, void>(`/manage/collect-sources/${enc(data.id)}`, toDTO(data))

/** PUT /manage/collect-sources/:id 切换状态 / 同步图片(整体 upsert) */
export const change = (data: CollectSource): Promise<void> =>
  http.put<unknown, void>(`/manage/collect-sources/${enc(data.id)}`, toDTO(data))

/** DELETE /manage/collect-sources/:id 删除 */
export const remove = (id: string): Promise<void> =>
  http.delete<unknown, void>(`/manage/collect-sources/${enc(id)}`)

// ============ 实测延时 / 成功率 ============

/** 单源实测结果(后端多次探测取统计) */
export interface SourceTest {
  ok: boolean
  probes: number
  okCount: number
  latencyMs: number
  bestMs: number
  films: number
  message: string
}

/** 批量测速单条(带站点标识) */
export interface SourceTestRow extends SourceTest {
  id: string
  name: string
}

/** POST /manage/collect-sources/:id/test 单源测速(连续探测 ac=detail) */
export const test = (id: string): Promise<SourceTest> =>
  http.post<unknown, SourceTest>(`/manage/collect-sources/${enc(id)}/test`)

/** POST /manage/collect-sources/test 全部源并发测速 */
export const testAll = (): Promise<SourceTestRow[]> =>
  http.post<unknown, SourceTestRow[]>('/manage/collect-sources/test')

/** 采集源健康度(持久化, 含自动停采标记) */
export interface SourceHealthRow {
  id: string
  name: string
  state: boolean // 管理员启用开关(与 suppressed 正交)
  grade: number
  isMaster: boolean // 当前主站(grade=0)
  status: 'healthy' | 'degraded' | 'down' | 'unknown'
  suppressed: boolean // 连续失败达阈值被自动停采
  latencyMs: number // 采集 API 延时(服务端)
  films: number
  pageCount: number
  collected: number // 已采集片数(该源 movie_play_source 行数, 实时)
  total: number // 目录总片数(资源最全)
  playLatencyMs: number // 服务端抽样 m3u8 延时(0=未测, 广告过滤可达性)
  okCount: number
  probes: number
  consecutiveFails: number
  message: string
  checkedAt: number // ms
  /** 测速拆分: 采集(服务端 API) / 播放(浏览器端) */
  apiCheckedAt: number
  playLatencyWeb: number
  playCheckedAt: number
  /** 样本 m3u8(端侧播放测速用) */
  sampleM3u8: string
  /** 服务端 m3u8 可达性: null=未测 */
  adFilterOk: boolean | null
  adFilterAt: number
}

/** GET /manage/collect-sources/health 健康度面板 */
export const health = (): Promise<SourceHealthRow[]> =>
  http.get<unknown, SourceHealthRow[]>('/manage/collect-sources/health')

/** GET /manage/collect-sources/:id/sample-m3u8 端侧播放测速的样本输入 */
export const sampleM3u8 = async (id: string): Promise<string> => {
  const data = await http.get<unknown, { sampleM3u8: string }>(
    `/manage/collect-sources/${enc(id)}/sample-m3u8`
  )
  return data?.sampleM3u8 ?? ''
}

/** POST /manage/collect-sources/:id/play-latency 浏览器端播放测速结果回传落库 */
export const recordPlayLatency = (id: string, ms: number): Promise<void> =>
  http.post<unknown, void>(`/manage/collect-sources/${enc(id)}/play-latency`, { ms })

/** POST /ad-filter/feedback (公开) 播放端兜底上报: 广告过滤链路实际失败 → 立即标不可达 */
export const reportAdFilterFailure = (sourceId: string): Promise<void> =>
  http.post<unknown, void>('/ad-filter/feedback', { sourceId, ok: false })

// ============ 采集任务 / 控制 ============

/** POST /manage/spider/jobs 启动采集 (duration: -1 全量 / >0 增量小时) */
export const startSpider = (data: CollectParams, config?: AxiosRequestConfig): Promise<void> =>
  http.post<unknown, void>('/manage/spider/jobs', { sourceId: data.id, duration: data.time }, config)

/** POST /manage/spider/reset 清空全部影片 + 全量重采 (confirm 须匹配服务端令牌, 不可逆) */
export const resetSpider = (confirm: string): Promise<void> =>
  http.post<unknown, void>('/manage/spider/reset', { confirm })

/** 采集任务进度 (后端 cache.JobProgress → 视图模型) */
export interface SpiderJob {
  sourceId: string
  sourceName: string
  state: 'pending' | 'running' | 'paused' | 'done' | 'error' | 'canceled'
  totalPages: number
  donePages: number
  failedPages: number
  /** 已用时长 ms(由 startedAt 推算; 无则 0) */
  elapsedMs: number
  /** 采集时长: -1 全量 / >0 增量小时(0=未知, 重跑时按此还原) */
  hour: number
  /** 开始时间 unix 秒 */
  startedAt: number
  /** 失败原因(error 态) */
  error?: string
  note?: string
}

interface JobProgressDTO {
  sourceId: string
  name: string
  total: number
  done: number
  failed: number
  state: SpiderJob['state']
  startedAt?: number
  hour?: number
  error?: string
}

/** GET /manage/spider/jobs 当前 + 近 30 分钟内结束的采集任务 */
export const spiderJobs = async (): Promise<SpiderJob[]> => {
  const data = await http.get<unknown, JobProgressDTO[]>('/manage/spider/jobs')
  const now = Date.now()
  return (data ?? []).map((j) => ({
    sourceId: j.sourceId,
    sourceName: j.name,
    state: j.state,
    totalPages: j.total,
    donePages: j.done,
    failedPages: j.failed,
    hour: j.hour ?? 0,
    startedAt: j.startedAt ?? 0,
    error: j.error,
    elapsedMs: j.startedAt ? now - j.startedAt * 1000 : 0,
    note: undefined
  }))
}

/** POST /manage/spider/jobs/:id/pause (id 即 sourceId) */
export const spiderJobPause = (id: string): Promise<void> =>
  http.post<unknown, void>(`/manage/spider/jobs/${enc(id)}/pause`)

/** POST /manage/spider/jobs/:id/resume */
export const spiderJobResume = (id: string): Promise<void> =>
  http.post<unknown, void>(`/manage/spider/jobs/${enc(id)}/resume`)

/** POST /manage/spider/jobs/:id/cancel */
export const spiderJobCancel = (id: string): Promise<void> =>
  http.post<unknown, void>(`/manage/spider/jobs/${enc(id)}/cancel`)

// ============ 采集失败台账（页级失败补采） ============

/** 失败台账列表的响应形态（与全站分页契约一致：{list,page}） */
export interface FailurePage {
  list: CollectFailure[]
  page: BackendPage
}

/** GET /manage/collect-failures?status=&page=&size=
 *  status: -1 不限 / 0 待补采 / 1 已处理 */
export const failures = (params?: {
  status?: number
  page?: number
  size?: number
}): Promise<FailurePage> =>
  http.get<unknown, FailurePage>('/manage/collect-failures', { params })

/** POST /manage/collect-failures/recover {ids?}
 *  ids 为空 → 补采全部待处理；非空 → 只补这些。后台异步执行，接口立即返回 202。 */
export const recoverFailures = (ids?: number[]): Promise<{ accepted: boolean; pending: number }> =>
  http.post<unknown, { accepted: boolean; pending: number }>('/manage/collect-failures/recover', {
    ids: ids ?? []
  })

/** DELETE /manage/collect-failures/handled 清理已处理记录 */
export const clearHandledFailures = (): Promise<{ deleted: number }> =>
  http.delete<unknown, { deleted: number }>('/manage/collect-failures/handled')

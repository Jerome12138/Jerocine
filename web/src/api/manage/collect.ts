import type { AxiosRequestConfig } from 'axios'
import { http } from '../http'
import type { CollectParams, CollectSource } from '@/types/manage'

const enc = encodeURIComponent

// ---- 后端契约 DTO (entity.CollectSource 子集) ----
interface SourceDTO {
  id: string
  name: string
  uri: string
  resultModel: number
  grade: number
  syncPictures: boolean
  collectType: number
  intervalMs: number
  state: number // 0 启用 / 1 停用
  /** 仅端侧可达(服务器地域封无法访问, 如 bf): 服务端不测速, 测速走浏览器 */
  clientOnly?: boolean
}

const toView = (d: SourceDTO): CollectSource => ({
  id: d.id,
  name: d.name,
  uri: d.uri,
  resultModel: d.resultModel as CollectSource['resultModel'],
  grade: d.grade as CollectSource['grade'],
  syncPictures: d.syncPictures,
  collectType: d.collectType as CollectSource['collectType'],
  state: d.state === 0,
  interval: d.intervalMs,
  clientOnly: d.clientOnly === true
})

const toDTO = (v: CollectSource): SourceDTO => ({
  id: v.id,
  name: v.name,
  uri: v.uri,
  resultModel: v.resultModel,
  grade: v.grade,
  syncPictures: v.syncPictures,
  collectType: v.collectType,
  intervalMs: Number(v.interval) || 0,
  state: v.state ? 0 : 1,
  clientOnly: v.clientOnly === true
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
  latencyMs: number // 采集 API 延时
  films: number
  pageCount: number
  collected: number // 已采集片数(该源 movie_play_source 行数, 实时)
  total: number // 目录总片数(资源最全)
  playLatencyMs: number // 抽样播放延时
  okCount: number
  probes: number
  consecutiveFails: number
  message: string
  checkedAt: number // ms
}

/** GET /manage/collect-sources/health 健康度面板 */
export const health = (): Promise<SourceHealthRow[]> =>
  http.get<unknown, SourceHealthRow[]>('/manage/collect-sources/health')

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
  /** 后端 JobProgress 暂未跟踪用时/时长, 固定 0(视图按 0 显示 全量/0s) */
  elapsedMs: number
  hour: number
  note?: string
}

interface JobProgressDTO {
  sourceId: string
  name: string
  total: number
  done: number
  failed: number
  state: SpiderJob['state']
}

/** GET /manage/spider/jobs 当前 + 近 30 分钟内结束的采集任务 */
export const spiderJobs = async (): Promise<SpiderJob[]> => {
  const data = await http.get<unknown, JobProgressDTO[]>('/manage/spider/jobs')
  return (data ?? []).map((j) => ({
    sourceId: j.sourceId,
    sourceName: j.name,
    state: j.state,
    totalPages: j.total,
    donePages: j.done,
    failedPages: j.failed,
    elapsedMs: 0,
    hour: 0
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

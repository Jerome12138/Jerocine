import { http } from '../http'

export interface OverviewResp {
  pv: number
  uv: number
  uniqueUsers: number
  errorCount: number
  apiCount: number
  avgApiMs: number
  windowDays: number
}

export interface IssueResolution {
  id: number
  category: string
  path: string
  label: string
  resolvedAt: string
  resolvedCommitId: string
  resolvedBy: number
  note: string
  reopenedCount: number
}

/** issueStatus:
 *   unresolved = 未处理 (默认 active 列表)
 *   reopened   = 已解决但 24h 灰度期后又触发, 默认 active 列表显示
 *   resolved   = 已解决且 <= 30 天内无 24h 后的新事件, 不在 active 列表显示
 *   expired    = 已解决超过 30 天无 reopen, 不显示
 */
export type IssueStatus = 'unresolved' | 'reopened' | 'resolved' | 'expired'

export interface TelemetryEventRow {
  id: number
  ts: number
  serverTs: string
  type: string
  category: string
  action: string
  label: string
  value: number
  path: string
  userId: number
  sessionId: string
  platform: string
  appVersion: string
  extra: string
  resolution?: IssueResolution
  issueStatus: IssueStatus
}

export interface ListEventsResp {
  total: number
  rows: TelemetryEventRow[]
}

export interface FilmStat {
  mid: number
  name: string
  cover: string
  count: number
}

export interface ApiPerfItem {
  action: string
  count: number
  avg: number
  p50: number
  p95: number
  p99: number
}

export const overview = (days = 7): Promise<OverviewResp> =>
  http.get('/manage/telemetry/overview', { params: { days } })

export const listEvents = (params: {
  type?: string
  days?: number
  limit?: number
  offset?: number
  /** active=未解决+重启+宽限期(默认) | all | resolved | reopened */
  status?: 'active' | 'all' | 'resolved' | 'reopened'
}): Promise<ListEventsResp> =>
  http.get('/manage/telemetry/events', { params })

/** 标记某 issue 已解决 — key = (category,path,label) 必须与 telemetry 行一致 */
export const resolveIssue = (body: {
  category: string
  path: string
  label: string
  commitId: string
  note?: string
}): Promise<void> => http.post('/manage/telemetry/resolve', body)

/** 撤销解决 (重新标记为未解决) */
export const unresolveIssue = (body: {
  category: string
  path: string
  label: string
}): Promise<void> => http.post('/manage/telemetry/unresolve', body)

/** 热点视频 — 按访问量 (PV) 排序. hours = days * 24, 服务端默认 168h(7天) */
export const hotFilms = (days = 7, limit = 10): Promise<FilmStat[]> =>
  http.get('/manage/telemetry/hot-films', { params: { hours: days * 24, limit } })

/** 收藏最多 — 按收藏数排序 */
export const mostFavorited = (limit = 10): Promise<FilmStat[]> =>
  http.get('/manage/telemetry/most-favorited', { params: { limit } })

export const apiPerf = (days = 7, limit = 10): Promise<ApiPerfItem[]> =>
  http.get('/manage/telemetry/api-perf', { params: { days, limit } })

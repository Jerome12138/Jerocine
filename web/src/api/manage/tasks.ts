import type { AxiosRequestConfig } from 'axios'
import { http } from '../http'
import type { BackendPage, TaskOverview, TaskRun } from '@/types/manage'

/**
 * 任务管理 API: 全部定时/手动任务的运行台账(状态、失败原因、重跑)。
 * 后端: /manage/tasks/*。
 */

/** GET /manage/tasks/runs?status=&type=&page=&size= 台账分页 */
export interface TaskRunPage {
  list: TaskRun[]
  page: BackendPage
}

export const runs = (params?: {
  status?: string
  type?: string
  page?: number
  size?: number
}): Promise<TaskRunPage> => http.get<unknown, TaskRunPage>('/manage/tasks/runs', { params })

/** GET /manage/tasks/overview 统计卡 */
export const overview = (config?: AxiosRequestConfig): Promise<TaskOverview> =>
  http.get<unknown, TaskOverview>('/manage/tasks/overview', config)

/** POST /manage/tasks/:id/rerun 重跑一条历史任务(按类型重放, 会登记一条新记录)。
 *  服务端后台受理(202): 榜单刷新等重活在请求外执行, 结果看台账, 不内联等待。 */
export const rerun = (id: number, config?: AxiosRequestConfig): Promise<{ accepted: boolean }> =>
  http.post<unknown, { accepted: boolean }>(`/manage/tasks/${id}/rerun`, undefined, config)

// ---------------------------------------------------------------------------
// 统一任务中心（PR1/PR2 后端已就绪）
// ---------------------------------------------------------------------------

/** 任务中心的一行（内置任务 + 采集定时任务统一口径） */
export interface TaskCenterItem {
  key: string
  kind: 'builtin' | 'collect' | string
  type: string
  name: string
  target: string
  detail?: string
  /** 内置=友好文本；采集=cron 表达式 */
  schedule: string
  /** 中文展示 */
  scheduleText: string
  enabled: boolean
  intervalMs: number
  /** 执行层下限（低于它会被 422） */
  minIntervalMs: number
  intervalLabel: string
  /** enabled / schedule / remark / interval */
  editable: string[]
  /** run / pause / resume / stop */
  caps: string[]
  state: string
  /** null = 总数未知（前端不画进度条） */
  progress: { total: number; done: number; failed: number; unit: string } | null
  startedAt: number
  elapsedMs: number
  lastRunAt: number
  message: string
  error: string
}

export interface TaskCenterOverview {
  running: number
  builtin: number
  collect: number
  failed24h: number
}

/** GET /manage/task-center/tasks */
export const taskCenterTasks = (kind = ''): Promise<{ list: TaskCenterItem[] }> =>
  http.get<unknown, { list: TaskCenterItem[] }>('/manage/task-center/tasks', {
    params: kind ? { kind } : undefined,
    silent: true
  })

/** GET /manage/task-center/overview */
export const taskCenterOverview = (): Promise<TaskCenterOverview> =>
  http.get<unknown, TaskCenterOverview>('/manage/task-center/overview', { silent: true })

/** PUT /manage/task-center/tasks/:key/setting */
export const taskCenterUpdateSetting = (
  key: string,
  payload: { enabled?: boolean; schedule?: string; remark?: string; intervalMs?: number }
): Promise<{ ok: boolean }> =>
  http.put<unknown, { ok: boolean }>(
    `/manage/task-center/tasks/${encodeURIComponent(key)}/setting`,
    payload,
    { silent: true }
  )

/** POST /manage/task-center/tasks/:key/run */
export const taskCenterRun = (key: string): Promise<{ accepted: boolean }> =>
  http.post<unknown, { accepted: boolean }>(
    `/manage/task-center/tasks/${encodeURIComponent(key)}/run`,
    {},
    { silent: true }
  )

/** 采集任务的暂停/继续/停止（key=job:<sourceId>） */
export const taskCenterPause = (key: string): Promise<{ ok: boolean }> =>
  http.post<unknown, { ok: boolean }>(`/manage/task-center/tasks/${encodeURIComponent(key)}/pause`, {}, { silent: true })
export const taskCenterResume = (key: string): Promise<{ ok: boolean }> =>
  http.post<unknown, { ok: boolean }>(`/manage/task-center/tasks/${encodeURIComponent(key)}/resume`, {}, { silent: true })
export const taskCenterStop = (key: string): Promise<{ ok: boolean }> =>
  http.post<unknown, { ok: boolean }>(`/manage/task-center/tasks/${encodeURIComponent(key)}/stop`, {}, { silent: true })

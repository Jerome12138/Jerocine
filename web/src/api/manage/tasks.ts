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
  /** true=只看已忽略 / false=只看未忽略 / 不传=全部 */
  ignored?: boolean
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
  /** 下次计划触发时刻（unix 毫秒；0=调度器还没算，如停用/周期待配置） */
  nextAt: number
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

/** PUT /manage/task-center/tasks/:key/setting
 *  deferred=true: 该任务此刻有一轮在跑 —— 停用只影响下次触发，本轮跑完才真正停下 */
export const taskCenterUpdateSetting = (
  key: string,
  payload: { enabled?: boolean; schedule?: string; remark?: string; intervalMs?: number }
): Promise<{ ok: boolean; deferred?: boolean }> =>
  http.put<unknown, { ok: boolean; deferred?: boolean }>(
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

/** POST /manage/task-center/tasks/batch-state 批量启用/停用
 *  deferred: 本次被停用但**此刻正在跑**的任务 key（等本轮跑完才真正停下） */
export const taskCenterBatchState = (
  keys: string[],
  enabled: boolean
): Promise<{ ok: number; deferred?: string[]; errors: string[] }> =>
  http.post<unknown, { ok: number; deferred?: string[]; errors: string[] }>(
    '/manage/task-center/tasks/batch-state',
    { keys, enabled },
    { silent: true }
  )

/** GET /manage/task-center/schedule 调度态只读诊断
 *  items[].running = 调度器内存里是否正有一轮在跑（**QuietOnSuccess 任务台账没有 running 行**，
 *  所以"正在运行"必须看这个，而不是查台账）。 */
export interface ScheduleItem {
  key: string
  name: string
  kind: string
  enabled: boolean
  schedule: string
  scheduleText: string
  nextAt: number
  running: boolean
}
export const taskCenterSchedule = (): Promise<{
  items: ScheduleItem[]
  gateOpen: boolean
  tickMs: number
  now: number
}> => http.get<unknown, { items: ScheduleItem[]; gateOpen: boolean; tickMs: number; now: number }>(
  '/manage/task-center/schedule',
  { silent: true }
)

/** GET /manage/task-center/runs/pending "待处理异常"
 *  口径：failed 且未忽略，且其后没有同一任务的成功运行 —— 已重跑成功的旧失败不会出现在这里。
 *  （与「历史记录」的全量台账是两个口径，别混用） */
export const pendingRuns = (
  limit = 200
): Promise<{ items: TaskRun[]; count: number; limit: number; truncated: boolean }> =>
  http.get<unknown, { items: TaskRun[]; count: number; limit: number; truncated: boolean }>(
    '/manage/task-center/runs/pending',
    { params: { limit } }
  )

/** POST /manage/douban/batch/stop 中止当前豆瓣批次（评分同步/欠账补查）
 *  stopped=false 表示当前没有批次在跑 */
export const doubanBatchStop = (): Promise<{ stopped: boolean }> =>
  http.post<unknown, { stopped: boolean }>('/manage/douban/batch/stop', {}, { silent: true })

/** POST /manage/task-center/runs/:id/rerun 异常重跑 */
export const rerunRun = (id: number): Promise<{ accepted: boolean }> =>
  http.post<unknown, { accepted: boolean }>(`/manage/task-center/runs/${id}/rerun`, {}, { silent: true })

/** POST /manage/task-center/runs/:id/ignore 忽略/恢复一条异常 */
export const ignoreRun = (id: number, ignored = true): Promise<{ ok: boolean; ignored: boolean }> =>
  http.post<unknown, { ok: boolean; ignored: boolean }>(
    `/manage/task-center/runs/${id}/ignore`,
    { ignored },
    { silent: true }
  )

/** 采集任务的暂停/继续/停止（key=job:<sourceId>） */
export const taskCenterPause = (key: string): Promise<{ ok: boolean }> =>
  http.post<unknown, { ok: boolean }>(`/manage/task-center/tasks/${encodeURIComponent(key)}/pause`, {}, { silent: true })
export const taskCenterResume = (key: string): Promise<{ ok: boolean }> =>
  http.post<unknown, { ok: boolean }>(`/manage/task-center/tasks/${encodeURIComponent(key)}/resume`, {}, { silent: true })
export const taskCenterStop = (key: string): Promise<{ ok: boolean }> =>
  http.post<unknown, { ok: boolean }>(`/manage/task-center/tasks/${encodeURIComponent(key)}/stop`, {}, { silent: true })

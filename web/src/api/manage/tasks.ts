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
export const overview = (): Promise<TaskOverview> =>
  http.get<unknown, TaskOverview>('/manage/tasks/overview')

/** POST /manage/tasks/:id/rerun 重跑一条历史任务(按类型重放, 会登记一条新记录) */
export const rerun = (id: number): Promise<{ accepted: boolean }> =>
  http.post<unknown, { accepted: boolean }>(`/manage/tasks/${id}/rerun`)

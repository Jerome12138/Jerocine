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

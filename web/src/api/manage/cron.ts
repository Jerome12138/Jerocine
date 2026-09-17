import { http } from '../http'
import type { CronTask } from '@/types/manage'

const enc = encodeURIComponent

// ---- 后端契约 DTO (entity.CronTask 子集 + 最近一次运行信息) ----
interface CronDTO {
  id: number
  sourceIds: string[]
  spec: string
  time: number
  model: number
  state: number // 0 启用 / 1 停用
  remark: string
  /** 最近一次运行(任务台账回填, 可能缺省) */
  lastRunAt?: number
  lastStatus?: string
  lastError?: string
  lastMessage?: string
}

const toView = (d: CronDTO): CronTask => ({
  id: String(d.id),
  ids: d.sourceIds ?? [],
  time: d.time,
  spec: d.spec,
  model: d.model === 1 ? 1 : d.model === 2 ? 2 : 0,
  state: d.state === 0,
  remark: d.remark,
  lastRunAt: d.lastRunAt ?? 0,
  lastStatus: d.lastStatus,
  lastError: d.lastError,
  lastMessage: d.lastMessage
})

const toDTO = (v: CronTask): CronDTO => ({
  id: Number(v.id) || 0,
  sourceIds: v.ids ?? [],
  spec: v.spec,
  time: Number(v.time) || 0,
  model: v.model,
  state: v.state ? 0 : 1,
  remark: v.remark ?? ''
})

/** GET /manage/cron-tasks 定时任务列表 */
export const list = async (): Promise<CronTask[]> => {
  const data = await http.get<unknown, CronDTO[]>('/manage/cron-tasks')
  return (data ?? []).map(toView)
}

/** POST /manage/cron-tasks 新增 */
export const add = (data: CronTask): Promise<void> =>
  http.post<unknown, void>('/manage/cron-tasks', toDTO(data))

/** PUT /manage/cron-tasks/:id 修改 */
export const update = (data: CronTask): Promise<void> =>
  http.put<unknown, void>(`/manage/cron-tasks/${enc(data.id)}`, toDTO(data))

/** PUT /manage/cron-tasks/:id 切换状态 */
export const change = (data: CronTask): Promise<void> =>
  http.put<unknown, void>(`/manage/cron-tasks/${enc(data.id)}`, toDTO(data))

/** DELETE /manage/cron-tasks/:id 删除 */
export const remove = (id: string): Promise<void> =>
  http.delete<unknown, void>(`/manage/cron-tasks/${enc(id)}`)

/** POST /manage/cron-tasks/:id/run 手动立即执行一次(登记为手动任务) */
export const runNow = (id: string): Promise<{ accepted: boolean }> =>
  http.post<unknown, { accepted: boolean }>(`/manage/cron-tasks/${enc(id)}/run`)

import { http } from './http'
import type { HistoryListResp, HistoryUpsertPayload } from '@/types/history'

/** 观看历史 API(登录态; 未登录由 stores/history 走本地兜底) */

/** POST /me/histories 上报/更新观看进度 (后台 fire-and-forget, 静默不弹"操作成功"
 *  —— 原生播放器进度/切集/退出事件会高频写, 否则从 TV 返回时刷几十个成功提示) */
export const upsert = (data: HistoryUpsertPayload): Promise<void> =>
  http.post<unknown, void>('/me/histories', data, { silent: true })

/** GET /me/histories 分页拉历史 */
export const list = (params?: { page?: number; size?: number }): Promise<HistoryListResp> =>
  http.get<unknown, HistoryListResp>('/me/histories', { params })

/** DELETE /me/histories?mid= 删除单条 */
export const remove = (params: { mid: number | string }): Promise<void> =>
  http.delete<unknown, void>('/me/histories', { params: { mid: params.mid } })

/** DELETE /me/histories/clear 清空 */
export const clear = (): Promise<void> => http.delete<unknown, void>('/me/histories/clear')

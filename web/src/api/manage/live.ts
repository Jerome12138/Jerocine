import { http } from '../http'
import { invalidateCache } from '@/utils/swrCache'
import type {
  LiveSourceManage,
  LiveSourceInput,
  LiveChannelsResp,
  LiveChannelsManageResp,
  LiveChannelInput,
  LiveProbeResult
} from '@/types/live'

/** GET /manage/live-sources 全量直播源（含停用） */
export const listLiveSources = (): Promise<{ sources: LiveSourceManage[] }> =>
  http.get<unknown, { sources: LiveSourceManage[] }>('/manage/live-sources')

/** POST /manage/live-sources 新增直播源 */
export const createLiveSource = (data: LiveSourceInput, config?: { silent?: boolean }): Promise<LiveSourceManage> =>
  http.post<unknown, LiveSourceManage>('/manage/live-sources', data, config).finally(() => {
    invalidateCache('live:')
  })

/** PUT /manage/live-sources/:id 整体覆盖直播源 */
export const updateLiveSource = (id: number, data: LiveSourceInput, config?: { silent?: boolean }): Promise<void> =>
  http.put(`/manage/live-sources/${id}`, data, config).then(() => {
    invalidateCache('live:')
  })

/** DELETE /manage/live-sources/:id 删除直播源 */
export const deleteLiveSource = (id: number, config?: { silent?: boolean }): Promise<void> =>
  http.delete(`/manage/live-sources/${id}`, config).then(() => {
    invalidateCache('live:')
  })

/** GET /live/sources/:id/channels 频道数预览（复用公开解析端点; 停用源会 404） */
export const previewChannels = (id: number): Promise<LiveChannelsResp> =>
  http.get<unknown, LiveChannelsResp>(`/live/sources/${id}/channels`, { silent: true })

/** GET /manage/live-channels?sourceId= 频道列表（解析+覆盖层合并, 含隐藏项 + 探源标记） */
export const listLiveChannels = (sourceId: number): Promise<LiveChannelsManageResp> =>
  http.get<unknown, LiveChannelsManageResp>('/manage/live-channels', {
    params: { sourceId },
    silent: true
  })

/** PUT /manage/live-channels 频道覆盖层写入（隐藏/改名/自定义频道增改） */
export const upsertLiveChannel = (data: LiveChannelInput, config?: { silent?: boolean }): Promise<void> =>
  http.put('/manage/live-channels', data, config)

/** DELETE /manage/live-channels?sourceId=&url= 删频道（自定义物理删/解析置 hidden） */
export const deleteLiveChannel = (sourceId: number, url: string, config?: { silent?: boolean }): Promise<void> =>
  http.delete('/manage/live-channels', { params: { sourceId, url }, ...config })

/** GET /manage/live-nav 直播顶栏入口开关(000038) */
export const getLiveNav = (): Promise<{ liveNavEnabled: boolean }> =>
  http.get<unknown, { liveNavEnabled: boolean }>('/manage/live-nav')

/** PUT /manage/live-nav {liveNavEnabled} 写直播顶栏开关并失效配置缓存 */
export const updateLiveNav = (liveNavEnabled: boolean, config?: { silent?: boolean }): Promise<void> =>
  http.put<unknown, void>('/manage/live-nav', { liveNavEnabled }, config)

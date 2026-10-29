import { http } from './http'
import { swrGet } from '@/utils/swrCache'
import type { LiveSourcesResp, LiveChannelsResp } from '@/types/live'

/** GET /live/sources 直播源列表（SWR: 源列表低频变更, 切回复用 + 后台刷新） */
export const getLiveSources = (): Promise<LiveSourcesResp> =>
  swrGet('live:sources', (silent) => http.get<unknown, LiveSourcesResp>('/live/sources', { silent }))

/** GET /live/sources/:id/channels 直播源频道列表（服务端解析 m3u + 缓存）
 *  options.signal 用于卸载中止。 */
export const getLiveChannels = (
  id: number,
  options?: { signal?: AbortSignal }
): Promise<LiveChannelsResp> =>
  http.get<unknown, LiveChannelsResp>(`/live/sources/${id}/channels`, {
    signal: options?.signal,
    silent: true
  })

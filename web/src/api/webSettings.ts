import { http } from './http'

/**
 * 账号级 Web 设置快照 (/me/web-settings) —— JSON blob 整存整取。
 * 语义: 未登录仅本地(localStorage); 登录后本地立即生效 + 防抖上报云端, 登录时云端拉取合并。
 * 结构由前端维护(见 useCloudSettings 的 SYNC_KEYS), 服务端只存不解释 —— 新设置项进 blob 即随账号记忆。
 */
export interface WebSettingsResp {
  data: Record<string, unknown>
}

export const getWebSettings = (): Promise<WebSettingsResp> =>
  http.get<unknown, WebSettingsResp>('/me/web-settings', { silent: true })

export const putWebSettings = (data: Record<string, unknown>): Promise<unknown> =>
  http.put('/me/web-settings', { data }, { silent: true })

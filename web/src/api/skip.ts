import { http } from './http'

/** 账号级片头/片尾跳过设置(后端 user_skip_setting, 跨设备) */
export interface SkipSettingRow {
  mid: number
  intro: number
  outro: number
}

/** GET /me/skip-settings 取当前用户全部跳过设置 */
export const skipList = (): Promise<SkipSettingRow[]> =>
  http.get<unknown, SkipSettingRow[]>('/me/skip-settings')

/** PUT /me/skip-settings/:mid upsert 某片跳过设置 */
export const skipSave = (mid: number, intro: number, outro: number): Promise<unknown> =>
  http.put(`/me/skip-settings/${mid}`, { intro, outro })

/** DELETE /me/skip-settings/:mid 删除某片跳过设置 */
export const skipReset = (mid: number): Promise<void> =>
  http.delete<unknown, void>(`/me/skip-settings/${mid}`)

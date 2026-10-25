import { http } from '../http'
import type { DanmuConfigResp, DanmuConfigUpdate } from '@/types/manage'

/** GET /manage/danmu-config 读弹幕后台配置（总开关 + 开放平台白名单 + 平台全集） */
export const getDanmuConfig = (): Promise<DanmuConfigResp> =>
  http.get<unknown, DanmuConfigResp>('/manage/danmu-config')

/** PUT /manage/danmu-config 保存（enabled 0/1 + providers 白名单；成功后回最新配置） */
export const saveDanmuConfig = (data: DanmuConfigUpdate): Promise<DanmuConfigResp> =>
  http.put<unknown, DanmuConfigResp>('/manage/danmu-config', data)

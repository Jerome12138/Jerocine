import { http } from './http'

/** 扫码登录(设备码流程) */
export interface DeviceCodeResp {
  deviceCode: string
  userCode: string
  expiresIn: number
  interval: number
}

export interface DevicePollResp {
  status: 'pending' | 'expired' | 'ok'
  token?: string
  userName?: string
  expires?: number
  role?: number
}

/** POST /auth/device/code 取设备码(待登录端) */
export const deviceCode = (): Promise<DeviceCodeResp> =>
  http.post<unknown, DeviceCodeResp>('/auth/device/code', undefined, { silent: true })

/** POST /auth/device/poll 轮询(待登录端)
 *  silent: 轮询是后台心跳, 不应弹出 loading 条 / "操作成功" toast(每 1~2s 一次)。
 *  拦截器已支持 silent 跳过 loading 与成功提示, 错误也静默(轮询失败由 LoginView 自行续轮)。 */
export const devicePoll = (dc: string): Promise<DevicePollResp> =>
  http.post<unknown, DevicePollResp>('/auth/device/poll', { deviceCode: dc }, { silent: true })

/** POST /auth/device/confirm 手机端确认授权(需登录) */
export const deviceConfirm = (userCode: string): Promise<unknown> =>
  http.post('/auth/device/confirm', { userCode })

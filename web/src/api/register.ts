import { http } from './http'
import type { LoginResult, RegisterConfig, RegisterPayload } from '@/types/user'

/** GET /auth/register-config 注册模式（登录页据此显隐注册入口） */
export const getRegisterConfig = (): Promise<RegisterConfig> =>
  http.get<unknown, RegisterConfig>('/auth/register-config')

/** POST /auth/register/code 发送邮箱验证码（purpose: register | reset） */
export const sendRegisterCode = (email: string, purpose = 'register'): Promise<{ sent: boolean }> =>
  http.post<unknown, { sent: boolean }>('/auth/register/code', { email, purpose })

/** POST /auth/register open 模式自注册（成功即登录，载荷同登录） */
export const registerAccount = (data: RegisterPayload): Promise<LoginResult> =>
  http.post<unknown, LoginResult>('/auth/register', data)

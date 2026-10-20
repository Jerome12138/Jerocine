import { http } from './http'
import type {
  ApplyPayload,
  ApplyStatusView,
  LoginResult,
  RegisterConfig,
  RegisterPayload
} from '@/types/user'

/** GET /auth/register-config 注册模式（登录页据此显隐注册入口） */
export const getRegisterConfig = (): Promise<RegisterConfig> =>
  http.get<unknown, RegisterConfig>('/auth/register-config')

/** POST /auth/register/code 发送邮箱验证码（purpose: register | reset） */
export const sendRegisterCode = (email: string, purpose = 'register'): Promise<{ sent: boolean }> =>
  http.post<unknown, { sent: boolean }>('/auth/register/code', { email, purpose })

/** POST /auth/register open 模式自注册（成功即登录，载荷同登录） */
export const registerAccount = (data: RegisterPayload): Promise<LoginResult> =>
  http.post<unknown, LoginResult>('/auth/register', data)

/** POST /auth/register/apply apply 模式申请（不收密码；成功只回 applied，等审核邮件） */
export const applyRegister = (data: ApplyPayload): Promise<{ applied: boolean }> =>
  http.post<unknown, { applied: boolean }>('/auth/register/apply', data)

/** GET /auth/apply/status?email= 查申请进度（pending / approved 可重发 / rejected+理由 / none） */
export const applyStatus = (email: string): Promise<ApplyStatusView> =>
  http.get<unknown, ApplyStatusView>('/auth/apply/status', { params: { email } })

/** POST /auth/apply/resend 重发激活邮件（approved 且未激活；作废旧 token，新链接 7 天有效） */
export const resendActivation = (email: string): Promise<{ resent: boolean }> =>
  http.post<unknown, { resent: boolean }>('/auth/apply/resend', { email })

/** POST /auth/activate 激活账号（一次性 token → 设密码建号 → 自动登录，载荷同登录） */
export const activateAccount = (data: {
  token: string
  userName: string
  password: string
}): Promise<LoginResult> => http.post<unknown, LoginResult>('/auth/activate', data)

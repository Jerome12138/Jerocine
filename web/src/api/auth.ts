import { http } from './http'
import type { Paginated } from '@/types/api'
import type {
  ChangePasswordPayload,
  CreateUserPayload,
  LoginPayload,
  LoginResult,
  ManageUserItem,
  UserInfo
} from '@/types/user'

/**
 * POST /auth/login 登录(契约: {account, password})。
 * silent: 登录失败由页面内 errorMsg 展示, 不走全局 toast ——
 * 尤其 apply 模式申请态反馈(403 审核中/已驳回)会被全局 403 文案误报成"权限不足"。
 */
export const login = (data: LoginPayload): Promise<LoginResult> =>
  http.post<unknown, LoginResult>('/auth/login', { account: data.userName, password: data.password }, { silent: true })

/** POST /auth/logout 退出登录 */
export const logout = (): Promise<void> => http.post<unknown, void>('/auth/logout')

/** PATCH /me/password 修改密码(契约: {oldPassword, newPassword}) */
export const changePassword = (data: ChangePasswordPayload): Promise<void> =>
  http.patch<unknown, void>('/me/password', {
    oldPassword: data.password,
    newPassword: data.newPassword
  })

/** GET /me 当前登录用户信息 */
export const getUserInfo = (): Promise<UserInfo> => http.get<unknown, UserInfo>('/me')

/* ============== 找回密码（P3） ============== */

/** POST /auth/password/reset-link {email} 忘记密码 — 发一次性重置链接邮件(30 分钟)；
 *  防账号枚举: 邮箱是否存在响应一致(200 {sent}) */
export const requestPasswordReset = (email: string): Promise<{ sent: boolean }> =>
  http.post<unknown, { sent: boolean }>('/auth/password/reset-link', { email })

/** POST /auth/password/reset {token, password} 用重置链接设新密码; 成功后该用户全部设备下线 */
export const resetPassword = (data: { token: string; password: string }): Promise<{ reset: boolean }> =>
  http.post<unknown, { reset: boolean }>('/auth/password/reset', data)

/* ============== 管理员后台用户管理 ============== */

/** POST /manage/users 创建用户 */
export const createUser = (data: CreateUserPayload): Promise<UserInfo> =>
  http.post<unknown, UserInfo>('/manage/users', data)

/** GET /manage/users 用户列表 */
export const listUsers = (params?: {
  page?: number
  size?: number
}): Promise<Paginated<ManageUserItem>> =>
  http.get<unknown, Paginated<ManageUserItem>>('/manage/users', { params })

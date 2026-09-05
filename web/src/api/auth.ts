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

/** POST /auth/login 登录(契约: {account, password}) */
export const login = (data: LoginPayload): Promise<LoginResult> =>
  http.post<unknown, LoginResult>('/auth/login', { account: data.userName, password: data.password })

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

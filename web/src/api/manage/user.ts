import { http } from '../http'
import type { UserInfo } from '@/types/user'
import type { ManageUserListResp } from '@/types/manage'

/** GET /me 当前登录用户信息(后端无独立 /manage/user/info, 复用 /me) */
export const info = (): Promise<UserInfo> => http.get<unknown, UserInfo>('/me')

/** GET /manage/users 用户列表(keyword 用户名模糊搜索) */
export const list = (params: {
  keyword?: string
  page?: number
  size?: number
}): Promise<ManageUserListResp> => http.get<unknown, ManageUserListResp>('/manage/users', { params })

/** PATCH /manage/users/:id/disabled 禁用/启用(禁用即踢下线; 不能操作自己) */
export const setDisabled = (id: number, disabled: boolean): Promise<unknown> =>
  http.patch(`/manage/users/${id}/disabled`, { disabled })

/** PATCH /manage/users/:id/password 管理员重置密码(6-64 位, 重置后该用户下线) */
export const resetPassword = (id: number, password: string): Promise<unknown> =>
  http.patch(`/manage/users/${id}/password`, { password })

/** POST /manage/users 新增用户(密码 6-64 位) */
export const create = (userName: string, password: string, role: number): Promise<unknown> =>
  http.post('/manage/users', { userName, password, role })

/** PUT /manage/users/:id 编辑用户(用户名+角色; 角色变化后该用户下线; 不能改自己的角色) */
export const update = (id: number, userName: string, role: number): Promise<unknown> =>
  http.put(`/manage/users/${id}`, { userName, role })

/** DELETE /manage/users/:id 删除用户(硬删, 连同其历史/收藏; 不能删除自己) */
export const remove = (id: number): Promise<unknown> => http.delete(`/manage/users/${id}`)

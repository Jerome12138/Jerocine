/** 登录请求体（后端 json 标签 userName，Go 默认大小写不敏感） */
export interface LoginPayload {
  userName: string
  password: string
}

/**
 * 登录响应体 (POST /user/login -> data)
 * - token: 不带 "Bearer " 前缀的原始 JWT, 由前端在请求头中拼成 Authorization: Bearer <token>
 * - expires: token 过期时间, unix 秒
 * - role: 0 普通用户 / 1 管理员
 */
export interface LoginResult {
  userName: string
  token: string
  expires: number
  role: UserRole | number
}

/** 用户角色：0 普通用户，1 管理员（与后端 model.User.Role 对齐） */
export type UserRole = 0 | 1

/** 后端用户信息（GET /user/info 与 /manage/user/info 复用） */
export interface UserInfo {
  id?: number
  uid?: string
  /** 后端规范字段 */
  userName?: string
  /** 兼容旧站可能的小写形态 */
  username?: string
  email?: string
  gender?: number
  /** 后端规范字段 */
  nickName?: string
  nickname?: string
  avatar?: string
  status?: number
  /** 0 普通用户 / 1 管理员 */
  role?: UserRole | number
}

/** 修改密码 payload（POST /user/changePassword） */
export interface ChangePasswordPayload {
  password: string
  newPassword: string
}

/** 管理员后台创建用户 payload（POST /manage/user/create） */
export interface CreateUserPayload {
  userName: string
  password: string
  email?: string
  nickName?: string
  role?: UserRole
}

/** 管理员后台用户列表项 */
export interface ManageUserItem {
  id: number
  userName: string
  email?: string
  nickName?: string
  avatar?: string
  status?: number
  role: UserRole | number
}

/** 管理员后台用户列表响应（{list, page}） */
export interface ManageUserListResp {
  list: ManageUserItem[]
  page: {
    pageSize: number
    current: number
    pageCount: number
    total: number
  }
}

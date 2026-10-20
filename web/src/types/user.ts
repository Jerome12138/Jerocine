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

/* ============== 账号注册（自注册 open / 申请 apply） ============== */

/** 注册模式：off 仅后台建号 / open 自由注册 / apply 申请审核 */
export type RegisterMode = 'off' | 'open' | 'apply'

/** GET /auth/register-config */
export interface RegisterConfig {
  mode: RegisterMode
  /** 邮件通道可用；false 时 open/apply 实际不可用（发码被服务端拒绝） */
  smtpReady: boolean
}

/** POST /auth/register 请求体 */
export interface RegisterPayload {
  userName: string
  password: string
  email: string
  code: string
}

/**
 * POST /auth/register/apply 请求体（apply 模式）。
 * 刻意无 password: 密码激活时才设(设计 §9.2 —— 申请到审批可能隔数天,
 * 激活时设当场记得住; pending 行也少一类敏感数据)。
 */
export interface ApplyPayload {
  userName: string
  email: string
  reason?: string
  code: string
}

/** GET /auth/apply/status 响应（进度查询: 三通道之一, 兜底邮件丢失/过期） */
export interface ApplyStatusView {
  /** pending 审核中 / approved 已通过(可重发) / rejected 已驳回 / none 无记录 */
  status: 'pending' | 'approved' | 'rejected' | 'none'
  /** 驳回理由(rejected 时) */
  reason: string
  /** approved 且未激活 → 激活邮件可重发 */
  canResend: boolean
}

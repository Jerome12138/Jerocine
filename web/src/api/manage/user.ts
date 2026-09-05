import { http } from '../http'
import type { UserInfo } from '@/types/user'

/** GET /me 当前登录用户信息(后端无独立 /manage/user/info, 复用 /me) */
export const info = (): Promise<UserInfo> => http.get<unknown, UserInfo>('/me')

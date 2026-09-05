import { http } from './http'
import type { FavoriteAddPayload, FavoriteCheckResp, FavoriteListResp } from '@/types/favorite'

/** 收藏 API(登录态) */

/** POST /me/favorites 添加收藏(后端幂等) */
export const add = (data: FavoriteAddPayload): Promise<void> =>
  http.post<unknown, void>('/me/favorites', data)

/** DELETE /me/favorites?mid= 取消收藏 */
export const remove = (mid: number | string): Promise<void> =>
  http.delete<unknown, void>('/me/favorites', { params: { mid } })

/** GET /me/favorites 分页拉收藏 */
export const list = (params?: { page?: number; size?: number }): Promise<FavoriteListResp> =>
  http.get<unknown, FavoriteListResp>('/me/favorites', { params })

/** GET /me/favorites/:mid 查询是否已收藏 */
export const check = (mid: number | string): Promise<FavoriteCheckResp> =>
  http.get<unknown, FavoriteCheckResp>(`/me/favorites/${mid}`)

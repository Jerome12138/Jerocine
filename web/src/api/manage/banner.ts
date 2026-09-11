import { http } from '../http'
import type { Banner, BannerBoard } from '@/types/manage'

const enc = encodeURIComponent

/** GET /manage/banners/effective 管理视图：生效位(前5, 手动+自动补位) + 未生效配置行 */
export const board = (): Promise<BannerBoard> =>
  http.get<unknown, BannerBoard>('/manage/banners/effective')

/** POST /manage/banners/move 生效位排序（手动位换位 / 自动位上移转手动），返回新 Board */
export const move = (slot: number, dir: 'up' | 'down'): Promise<BannerBoard> =>
  http.post<unknown, BannerBoard>('/manage/banners/move', { slot, dir })

/** POST /manage/banners 新建（slot = 钉入位置，采纳自动位时传）；PUT /manage/banners/:id 更新 */
export const save = (data: Partial<Banner> & { id: number }, slot?: number): Promise<Banner> =>
  data.id > 0
    ? http.put<unknown, Banner>(`/manage/banners/${enc(data.id)}`, data)
    : http.post<unknown, Banner>('/manage/banners', slot === undefined ? data : { ...data, slot })

/** DELETE /manage/banners/:id */
export const remove = (id: number): Promise<void> =>
  http.delete<unknown, void>(`/manage/banners/${enc(id)}`)

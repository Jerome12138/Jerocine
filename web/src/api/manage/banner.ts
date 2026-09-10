import { http } from '../http'
import type { Banner } from '@/types/manage'

const enc = encodeURIComponent

/** GET /manage/banners 全量轮播列表（含停用/未到期） */
export const list = (): Promise<Banner[]> =>
  http.get<unknown, Banner[]>('/manage/banners')

/** POST /manage/banners 新建；PUT /manage/banners/:id 更新（id 存在则走 PUT） */
export const save = (data: Banner): Promise<Banner> =>
  data.id > 0
    ? http.put<unknown, Banner>(`/manage/banners/${enc(data.id)}`, data)
    : http.post<unknown, Banner>('/manage/banners', data)

/** DELETE /manage/banners/:id */
export const remove = (id: number): Promise<void> =>
  http.delete<unknown, void>(`/manage/banners/${enc(id)}`)

import { http } from '../http'
import type { DashboardStat, SiteBasic } from '@/types/manage'

// ---- 后端契约 DTO (entity.SiteConfig 子集) ----
interface SiteConfigDTO {
  id: number
  siteName: string
  domain: string
  logo: string
  keyword: string
  description: string
  state: number // 0 开启 / 1 关闭
  hint: string
  /** TMDB 凭据掩码(如 0c06…676b), 未配置为空串; 明文永不回传 */
  tmdbKeyMasked?: string
  tmdbKeySet?: boolean
}

/** GET /manage/dashboard 仪表盘统计 */
export const dashboard = (): Promise<DashboardStat> =>
  http.get<unknown, DashboardStat>('/manage/dashboard')

/** GET /manage/site-config 站点基础配置 */
export const getBasic = async (): Promise<SiteBasic> => {
  const c = await http.get<unknown, SiteConfigDTO>('/manage/site-config')
  return {
    siteName: c.siteName,
    logo: c.logo,
    keyword: c.keyword,
    describe: c.description,
    domain: c.domain,
    state: c.state === 0,
    hint: c.hint
  }
}

/** POST /manage/site-config 更新站点基础配置 */
export const updateBasic = (data: SiteBasic): Promise<void> =>
  http.post<unknown, void>('/manage/site-config', {
    id: 1,
    siteName: data.siteName,
    domain: data.domain ?? '',
    logo: data.logo,
    keyword: data.keyword,
    description: data.describe,
    state: data.state ? 0 : 1,
    hint: data.hint ?? ''
  })

// ---- TMDB 凭据(独立端点, 明文永不回传) ----

export interface TMDBKeyState {
  /** 已配置时的掩码(如 0c06…676b), 未配置为空串 */
  masked: string
  set: boolean
}

/** GET /manage/site-config 读取 TMDB key 状态(掩码) */
export const getTMDBKey = async (): Promise<TMDBKeyState> => {
  const c = await http.get<unknown, SiteConfigDTO>('/manage/site-config')
  return { masked: c.tmdbKeyMasked ?? '', set: c.tmdbKeySet ?? false }
}

/** POST /manage/tmdb-key 保存凭据(v3 key / v4 token), 服务端验真后热生效 */
export const setTMDBKey = (key: string): Promise<void> =>
  http.post<unknown, void>('/manage/tmdb-key', { key })

/** DELETE /manage/tmdb-key 清除凭据(横图回填停摆, 已落地图不受影响) */
export const clearTMDBKey = (): Promise<void> =>
  http.delete<unknown, void>('/manage/tmdb-key')

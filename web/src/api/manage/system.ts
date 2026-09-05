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

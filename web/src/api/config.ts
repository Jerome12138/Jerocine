import { http } from './http'
import type { AppVersion, SiteConfig } from '@/types/config'

/** GET /config/site 站点基础配置 */
export const getSiteConfig = (): Promise<SiteConfig> =>
  http.get<unknown, SiteConfig>('/config/site')

/** GET /app/version/latest APK 最新版本 */
export const getLatestVersion = (channel = 0): Promise<AppVersion> =>
  http.get<unknown, AppVersion>('/app/version/latest', { params: { channel } })

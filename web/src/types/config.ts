/** 站点配置(GET /config/site, 对齐后端 entity.SiteConfig) */
export interface SiteConfig {
  id: number
  siteName: string
  domain: string
  logo: string
  keyword: string
  description: string
  state: number
  hint: string
  updatedAt: number
}

/** APK 版本(GET /app/version/latest) */
export interface AppVersion {
  id: number
  channel: number
  versionCode: number
  versionName: string
  apkUrl: string
  changelog: string
  force: boolean
  whitelist: string[]
  createdAt: number
}

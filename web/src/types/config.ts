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
  /** 直播顶栏入口开关(000038): true=显示, false=隐藏; 无启用源时前端自动全隐, 该开关不生效 */
  liveNavEnabled?: boolean
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

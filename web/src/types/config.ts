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
  /** 直播顶栏入口开关(000038): 后端 int 直出 1=显示 / 0=隐藏(旧后端缺字段视为显示);
   *  无启用源时前端自动全隐, 该开关不生效。判断用 `!== 0`, 勿用布尔比较(0 !== false 恒真)。 */
  liveNavEnabled?: number
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

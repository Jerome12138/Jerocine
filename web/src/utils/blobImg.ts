/**
 * 相对 blob 图片 URL(/api/upload/...) 规整为 API 域绝对地址。
 *
 * 背景(2026-10-28): 后端横图等 blob 资源以相对路径下发(如 /api/upload/backdrop/x.jpg),
 * 主站同源部署没问题; 但跨域静态部署(web.jerocine.art 等 EdgeOne 域)上, 相对路径会解析到
 * 静态域自身 → 命中 SPA 回退(200 text/html) → <img>/background-image 裂图。
 *
 * 规则:
 *  - 仅处理以 /api/upload/ 开头的根相对路径(BLOB_BASE_URL 约定前缀), 其余原样;
 *  - 同源部署(主站, location.origin === API 域)原样返回 —— 行为与历史版本完全一致;
 *  - 无 window(SSR/单测 node 环境)原样返回。
 */
const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) || '/api'

/** API 域 origin: VITE_API_BASE 约定以 /api 结尾, 剥掉尾段; 相对配置(本地开发)时为空串。 */
const API_ORIGIN = API_BASE.replace(/\/api\/?$/, '')

export function resolveBlobUrl(url: string): string {
  if (!url || !url.startsWith('/api/upload/')) return url
  if (typeof window === 'undefined') return url
  if (window.location.origin === API_ORIGIN) return url
  return API_ORIGIN + url
}

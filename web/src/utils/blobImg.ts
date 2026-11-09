/**
 * 相对 blob 图片 URL 规整为 API 域绝对地址。
 *
 * 背景(2026-10-28): 后端横图等 blob 资源以相对路径下发(如 /filestore/backdrop/x.jpg),
 * 主站同源部署没问题; 但跨域静态部署(web.jerocine.art 等 EdgeOne 域)上, 相对路径会解析到
 * 静态域自身 → 命中 SPA 回退(200 text/html) → <img>/background-image 裂图。
 *
 * 前缀来源(2026-11-09): 后端把 blob 对外基址(BLOB_BASE_URL)经 GET /config/site 的
 * `fileBase` 字段下发(见 stores/site.ts → setBlobBase)。基址可为:
 *  - 路径(如 /filestore/、/media/): 后端出参是根相对 ⇒ 跨域时归到 API 域, 需按前缀识别;
 *  - 绝对域(如 https://img.example.com/): 后端出参已是绝对 URL ⇒ 直接原样返回。
 *
 * 规则:
 *  - 绝对 URL(http/https/协议相对)原样返回;
 *  - 仅处理以「配置前缀 或 默认 /filestore/ 或 旧 /api/upload/」开头的根相对路径, 其余原样;
 *  - 同源部署(主站, location.origin === API 域)原样返回;
 *  - 无 window(SSR/单测 node 环境)原样返回。
 */
const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) || '/api'

/** API 域 origin: VITE_API_BASE 约定以 /api 结尾, 剥掉尾段; 相对配置(本地开发)时为空串。 */
const API_ORIGIN = API_BASE.replace(/\/api\/?$/, '')

/** 后端 /config/site 下发的 blob 对外基址(路径或绝对域); 未取到前为空串。 */
let blobBase = ''

/** 由 site store 在拿到站点配置后注入(对齐 imgFallbackHosts 的模块级单供体约定)。 */
export function setBlobBase(base?: string): void {
  blobBase = (base ?? '').trim()
}

/** 默认路径前缀(BLOB_BASE_URL 缺省值) + 旧前缀(迁移前的存量响应/缓存, 过渡期兼容)。 */
const DEFAULT_PREFIX = '/filestore/'
const LEGACY_PREFIX = '/api/upload/'

/** 需要归到 API 域的「路径型」前缀集合(绝对域基址不需要, 因为其后端出参本就是绝对 URL)。 */
function pathPrefixes(): string[] {
  const out: string[] = []
  if (blobBase.startsWith('/')) {
    out.push(blobBase.endsWith('/') ? blobBase : blobBase + '/')
  }
  out.push(DEFAULT_PREFIX, LEGACY_PREFIX)
  return out
}

export function resolveBlobUrl(url: string): string {
  if (!url) return url
  if (/^(https?:)?\/\//i.test(url)) return url // 绝对 / 协议相对(独立域/CDN) 原样
  if (typeof window === 'undefined') return url
  if (!pathPrefixes().some((p) => url.startsWith(p))) return url
  if (window.location.origin === API_ORIGIN) return url
  return API_ORIGIN + url
}

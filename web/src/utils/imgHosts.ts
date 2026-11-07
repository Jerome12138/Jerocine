import { ref } from 'vue'

/**
 * 图床域名链（000041, 2026-11-07, 后台「站点信息」可配）: 逗号分隔按序尝试 ——
 * 第一个为主图床, 其余为备用域; 封面 onerror 后在链内自动切下一个域。
 *
 * 模块级单例而非 pinia store: BaseImage 被全站数百处使用, 绝大多数组件 spec
 * 不装 pinia —— store 强依赖会打破全部挂载。站点配置载入/保存时由 stores/site.ts
 * 与 SiteConfigView 同步进来。
 */
export const imgFallbackHosts = ref('')

/** 解析成去空白的域名数组 */
export function imgHostsList(): string[] {
  return imgFallbackHosts.value
    .split(/[,，\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}

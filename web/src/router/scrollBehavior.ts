import type { RouterScrollBehavior } from 'vue-router'

/**
 * 切页一律回顶, 不还原 savedPosition — TV 上"back 回首页停在旧位置"体验差,
 * 用户明确要求每次进新页面 (含 back) 从头开始. 例外: URL 带 hash 锚点平滑滚到目标.
 *
 * 单独 export 以便 vitest 单测覆盖, 别在 createRouter 里写匿名函数.
 */
export const scrollBehavior: RouterScrollBehavior = (to) => {
  if (to.hash) {
    return { el: to.hash, behavior: 'smooth' }
  }
  return { top: 0 }
}

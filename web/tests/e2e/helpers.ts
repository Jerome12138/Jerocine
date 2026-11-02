import { expect, type Page } from '@playwright/test'

/**
 * 等待"应用就绪"——网络空闲 + #app 可见 + 至少一次拦截器响应。
 * 不依赖具体 selector，留给各 spec 用 expect 断言后续元素。
 */
export async function waitAppReady(page: Page): Promise<void> {
  await expect(page.locator('#app')).toBeVisible()
  // 让 mock 拦截器和 base toast 至少跑过一轮
  await page.waitForLoadState('networkidle', { timeout: 10_000 }).catch(() => {})
}

/**
 * 模拟登录：写 utils/token.ts 现行结构
 *   localStorage['auth-token'] = '<token>' (+ 'auth-expires' 过期秒, 可省)
 * 旧版 'auth'={key,value} 包装会被 getToken() 当残留主动清除(见 token.ts LEGACY_KEY),
 * 写旧结构 = 没登录, 路由守卫会把页面打回 /login。
 *
 * @param opts.role 0 普通用户 / 1 管理员（默认 1）
 *                  store fetchInfo 时仍会从后端 /user/info 拉真实 role;
 *                  但路由守卫在 store.info 缺失时也会 fetchInfo 兜底。
 *                  注意: mock 适配器已移除(main.ts), e2e 需本地后端或页面级路由拦截
 *                  提供数据, 本 helper 只负责把"已登录"状态写进存储。
 */
export async function fakeLogin(
  page: Page,
  opts: { role?: 0 | 1 } = {}
): Promise<void> {
  const role = opts.role ?? 1
  await page.addInitScript((r) => {
    // 现行结构: token 明文 + 过期时间(unix 秒), 见 utils/token.ts setToken
    localStorage.setItem('auth-token', 'mock-token-e2e')
    // 给一个远期过期时间, 避免 getToken() 按已过期清 token
    localStorage.setItem('auth-expires', String(Math.floor(Date.now() / 1000) + 3600))
    // 暂存预期角色，让后端/拦截按需选择返回（预留, 现无 mock 消费方）
    localStorage.setItem('__mock_role', String(r))
  }, role)
}

/** 把视图模式写到 localStorage（强制 TV 等） */
export async function setViewMode(
  page: Page,
  mode: 'mobile' | 'desktop' | 'tv'
): Promise<void> {
  await page.addInitScript((m) => {
    localStorage.setItem('jc-mode', m)
  }, mode)
}

/** 把首屏图片加载策略改快：插入 CSS 隐藏所有 img onload 等待，仅做 DOM 可见性测试 */
export async function muteImages(page: Page): Promise<void> {
  await page.addStyleTag({
    content: `img { transition: none !important; opacity: 1 !important; }`
  })
}

/** 是否当前 project 视口属于"小屏" */
export function isMobile(viewport: { width: number; height: number } | null): boolean {
  if (!viewport) return false
  return viewport.width < 768
}

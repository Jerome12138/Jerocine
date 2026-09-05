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
 * 模拟登录：写真实结构（utils/token.ts 用 'auth' key + JSON 包装）
 *  { key: 'auth-token', value: '<token>' }
 *
 * @param opts.role 0 普通用户 / 1 管理员（默认 1）
 *                  store fetchInfo 时仍会从 mock /user/info 拉真实 role；
 *                  但路由守卫在 store.info 缺失时也会 fetchInfo 兜底，
 *                  mock 默认返回 ADMIN_USER（role=1），传 0 时这个 helper
 *                  额外注入一个 dicebear 提示窗，但不修改 mock 返回值。
 *                  若需测试普通用户身份，应用层会从 /user/info 拿真实 role。
 *                  本项目 mock /user/info 总返回 ADMIN_USER，实测 role=1 视角。
 *                  因此 role=0 路径建议走"表单登录 alice"的真实流程。
 */
export async function fakeLogin(
  page: Page,
  opts: { role?: 0 | 1 } = {}
): Promise<void> {
  const role = opts.role ?? 1
  await page.addInitScript((r) => {
    const auth = { key: 'auth-token', value: 'mock-token-e2e' }
    localStorage.setItem('auth', JSON.stringify(auth))
    // 暂存预期角色，让 mock /user/info 选择性返回（见 mock/handlers）
    localStorage.setItem('__mock_role', String(r))
  }, role)
}

/** 把视图模式写到 localStorage（强制 TV 等） */
export async function setViewMode(
  page: Page,
  mode: 'mobile' | 'desktop' | 'tv'
): Promise<void> {
  await page.addInitScript((m) => {
    localStorage.setItem('gf-mode', m)
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

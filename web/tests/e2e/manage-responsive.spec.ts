import { test, expect } from '@playwright/test'
import { fakeLogin, muteImages, waitAppReady } from './helpers'

/**
 * Manage 后台响应式 smoke (P5):
 * 验证关键 manage 页在 mobile-portrait / tablet / desktop 三视口下:
 *  - 页面无 JS error 加载完毕
 *  - mobile: 顶部汉堡按钮可见, sidebar 默认收起 (drawer translate-x-full)
 *  - tablet: sidebar w-[64px] 图标栏常驻
 *  - desktop: sidebar w-[220px] 或 w-[64px] 完整菜单常驻
 *
 * 用 fakeLogin (admin role=1) 跳过表单登录, 走 mock dev server.
 */

const MANAGE_PAGES = [
  { path: '/manage/index', name: 'dashboard' },
  { path: '/manage/film', name: 'film-list' },
  { path: '/manage/cron/index', name: 'cron' },
  { path: '/manage/file/gallery', name: 'file-gallery' }
]

test.describe('manage 响应式 smoke', () => {
  test.beforeEach(async ({ page }) => {
    await muteImages(page)
    await fakeLogin(page, { role: 1 })
  })

  for (const p of MANAGE_PAGES) {
    test(`smoke ${p.name} 加载无 JS 错误`, async ({ page }) => {
      const jsErrors: string[] = []
      page.on('pageerror', (err) => jsErrors.push(err.message))

      await page.goto(p.path)
      await waitAppReady(page)

      // 主区可见
      await expect(page.locator('main')).toBeVisible()
      // 页面没抛 JS 错误
      expect(jsErrors).toEqual([])
    })
  }

  test('mobile: 汉堡按钮存在, sidebar 默认 translate-x-full 收起', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'mobile-portrait', '仅 mobile-portrait 跑')
    await page.goto('/manage/index')
    await waitAppReady(page)
    // 汉堡按钮 (sr-only "打开菜单" 文案)
    const hamburger = page.locator('header button').filter({ hasText: '打开菜单' })
    await expect(hamburger).toBeVisible()
    // sidebar 默认 -translate-x-full (drawer 关闭)
    await expect(page.locator('aside')).toHaveClass(/-translate-x-full/)
    // 点击汉堡 → 滑出
    await hamburger.click()
    await expect(page.locator('aside')).toHaveClass(/translate-x-0/)
  })

  test('tablet: sidebar w-[64px] icon-rail 常驻', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'tablet', '仅 tablet 跑')
    await page.goto('/manage/index')
    await waitAppReady(page)
    await expect(page.locator('aside')).toHaveClass(/w-\[64px\]/)
  })

  test('desktop: sidebar w-[220px] 或 w-[64px] 常驻 (按用户折叠偏好)', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', '仅 desktop 跑')
    await page.goto('/manage/index')
    await waitAppReady(page)
    await expect(page.locator('aside')).toHaveClass(/w-\[(220|64)px\]/)
  })
})

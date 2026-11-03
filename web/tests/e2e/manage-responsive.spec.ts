import { test, expect } from '@playwright/test'
import { fakeLogin, muteImages, waitAppReady } from './helpers'

/**
 * Manage 后台响应式 smoke:
 * 验证关键 manage 页在 mobile-portrait / tablet / desktop 三视口下:
 *  - 页面无 JS error 加载完毕
 *  - mobile (<768): 顶部汉堡按钮可见, 侧栏为抽屉(drawer), 默认不渲染, 点击后滑出
 *  - tablet (768–1023): 侧栏为 mini 图标栏, 固定宽 72px（inline style）
 *  - desktop (>=1024): 侧栏为 full 完整菜单, 固定宽 220px（inline style）
 *
 * 断点与形态来源: useViewMode(<768 mobile / <1024 tablet / else desktop) +
 * ManageLayout(:variant="isMobile?'drawer':isTablet?'mini':'full'"), 侧栏宽度由
 * ManageSidebar 的 inline style 给出（collapsed ? 72px : 220px），不是 w-* 类。
 *
 * 用 fakeLogin (admin role=1) 跳过表单登录; 数据类断言不在此 spec（仅结构 smoke）。
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

  test('mobile: 汉堡按钮存在, 侧栏抽屉默认关闭/点击滑出', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'mobile-portrait', '仅 mobile-portrait 跑')
    await page.goto('/manage/index')
    await waitAppReady(page)
    // 汉堡按钮 (sr-only "打开菜单" 文案)
    const hamburger = page.locator('header button').filter({ hasText: '打开菜单' })
    await expect(hamburger).toBeVisible()
    // 抽屉默认关闭: v-if 不渲染面板
    await expect(page.locator('.jc-drawer-panel')).toHaveCount(0)
    // 点击汉堡 → 抽屉滑出 (ManageSidebar drawer 变体)
    await hamburger.click()
    await expect(page.locator('.jc-drawer-panel')).toBeVisible()
  })

  test('tablet: 侧栏 mini 图标栏常驻, 固定宽 72px', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'tablet', '仅 tablet 跑')
    await page.goto('/manage/index')
    await waitAppReady(page)
    const aside = page.locator('aside')
    await expect(aside).toBeVisible()
    await expect(aside).toHaveCSS('width', '72px')
  })

  test('desktop: 侧栏 full 完整菜单常驻, 固定宽 220px', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', '仅 desktop 跑')
    await page.goto('/manage/index')
    await waitAppReady(page)
    const aside = page.locator('aside')
    await expect(aside).toBeVisible()
    await expect(aside).toHaveCSS('width', '220px')
  })
})

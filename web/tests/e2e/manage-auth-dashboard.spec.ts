import { expect, test } from '@playwright/test'
import { fakeLogin, muteImages, waitAppReady } from './helpers'

/**
 * 管理端鉴权 + 仪表盘
 *
 * 与新的角色路由协议对齐：
 *  - 未登录访问 /manage/* → /login?redirect=...
 *  - 登录用户名 admin → 自动跳 /manage/index（mock 视为 role=1）
 *  - 登录其它用户名 → 跳 /index（普通用户，role=0），不能进 /manage
 *  - 已登录管理员访问 /login → 兜底跳 /manage/index
 *  - 已登录普通用户访问 /login → 兜底跳 /index
 *  - 普通用户硬闯 /manage/index → 被路由守卫拦回 /index
 */

test.describe('管理端鉴权 + 仪表盘', () => {
  test('未登录访问 /manage/index → 重定向 /login', async ({ page }) => {
    await muteImages(page)
    await page.goto('/manage/index')
    await page.waitForURL(/\/login/, { timeout: 8_000 })
    expect(page.url()).toContain('/login')
  })

  test('admin 登录后跳转 /manage/index', async ({ page }) => {
    await muteImages(page)
    await page.goto('/login')
    await waitAppReady(page)

    await page.locator('input[autocomplete="username"]').fill('admin')
    await page.locator('input[autocomplete="current-password"]').fill('Anything1!')
    await page.locator('button[type="submit"]').click()

    await page.waitForURL(/\/manage\/index/, { timeout: 8_000 })
    expect(page.url()).toContain('/manage/index')
  })

  test('普通用户登录后跳转 /index（不进后台）', async ({ page }) => {
    await muteImages(page)
    await page.goto('/login')
    await waitAppReady(page)

    await page.locator('input[autocomplete="username"]').fill('alice')
    await page.locator('input[autocomplete="current-password"]').fill('Anything1!')
    await page.locator('button[type="submit"]').click()

    await page.waitForURL(/\/index$|\/$/, { timeout: 8_000 })
    expect(page.url()).not.toContain('/manage/')
  })

  test('已登录 admin 访问 /login → 兜底跳 /manage/index', async ({ page }) => {
    await fakeLogin(page, { role: 1 })
    await page.goto('/login')
    await page.waitForURL(/\/manage\/index/, { timeout: 8_000 })
  })

  test('已登录普通用户访问 /login → 兜底跳 /index', async ({ page }) => {
    await fakeLogin(page, { role: 0 })
    await page.goto('/login')
    // 普通用户兜底是 /index
    await page.waitForURL(/\/index$|\/$/, { timeout: 8_000 })
    expect(page.url()).not.toContain('/manage/')
  })

  test('普通用户硬闯 /manage/index → 守卫拦回 /index', async ({ page }) => {
    await fakeLogin(page, { role: 0 })
    await muteImages(page)
    await page.goto('/manage/index')
    await page.waitForURL(/\/index$|\/$/, { timeout: 8_000 })
    expect(page.url()).not.toContain('/manage/')
  })

  test('Dashboard 页面渲染（admin）', async ({ page }) => {
    await fakeLogin(page, { role: 1 })
    await muteImages(page)
    await page.goto('/manage/index')
    await waitAppReady(page)
    const header = page.locator('header').first()
    await expect(header).toBeVisible({ timeout: 5_000 })
  })

  test('登录表单空字段 → 显示提示', async ({ page }) => {
    await muteImages(page)
    await page.goto('/login')
    await waitAppReady(page)
    await page.locator('button[type="submit"]').click()
    await expect(page.locator('[role="alert"]').first()).toContainText(
      /请输入用户名|请输入密码/
    )
  })

  test('登录页有"联系管理员"指引（公共注册下线）', async ({ page }) => {
    await muteImages(page)
    await page.goto('/login')
    await waitAppReady(page)
    // 不再有"注册"按钮；提示"如需账号请联系管理员"
    await expect(page.locator('text=/注册功能已暂时下线/')).toBeVisible()
    await expect(page.locator('text=/联系管理员/')).toBeVisible()
    await expect(page.locator('button[disabled]:has-text("注册")')).toHaveCount(0)
  })
})

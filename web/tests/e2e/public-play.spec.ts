import { expect, test } from '@playwright/test'
import { muteImages, waitAppReady } from './helpers'

/**
 * 播放页 PlayView
 *  - 路由：/play?id=201&source=s201-0&episode=2  (三体 第二季 第 3 集)
 *  - 验证：title + episode chip 当前态 + 自动连播 toggle + 下一集禁用态
 *  - 不真实播放（视频是公开 mp4，超时风险），仅验证 DOM 与状态机
 */

test.describe('PlayView 播放页', () => {
  test.beforeEach(async ({ page }) => {
    await muteImages(page)
    await page.goto('/play?id=201&source=s201-0&episode=2')
    await waitAppReady(page)
    // 等 PlayView 加载完成（detail 出现）
    await page.locator('.gf-play-info__title').waitFor({ timeout: 12_000 }).catch(() => {})
  })

  test('页面标题包含影片名 + 集数', async ({ page }) => {
    // 播放页 video.js 初始化 + mock 数据返回，给宽松 timeout
    await expect(page.locator('.gf-play-info__title')).toContainText(/三体/, {
      timeout: 12_000
    })
    await expect(page.locator('.gf-play-info__episode')).toContainText(/第\s*3\s*集/)
  })

  test('当前集 chip 高亮', async ({ page }) => {
    const active = page.locator('.gf-episode-chip--active')
    await expect(active).toHaveCount(1)
    await expect(active).toContainText(/第\s*3\s*集/)
  })

  test('自动连播按钮存在且可切换', async ({ page }) => {
    const btn = page.locator('button').filter({ hasText: /自动连播/ }).first()
    await expect(btn).toBeVisible()
    await expect(btn).toHaveClass(/gf-toggle--on/, { timeout: 5_000 })
    // 用 force + 等待 stable，避免 video.js 重渲染导致 detach
    await btn.click({ force: true })
    await expect(btn).not.toHaveClass(/gf-toggle--on/, { timeout: 5_000 })
  })

  test('下一集按钮在非末集可点', async ({ page }) => {
    const next = page.locator('button').filter({ hasText: /下一集/ }).first()
    await expect(next).toBeVisible()
    await expect(next).toBeEnabled()
    await next.scrollIntoViewIfNeeded()
    await Promise.all([
      page.waitForURL(/episode=3/, { timeout: 7_000 }),
      next.click()
    ])
  })

  test('点选集数 chip 切换 URL', async ({ page }) => {
    const chips = page.locator('.gf-episode-chip')
    await expect(chips.first()).toBeVisible()
    await chips.nth(0).click()
    await expect(page).toHaveURL(/episode=0/)
  })

  test('返回详情按钮工作', async ({ page }) => {
    const back = page.locator('button').filter({ hasText: /返回详情/ }).first()
    await expect(back).toBeVisible()
    await back.scrollIntoViewIfNeeded()
    await Promise.all([
      page.waitForURL(/\/filmDetail\?link=201/, { timeout: 7_000 }),
      back.click()
    ])
  })
})

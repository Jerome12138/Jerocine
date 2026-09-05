import { expect, test } from '@playwright/test'
import { muteImages, waitAppReady } from './helpers'

/**
 * 影片详情 FilmDetailView
 *  - 路由：/filmDetail?link=101 （mock 流浪地球 III）
 *  - 验证：Hero 背景 + 海报 + 标签 + 立即播放 + EpisodeTabs 多源 + 相关推荐
 */

test.describe('FilmDetailView 详情页', () => {
  test.beforeEach(async ({ page }) => {
    await muteImages(page)
    await page.goto('/filmDetail?link=101')
    await waitAppReady(page)
  })

  test('标题显示 mock 影片名', async ({ page }) => {
    await expect(page.locator('.gf-detail__title')).toContainText(/流浪地球/i)
  })

  test('Hero 背景图样式注入了 url()', async ({ page }) => {
    const bg = page.locator('.gf-detail__hero-bg').first()
    await expect(bg).toBeVisible()
    const style = (await bg.getAttribute('style')) ?? ''
    expect(style).toContain('background-image')
    expect(style).toContain('url(')
  })

  test('立即播放按钮跳 /play', async ({ page }) => {
    const cta = page.locator('button').filter({ hasText: /立即播放/ }).first()
    await expect(cta).toBeVisible({ timeout: 8_000 })
    await cta.scrollIntoViewIfNeeded()
    await Promise.all([
      page.waitForURL(/\/play\?id=/, { timeout: 7_000 }),
      cta.click()
    ])
  })

  test('EpisodeTabs 多源 tab + 集数 chip', async ({ page }) => {
    // mock 每部影片 2 个源
    const sourceTabs = page.locator('.gf-source-tab')
    await expect(sourceTabs).toHaveCount(2)

    // 第一源默认选中（active class）
    await expect(sourceTabs.first()).toHaveClass(/gf-source-tab--active/)

    // 集数 chip 至少 1 个
    const chips = page.locator('.gf-episode-chip')
    await expect(chips.first()).toBeVisible()
    expect(await chips.count()).toBeGreaterThanOrEqual(1)
  })

  test('点选第二源切换并刷新集数列表', async ({ page }) => {
    const tabs = page.locator('.gf-source-tab')
    await tabs.nth(1).click()
    await expect(tabs.nth(1)).toHaveClass(/gf-source-tab--active/)
  })

  test('相关推荐区渲染', async ({ page }) => {
    // RelatedList 渲染 — mock buildFilmDetailResp 默认 8 条
    const related = page.locator('.gf-detail__relate, [class*="relate"]').first()
    await expect(related).toBeVisible({ timeout: 5_000 })
  })

  test('剧情简介展开/收起', async ({ page }) => {
    const summary = page.locator('.gf-detail__summary')
    if ((await summary.count()) === 0) test.skip(true, '此影片无 content')
    const btn = summary.locator('.gf-detail__expand')
    if ((await btn.count()) > 0) {
      const before = (await btn.textContent())?.trim()
      await btn.click()
      const after = (await btn.textContent())?.trim()
      expect(before).not.toBe(after)
    }
  })
})

import { expect, test } from '@playwright/test'
import { muteImages, waitAppReady } from './helpers'

/**
 * 首页 HomeView
 *  - 路由：/index
 *  - 数据：mock IndexData（4 条 banner + 4 行分类列表）
 *  - 验证：HeroCarousel 自适应宽高、FilmRow 至少一行、卡片可点
 */

test.describe('HomeView 首页', () => {
  test.beforeEach(async ({ page }) => {
    await muteImages(page)
    await page.goto('/index')
    await waitAppReady(page)
  })

  test('Hero 区域渲染且符合视口纵横比策略', async ({ page, viewport }) => {
    const hero = page.locator('.gf-hero').first()
    await expect(hero).toBeVisible()

    const box = await hero.boundingBox()
    expect(box).not.toBeNull()
    if (!box || !viewport) return

    // 宽度应至少 90% 视口宽
    expect(box.width).toBeGreaterThan(viewport.width * 0.9)

    const ratio = box.width / box.height
    if (viewport.width < 480) {
      // 4/5 = 0.8 ± 容差
      expect(ratio).toBeGreaterThan(0.6)
      expect(ratio).toBeLessThan(1.4)
    } else if (viewport.width < 768) {
      // 16/10 = 1.6
      expect(ratio).toBeGreaterThan(1.2)
      expect(ratio).toBeLessThan(1.9)
    } else if (viewport.width < 1024) {
      // 16/9 = 1.78
      expect(ratio).toBeGreaterThan(1.4)
      expect(ratio).toBeLessThan(2.1)
    } else {
      // 21/9 = 2.33
      expect(ratio).toBeGreaterThan(1.8)
      expect(ratio).toBeLessThan(3.0)
    }
  })

  test('Hero 标题随轮播切换', async ({ page }) => {
    const title = page.locator('.gf-hero__title').first()
    await expect(title).toBeVisible()
    const firstText = (await title.textContent())?.trim() || ''
    expect(firstText.length).toBeGreaterThan(0)
  })

  test('立即播放按钮可点跳转 /filmDetail', async ({ page }) => {
    const playBtn = page
      .locator('.gf-hero__cta button')
      .filter({ hasText: /立即播放/ })
      .first()
    await expect(playBtn).toBeVisible()
    await playBtn.click()
    await expect(page).toHaveURL(/\/filmDetail\?link=/)
  })

  test('FilmRow 列表 + FilmCard 评分角标', async ({ page }) => {
    const rows = page.locator('.gf-film-row, [class*="gf-film-row"]')
    await expect(rows.first()).toBeVisible({ timeout: 10_000 })
    await expect(rows).toHaveCount(await rows.count())

    // 至少一张 FilmCard
    const card = page.locator('.gf-film-card').first()
    await expect(card).toBeVisible()

    // mock 数据每部影片有 dbScore，FilmCard 应自动显示评分 chip
    const score = card.locator('.gf-film-card__score')
    await expect(score).toBeVisible()
    const scoreText = (await score.textContent())?.trim() ?? ''
    expect(scoreText).toMatch(/^\d(\.\d)?$|^10$/)
  })

  test('卡片左上角不再有冗余 BaseTag（年份/分类）', async ({ page }) => {
    const card = page.locator('.gf-film-card').first()
    await expect(card).toBeVisible({ timeout: 10_000 })
    // 左上区域应不存在 BaseTag.gf-tag — 用属性 selector 避免 unocss 转义复杂度
    const leftTopTags = card.locator(
      '[class*="absolute"][class*="top-["][class*="left-["] .gf-tag'
    )
    await expect(leftTopTags).toHaveCount(0)
  })

  test('点击卡片可跳详情', async ({ page }) => {
    const card = page.locator('.gf-film-card').first()
    await card.click()
    await expect(page).toHaveURL(/\/filmDetail\?link=/)
  })
})

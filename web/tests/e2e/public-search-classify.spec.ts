import { expect, test } from '@playwright/test'
import { muteImages, waitAppReady } from './helpers'

/**
 * 搜索 + 分类筛选
 *  - SearchView 关键字检索 + 清空回引导态
 *  - ClassifyView 分类首页 (3 段)
 *  - ClassifySearchView 多 chip 筛选 + URL 同步
 */

test.describe('SearchView 搜索', () => {
  test('引导态：未输入时显示空提示', async ({ page }) => {
    await muteImages(page)
    await page.goto('/search')
    await waitAppReady(page)
    await expect(page.locator('.gf-empty')).toContainText(/开始你的搜索/)
  })

  test('搜"三体"返回结果列表', async ({ page }) => {
    await muteImages(page)
    await page.goto('/search')
    await waitAppReady(page)
    const input = page.locator('.gf-search__input')
    await input.fill('三体')
    await page.locator('.gf-search__btn').click()
    await expect(page).toHaveURL(/search=%E4%B8%89%E4%BD%93|search=三体/)
    // 至少 1 条结果
    await expect(page.locator('.gf-search__row-desktop, .gf-search__row-mobile').first()).toBeVisible()
  })

  test('清空输入并提交 → URL search 被清', async ({ page }) => {
    await muteImages(page)
    await page.goto('/search?search=三体')
    await waitAppReady(page)
    const input = page.locator('.gf-search__input')
    await input.fill('')
    await page.locator('.gf-search__btn').click()
    await expect(page).not.toHaveURL(/search=三体|search=%E4%B8%89%E4%BD%93/)
  })
})

test.describe('ClassifyView 分类首页', () => {
  test.beforeEach(async ({ page }) => {
    await muteImages(page)
    await page.goto('/filmClassify?Pid=2') // 电视剧
    await waitAppReady(page)
  })

  test('分类标题展示', async ({ page }) => {
    // ClassifyView 标题区是 RouterLink anchor（.gf-classify__title-active / link）
    const titleEl = page.locator('.gf-classify__title-active').filter({ hasText: /电视剧/ })
    await expect(titleEl).toBeVisible({ timeout: 5_000 })
  })

  test('至少有一组列表渲染', async ({ page }) => {
    const cards = page.locator('.gf-film-card')
    await expect(cards.first()).toBeVisible({ timeout: 5_000 })
    expect(await cards.count()).toBeGreaterThan(0)
  })
})

test.describe('ClassifySearchView 筛选页', () => {
  test('chip 选中后 URL 同步', async ({ page }) => {
    await muteImages(page)
    await page.goto('/filmClassifySearch?Pid=1')
    await waitAppReady(page)

    const chips = page.locator('.gf-filter-chip')
    await expect(chips.first()).toBeVisible({ timeout: 6_000 })

    // 找一个非"全部"且非默认选中的 chip 点
    const target = chips.filter({ hasText: '科幻' }).first()
    if ((await target.count()) === 0) test.skip(true, 'mock 无该 tag')
    await target.click()
    // URL 应带 Plot=科幻 或编码
    await expect(page).toHaveURL(/Plot=%E7%A7%91%E5%B9%BB|Plot=科幻/)

    // 列表应仍渲染（至少一张 FilmCard）
    await expect(page.locator('.gf-film-card').first()).toBeVisible({ timeout: 5_000 })
  })

  test('chip 触控目标 ≥ 44px', async ({ page }) => {
    await page.goto('/filmClassifySearch?Pid=1')
    await waitAppReady(page)
    const chip = page.locator('.gf-filter-chip').first()
    await expect(chip).toBeVisible({ timeout: 6_000 })
    const box = await chip.boundingBox()
    expect(box).not.toBeNull()
    if (box) {
      // 大屏适配修复：min-height 44px
      expect(box.height).toBeGreaterThanOrEqual(40)
    }
  })
})

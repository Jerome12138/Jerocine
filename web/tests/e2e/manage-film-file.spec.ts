import { expect, test } from '@playwright/test'
import { fakeLogin, muteImages, waitAppReady } from './helpers'

/**
 * 管理端：影片管理 + 文件库
 */

test.describe('管理端 影片列表', () => {
  test.beforeEach(async ({ page }) => {
    await fakeLogin(page)
    await muteImages(page)
    await page.goto('/manage/film')
    await waitAppReady(page)
  })

  test('列表至少 1 行', async ({ page }) => {
    const rows = page.locator('table tbody tr')
    await expect(rows.first()).toBeVisible({ timeout: 5_000 })
    expect(await rows.count()).toBeGreaterThan(0)
  })

  test('搜索输入框存在', async ({ page }) => {
    const input = page.locator('input[placeholder*="影片"], input[placeholder*="搜索"]').first()
    await expect(input).toBeVisible()
  })
})

test.describe('管理端 影片新增', () => {
  test('海报上传按钮可触发 file picker（label 修复回归）', async ({ page }) => {
    await fakeLogin(page)
    await muteImages(page)
    await page.goto('/manage/film/add')
    await waitAppReady(page)

    const fileInput = page.locator('input[type="file"]').first()
    await expect(fileInput).toBeAttached()

    // 验证按钮 click → file input click 桥接（无法真触发系统文件框，但能监听 input click 事件）
    const pickBtn = page.locator('button').filter({ hasText: /选择图片/ })
    let clicked = false
    await page.exposeFunction('onInputClicked', () => {
      clicked = true
    })
    await page.evaluate(() => {
      const input = document.querySelector('input[type="file"]') as HTMLElement
      input?.addEventListener('click', () => {
        (window as unknown as { onInputClicked?: () => void }).onInputClicked?.()
      })
    })
    await pickBtn.click()
    expect(clicked).toBe(true)
  })

  test('数字 input 清空后不强转 0', async ({ page }) => {
    await fakeLogin(page)
    await muteImages(page)
    await page.goto('/manage/film/add')
    await waitAppReady(page)

    const cidInput = page.locator('input[type="number"]').first()
    await cidInput.fill('5')
    await expect(cidInput).toHaveValue('5')
    await cidInput.fill('')
    await expect(cidInput).toHaveValue('')
  })
})

test.describe('管理端 文件库', () => {
  test.beforeEach(async ({ page }) => {
    await fakeLogin(page)
    await muteImages(page)
    await page.goto('/manage/file/gallery')
    await waitAppReady(page)
  })

  test('文件网格展示', async ({ page }) => {
    // FileGalleryView 渲染卡片
    const cards = page.locator('article').filter({ has: page.locator('img, .gf-base-image') })
    await expect(cards.first()).toBeVisible({ timeout: 6_000 })
  })

  test('删除文件触发 BaseConfirmDialog', async ({ page }) => {
    const cards = page.locator('article')
    await expect(cards.first()).toBeVisible({ timeout: 6_000 })
    const delBtn = cards.first().locator('button').filter({ hasText: /删除/ })
    await delBtn.click()
    const dialog = page.locator('[role="dialog"]')
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText(/确认删除/)
    await dialog.locator('button').filter({ hasText: /取消/ }).click()
  })
})

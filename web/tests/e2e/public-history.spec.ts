import { expect, test } from '@playwright/test'
import { muteImages, waitAppReady } from './helpers'

/**
 * HistoryView 观看历史
 *  - 空态：未播放过应展示 BaseEmpty
 *  - 有数据：注入 localStorage filmHistory → 网格 + 进度小标 + 移除按钮
 */

test.describe('HistoryView 观看历史', () => {
  test('空态展示提示', async ({ page }) => {
    await muteImages(page)
    await page.goto('/history')
    await waitAppReady(page)
    await expect(page.locator('.gf-empty')).toContainText(/还没有观看记录/)
  })

  test('注入历史记录后渲染卡片网格', async ({ page }) => {
    const seed = {
      '101': {
        id: '101',
        name: '流浪地球 III',
        link: '/play?id=101&source=s101-0&episode=0&currentTime=180',
        episode: '正片',
        timeStamp: Date.now(),
        picture:
          'https://picsum.photos/seed/test-history/300/450',
        source: 's101-0',
        episodeIndex: 0,
        currentTime: 180
      }
    }
    await page.addInitScript((s) => {
      localStorage.setItem('filmHistory', JSON.stringify(s))
    }, seed)

    await muteImages(page)
    await page.goto('/history')
    await waitAppReady(page)

    // 卡片应可见
    const card = page.locator('.gf-history-card')
    await expect(card.first()).toBeVisible()
    expect(await card.count()).toBe(1)

    // 进度小标 3:00
    const progress = card.first().locator('div').filter({ hasText: /^\s*3:00\s*$/ })
    await expect(progress.first()).toBeVisible()

    // 集数 tag
    await expect(card.first()).toContainText(/正片/)
  })

  test('移除单条按钮工作', async ({ page }) => {
    const seed = {
      '101': {
        id: '101',
        name: 'Demo',
        link: '/play?id=101&source=s&episode=0',
        episode: 'EP1',
        timeStamp: Date.now(),
        source: 's',
        episodeIndex: 0
      }
    }
    await page.addInitScript((s) => {
      localStorage.setItem('filmHistory', JSON.stringify(s))
    }, seed)

    await muteImages(page)
    await page.goto('/history')
    await waitAppReady(page)

    const removeBtn = page
      .locator('button[aria-label*="从历史中移除"]')
      .first()
    await expect(removeBtn).toBeVisible()
    await removeBtn.click()

    // 应回到空态
    await expect(page.locator('.gf-empty')).toContainText(/还没有观看记录/)
  })

  test('清空全部触发 BaseConfirmDialog（非原生 confirm）', async ({ page }) => {
    const seed = {
      '101': {
        id: '101',
        name: 'Demo',
        link: '/play?id=101&source=s&episode=0',
        episode: 'EP1',
        timeStamp: Date.now()
      }
    }
    await page.addInitScript((s) => {
      localStorage.setItem('filmHistory', JSON.stringify(s))
    }, seed)
    await muteImages(page)
    await page.goto('/history')
    await waitAppReady(page)

    const clearBtn = page.locator('button:has-text("清空")').filter({ hasNotText: /历史|清空全部/ }).first()
    await expect(clearBtn).toBeVisible()
    await clearBtn.click()

    // BaseDialog 应可见
    const dialog = page.locator('[role="dialog"]')
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText(/确认清空/)

    // 取消
    await dialog.locator('button').filter({ hasText: /取消/ }).click()
    await expect(dialog).not.toBeVisible()
    // 卡片仍在
    await expect(page.locator('.gf-history-card').first()).toBeVisible()

    // 再点清空 → 确认
    await clearBtn.click()
    await expect(dialog).toBeVisible()
    await dialog.locator('button').filter({ hasText: /清空/ }).click()

    // 应回到空态
    await expect(page.locator('.gf-empty')).toContainText(/还没有观看记录/)
  })
})

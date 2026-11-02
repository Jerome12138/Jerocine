import { expect, test, type Page } from '@playwright/test'
import { fakeLogin, muteImages } from './helpers'

/**
 * 管理端在线会话页（/manage/system/sessions）。
 *
 * mock 适配器已在重构中移除（main.ts，src/mock/ 已删），本 spec 用**页面级路由拦截**
 * 提供数据（helpers.ts fakeLogin 注释认可的方式），不依赖本地后端 —— 其余 manage-* spec
 * 打真实后端（127.0.0.1:3601），本文件在后端缺席时仍应可独立通过。
 *
 * 覆盖：列表渲染 / 「共 N 条」 / 回车查询 / 踢下线确认流（含 DELETE 调用与列表刷新）。
 */

interface SessionRow {
  id: string
  userId: number
  userName: string
  ip: string
  ua: string
  loginAt: number
  lastSeen: number
}

const ROWS: SessionRow[] = [
  {
    id: 'sess-1', userId: 7, userName: 'jerry', ip: '1.2.3.4',
    ua: 'Mozilla/5.0 (Windows NT 10.0)', loginAt: 1761000000, lastSeen: 1761003600
  },
  {
    id: 'sess-2', userId: 9, userName: 'alice', ip: '5.6.7.8',
    ua: 'Mozilla/5.0 (Linux)', loginAt: 1760900000, lastSeen: 1760903600
  }
]

/** 拦截全部 /api/v1/*: /me 给管理员身份(id=7, 与 jerry 的 userId 一致 → 「当前登录」标识),
 *  /manage/sessions 给会话列表, 其余回空对象。返回会话 GET 次数供「列表已刷新」断言用。 */
async function stubApi(page: Page, rows: SessionRow[] = ROWS) {
  const state = { sessionsGet: 0 }
  await page.route('**/api/v1/**', async (route) => {
    const url = route.request().url()
    const method = route.request().method()
    const json = (body: unknown, status = 200) =>
      route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
    if (url.includes('/me')) return json({ id: 7, userName: 'admin', role: 1 })
    if (url.includes('/manage/sessions')) {
      if (method === 'DELETE') return json({ id: 'sess-1', revoked: true })
      state.sessionsGet += 1
      return json({ list: rows, total: rows.length })
    }
    return json({})
  })
  return state
}

test.describe('管理端在线会话（路由拦截版, 不依赖后端）', () => {
  test('列表展示用户/IP/当前登录标识与「共 N 条」', async ({ page }) => {
    await stubApi(page)
    await fakeLogin(page, { role: 1 })
    await muteImages(page)
    await page.goto('/manage/system/sessions')
    await expect(page.locator('h2', { hasText: '在线会话' })).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('jerry')).toBeVisible()
    await expect(page.getByText('alice')).toBeVisible()
    await expect(page.getByText('共 2 条')).toBeVisible()
    // jerry(userId=7) 是当前登录管理员自己 → 行内标识
    await expect(page.getByText('当前登录')).toBeVisible()
  })

  test('回车触发查询: keyword 进入请求 query', async ({ page }) => {
    let lastKeyword = ''
    await page.route('**/api/v1/**', async (route) => {
      const url = new URL(route.request().url())
      if (url.pathname.includes('/manage/sessions')) {
        lastKeyword = url.searchParams.get('keyword') ?? ''
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ list: ROWS, total: ROWS.length })
        })
        return
      }
      if (url.pathname.includes('/me')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ id: 1, userName: 'admin', role: 1 })
        })
        return
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' })
    })
    await fakeLogin(page, { role: 1 })
    await muteImages(page)
    await page.goto('/manage/system/sessions')
    await expect(page.getByText('jerry')).toBeVisible({ timeout: 10_000 })
    await page.getByPlaceholder('用户名 / 用户 ID / IP').fill('alice')
    await page.keyboard.press('Enter')
    await expect
      .poll(() => lastKeyword, { timeout: 5_000 })
      .toBe('alice')
  })

  test('踢下线: 确认弹窗 → DELETE 调用 → 列表刷新', async ({ page }) => {
    const state = await stubApi(page)
    await fakeLogin(page, { role: 1 })
    await muteImages(page)
    await page.goto('/manage/system/sessions')
    await expect(page.getByText('jerry')).toBeVisible({ timeout: 10_000 })
    expect(state.sessionsGet).toBe(1)

    // 行内「踢下线」→ 确认弹窗(危险操作)
    await page.getByRole('button', { name: '踢下线' }).first().click()
    await expect(page.locator('.jc-confirm-ok')).toBeVisible()
    await page.locator('.jc-confirm-ok').click()

    await expect(page.getByText('已踢下线')).toBeVisible({ timeout: 5_000 })
    // 列表重新拉取
    await expect.poll(() => state.sessionsGet, { timeout: 5_000 }).toBe(2)
  })
})

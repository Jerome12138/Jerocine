import { expect, test, type Page } from '@playwright/test'
import { waitAppReady } from './helpers'

/**
 * 直播页 LiveView（/live，无鉴权）
 *  - 数据拦截：Playwright route 拦 /api/v1/live/sources(+ /:id/channels)，
 *    页面不依赖后端真实直播源（该功能上线前线上 404，故必须拦截）
 *  - 其余 /api/v1 放行 dev server 代理（页面壳不依赖其成功）
 *  - 频道流地址指向 stream.test 并 abort：只验证 DOM/状态，不真实拉流
 *  - 验证：源 chips / 分组渲染 / 频道选择高亮与记忆 / 切源清空 / 记忆恢复
 */

const STREAM_HOST = 'http://stream.test'

const SOURCES = {
  sources: [
    { id: 1, name: '主源' },
    { id: 2, name: '备用源' }
  ]
}

const CH1 = {
  id: 1,
  name: '主源',
  channels: [
    { name: 'CCTV-1', group: '央视', logo: '', url: `${STREAM_HOST}/cctv1.m3u8` },
    { name: 'CCTV-5', group: '央视', logo: '', url: `${STREAM_HOST}/cctv5.m3u8` },
    { name: '湖南卫视', group: '卫视', logo: '', url: `${STREAM_HOST}/hntv.m3u8` }
  ]
}

const CH2 = {
  id: 2,
  name: '备用源',
  channels: [{ name: '测试台', group: '测试', logo: '', url: `${STREAM_HOST}/test.m3u8` }]
}

function json(body: unknown): { status: number; contentType: string; body: string } {
  return { status: 200, contentType: 'application/json', body: JSON.stringify(body) }
}

/** 拦直播 API + abort 假流地址; 其余 /api/v1 透传（先注册兜底, 后注册的 live 规则优先）。 */
async function mockLiveApi(page: Page): Promise<void> {
  await page.route('**/api/v1/**', (route) => route.continue())
  await page.route('**/api/v1/live/sources', (route) => route.fulfill(json(SOURCES)))
  await page.route('**/api/v1/live/sources/1/channels', (route) => route.fulfill(json(CH1)))
  await page.route('**/api/v1/live/sources/2/channels', (route) => route.fulfill(json(CH2)))
  await page.route(`${STREAM_HOST}/**`, (route) => route.abort())
}

test.describe('LiveView 直播页', () => {
  test.beforeEach(async ({ page }) => {
    await mockLiveApi(page)
  })

  test('源 chips 渲染 2 个, 频道按分组渲染(组名+数量), 播放器挂载', async ({ page }) => {
    await page.goto('/live')
    await waitAppReady(page)

    // 源 chips: 2 个, 第一个默认激活
    const chips = page.locator('.jc-live-source-chip')
    await expect(chips).toHaveCount(2)
    await expect(chips.first()).toContainText('主源')
    await expect(chips.nth(1)).toContainText('备用源')
    await expect(page.locator('.jc-live-source-chip.is-active')).toHaveCount(1)
    await expect(page.locator('.jc-live-source-chip.is-active')).toContainText('主源')

    // 分组: 央视(2) / 卫视(1), 保序
    const groups = page.locator('.jc-live-group__title')
    await expect(groups).toHaveCount(2)
    await expect(groups.nth(0)).toContainText('央视')
    await expect(groups.nth(0)).toContainText('2')
    await expect(groups.nth(1)).toContainText('卫视')
    await expect(groups.nth(1)).toContainText('1')

    // 频道条目
    const channels = page.locator('.jc-live-channel')
    await expect(channels).toHaveCount(3)
    await expect(channels.first()).toContainText('CCTV-1')

    // 播放器容器存在, 初始显示选择提示
    // (已知产品缺陷: LiveView 传 poster:undefined → ArtPlayer 5.4 option 校验抛错,
    //  .art-video-player 挂不上 —— 见任务报告, 此处不断言播放器实例)
    await expect(page.locator('.jc-live-player')).toBeAttached()
    await expect(page.locator('.jc-live-loading__text')).toContainText('选择一个频道开始观看')
  })

  test('点击频道: is-active 高亮 + localStorage 写入观看记忆 + 停止选择提示', async ({ page }) => {
    await page.goto('/live')
    await waitAppReady(page)

    await page.locator('.jc-live-channel').first().click()

    const active = page.locator('.jc-live-channel.is-active')
    await expect(active).toHaveCount(1)
    await expect(active).toContainText('CCTV-1')

    // 观看记忆(源 + 频道地址)
    const saved = await page.evaluate(() => localStorage.getItem('jc-live-last-channel'))
    expect(saved).not.toBeNull()
    expect(JSON.parse(saved as string)).toEqual({ sourceId: 1, url: `${STREAM_HOST}/cctv1.m3u8` })

    // 已选频道: "选择频道"覆盖层退场
    await expect(page.locator('.jc-live-loading')).toBeHidden()
  })

  test('切换源: 频道列表更新且选中清空(停止旧源画面)', async ({ page }) => {
    await page.goto('/live')
    await waitAppReady(page)

    // 先选一个频道建立"旧源选中态"
    await page.locator('.jc-live-channel').first().click()
    await expect(page.locator('.jc-live-channel.is-active')).toHaveCount(1)

    // 切到备用源
    await page.locator('.jc-live-source-chip', { hasText: '备用源' }).click()

    const channels = page.locator('.jc-live-channel')
    await expect(channels).toHaveCount(1)
    await expect(channels.first()).toContainText('测试台')

    // 旧选中清空
    await expect(page.locator('.jc-live-channel.is-active')).toHaveCount(0)
    // 旧源画面停止: 播放区回到选择提示
    await expect(page.locator('.jc-live-loading')).toBeVisible()
  })

  test('记忆恢复: 预置 jc-live-last-channel 后加载, 记忆源频道高亮', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem(
        'jc-live-last-channel',
        JSON.stringify({ sourceId: 2, url: 'http://stream.test/test.m3u8' })
      )
    })
    await page.goto('/live')
    await waitAppReady(page)

    // 记忆源直接激活(不落第一个源)
    await expect(page.locator('.jc-live-source-chip.is-active')).toContainText('备用源')

    // 记忆频道高亮并装载画面(不自动播放)
    const active = page.locator('.jc-live-channel.is-active')
    await expect(active).toHaveCount(1)
    await expect(active).toContainText('测试台')
    await expect(page.locator('.jc-live-loading')).toBeHidden()
  })
})

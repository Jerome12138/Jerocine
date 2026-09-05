import { defineConfig, devices } from '@playwright/test'

/**
 * Playwright 配置
 *
 * 测试矩阵：
 *  - mobile-portrait (iPhone 12)         390 x 844
 *  - mobile-landscape                     844 x 390
 *  - tablet (iPad Mini)                   768 x 1024
 *  - desktop                              1366 x 768
 *  - desktop-2k                           2560 x 1440
 *  - tv                                   1920 x 1080  (启用 ?mode=tv)
 *
 * 测试时自动启动 dev server（VITE_USE_MOCK=1 走 mock 数据）。
 */

const PORT = 3600
const baseURL = `http://127.0.0.1:${PORT}`

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : 4,
  timeout: 30_000,
  expect: { timeout: 7_000 },

  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }]
  ],

  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    /** 跳过外部 picsum.photos 等慢图片不影响测试速度 */
    actionTimeout: 6_000,
    navigationTimeout: 15_000
  },

  /**
   * 全部项目固定用 Chromium，避免 webkit 浏览器额外下载（200+MB）。
   * Chromium 模拟移动端足够覆盖响应式 + UA 行为，对本项目（无 Safari 特性）够用。
   */
  projects: [
    {
      name: 'mobile-portrait',
      use: {
        browserName: 'chromium',
        viewport: { width: 390, height: 844 },
        userAgent:
          'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0.0.0 Mobile/15E148 Safari/604.1',
        deviceScaleFactor: 3,
        isMobile: true,
        hasTouch: true
      }
    },
    {
      name: 'mobile-landscape',
      use: {
        browserName: 'chromium',
        viewport: { width: 844, height: 390 },
        userAgent:
          'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0.0.0 Mobile/15E148 Safari/604.1',
        deviceScaleFactor: 3,
        isMobile: true,
        hasTouch: true
      }
    },
    {
      name: 'tablet',
      use: {
        browserName: 'chromium',
        viewport: { width: 768, height: 1024 },
        deviceScaleFactor: 2,
        hasTouch: true
      }
    },
    {
      name: 'desktop',
      use: {
        browserName: 'chromium',
        viewport: { width: 1366, height: 768 }
      }
    },
    {
      name: 'desktop-2k',
      use: {
        browserName: 'chromium',
        viewport: { width: 2560, height: 1440 }
      }
    },
    {
      name: 'tv',
      use: {
        browserName: 'chromium',
        viewport: { width: 1920, height: 1080 }
      }
    }
  ],

  webServer: {
    command: 'npm run dev',
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
    env: {
      VITE_USE_MOCK: '1'
    }
  }
})

import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config'

/**
 * vitest 配置. 与 vite.config 共享 (alias / plugins 等), 额外加测试相关字段:
 *  - include: 仅匹配单元测试 (src/**\/*.{test,spec}.ts) — 避免误吃 tests/e2e/**.spec.ts
 *    (e2e 是 Playwright 的, 不能被 vitest 跑)
 *  - environment: jsdom, 单测里用到 DOM API
 */
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      include: ['src/**/*.{test,spec}.{ts,tsx,js,jsx}'],
      exclude: ['tests/e2e/**', 'node_modules/**', 'dist/**', 'android/**'],
      environment: 'jsdom',
      globals: true
    }
  })
)

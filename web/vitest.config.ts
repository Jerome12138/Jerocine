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
    // 本仓在 Windows 上经 symlink(D:\Git\JerocineHarness\Jerocine → D:\Git\Jerocine)访问。
    // node 默认 realpath 会把模块解析到物理路径 D:/Git/Jerocine/... —— WorkBuddy 的
    // brokered-fs 垫片只放行工作区路径, 于是"文件存在却加载失败"。preserveSymlinks
    // 让解析路径保持工作区形态(D:/Git/JerocineHarness/...), 垫片放行。
    resolve: {
      preserveSymlinks: true
    },
    test: {
      include: ['src/**/*.{test,spec}.{ts,tsx,js,jsx}'],
      exclude: ['tests/e2e/**', 'node_modules/**', 'dist/**', 'android/**'],
      environment: 'jsdom',
      globals: true,
      // 2026-10-25 实测: 本机(symlink 工作区 + WorkBuddy brokered-fs 垫片)下并行收集
      // 会静默漏文件 —— 连续两次全量分别只收集到 89/88 个(应 91), 无任何 FAIL 行,
      // "全绿"具有欺骗性。串行后稳定 91 文件/730 用例。勿删。
      fileParallelism: false
    }
  })
)

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import UnoCSS from 'unocss/vite'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { fileURLToPath, URL } from 'node:url'

// 构建期版本目录: 部署脚本注入 JC_BUILD_TS=YYYYMMDD-HHMMSS, 产物落 dist/assets/<TS>/.
// 目的: 让 index.html 的资源引用与 Workbox precache manifest 在同一次构建里天然自洽
// (部署期不再对 index.html 做 sed 改写 —— 那会让 sw.js 的 manifest 与实际 URL 不一致,
//  导致 precache 全量 404、Service Worker 永远装不上).
// 未注入时 (本地 dev / 普通构建) 行为完全不变, 仍是 dist/assets/.
const BUILD_TS = process.env.JC_BUILD_TS ?? ''

// https://vitejs.dev/config/
export default defineConfig({
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  server: {
    host: '0.0.0.0',
    port: 3600,
    strictPort: false,
    // JEROCINE_DEV_PROXY 指向一个完整后端站点(如 https://jerocine.art)时,
    // /api 原样转发(线上后端就挂在 /api/v1 下, 不 rewrite); 真机联调用这条.
    // 未设置时走本地后端 127.0.0.1:3601 并 rewrite 掉 /api 前缀(原行为).
    proxy: process.env.JEROCINE_DEV_PROXY
      ? {
          '/api': {
            target: process.env.JEROCINE_DEV_PROXY,
            changeOrigin: true,
            secure: false
          }
        }
      : {
          '/api': {
            target: 'http://127.0.0.1:3601',
            changeOrigin: true,
            rewrite: (path) => path.replace(/^\/api/, '')
          }
        }
  },
  plugins: [
    UnoCSS(),
    vue(),
    AutoImport({
      imports: ['vue', 'vue-router', 'pinia', '@vueuse/core'],
      dts: 'src/types/auto-imports.d.ts',
      eslintrc: { enabled: false }
    }),
    Components({
      dirs: ['src/components/base'],
      extensions: ['vue'],
      deep: true,
      dts: 'src/types/components.d.ts'
    })
  ],
  build: {
    target: 'es2020',
    cssCodeSplit: true,
    // 版本目录(见顶部 BUILD_TS 说明): 有 TS 时资源落 assets/<TS>/, 否则保持 assets/
    assetsDir: BUILD_TS ? `assets/${BUILD_TS}` : 'assets',
    // sourcemap 开 hidden 模式 — .map 文件正常生成但 bundle 里不引用,
    // 浏览器不会自动加载 (不增加首屏 cost). 但 server 上文件存在,
    // 调试时手动下载 .map 配合 stack 可还原源码定位.
    sourcemap: 'hidden',
    rollupOptions: {
      output: {
        manualChunks: {
          'vue-vendor': ['vue', 'vue-router', 'pinia'],
          'video-vendor': ['video.js', '@videojs-player/vue'],
          'utils-vendor': ['axios', '@vueuse/core']
        }
      }
    },
    minify: 'esbuild'
  },
  esbuild: {
    drop: process.env.NODE_ENV === 'production' ? ['console', 'debugger'] : []
  }
})

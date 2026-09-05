import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import UnoCSS from 'unocss/vite'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { fileURLToPath, URL } from 'node:url'

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

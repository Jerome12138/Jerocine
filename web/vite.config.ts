import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import legacy from '@vitejs/plugin-legacy'
import UnoCSS from 'unocss/vite'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { VitePWA } from 'vite-plugin-pwa'
import postcss from 'postcss'
import { fileURLToPath, URL } from 'node:url'

// 老内核 flex-gap 兼容 PostCSS 插件(2026-10-23, TV 盒 Chrome 66 实测反馈):
// flex 容器的 gap 在 Chrome < 84 完全失效 —— 对产物 CSS 里所有"display:flex + gap
// 同条声明"的规则, 在 @supports not (aspect-ratio: 1/1) 闸门下(Chrome 88+/Safari 15
// 才有 aspect-ratio, 现代内核命中 false 整块跳过, 不产生双重间距)生成 sibling margin
// 等价回退: 横向容器 margin-left、flex-direction: column 容器 margin-top;
// grid 的 gap Chrome 66 本就支持, 明确跳过; display 不在同条声明的 gap 规则跳过
// (那类由 compat.css 的 .flex.gap-[...] 前缀选择器覆盖)。媒体查询上下文原样保留。
// ⚠️ 接线必须走 generateBundle 后处理而非 postcss.config: 后者会把 UnoCSS 注入的
// 虚拟 CSS 也拉进 postcss 解析(其产物有 postcss 8 解析不了的语法, 实测报
// Missed semicolon), 这里跳过 unocss 资产。
function flexGapCompatPostCSS() {
  const pending: { selector: string; prop: string; value: string; media: string | null }[] = []
  return {
    postcssPlugin: 'flex-gap-compat',
    Rule(rule: postcss.Rule) {
      let display = ''
      let direction = ''
      let gap = ''
      rule.each((node) => {
        if (node.type !== 'decl') return
        const prop = node.prop.toLowerCase()
        if (prop === 'display') display = node.value.toLowerCase()
        else if (prop === 'flex-direction') direction = node.value.toLowerCase()
        else if (prop === 'gap') gap = node.value.trim()
      })
      if (!gap || (display !== 'flex' && display !== 'inline-flex')) return
      const parts = gap.split(/\s+/)
      const rowGap = parts[0]
      const colGap = parts[1] || parts[0]
      const isColumn = direction.includes('column')
      const media =
        rule.parent?.type === 'atrule' && (rule.parent as postcss.AtRule).name === 'media'
          ? (rule.parent as postcss.AtRule).params
          : null
      pending.push({
        selector: `${rule.selector} > * + *`,
        prop: isColumn ? 'margin-top' : 'margin-left',
        value: isColumn ? rowGap : colGap,
        media
      })
    },
    RootExit(root: postcss.Root) {
      if (pending.length === 0) return
      const supports = postcss.atRule({
        name: 'supports',
        params: 'not (aspect-ratio: 1 / 1)',
        raws: { before: '\n/* 以下由 flex-gap-compat 自动生成(老内核 flex gap 回退), 请勿手改 */\n' }
      })
      const byMedia = new Map<string, typeof pending>()
      for (const item of pending) {
        const key = item.media ?? ''
        if (!byMedia.has(key)) byMedia.set(key, [])
        byMedia.get(key)!.push(item)
      }
      for (const [media, items] of byMedia) {
        let container: postcss.AtRule = supports
        if (media) {
          container = postcss.atRule({ name: 'media', params: media })
          supports.append(container)
        }
        for (const item of items) {
          container.append(postcss.rule({ selector: item.selector, nodes: [postcss.decl({ prop: item.prop, value: item.value })] }))
        }
      }
      root.append(supports)
      pending.length = 0
    }
  }
}
flexGapCompatPostCSS.postcssPlugin = 'flex-gap-compat'

// 构建期后处理: 逐个 CSS 产物跑上面的插件(跳过 UnoCSS 的虚拟资产)。
// ⚠️ 必须 enforce: 'post' + 挂在插件列表末尾 —— vite:css-post 会在自己的
// generateBundle 里重写 CSS 资产内容, normal 阶段的修改会被它整体覆盖。
async function flexGapCompatPlugin(): Promise<import('vite').Plugin> {
  return {
    name: 'flex-gap-compat',
    enforce: 'post',
    apply: 'build',
    async generateBundle(_, bundle) {
      for (const file of Object.keys(bundle)) {
        if (!file.endsWith('.css') || file.includes('unocss')) continue
        const asset = bundle[file]
        if (asset.type !== 'asset' || typeof asset.source !== 'string') continue
        try {
          const result = await postcss([flexGapCompatPostCSS()]).process(asset.source, { from: file })
          asset.source = result.css
        } catch (e) {
          const err = e as { line?: number; column?: number; reason?: string }
          if (err?.column) {
            const src = asset.source as string
            const at = src.split('\n').slice(0, (err.line ?? 1) - 1).join('\n').length + err.column
            console.error(`[flex-gap-compat] ${file} @${at}: ${err.reason}\n...${src.slice(Math.max(0, at - 120), at + 120)}...`)
          }
          throw e
        }
      }
    }
  }
}

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
    // 低版本 WebView 兼容打包(2026-10-15, 方案 B):
    // - targets chrome61: WebView 61~86 的电视/盒子(无 Play 商店, WebView 停在出厂版)也能跑 web 壳。
    //   modern 产物经 build.target es2018 降级语法(?. / ?? 由 esbuild 转换)吃下 61+;
    //   legacy 产物(SystemJS + core-js)给无 ESM 的 <61 浏览器(nomodule 自动分流)。
    // - modernPolyfills: true —— modern 产物也注入 core-js 补运行时 API 缺口(replaceAll/at 等,
    //   61~86 区间的 WebView 常缺), modern 用户多下一个 ~10KB 级并行小文件。
    // - ⚠️ 硬地板 Chrome 49: Vue 3 响应式依赖 Proxy, 无法 polyfill —— 低于它谁来打包都无解,
    //   壳侧 DeviceProbe 门槛(JcDeviceDiagnostics.MIN_WEBVIEW_MAJOR=61)与此对齐。
    legacy({
      targets: ['chrome >= 61'],
      modernPolyfills: true
    }),
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
    }),
    // Service Worker(离线包) — 见方案 §4.
    // 关键: 与上面的 build.assetsDir 版本目录配合 —— precache manifest 在**同一次构建**里
    // 按 dist 实际布局生成, 路径天然带 <TS>, 与线上 URL 自洽(不做任何部署期改写).
    VitePWA({
      // 静默更新: 新 SW 装好即 skipWaiting 激活, 用户不点下次导航/启动也是新版本
      registerType: 'autoUpdate',
      // 注册脚本内联进 index.html —— index.html 本身 no-cache, 无需再为根目录的无 hash 文件
      // (registerSW.js) 单独加缓存规则, 少一个需运维关注的产物。
      injectRegister: 'inline',
      // 不生成 web app manifest(无需"添加到主屏", 避免多一份需维护的清单)
      manifest: false,
      includeAssets: [
        'favicon*.png',
        'apple-touch-icon.png',
        'android-chrome*.png',
        'default-avatar.svg'
      ],
      workbox: {
        // 显式声明, 不依赖插件默认值(§4.2 C5)
        skipWaiting: true,
        clientsClaim: true,
        cleanupOutdatedCaches: true,
        // SW 自身不生成 sourcemap(减少产物; 业务 .map 由部署脚本统一排除)
        sourcemap: false,
        // 按真实布局: 有 JC_BUILD_TS 时资源在 assets/<TS>/*, 无则 assets/*; 两种都被 ** 覆盖
        globPatterns: ['index.html', 'assets/**/*.{js,css,svg,png,ico,woff2}'],
        // 导航命中 precache 的 index.html(不是 NetworkFirst); 新鲜度由 SW 更新通道保证
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api\//],
        // 不做 API/字体 runtimeCaching: 字体外链已删除(§5.2), API 一律走网络不缓存。
        // 但静态资源要留一条 CacheFirst 运行时缓存(2026-10-15 方案 B): legacy 产物被 globIgnores
        // 排除在 precache 之外(不让所有设备离线包翻倍), 低版本 WebView 靠这条在首次加载后
        // 把自己的 bundle 存进 Cache Storage, 二次启动走缓存。产物文件名带 hash 不可变, CacheFirst 安全。
        runtimeCaching: [
          {
            urlPattern: ({ url }) => url.pathname.startsWith('/assets/'),
            handler: 'CacheFirst',
            options: {
              cacheName: 'jc-assets',
              expiration: { maxEntries: 200, maxAgeSeconds: 60 * 60 * 24 * 30 },
              cacheableResponse: { statuses: [200] }
            }
          }
        ],
        // legacy 产物(-legacy-* / legacy-polyfills)不进 precache: 只有 <61 的 WebView 会下载它们,
        // precache 进清单等于让 100% 的 modern 设备为 0% 用得上的文件付离线包体积。
        // 低版本设备的缓存由上面 runtimeCaching 的 CacheFirst 在运行时补上。
        globIgnores: ['**/*legacy*']
      }
    }),
    // 老内核 flex-gap 回退必须最后跑(enforce post): 见函数内注释
    await flexGapCompatPlugin()
  ],
  build: {
    // es2018: modern 产物要能跑在 WebView 61+(es2020 的 ?. / ?? 需要 Chrome 80)。
    // esbuild 降级语法; 与上面 legacy 插件的 chrome61 targets 同一口径。
    target: 'es2018',
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
          'video-vendor': ['artplayer', 'hls.js'],
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

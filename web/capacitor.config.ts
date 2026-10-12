import type { CapacitorConfig } from '@capacitor/cli'

/**
 * Jerocine Capacitor 配置（Android TV 打包）
 *
 * - 单包合并后 APK 不携带前端产物: WebShell 远程加载线上 SPA,
 *   webDir 指向 android-shell-stub (仅含占位 index.html), 由 cap sync 灌进 assets/public。
 * - allowMixedContent 打开：第三方采集源可能为 http
 * - 包名 art.jerocine.app（Android TV Leanback Launcher 启动）
 * - SplashScreen 短暂展示主背景色（#0b0b0f）后即进 WebView
 */
const config: CapacitorConfig = {
  appId: 'art.jerocine.app',
  appName: 'Jerocine影视',
  webDir: 'android-shell-stub',
  server: {
    // 不写死 server.url. APK 启动时 EntryRouterActivity 按设备能力路由到 WebShellActivity,
    // 由它弹原生输入框; 用户填入实际服务器地址后 SharedPreferences 持久化 + webView.loadUrl().
    // 这样同一个 APK 可装到不同 TV 连不同 Jerocine 后端.
    androidScheme: 'http',
    cleartext: true
  },
  android: {
    allowMixedContent: true,
    backgroundColor: '#0b0b0fff'
  },
  plugins: {
    // 启动观感统一到原生侧(方案 §7, 评审 C10): AndroidX core-splashscreen(系统 splash, 纯黑)
    // → BrandSplashView(品牌首屏)。Capacitor SplashScreen 插件必须关掉, 否则会出现
    // "系统 splash → 插件 800ms 覆盖 → 品牌首屏"的三段跳。保留依赖但不再展示。
    SplashScreen: {
      launchShowDuration: 0,
      launchAutoHide: true
    }
  }
}

export default config

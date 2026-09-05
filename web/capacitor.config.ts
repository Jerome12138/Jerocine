import type { CapacitorConfig } from '@capacitor/cli'

/**
 * Jerocine Capacitor 配置（Android TV 打包）
 *
 * - 用户端 Vue3 SPA 编译产物 dist/ 作为 webDir
 * - allowMixedContent 打开：第三方采集源可能为 http
 * - 包名 com.jerocine.app（Android TV Leanback Launcher 启动）
 * - SplashScreen 短暂展示主背景色（#0b0b0f）后即进 WebView
 */
const config: CapacitorConfig = {
  appId: 'art.jerocine.app',
  appName: 'Jerocine影视',
  webDir: 'dist',
  server: {
    // 不写死 server.url. APK 启动时由 MainActivity 弹原生输入框,
    // 用户填入实际服务器地址后 SharedPreferences 持久化 + webView.loadUrl().
    // 这样同一个 APK 可装到不同 TV 连不同 Jerocine 后端.
    androidScheme: 'http',
    cleartext: true
  },
  android: {
    allowMixedContent: true,
    backgroundColor: '#0b0b0fff'
  },
  plugins: {
    SplashScreen: {
      launchShowDuration: 800,
      backgroundColor: '#0b0b0f',
      androidScaleType: 'CENTER_CROP',
      showSpinner: false,
      splashFullScreen: true,
      splashImmersive: true
    }
  }
}

export default config

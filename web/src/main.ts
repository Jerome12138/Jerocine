import { createApp } from 'vue'
import { createPinia } from 'pinia'

import 'virtual:uno.css'
import '@/assets/styles/reset.css'
import '@/assets/styles/theme.css'
import '@/assets/styles/tv-cards.css'
import '@/assets/styles/iconfont.css'

import App from './App.vue'
import router from './router'
import { installViewMode } from '@/composables/useViewMode'
import { installCapacitorShim } from '@/utils/capacitorShim'
import { telemetry } from '@/utils/telemetry'

// Mock 适配器已移除: 重构后对接真实 /api/v1 后端, 不再需要前端 mock。

// 远程加载(APK 壳 loadUrl https://...)下 window.Capacitor 不注入, 但壳在 pause/resume
// 会调 window.Capacitor.triggerEvent(...) → 报 undefined.triggerEvent. 入口尽早装兜底垫片。
installCapacitorShim()

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(router)

// Vue 组件级错误捕获 — 比 window.onerror 拿得到组件名 + props + lifecycle 信息,
// 是 "Script error. @ :0:0" 的替代信号源 (那个被 WebView 抹掉 source).
app.config.errorHandler = (err, instance, info) => {
  const compName = (instance as { $options?: { __name?: string; name?: string } } | null)
    ?.$options?.__name ??
    (instance as { $options?: { __name?: string; name?: string } } | null)?.$options?.name ??
    'unknown'
  telemetry.trackError(err as Error, 'vue-error', {
    component: compName,
    info, // lifecycle hook / event handler name
    path: window.location.pathname
  })
  console.error('[vue-errorHandler]', info, compName, err)
}

// 全局安装一次 viewMode（resize / storage 监听绑定到 window 生命周期）
installViewMode()

app.mount('#app')

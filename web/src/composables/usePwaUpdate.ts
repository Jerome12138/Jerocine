import { ref, onMounted, onBeforeUnmount, type Ref } from 'vue'
import { CHUNK_RELOAD_GUARD_KEY } from '@/utils/chunkReload'

/**
 * Service Worker 更新感知 — autoUpdate 静默更新模式下的"新版本已就绪"信号.
 *
 * 背景(见方案 §4.3):
 *  - 用 `registerType: 'autoUpdate'` (skipWaiting + clientsClaim) ⇒ 新 SW 装好即激活接管页面,
 *    用户不点, 下次导航/启动也一定是新版本; 但 TV 壳单页常驻时"根本没有下次导航",
 *    所以必须给一个立刻生效的出口 —— 就是本 composable 驱动的小图标.
 *  - 为什么不用插件内置 needRefresh: prompt 模式下新 SW 停在 waiting, 等所有旧页面关闭才激活;
 *    TV App 常驻后台不杀进程 ⇒ 会长期停在旧版本.
 *
 * 关键坑(必须显式处理, 否则首访就误报"新版本"):
 *  客户端带了 `clientsClaim: true` 后, **首次安装** SW 激活时会立刻 claim 当前未被控制的页面,
 *  同样会触发 `controllerchange`. 若不区分, 首访就会显示"新版本"。
 *  ⇒ 用 `hadController`(进入页面时是否已被 SW 控制) 区分:
 *     - 首访: hadController=false ⇒ 这一次 controllerchange 是"初次接管", 忽略
 *     - 回访/已控制: hadController=true ⇒ 之后的 controllerchange 才是"换了新版本"
 */

export interface PwaUpdate {
  /** 新版本已接管当前页面(可立即刷新切换) */
  updateReady: Ref<boolean>
  /** 立即刷新到新版本 */
  applyUpdate: () => void
  /** 当前环境是否支持 Service Worker */
  supported: boolean
}

/** Service Worker 是否可用(不支持则整体降级为"无更新提示", 不影响主路径) */
function detectSupported(): boolean {
  return typeof navigator !== 'undefined' && 'serviceWorker' in navigator
}

export function usePwaUpdate(): PwaUpdate {
  const supported = detectSupported()
  const updateReady = ref(false)

  // 进入页面时是否已被 SW 控制 —— 决定 controllerchange 是"初次接管"还是"换新版本"
  const hadController = supported && !!navigator.serviceWorker.controller

  let listener: (() => void) | null = null

  onMounted(() => {
    if (!supported) return
    listener = () => {
      if (!hadController) return // 首次接管, 不是"更新", 不提示
      updateReady.value = true
      // 新 SW 已接管 ⇒ 之后 reload 会拿到新 index.html, chunk 404 自愈可重新生效 ⇒ 清闸门
      try {
        sessionStorage.removeItem(CHUNK_RELOAD_GUARD_KEY)
      } catch {
        /* sessionStorage 不可用时忽略 */
      }
      void import('@/utils/telemetry')
        .then(({ telemetry }) => {
          telemetry.track('action', { category: 'pwa', action: 'update-ready' })
        })
        .catch(() => {})
    }
    navigator.serviceWorker.addEventListener('controllerchange', listener)
  })

  onBeforeUnmount(() => {
    if (listener) navigator.serviceWorker.removeEventListener('controllerchange', listener)
    listener = null
  })

  function applyUpdate(): void {
    if (typeof window !== 'undefined') window.location.reload()
  }

  return { updateReady, applyUpdate, supported }
}

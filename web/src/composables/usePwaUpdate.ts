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
  /** 检测到新版本(开始下载, 可能尚未接管) —— 徽标此刻即亮 */
  updateFound: Ref<boolean>
}

/** Service Worker 是否可用(不支持则整体降级为"无更新提示", 不影响主路径) */
function detectSupported(): boolean {
  return typeof navigator !== 'undefined' && 'serviceWorker' in navigator
}

/** 清掉本应用的 SW 缓存(precache + 静态 runtime cache)。
 *  用于"新版本预取未完成就点立即更新": 清掉旧缓存后 reload, 导航请求 miss →
 *  直接走网络拿新 index.html 与新资源, 立即生效; 新 SW 随后自行完成 install 重建缓存。 */
async function clearAppCaches(): Promise<void> {
  try {
    const keys = await caches.keys()
    await Promise.all(
      keys
        .filter((k) => k.startsWith('workbox') || k === 'jc-assets')
        .map((k) => caches.delete(k))
    )
  } catch {
    /* Cache API 不可用时忽略 —— reload 后靠浏览器 HTTP 缓存策略兜底 */
  }
}

export function usePwaUpdate(): PwaUpdate {
  const supported = detectSupported()
  const updateReady = ref(false)
  /** 检测到新版本开始下载(install 中, 尚未接管) —— 徽标此刻即出现, 不必等预取完成 */
  const updateFound = ref(false)

  // 进入页面时是否已被 SW 控制 —— 决定 controllerchange 是"初次接管"还是"换新版本"
  const hadController = supported && !!navigator.serviceWorker.controller

  let listener: (() => void) | null = null
  /** 主动检测链路的清理(定时器/事件), 与 controllerchange 监听同生命周期 */
  let cleanupExtra: (() => void) | null = null

  onMounted(() => {
    if (!supported) return
    listener = () => {
      if (!hadController) return // 首次接管, 不是"更新", 不提示
      updateReady.value = true
      updateFound.value = true
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

    // updatefound: 有新版本开始下载(install 中) —— 此刻即亮徽标, 不等预取完成。
    // 首访(未被 SW 控制)的初次安装不算更新。
    navigator.serviceWorker.getRegistration()?.then((reg) => {
      reg?.addEventListener('updatefound', () => {
        if (hadController) updateFound.value = true
      })
    }).catch(() => {})

    // 主动更新检测: 浏览器默认只在导航时 check, SW install 又要预取全部新资源(弱网数十秒),
    // "发版后刷新也拿不到新版"由此而来 —— 这里周期性 + 可见性恢复时立即 registration.update(),
    // 让 skipWaiting 的新 SW 尽早激活接管(controllerchange → updateReady → 各头部「新版本」徽标)。
    let checkTimer: number | undefined
    const checkNow = (): void => {
      navigator.serviceWorker.getRegistration()?.then((reg) => reg?.update()).catch(() => {})
    }
    const onVisible = (): void => {
      if (document.visibilityState === 'visible') checkNow()
    }
    checkNow()
    checkTimer = window.setInterval(checkNow, 60_000)
    document.addEventListener('visibilitychange', onVisible)
    window.addEventListener('pageshow', onVisible)
    cleanupExtra = () => {
      if (checkTimer !== undefined) window.clearInterval(checkTimer)
      document.removeEventListener('visibilitychange', onVisible)
      window.removeEventListener('pageshow', onVisible)
    }
  })

  onBeforeUnmount(() => {
    if (listener) navigator.serviceWorker.removeEventListener('controllerchange', listener)
    listener = null
    if (cleanupExtra) cleanupExtra()
    cleanupExtra = null
  })

  function applyUpdate(): void {
    if (typeof window === 'undefined') return
    if (updateReady.value) {
      // 新 SW 已接管: 直接刷新即拿到新 index.html
      window.location.reload()
      return
    }
    // 新 SW 仍在 install(预取未完): 清掉旧缓存后 reload —— 导航请求 miss precache,
    // 直接走网络拿新版本, 不等漫长的预取; 新 SW 随后自行完成 install 重建缓存。
    void clearAppCaches().finally(() => window.location.reload())
  }

  return { updateReady, updateFound, applyUpdate, supported }
}

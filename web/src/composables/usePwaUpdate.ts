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
 *
 * 2026-10-31 修: hadController 只能排除"首访", **不能判断页面内容是否已是最新**。
 *  实测路径(用户报障): 旧页面点橙色「发现更新」→ clearAppCaches() 清缓存 + reload ⇒
 *  这次导航躲过了旧 SW 的 precache, 直接走网络拿到**新** index.html + 新资源(页面已是新版),
 *  但**控制器仍是旧 SW**(新 SW 还在 install)。等新 SW 激活 claim 时又触发一次 controllerchange,
 *  hadController 仍是 true ⇒ 误报蓝色「新版本」。即"页面已是新版, 徽标却说有新版"。
 *  ⇒ 判据改为**构建号比对**: 本页构建号(vite define `__APP_BUILD__` = 部署的 JC_BUILD_TS)
 *    与"当前能拿到的最新构建号"(抓 index.html 里的 assets/<TS>/ 目录名)比对,
 *    相同 ⇒ 页面本就是最新, 不提示; 不同/探测失败 ⇒ 视为过旧, 提示。见 isPageStale。
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

/** 本页所属构建号 —— 构建期 define 注入(部署脚本的 JC_BUILD_TS); dev / 未注入时为空串 */
function readOwnBuild(): string {
  try {
    return typeof __APP_BUILD__ === 'string' ? __APP_BUILD__ : ''
  } catch {
    return ''
  }
}

/**
 * 从 index.html 里取资源版本目录名(`assets/<TS>/...` 里的 TS) —— 即这份 HTML 所属构建。
 * 未注入版本的构建资源仍在 `assets/`(无子目录), 解析不出 ⇒ 返回空串。
 */
export function parseBuildTag(html: string): string {
  const m = /\/assets\/(\d{8}-\d{6})\//.exec(html)
  return m?.[1] ?? ''
}

/**
 * "当前页面是否确实落后于可用版本" 的纯判据 —— 徽标要不要亮就看它。
 *  - 首访(进入时无 controller): 页面就是刚拿的, 不算更新
 *  - 构建号相同: 页面内容已是最新(典型场景: 强制刷新后旧 SW 才被替换) ⇒ 不提示
 *  - 构建号不同 / 任一未知: 判定不了就保守提示(宁可多给一个刷新入口, 也别让 TV 常驻停在旧版)
 */
export function isPageStale(input: {
  hadController: boolean
  ownBuild: string
  latestBuild: string
}): boolean {
  if (!input.hadController) return false
  if (!input.ownBuild || !input.latestBuild) return true
  return input.ownBuild !== input.latestBuild
}

/**
 * 探测"当前能拿到的最新构建号": 抓一份 index.html。
 *  - 带随机 query ⇒ 不会命中 Workbox precache 的 `/index.html` 条目(参数不在
 *    ignoreURLParametersMatching 白名单里), 也不会被 HTTP 缓存糊住(cache: no-store);
 *  - 若确实被 SW precache 命中, 返回的是**该 SW 的**版本 —— 同样满足判据语义
 *    ("控制器持有的版本" vs "本页运行中的版本")。
 */
async function fetchLatestBuild(): Promise<string> {
  if (typeof fetch !== 'function') return ''
  try {
    const res = await fetch(`/index.html?__jc_build_probe=${Date.now()}`, { cache: 'no-store' })
    if (!res.ok) return ''
    return parseBuildTag(await res.text())
  } catch {
    return ''
  }
}

export interface PwaUpdateDeps {
  /** 本页构建号(默认取构建期 define `__APP_BUILD__`) —— 仅测试注入 */
  ownBuild?: string
  /** 最新构建号探测(默认抓 index.html) —— 仅测试注入 */
  probeLatestBuild?: () => Promise<string>
}

export function usePwaUpdate(deps: PwaUpdateDeps = {}): PwaUpdate {
  const supported = detectSupported()
  const updateReady = ref(false)
  /** 检测到新版本开始下载(install 中, 尚未接管) —— 徽标此刻即出现, 不必等预取完成 */
  const updateFound = ref(false)
  const ownBuild = deps.ownBuild ?? readOwnBuild()
  const probeLatestBuild = deps.probeLatestBuild ?? fetchLatestBuild

  // 进入页面时是否已被 SW 控制 —— 决定 controllerchange 是"初次接管"还是"换新版本"
  const hadController = supported && !!navigator.serviceWorker.controller

  let listener: (() => void) | null = null

  /** controllerchange 后判定"这次接管对本页是否意味着有新版本" */
  async function evaluateStale(): Promise<boolean> {
    if (!hadController) return false
    if (!ownBuild) return true // 拿不到本页构建号(dev/未注入) ⇒ 沿用旧语义: 有接管就提示
    let latest = ''
    try {
      latest = await probeLatestBuild()
    } catch {
      latest = ''
    }
    return isPageStale({ hadController, ownBuild, latestBuild: latest })
  }

  function track(action: string): void {
    void import('@/utils/telemetry')
      .then(({ telemetry }) => {
        telemetry.track('action', { category: 'pwa', action })
      })
      .catch(() => {})
  }

  onMounted(() => {
    if (!supported) return
    listener = () => {
      // 新 SW 已接管 ⇒ 之后 reload 会拿到新 index.html, chunk 404 自愈可重新生效 ⇒ 清闸门
      try {
        sessionStorage.removeItem(CHUNK_RELOAD_GUARD_KEY)
      } catch {
        /* sessionStorage 不可用时忽略 */
      }
      void evaluateStale().then((stale) => {
        if (!stale) {
          // 页面内容已是新版(强制刷新后旧 SW 才被替换), 不给"新版本"误导
          track('update-skip-current')
          return
        }
        updateReady.value = true
        updateFound.value = true
        track('update-ready')
      })
    }
    navigator.serviceWorker.addEventListener('controllerchange', listener)

    // updatefound: 有新版本开始下载(install 中) —— 刷新后浏览器导航时机会自动 check sw.js,
    // 此刻即亮提示, 不等预取完成。首访(未被 SW 控制)的初次安装不算更新。
    // (2026-10-27 用户确认: 站点无强 web 即时性需求, 不做周期性主动 update 检测)
    navigator.serviceWorker.getRegistration()?.then((reg) => {
      reg?.addEventListener('updatefound', () => {
        if (hadController) updateFound.value = true
      })
    }).catch(() => {})
  })

  onBeforeUnmount(() => {
    if (listener) navigator.serviceWorker.removeEventListener('controllerchange', listener)
    listener = null
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

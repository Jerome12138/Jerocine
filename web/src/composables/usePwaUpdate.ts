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
 *
 * 2026-11-09 修(用户报障: "一个版本提示好几遍, 即使刷新了也一样")。三处缺陷:
 *  ① `updatefound` 分支原来**无条件**亮灯(`if (hadController) updateFound = true`),
 *    完全没有构建号比对、没有去重。丢包链路上 3MB precache 安装失败是常态, 下次导航
 *    浏览器重试 install ⇒ 又 updatefound ⇒ 又亮一次。**这是重复提示的主因。**
 *    ⇒ 现在 updatefound 只是"去核对一次"的**信号**, 统一走 checkForUpdate() 的构建号判据。
 *  ② `applyUpdate` 在 install 未完成时"清缓存 + 立刻 reload": 此时旧 SW 仍控制页面,
 *    reload 的导航被它应答 ⇒ 拿到的还是旧 index.html ⇒ 页面没换 ⇒ 稍后 controllerchange
 *    再判一次"落后" ⇒ 又亮。**这是"刷新了也一样"的主因**, 且 clearAppCaches 把 3MB
 *    precache 白清了。
 *    ⇒ 改为确定性流程: reg.update() → 等新 SW 接管(controllerchange, 带超时兜底) → 才 reload。
 *  ③ `isPageStale` 把"探测失败(空串)"当"落后", 而探测失败在 5–10% 丢包链路上是常态
 *    ⇒ 随机误报。现在探测失败只在"sw.js 确实变了"(updatefound 硬证据)时才提示,
 *    单纯的 controllerchange 遇到探测失败一律不提示。
 *  另外新增**同构建号每会话只提示一次**(sessionStorage): 点过更新、刷新后仍是旧版时
 *  不再重复弹; 想彻底拿新版仍可用顶栏「刷新」按钮(reloadToLatest)。
 */

export interface PwaUpdate {
  /** 新版本已接管当前页面(可立即刷新切换) */
  updateReady: Ref<boolean>
  /** 检测到新版本(开始下载, 可能尚未接管) —— 徽标此刻即亮 */
  updateFound: Ref<boolean>
  /** 正在等待新 SW 接管(用户已点更新) —— 徽标显示"更新中"并禁止重复点击 */
  applying: Ref<boolean>
  /** 立即刷新到新版本 */
  applyUpdate: () => void
  /** 当前环境是否支持 Service Worker */
  supported: boolean
}

/** 本会话已提示过的构建号 —— 同一版本只提示一次, 消除"一个版本提示好几遍" */
const PROMPTED_BUILD_KEY = 'jc-pwa-prompted-build'
/** 点「更新」后等新 SW 接管的兜底时长(ms): 慢链路上 install 可能很久, 超时也要给出口 */
const APPLY_WAIT_MS = 8000
/** 回到前台重查的最小间隔(ms): TV 壳常驻没有导航机会, 靠 visibilitychange 兜底 */
const VISIBLE_RECHECK_MS = 60_000

/**
 * 触发核对的原因 —— 决定"探测失败"时该不该保守提示:
 *  - `updatefound`: sw.js 内容确实变了(硬证据), 探测失败也提示
 *  - `controllerchange`: 控制器换过(可能只是 clientsClaim), 探测失败不提示(避免误报)
 *  - `visible`: 回到前台例行核对, 探测失败不提示
 */
type UpdateSignal = 'controllerchange' | 'updatefound' | 'visible'

/** Service Worker 是否可用(不支持则整体降级为"无更新提示", 不影响主路径) */
function detectSupported(): boolean {
  return typeof navigator !== 'undefined' && 'serviceWorker' in navigator
}

/** 本页所属构建号 —— 构建期 define 注入(部署脚本的 JC_BUILD_TS); dev / 未注入时为空串 */
function readOwnBuild(): string {
  try {
    return typeof __APP_BUILD__ === 'string' ? __APP_BUILD__ : ''
  } catch {
    return ''
  }
}

/** 本会话已提示到的构建号(读失败按"没提示过"处理) */
function readPromptedBuild(): string {
  try {
    return sessionStorage.getItem(PROMPTED_BUILD_KEY) ?? ''
  } catch {
    return ''
  }
}

/** 记下"这个构建号已经提示过" —— 同一版本刷新后不再重复弹 */
function markPromptedBuild(build: string): void {
  if (!build) return
  try {
    sessionStorage.setItem(PROMPTED_BUILD_KEY, build)
  } catch {
    /* sessionStorage 不可用(隐私模式): 退化为"每次都可能提示", 不影响更新能力 */
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
 *
 * ⚠️ `latestBuild === ''`(探测失败) 由调用方先行分流: 只有 updatefound 硬证据才走这里,
 *    单纯的 controllerchange 遇到探测失败不提示(见 checkForUpdate)。
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
 *  - 线上 index.html 是 `Cache-Control: no-cache`, EO 也是 MISS 直回源(已实测),
 *    因此拿到的就是当前部署的真实版本号。
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
  /** 等待接管超时(ms) —— 仅测试注入, 生产用 APPLY_WAIT_MS */
  applyWaitMs?: number
  /** 重新加载页面(默认 location.reload) —— 仅测试注入 */
  reload?: () => void
}

export function usePwaUpdate(deps: PwaUpdateDeps = {}): PwaUpdate {
  const supported = detectSupported()
  const updateReady = ref(false)
  /** 检测到新版本开始下载(install 中, 尚未接管) —— 徽标此刻即出现, 不必等预取完成 */
  const updateFound = ref(false)
  /** 用户已点更新, 正在等新 SW 接管 —— 徽标转"更新中", 禁止重复点击 */
  const applying = ref(false)
  const ownBuild = deps.ownBuild ?? readOwnBuild()
  const probeLatestBuild = deps.probeLatestBuild ?? fetchLatestBuild
  const applyWaitMs = deps.applyWaitMs ?? APPLY_WAIT_MS
  const doReload = deps.reload ?? ((): void => window.location.reload())

  // 进入页面时是否已被 SW 控制 —— 决定 controllerchange 是"初次接管"还是"换新版本"
  const hadController = supported && !!navigator.serviceWorker.controller

  let controllerListener: (() => void) | null = null
  let updateFoundListener: (() => void) | null = null
  let visibleListener: (() => void) | null = null
  let reg: ServiceWorkerRegistration | null = null
  /** 同一次核对只跑一趟(探测是网络请求, 多个信号并发时会重复发) */
  let checking = false
  /** 核对期间又来了 controllerchange —— 会补跑一次, 否则会漏掉"已接管"这个最强信号 */
  let pendingControllerChange = false
  /** 本页实例内是否已经提示过 —— 跨"刷新/重新挂载"由 sessionStorage 去重, 本实例内允许升级到 ready */
  let promptedInThisInstance = false
  /** 上次"回到前台"核对的时刻, 用于节流 */
  let lastVisibleCheck = 0

  function track(action: string): void {
    void import('@/utils/telemetry')
      .then(({ telemetry }) => {
        telemetry.track('action', { category: 'pwa', action })
      })
      .catch(() => {})
  }

  /**
   * 统一的"是否要给用户提个醒"判据 —— 所有入口信号都收敛到这里。
   * 顺序很关键: 先判"页面已最新"(收掉可能的误亮) → 再判"探测可信度" → 最后去重。
   */
  async function checkForUpdate(signal: UpdateSignal): Promise<void> {
    if (!hadController) return
    if (checking) {
      // 正在核对时又来了更强的信号: 记下来, 等这一趟结束补跑(否则会漏掉 ready 升级)
      if (signal === 'controllerchange') pendingControllerChange = true
      return
    }
    checking = true
    try {
      let latest = ''
      try {
        latest = await probeLatestBuild()
      } catch {
        latest = ''
      }

      if (latest) {
        if (!isPageStale({ hadController, ownBuild, latestBuild: latest })) {
          // 页面内容已是新版(典型: 强制刷新后旧 SW 才被替换) —— 收掉徽标, 不给误导
          if (updateFound.value) {
            updateFound.value = false
            updateReady.value = false
          }
          track('update-skip-current')
          return
        }
      } else if (signal !== 'updatefound') {
        // 探测失败: 只有"sw.js 确实变了"这条硬证据才值得提示。
        // 单纯的 controllerchange 在丢包链路上遇到探测失败是常态, 提示就是误报。
        track('update-skip-probe-failed')
        return
      }

      // 去重: 同一构建号本会话只提示一次 —— 点过更新、刷新后仍是旧版时不再重复弹。
      // 注意判据分两层:
      //  - 跨页面实例(刷新/重新挂载): 看 sessionStorage, 已提示过就完全静默;
      //  - 同一实例内: 允许把 amber「发现更新」升级成蓝色「新版本」(同一次提示的进展, 不是重复)
      const key = latest || `unknown:${signal}`
      if (!promptedInThisInstance && readPromptedBuild() === key) {
        track('update-skip-already-prompted')
        return
      }
      markPromptedBuild(key)
      promptedInThisInstance = true

      updateFound.value = true
      // 只有 controllerchange 才意味着"新 SW 已接管, 直接刷新即可"; 一旦 ready 不因后续信号降级
      if (signal === 'controllerchange') updateReady.value = true
      track(updateReady.value ? 'update-ready' : 'update-found')
    } finally {
      checking = false
      if (pendingControllerChange) {
        pendingControllerChange = false
        void checkForUpdate('controllerchange')
      }
    }
  }

  /** 等新 SW 接管(controllerchange) —— 超时也 resolve, 保证用户总能刷新 */
  function waitForControllerChange(timeoutMs: number): Promise<void> {
    return new Promise((resolve) => {
      let done = false
      const finish = (): void => {
        if (done) return
        done = true
        try {
          navigator.serviceWorker.removeEventListener('controllerchange', finish)
        } catch {
          /* 忽略 */
        }
        resolve()
      }
      try {
        navigator.serviceWorker.addEventListener('controllerchange', finish)
      } catch {
        resolve()
        return
      }
      window.setTimeout(finish, timeoutMs)
    })
  }

  onMounted(() => {
    if (!supported) return

    controllerListener = () => {
      // 新 SW 已接管 ⇒ 之后 reload 会拿到新 index.html, chunk 404 自愈可重新生效 ⇒ 清闸门
      try {
        sessionStorage.removeItem(CHUNK_RELOAD_GUARD_KEY)
      } catch {
        /* sessionStorage 不可用时忽略 */
      }
      void checkForUpdate('controllerchange')
    }
    navigator.serviceWorker.addEventListener('controllerchange', controllerListener)

    // updatefound 只当"去核对一次"的信号(不再无条件亮灯): 安装可能失败、下次导航又重试,
    // 无条件亮灯就是"同一个版本提示好几遍"的来源。
    // 老内核可能没有 getRegistration / 返回非 Promise ⇒ 全程可选链, 失败就当"无此信号"
    try {
      void navigator.serviceWorker
        .getRegistration?.()
        ?.then?.((r) => {
          if (!r) return
          reg = r
          updateFoundListener = () => void checkForUpdate('updatefound')
          r.addEventListener('updatefound', updateFoundListener)
        })
        ?.catch?.(() => {})
    } catch {
      /* 忽略: 没有 updatefound 通道时, controllerchange / visibilitychange 仍能兜住 */
    }

    // TV 壳常驻 / 移动端切回前台: 没有导航机会, 靠 visibilitychange 补一次核对(节流)
    if (typeof document !== 'undefined') {
      visibleListener = () => {
        if (document.visibilityState !== 'visible') return
        const now = Date.now()
        if (now - lastVisibleCheck < VISIBLE_RECHECK_MS) return
        lastVisibleCheck = now
        void checkForUpdate('visible')
      }
      document.addEventListener('visibilitychange', visibleListener)
    }

    // 页面刚起来就核对一次: 覆盖"TV 常驻后重新显示、但 WebView 没触发可见性事件"的情况。
    // 页面本来就是最新时这次探测只会得到"相同构建号", 不会亮灯。
    lastVisibleCheck = Date.now()
    void checkForUpdate('visible')
  })

  onBeforeUnmount(() => {
    if (controllerListener) navigator.serviceWorker.removeEventListener('controllerchange', controllerListener)
    controllerListener = null
    if (reg && updateFoundListener) reg.removeEventListener('updatefound', updateFoundListener)
    updateFoundListener = null
    if (visibleListener && typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', visibleListener)
    }
    visibleListener = null
  })

  /**
   * 点「更新」后的确定性切换:
   *  - 已接管(updateReady) ⇒ 直接刷新, 拿新 index.html
   *  - 仍在 install ⇒ 先让 SW update 一次, 再**等它真正接管**才刷新。
   *    旧实现在这里"清缓存 + 立刻 reload", 而旧 SW 仍控制页面 ⇒ reload 被它应答
   *    ⇒ 拿回旧 index.html ⇒ 页面没换 ⇒ 稍后又亮一次("刷新了也一样")。
   *    这里不再清缓存: precache 正是我们想保留的东西, 清掉等于让用户重下 3MB。
   */
  function applyUpdate(): void {
    if (typeof window === 'undefined' || applying.value) return
    applying.value = true
    void (async () => {
      if (updateReady.value) {
        doReload()
        return
      }
      try {
        const r = reg ?? (await navigator.serviceWorker.getRegistration())
        if (r) {
          reg = r
          try {
            await r.update()
          } catch {
            /* 网络差时 update 可能抛错 —— 不影响后面的等待与兜底刷新 */
          }
          // prompt 模式下新 SW 会停在 waiting, 主动放行; autoUpdate 下这行是空操作
          try {
            r.waiting?.postMessage({ type: 'SKIP_WAITING' })
          } catch {
            /* 忽略 */
          }
        }
      } catch {
        /* getRegistration 失败: 直接走兜底 */
      }
      await waitForControllerChange(applyWaitMs)
      doReload()
    })()
  }

  return { updateReady, updateFound, applying, applyUpdate, supported }
}

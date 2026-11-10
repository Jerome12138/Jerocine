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
 *  另外新增**同构建号每设备只提示一次**(localStorage): 点过更新、刷新后仍是旧版时
 *  不再重复弹; 想彻底拿新版仍可用顶栏「刷新」按钮(reloadToLatest)。
 *
 * 2026-11-09 补修(用户要求"好好修"): 上一版把去重记在 **sessionStorage**, 而 sessionStorage
 *  的生命周期是"一个标签页" —— TV 壳每次冷启动、Web 每开一个新标签都是全新 session,
 *  于是同一个版本照样"每启动一次提示一次", 报障现象只轻了一半。
 *  ⇒ 改记 **localStorage**(按设备+源持久化): 一个构建号只提示一次, 出现**更新的**构建号
 *    key 不同 ⇒ 照常提示。存储不可用(隐私模式/配额满)时退回 sessionStorage, 再不行就
 *    退化为"每次都可能提示"——只影响提示频次, 不影响更新能力。
 *
 * 2026-11-09 补修②(核对逻辑时发现的死角): 点「发现更新」后要等新 SW **接管**才刷新
 *  (最多 8s)。慢链路(跨境 3MB precache 要 30–40s)必然超时 ⇒ 兜底刷新时控制器还是旧的,
 *  导航被旧 SW 的 precache 应答 ⇒ **拿回旧页面**; 而"同构建号已提示"的记录又会把刷新后的
 *  核对直接静默 ⇒ 用户点了更新、页面没变、还再也看不到入口。
 *  ⇒ 超时兜底刷新的分支里**清掉去重记录**: 上一次提示没兑现成新版本, 就该允许再提一次
 *    (刷新后要么已是新版(不提示), 要么仍落后(再给一次入口))。
 *  语义边界: 只有"超时(没接管)"才清; 已接管后的正常刷新**不清** —— 那一次是兑现了的。
 *
 * 2026-11-09 补修③(对齐用户对更新链路的预期): 两处"说的和做的不一样"。
 *  ① 徽标亮起 ≠ 正在下载: 页面侧的核对只抓 index.html 比构建号, 从不触发 SW 下载;
 *    真要开始下得靠浏览器自己的检查(导航/约 24h)或用户点击 ⇒ 琥珀态可能"亮很久其实
 *    一个字节没下", 点了还得从头等。⇒ 徽标亮起时**主动 `reg.update()`** 一次(按构建号
 *    去重, 不重复抓 sw.js), 让「发现更新」名副其实。
 *  ② 用户期望"点一下就变新", 而琥珀态点击只能等 install。原 8s 上限对跨境 3MB precache
 *    (30–40s)必然超时 ⇒ 等于"点了没用"。⇒ 上限放宽到 40s(覆盖跨境整包), 装好即刷;
 *    **只有等满 40s 仍未接管**才走"硬重置"(拆 SW + 清本应用 Cache Storage)再刷新 ——
 *    那是唯一能保证"绕开旧 precache 拿到新页面"的办法, 代价是正在下的 precache 作废重下,
 *    所以只压在最坏分支上, 常态一分钱不花(已接管⇒立即刷; 未接管⇒等下载完成即刷)。
 *
 * 2026-11-10 修(首访会话的更新盲区)。两处都是"注册时机被 load 门控"的连带后果,
 *  但**不靠提前注册解决** —— 那条路会把 1.36MB precache 的 install 塞进首屏关键路径,
 *  在窄链路上是拿首屏换离线包。真正该修的是状态维护:
 *  ① `hadController` 原来是 setup 时算一次的常量, 首访恒 false ⇒ checkForUpdate 首句
 *    就 return ⇒ **首访整条会话都不提示任何更新**(TV 壳常驻可达数天), 只能等下次访问自愈。
 *    ⇒ 改为可变的 `controlled`: 首访的**第一次** controllerchange 是 clientsClaim 的初次
 *      接管, 跳过这一次判定但把它置 true —— 页面从此就是正常的"被控制"状态。
 *  ② `reg` 只在 mount 取一次 `getRegistration()`。首访那时还没有注册(要等 load 之后),
 *    取到 undefined ⇒ `updatefound` 永远挂不上、`kickDownload` 一直空转。
 *    ⇒ 取不到就等 `navigator.serviceWorker.ready`(只在存在 active worker 时 resolve,
 *      正好是"注册 → install → activate"的终点)再挂, 幂等封装在 attachUpdateFound。
 */

export interface PwaUpdate {
  /** 新版本已接管当前页面(可立即刷新切换) */
  updateReady: Ref<boolean>
  /** 检测到新版本(开始下载, 可能尚未接管) —— 徽标此刻即亮 */
  updateFound: Ref<boolean>
  /** 正在等待新 SW 接管(用户已点更新) —— 徽标显示"更新中"并禁止重复点击 */
  applying: Ref<boolean>
  /** 新版本落地进度 0–1(按已写入 precache 的文件数估算, 见 startProgressSampling) */
  progress: Ref<number>
  /** 立即刷新到新版本 */
  applyUpdate: () => void
  /** 当前环境是否支持 Service Worker */
  supported: boolean
}

/** 已提示过的构建号 —— 同一版本只提示一次, 消除"一个版本提示好几遍" */
const PROMPTED_BUILD_KEY = 'jc-pwa-prompted-build'
/** 探测存储可用性用的临时键(写完立刻删, 不留下垃圾) */
const STORAGE_PROBE_KEY = 'jc-pwa-storage-probe'
/**
 * 点「更新」后等新 SW 接管的时长(ms)。
 * 8s 对跨境链路是必然超时(3MB precache 要 30–40s), 等于"点了没反应";
 * 40s 覆盖整包下载, 装好立刻刷。超时才走硬重置兜底(见 applyUpdate)。
 */
const APPLY_WAIT_MS = 40_000
/** 回到前台重查的最小间隔(ms): TV 壳常驻没有导航机会, 靠 visibilitychange 兜底 */
const VISIBLE_RECHECK_MS = 60_000
/** 进度采样间隔(ms) —— 只数 Cache Storage 条目数, 开销可忽略 */
const PROGRESS_POLL_MS = 800
/** 拿不到基线时按耗时缓动到的"预估整包时长"(ms): 跨境 3MB 约 30–40s */
const PROGRESS_ESTIMATE_MS = 30_000

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

/**
 * 去重存储的句柄(懒探测, 只探一次) —— 优先 localStorage, 不可用时退回 sessionStorage。
 *
 * 为什么必须 localStorage: sessionStorage 跟着"标签页"走, TV 壳每次冷启动、Web 每开新标签
 * 都是新 session ⇒ 同一个版本"每启动一次提示一次"。localStorage 按设备+源持久化,
 * 才真正做到"一个版本只提示一次"。
 */
let promptedStore: Storage | null | undefined
function getPromptedStore(): Storage | null {
  if (promptedStore !== undefined) return promptedStore
  promptedStore = null
  const usable = (s: Storage | undefined): boolean => {
    if (!s) return false
    try {
      s.setItem(STORAGE_PROBE_KEY, '1')
      s.removeItem(STORAGE_PROBE_KEY)
      return true
    } catch {
      return false
    }
  }
  let local: Storage | undefined
  let session: Storage | undefined
  try {
    local = typeof localStorage === 'undefined' ? undefined : localStorage
  } catch {
    local = undefined
  }
  try {
    session = typeof sessionStorage === 'undefined' ? undefined : sessionStorage
  } catch {
    session = undefined
  }
  if (usable(local)) promptedStore = local ?? null
  else if (usable(session)) promptedStore = session ?? null
  return promptedStore
}

/** 已提示到的构建号(读失败按"没提示过"处理) */
function readPromptedBuild(): string {
  try {
    return getPromptedStore()?.getItem(PROMPTED_BUILD_KEY) ?? ''
  } catch {
    return ''
  }
}

/** 记下"这个构建号已经提示过" —— 同一版本刷新/重启后都不再重复弹 */
function markPromptedBuild(build: string): void {
  if (!build) return
  try {
    getPromptedStore()?.setItem(PROMPTED_BUILD_KEY, build)
  } catch {
    /* 存储不可用(隐私模式/配额满): 退化为"每次都可能提示", 不影响更新能力 */
  }
}

/** 忘掉去重记录 —— 用于"提示过但没能真的切到新版"的情况(见 applyUpdate 超时分支) */
function clearPromptedBuild(): void {
  try {
    getPromptedStore()?.removeItem(PROMPTED_BUILD_KEY)
  } catch {
    /* 忽略 */
  }
}

/** 清掉去重记录 —— 仅测试用(生产没有"重新允许提示同一版本"的语义) */
export function resetPromptedBuildForTest(): void {
  promptedStore = undefined
  try {
    localStorage.removeItem(PROMPTED_BUILD_KEY)
  } catch {
    /* 忽略 */
  }
  try {
    sessionStorage.removeItem(PROMPTED_BUILD_KEY)
  } catch {
    /* 忽略 */
  }
}

/**
 * 硬重置: 清掉本应用的 Cache Storage, 再注销 SW —— 让下一次导航绕开 precache 直接走网络。
 *
 * 顺序必须是**先清缓存再注销**: 注销并不会立刻让当前页面脱控(控制器要等页面卸载才摘),
 * 这次导航仍可能被旧 worker 应答 —— 那时 precache 已空, PrecacheStrategy 的
 * `fallbackToNetwork`(默认 true)会去网络取, 拿到的就是新 index.html; 反过来先注销后清缓存,
 * 缓存还在, 仍可能被旧 HTML 糊住。
 *
 * 代价(只在"等满 APPLY_WAIT_MS 仍未接管"时付一次): 正在下的 precache 作废,
 * 刷新后从 0 重下新版本。
 */
async function hardResetSw(): Promise<void> {
  try {
    if (typeof caches !== 'undefined') {
      const names = await caches.keys()
      await Promise.all(
        names
          .filter((n) => n.startsWith('jc-') || n.startsWith('workbox-precache'))
          .map((n) => caches.delete(n))
      )
    }
  } catch {
    /* Cache Storage 不可用(隐私模式/老内核): 只靠注销 */
  }
  try {
    const r = await navigator.serviceWorker.getRegistration?.()
    await r?.unregister?.()
  } catch {
    /* 注销失败: 缓存已清, 这次导航大概率仍能走网络拿到新页面 */
  }
}

/**
 * 列出 precache 缓存里的条目(键 = `<url>?__WB_REVISION__=<hash>`)。
 *
 * 浏览器**不对外暴露** install 的字节进度, 但 workbox 每下完一个文件就 `cache.put` 一条,
 * 于是"新出现的键数量"就是可用的真实进度信号 —— 零额外流量, 也不依赖清单解析。
 * 用 `workbox-precache*` 前缀找缓存名, 不比拼接 scope(注册 scope 可能不是 `/`)。
 */
async function listPrecacheKeys(): Promise<string[]> {
  if (typeof caches === 'undefined') return []
  try {
    const names = await caches.keys()
    const name = names.find((n) => n.startsWith('workbox-precache'))
    if (!name) return []
    const cache = await caches.open(name)
    return (await cache.keys()).map((r) => r.url)
  } catch {
    return []
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
 *  - 此刻还没有 controller: 页面就是刚拿的, 不算更新
 *    (注意传的是**此刻**的控制状态, 不是"进页面时" —— 首访被 clientsClaim 接管后即为 true)
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
  /** 超时后的硬重置(拆 SW + 清本应用缓存) —— 仅测试注入, 生产走 hardResetSw */
  hardReset?: () => Promise<void>
  /** 列 precache 条目(进度采样) —— 仅测试注入, 生产走 listPrecacheKeys */
  listPrecacheKeys?: () => Promise<string[]>
  /** 进度采样间隔(ms) —— 仅测试注入 */
  progressPollMs?: number
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
  const hardReset = deps.hardReset ?? hardResetSw
  const doReload = deps.reload ?? ((): void => window.location.reload())
  const listKeys = deps.listPrecacheKeys ?? listPrecacheKeys
  const progressPollMs = deps.progressPollMs ?? PROGRESS_POLL_MS

  /** 新版本落地进度 0–1(驱动标题旁图标的填充动效) */
  const progress = ref(0)
  let progTimer: number | null = null
  /** 开始采样时的 precache 条目快照 + 条目总数(两次构建的文件数基本一致) */
  let progBaseline: Set<string> | null = null
  let progExpected = 0
  let progStartAt = 0
  let progSampling = false

  // 进入页面时是否已被 SW 控制 —— 决定 controllerchange 是"初次接管"还是"换新版本"
  const hadControllerAtLoad = supported && !!navigator.serviceWorker.controller
  /**
   * **当前**是否已被 SW 控制(会随初次接管翻成 true)。
   *
   * 为什么不能钉死在"进页面时有没有 controller"(2026-11-10 修):
   *  首访时它恒为 false, 而 checkForUpdate 第一句就是 `if (!controlled) return`。
   *  但首访的注册走的是 `load` 之后 → install → activate → clientsClaim, 页面随即被接管,
   *  此后本页就是"旧内容 + 新控制器"的正常状态, 与回访没有区别。
   *  钉死常量 ⇒ 首访这一整条会话都收不到任何更新提示(TV 壳常驻可达数天), 只能等下次访问自愈。
   */
  let controlled = hadControllerAtLoad

  let controllerListener: (() => void) | null = null
  let updateFoundListener: (() => void) | null = null
  let visibleListener: (() => void) | null = null
  let reg: ServiceWorkerRegistration | null = null
  /** 已经为哪个构建号主动踢过下载 —— 同一构建号不重复抓 sw.js(核对可能每分钟触发) */
  let kickedKey = ''
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
   * 进度采样 —— 让图标能"按进度填满", 而不是空转。
   *
   * 依据: workbox 每下完一个文件就 `cache.put` 进 precache(键带 `__WB_REVISION__`),
   * 所以"相对开始时的快照新出现的键数 / 快照里的键数"就是真实落地比例 —— 两次构建的
   * 文件数基本一致, 用来驱动一个小图标够了, 且比按时间假装更不容易"卡在 90% 不动"。
   *
   * 兜底: 拿不到缓存(首访/隐私模式/列表为空) 时按耗时缓动到 0.9, 接管瞬间补满。
   * 上限 0.95 直到真正接管 —— 别在"还在装"的时候显示 100%。
   */
  async function sampleProgress(): Promise<void> {
    if (!progBaseline) return
    let keys: string[] = []
    try {
      keys = await listKeys()
    } catch {
      keys = []
    }
    let next: number
    if (progExpected >= 10 && keys.length > 0) {
      const fresh = keys.filter((k) => !progBaseline!.has(k)).length
      next = Math.min(0.95, fresh / progExpected)
    } else {
      next = Math.min(0.9, (Date.now() - progStartAt) / PROGRESS_ESTIMATE_MS)
    }
    if (next > progress.value) progress.value = next
  }

  function stopProgressSampling(): void {
    if (progTimer !== null) {
      window.clearInterval(progTimer)
      progTimer = null
    }
    progSampling = false
  }

  /** 开始采样(幂等) —— 从"发现更新"那一刻起算, 因为下载也在那一刻开始 */
  function startProgressSampling(): void {
    if (progSampling) return
    progSampling = true
    progStartAt = Date.now()
    if (progress.value < 0.02) progress.value = 0.02 // 亮起就有可见的一小格, 不是"没反应"
    void (async () => {
      try {
        progBaseline = new Set(await listKeys())
      } catch {
        progBaseline = new Set()
      }
      progExpected = progBaseline.size
      await sampleProgress()
    })()
    progTimer = window.setInterval(() => void sampleProgress(), progressPollMs)
  }

  /**
   * 主动让 SW 去检查并开始安装 —— 让"发现更新"名副其实。
   *
   * 页面侧的例行核对只抓 index.html 比构建号, 从不触发下载; 真正开始装 precache 的是
   * 浏览器自己的检查(导航 / 约 24h)或用户点击。不补这一下, 琥珀态可能亮很久而一个字节
   * 都没下, 用户点了还得从头等整包。
   *
   * 按构建号去重: 核对每次回到前台都可能跑, 同一个版本只踢一次(失败也不重踢 —— 点徽标
   * 时 applyUpdate 会再 update 一次, 别把丢包链路刷成 sw.js 轮询)。
   */
  function kickDownload(key: string): void {
    if (!key || kickedKey === key) return
    kickedKey = key
    void (async () => {
      try {
        const r = reg ?? (await navigator.serviceWorker.getRegistration?.())
        if (!r) return
        reg = r
        await r.update()
      } catch {
        /* 丢包链路上 update 抛错是常态: 不重试, 等用户点击那条路再试 */
      }
    })()
  }

  /**
   * 统一的"是否要给用户提个醒"判据 —— 所有入口信号都收敛到这里。
   * 顺序很关键: 先判"页面已最新"(收掉可能的误亮) → 再判"探测可信度" → 最后去重。
   */
  async function checkForUpdate(signal: UpdateSignal): Promise<void> {
    if (!controlled) return
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
        if (!isPageStale({ hadController: controlled, ownBuild, latestBuild: latest })) {
          // 页面内容已是新版(典型: 强制刷新后旧 SW 才被替换) —— 收掉徽标, 不给误导
          if (updateFound.value) {
            updateFound.value = false
            updateReady.value = false
          }
          stopProgressSampling()
          progress.value = 0
          track('update-skip-current')
          return
        }
      } else if (signal !== 'updatefound') {
        // 探测失败: 只有"sw.js 确实变了"这条硬证据才值得提示。
        // 单纯的 controllerchange 在丢包链路上遇到探测失败是常态, 提示就是误报。
        track('update-skip-probe-failed')
        return
      }

      // 去重: 同一构建号每设备只提示一次 —— 点过更新、刷新/重启后仍是旧版时不再重复弹。
      // 注意判据分两层:
      //  - 跨页面实例(刷新/重新挂载/新标签/TV 冷启动): 看 localStorage, 已提示过就完全静默;
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
      if (updateReady.value) {
        // 接管 = 整包已落地: 补齐进度环并停采样
        stopProgressSampling()
        progress.value = 1
      } else {
        // 徽标亮起 ⇒ 让下载**真的开始**。否则"发现更新"可能只是比出了构建号差异, 而
        // precache 一个字节都没下(页面侧的核对不触发 SW 下载), 用户点了还得从头等。
        startProgressSampling()
        kickDownload(key)
      }
      track(updateReady.value ? 'update-ready' : 'update-found')
    } finally {
      checking = false
      if (pendingControllerChange) {
        pendingControllerChange = false
        void checkForUpdate('controllerchange')
      }
    }
  }

  /**
   * 等新 SW 接管(controllerchange) —— 超时也 resolve, 保证用户总能刷新。
   * @returns true = 真的收到 controllerchange(新 SW 已接管, 刷新必得新版);
   *          false = 等到超时(install 还没完 —— 此时刷新拿到的仍是旧页面)
   */
  function waitForControllerChange(timeoutMs: number): Promise<boolean> {
    return new Promise((resolve) => {
      let done = false
      const settle = (tookOver: boolean): void => {
        if (done) return
        done = true
        try {
          navigator.serviceWorker.removeEventListener('controllerchange', onControllerChange)
        } catch {
          /* 忽略 */
        }
        resolve(tookOver)
      }
      const onControllerChange = (): void => settle(true)
      try {
        navigator.serviceWorker.addEventListener('controllerchange', onControllerChange)
      } catch {
        resolve(false)
        return
      }
      window.setTimeout(() => settle(false), timeoutMs)
    })
  }

  /**
   * 拿到注册对象后挂 `updatefound`(幂等)。
   *
   * 两条来路: ① mount 时已存在的注册(`getRegistration`, 回访); ② 首访 / 被硬重置之后
   * 注册完成激活(`ready`)。第 ② 条是 2026-11-10 补的 —— 原来只在 mount 取一次注册,
   * 首访那次注册完成后 `reg` 仍为 null ⇒ 监听挂不上、`kickDownload` 空转。
   *
   * 换注册对象时先摘旧监听, 免得同一个核对被触发两次。
   */
  function attachUpdateFound(r: ServiceWorkerRegistration): void {
    if (reg === r && updateFoundListener) return
    if (reg && updateFoundListener) {
      try {
        reg.removeEventListener('updatefound', updateFoundListener)
      } catch {
        /* 忽略 */
      }
    }
    reg = r
    updateFoundListener = () => void checkForUpdate('updatefound')
    try {
      r.addEventListener('updatefound', updateFoundListener)
    } catch {
      /* 老内核可能没有 addEventListener: 靠 controllerchange / visibilitychange 兜住 */
    }
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
      // 首访的**第一次** controllerchange 是 clientsClaim 的"初次接管" —— 页面内容是刚下
      // 载的, 谈不上"落后", 提示就是误报(见 isPageStale 的 hadController 判据)。但这次
      // 接管之后页面就处于正常的"被控制"状态了 ⇒ 置 true, 让后续信号具备判定资格。
      if (!controlled) {
        controlled = true
        return
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
          if (r) {
            attachUpdateFound(r)
            return
          }
          // 首访 / 被硬重置之后: 还没有注册 ⇒ 等它装好激活再挂。
          // `ready` 只在**存在 active worker** 时 resolve, 正好是"注册脚本(load 后) →
          // install → activate"这条链的终点; 不挂这一步的话, 首访那次注册完成后 reg 仍为
          // null, updatefound 永远挂不上、kickDownload 也一直空转。
          // ⚠️ ready 不会 reject: 注册被拒时它会一直 pending —— 只是不做事, 不泄漏。
          void navigator.serviceWorker.ready
            ?.then?.((rr) => {
              if (rr) attachUpdateFound(rr)
            })
            ?.catch?.(() => {})
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
    stopProgressSampling()
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
   *  - 仍在 install ⇒ 先让 SW update 一次, 再**等它真正接管**才刷新(最多 applyWaitMs=40s)。
   *    旧实现在这里"清缓存 + 立刻 reload", 而旧 SW 仍控制页面 ⇒ reload 被它应答
   *    ⇒ 拿回旧 index.html ⇒ 页面没换 ⇒ 稍后又亮一次("刷新了也一样")。
   *  - 等满 40s 仍未接管 ⇒ 走硬重置(拆 SW + 清本应用缓存)再刷。这是唯一能保证
   *    "点了就拿到新页面"的手段(旧 SW 会用 precache 应答导航), 代价是正在下的 precache
   *    作废重下 —— 所以只压在这个最坏分支, 常态(装好即刷)一分钱不花。
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
      await waitForControllerChange(applyWaitMs).then(async (tookOver) => {
        if (tookOver) {
          doReload()
          return
        }
        // 等满超时仍未接管 ⇒ 硬重置: 清 precache + 注销 SW, 让这次导航绕开旧 worker
        // 直接走网络拿新 index.html(详见 hardResetSw 的注释)。
        track('update-apply-hard-reset')
        await hardReset()
        // 万一硬重置没生效(注销被拦/缓存清不掉), 刷新后仍落后时允许再提示一次 ——
        // 上一次提示没兑现成新版本, 不该把入口永久锁死。
        clearPromptedBuild()
        promptedInThisInstance = false
        doReload()
      })
    })()
  }

  return { updateReady, updateFound, applying, progress, applyUpdate, supported }
}

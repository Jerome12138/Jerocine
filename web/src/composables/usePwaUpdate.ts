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
 * ⚠️ 下面 2026-11-09 的三条补修里提到的"去重记录""等 40s / 超时硬重置"都已在
 *   2026-11-10 移除(见本段之后的「交互重做」)。原文保留只为追溯当时的判据与踩坑,
 *   不要再照着实现。
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
 *
 * 2026-11-10 交互重做(用户报障: "点一下就消失了, SW 还没加载完, 不知道到底好没好")。
 *  根因不是状态机错, 而是**允许在"还没装完"的时候点**:
 *  旧的琥珀态是可点的, 点击后进入 applying → 等 40s → 装不完就硬重置清缓存重下 → 刷新。
 *  用户视角是"进度条走到 40%, 突然整页刷新, 徽标没了, 到底更没更不知道"。
 *  ⇒ 语义收紧成一条铁律: **只有"刷新一定拿得到新版"才可点**。
 *     - 新 worker 已经接管(controllerchange) ⇒ 可点;
 *     - 控制器本来就是最新、只是本页旧了(`reg.update()` 后没有任何 installing/waiting)
 *       ⇒ 可点;
 *     - 其余(正在装 / 判断不出来) ⇒ 琥珀态, **不可点**, 悬停给出百分比。
 *  为什么"没装完就刷新"根本没用(用户的原话"怎么刷都是旧页面"是对的):
 *    workbox 配了 `navigateFallback: '/index.html'`, 导航请求由**当前控制器**的 precache
 *    应答 —— 旧 worker 控制着页面时, 刷新拿回的必然是它缓存里的旧 index.html。
 *    所以"下载中刷新"只是在骗自己, 还会白扔正在下的 precache。
 *  ⇒ 顺带把"刷新后看不到进度"补上: 刷新后新页面 mount 时, 若 `reg.installing` 还在,
 *    立刻接着显示进度(按 `assets/<最新构建号>/` 目录的落地条目数算, 不依赖采样起点),
 *    于是"装到一半刷新, 进度接着走"成立。
 *  ⇒ `APPLY_WAIT_MS` / `waitForControllerChange` / `hardResetSw` 一并删除: 它们兜的是
 *    "下载中点刷新"那条路, 那条路现在从 UI 上就不存在了。
 *  ⇒ "同一构建号只提示一次"的去重(localStorage `jc-pwa-prompted-build`)同时取消。
 *    它要压的"一个版本提示好几遍", 两个根因(updatefound 无条件亮灯、刷新后仍是旧页)
 *    在前两轮就修掉了; 留着它反而制造本次这条链 —— "刷新后仍落后, 却因为提示过而
 *    再也不给入口"。现在显示与否完全由真实状态决定: 页面确实旧就亮, 亮灯时要么能点、
 *    要么在装; 真的升级成功, 构建号相同自然就熄了 —— 闭环靠构建号, 不靠记忆。
 */

export interface PwaUpdate {
  /** 新版本已就绪 —— **此刻刷新一定拿得到新版**, 徽标青色满格且可点 */
  updateReady: Ref<boolean>
  /** 检测到新版本、正在下载 —— 徽标琥珀态, **不可点**(点了也刷不出新版) */
  updateFound: Ref<boolean>
  /** 用户已点刷新, 正在切换 —— 徽标显示"更新中"并禁止重复点击 */
  applying: Ref<boolean>
  /** 新版本落地进度 0–1(按已写入 precache 的文件数估算, 见 startProgressSampling); 就绪为 1 */
  progress: Ref<number>
  /** 立即刷新到新版本(仅在 updateReady 时生效) */
  applyUpdate: () => void
  /** 当前环境是否支持 Service Worker */
  supported: boolean
}

/**
 * 回到前台重查的最小间隔(ms): TV 壳常驻没有导航机会, 靠 visibilitychange 兜底
 */
const VISIBLE_RECHECK_MS = 60_000
/** 进度采样间隔(ms) —— 只数 Cache Storage 条目数, 开销可忽略 */
const PROGRESS_POLL_MS = 800
/** 拿不到基线时按耗时缓动到的"预估整包时长"(ms): 跨境 3MB 约 30–40s */
const PROGRESS_ESTIMATE_MS = 30_000
/** 进度分母的兜底下限 —— 只在"首次安装、缓存里什么都还没有"时防除零 */
const PROGRESS_MIN_ENTRIES = 10

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
 * 探一次 sw.js, 判断"新版本到底在不在装" —— 决定徽标可不可点。
 *
 * 页面侧能看到的只有"本页构建号 vs 服务器最新构建号", **看不到控制器是不是最新的**,
 * 而这两者对用户是完全不同的处境:
 *  - 控制器已是最新 ⇒ 只是本页旧了, 刷新必得新版 ⇒ **可点**;
 *  - 控制器还是旧的 ⇒ 刷新会被它用 precache 应答(navigateFallback: /index.html),
 *    拿回的还是旧页面 ⇒ **不能点**, 得等新 worker 装完接管。
 * `reg.update()` 正好能分开这两者: 它拉 sw.js 逐字节比对, 有差异才会创建 installing。
 * 所以"update() 回来时没有任何 installing/waiting" = 控制器即服务器上的版本。
 *
 * @returns 'installing' 有新 worker 在装(不可点); 'latest' 控制器已最新(可点刷新);
 *          'unknown' 拿不到注册或 update 失败(丢包链路常态) —— 调用方按"还在装"处理,
 *          宁可暂不给入口, 也不要在装不完时让用户点了白刷。
 */
async function probeSwState(
  getReg: () => Promise<ServiceWorkerRegistration | null>
): Promise<'installing' | 'latest' | 'unknown'> {
  try {
    const r = await getReg()
    if (!r) return 'unknown'
    if (r.installing || r.waiting) return 'installing'
    await r.update()
    // update() 只保证"检查完成"; 真有差异时 installing 在它返回前就已挂上
    if (r.installing || r.waiting) return 'installing'
    return 'latest'
  } catch {
    return 'unknown'
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
  /** 用户已点刷新, 正在切换 —— 徽标转"更新中", 禁止重复点击 */
  const applying = ref(false)
  const ownBuild = deps.ownBuild ?? readOwnBuild()
  const probeLatestBuild = deps.probeLatestBuild ?? fetchLatestBuild
  const doReload = deps.reload ?? ((): void => window.location.reload())
  const listKeys = deps.listPrecacheKeys ?? listPrecacheKeys
  const progressPollMs = deps.progressPollMs ?? PROGRESS_POLL_MS

  /** 新版本落地进度 0–1(驱动标题旁图标的填充动效) */
  const progress = ref(0)
  let progTimer: number | null = null
  /**
   * 进度分母的两种来源:
   *  - `progTag` 已知(探测到了最新构建号) ⇒ 按"已落地的 `assets/<tag>/` 条目数"算,
   *    分母取"缓存里不属于该构建的条目数"(≈ 上一版的清单规模)。这样**刷新后接着算也准**
   *    —— 不依赖"从哪一刻开始采样", 装到一半刷新, 新页面照样能显示 60% 而不是 2%。
   *  - `progTag` 未知(探测失败 / 首访刚亮灯) ⇒ 退回"相对快照新出现的键数", 只是估算。
   */
  let progTag = ''
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
  /** 同一次核对只跑一趟(探测是网络请求, 多个信号并发时会重复发) */
  let checking = false
  /** 核对期间又来了 controllerchange —— 会补跑一次, 否则会漏掉"已接管"这个最强信号 */
  let pendingControllerChange = false
  /**
   * 已经比对确认"本页就是最新"。
   * 只为一个竞态存在: mount 时 `resumeInFlight`(看 `reg.installing` 亮灯) 与
   * `checkForUpdate('visible')`(抓 index.html 比对) 是两条并行的异步链, 若比对先落地
   * 判出"页面已最新"并收灯, 慢一步的 resumeInFlight 不该又把灯点亮。
   */
  let verifiedCurrent = false
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
   * 而资源目录形如 `assets/<部署时间戳>/` —— 于是"缓存里命中该目录的条目数"就是
   * 新版本已落地的文件数, 分母用"缓存里**不属于**该目录的条目数"(≈ 上一版清单规模,
   * 两次部署的文件数基本一致)。这个口径的好处是**刷新后接着算也准**: 装到一半刷新,
   * 新页面 mount 时同样能算出真实百分比, 不必从头数起。
   *
   * 拿不到构建号(探测失败 / 首访亮灯那一刻)时退回"相对开始快照新出现的键数", 只是估算。
   * 兜底: 连缓存都读不到(隐私模式 / 老内核)时按耗时缓动到 0.9。
   * 上限 0.99 —— 满格只留给"真能点了"(见 PwaUpdateBadge 的 fill)。
   */
  async function sampleProgress(): Promise<void> {
    let keys: string[] = []
    try {
      keys = await listKeys()
    } catch {
      keys = []
    }
    let next: number
    if (progTag && keys.length > 0) {
      const placed = keys.filter((k) => k.includes(`/assets/${progTag}/`)).length
      const prevScale = keys.length - placed
      const total = Math.max(prevScale, placed, PROGRESS_MIN_ENTRIES)
      next = Math.min(0.99, placed / total)
    } else if (progBaseline && progExpected >= PROGRESS_MIN_ENTRIES && keys.length > 0) {
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

  /**
   * 开始采样(幂等)。
   * @param tag 最新构建号(可空) —— 已知就按目录精确算; 空则退回快照估算。
   *        采样已在进行中时, 仍然接受补进来的 tag(先亮灯后探到构建号的常见顺序)。
   */
  function startProgressSampling(tag = ''): void {
    if (tag) progTag = tag
    if (progSampling) return
    progSampling = true
    progStartAt = Date.now()
    void (async () => {
      try {
        progBaseline = new Set(await listKeys())
      } catch {
        progBaseline = new Set()
      }
      progExpected = progBaseline.size
      await sampleProgress()
      // 亮起就有可见的一小格, 不是"没反应"(但别盖掉已经算出来的真实进度)
      if (progress.value < 0.02) progress.value = 0.02
    })()
    progTimer = window.setInterval(() => void sampleProgress(), progressPollMs)
  }

  /** 收灯 —— 状态归零, 采样停掉(页面已最新 / 安装失败时用) */
  function hide(): void {
    updateFound.value = false
    updateReady.value = false
    stopProgressSampling()
    progress.value = 0
    progTag = ''
    progBaseline = null
    progExpected = 0
  }

  /** 取注册对象(没有就地取并回填, 顺带把 updatefound 挂上) */
  async function getRegForUpdate(): Promise<ServiceWorkerRegistration | null> {
    if (reg) return reg
    try {
      const r = (await navigator.serviceWorker.getRegistration?.()) ?? null
      if (r) attachUpdateFound(r)
      return r
    } catch {
      return null
    }
  }

  /**
   * 统一的"是否要给用户提个醒"判据 —— 所有入口信号都收敛到这里。
   * 顺序很关键: 先判"页面已最新"(收掉可能的误亮) → 再判"探测可信度" → 最后判"能不能点"。
   *
   * 「能不能点」是 2026-11-10 立的铁律(见文件头): 只有"这次刷新一定拿得到新版"才 ready。
   * 拿不准就报"下载中"(不可点) —— 让用户点一个刷不出新版的按钮, 比暂时不给入口更糟。
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
          verifiedCurrent = true
          if (updateFound.value || updateReady.value) {
            hide()
            track('update-skip-current')
          }
          return
        }
        verifiedCurrent = false
      } else if (signal !== 'updatefound' && !reg?.installing) {
        // 探测失败 + 弱信号 + 也没有在途安装 ⇒ 什么都判断不了, 不提示。
        // (丢包链路上探测失败是常态, 硬提示就是误报; 有 installing 时至少要显示进度)
        track('update-skip-probe-failed')
        return
      }

      // ---- 判定"能不能让用户点" ----
      // ready 的两个可信来源:
      //  ① 新 worker 已接管页面(controllerchange) ⇒ 刷新必得新版;
      //  ② 控制器本来就是最新的, 只是本页旧了(update() 后没有任何 installing/waiting)
      //     ⇒ 刷新命中的是该 worker 的 precache, 里面就是新 index.html。
      // 其余一律"下载中": 此时刷新会被旧 worker 用 precache 应答(navigateFallback),
      // 拿回的还是旧页面 —— 点了只会让人更迷惑。
      let ready = signal === 'controllerchange'
      if (!ready && !reg?.installing) {
        // 已经有 worker 在装就不用再探; 探出来的 'installing' 与 'unknown' 都按"还在装/
        // 判断不出"处理(ready 保持 false, 不可点) —— 只有确证"控制器已是最新"才放行。
        const st = await probeSwState(getRegForUpdate)
        ready = st === 'latest'
      }

      const wasFound = updateFound.value
      const wasReady = updateReady.value
      updateFound.value = true
      updateReady.value = ready
      if (ready) {
        // 就绪 = 整包已落地并可由本页消费: 水位补满, 停采样
        stopProgressSampling()
        progress.value = 1
      } else {
        // 下载中: 按最新构建号精确推进水位 + 盯住 worker 的 statechange(装完立刻补满)
        // 从"就绪"降级下来时水位可能是满的(上一轮判过 ready) —— 必须清掉, 否则会出现
        // "还没装完却显示满格"的矛盾信号(满格 = 可以点, 见 PwaUpdateBadge 的 fill)。
        if (progress.value >= 1) progress.value = 0
        startProgressSampling(latest)
        watchInstalling()
      }
      // 只在"刚亮灯"或"升级到就绪"时埋一条 —— 核对每 60s 都会跑, 无脑 track 会刷屏
      if (!wasFound) track(ready ? 'update-ready' : 'update-found')
      else if (!wasReady && ready) track('update-ready')
    } finally {
      checking = false
      if (pendingControllerChange) {
        pendingControllerChange = false
        void checkForUpdate('controllerchange')
      }
    }
  }

  /**
   * 盯住当前正在安装的 worker, 用它的 `statechange` 判定"整包到底装完没有"。
   *
   * 为什么不能只靠采样: 条目数是估算(新旧清单文件数不同), 而 `installed` 是浏览器的
   * 权威结论 —— 装完了就把水位顶到 0.99(不顶到 1:满格留给"可点", 见 Badge 的 fill)。
   * `redundant` = 安装失败或被替换, 直接收灯, 等下一次 updatefound 再从头来,
   * 免得留一个永远不动的琥珀图标。
   */
  let watchedWorker: ServiceWorker | null = null
  function watchInstalling(): void {
    const w = reg?.installing
    if (!w || watchedWorker === w) return
    watchedWorker = w
    try {
      w.addEventListener('statechange', () => {
        if (w.state === 'installed') {
          progress.value = 0.99
        } else if (w.state === 'redundant') {
          if (!updateReady.value) hide()
        }
      })
    } catch {
      /* 老内核没有 statechange: 靠采样与 controllerchange 兜底 */
    }
  }

  /**
   * 拿到注册对象后挂 `updatefound`(幂等)。
   *
   * 两条来路: ① mount 时已存在的注册(`getRegistration`, 回访); ② 首访 / 被硬重置之后
   * 注册完成激活(`ready`)。第 ② 条是 2026-11-10 补的 —— 原来只在 mount 取一次注册,
   * 首访那次注册完成后 `reg` 仍为 null ⇒ updatefound 监听挂不上、核对也不会被它触发。
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

  /**
   * 刷新后恢复"正在下载"的显示 —— 用户明确要求: 没更新好就保留图标, 刷新也要显示当前进度。
   *
   * 内存态随刷新清零, 但 `reg.installing` 是浏览器侧的真相: 装到一半刷新, 新页面照样能看
   * 到那个 worker 还在装。这里先亮灯接着走(进度按 `assets/<tag>/` 目录落地数算, 见
   * sampleProgress —— 所以刷新后不会从 0 重来), 构建号比对随后会判断要不要收灯
   * (页面本来就已是最新的话, 那颗灯不该亮)。
   */
  function resumeInFlight(r: ServiceWorkerRegistration): void {
    if (!r.installing || verifiedCurrent) return
    updateFound.value = true
    startProgressSampling()
    watchInstalling()
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
            resumeInFlight(r)
            return
          }
          // 首访: 还没有注册 ⇒ 等它装好激活再挂。
          // `ready` 只在**存在 active worker** 时 resolve, 正好是"注册脚本(load 后) →
          // install → activate"这条链的终点; 不挂这一步的话, 首访那次注册完成后 reg 仍为
          // null, updatefound 永远挂不上、它带来的核对信号一条都收不到。
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
   * 点击刷新 —— **只有"新版本已就绪"才允许**(2026-11-10 铁律, 见文件头)。
   *
   * 之所以能这么简单: 就绪的两个来源(新 worker 已接管 / 控制器已是最新)都保证"这次 reload
   * 的导航会拿到新 index.html"。旧实现里那套"等 40s → 硬重置"兜的是"下载还没完用户就点了"
   * 那条路 —— 现在按钮在下载中根本不可点, 那条路从 UI 上不存在了, 兜底也就没必要留。
   */
  function applyUpdate(): void {
    if (typeof window === 'undefined' || applying.value || !updateReady.value) return
    applying.value = true
    track('update-apply')
    doReload()
  }

  return { updateReady, updateFound, applying, progress, applyUpdate, supported }
}

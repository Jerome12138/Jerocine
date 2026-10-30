<script setup lang="ts">
/**
 * 播放页 PlayView
 *
 * 路由：/play?id=&source=&episode=&currentTime=
 *
 * 数据：filmApi.getPlayInfo({ id, playFrom, episode })
 *   响应：{ detail, current, currentPlayFrom, currentEpisode, relate }
 *
 * 本视图职责：
 *  1. 加载并渲染播放器（usePlayer + 原生 <video>）
 *  2. 标题行：影片名 / 当前集 / 标签 / 自动播放开关 / 下一集按钮
 *  3. EpisodeTabs 切换播放源 & 集数（不重建 player，仅 player.src(...)）
 *  4. RelatedList 相关推荐
 *  5. 键盘 / D-pad 快捷键（空格 暂停 / 左右 ±10s / 上下 音量 / Esc 返回）
 *  6. 历史记录写入（卸载 / beforeunload / 切换 episode 都写一次）
 *  7. 错误处理：API 失败 → BaseEmpty + 返回首页；视频源 error → on-page banner + 自动尝试下一个 source
 */

import {
  computed,
  nextTick,
  onMounted,
  onScopeDispose,
  ref,
  watch
} from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { filmApi } from '@/api'
import type { PlayInfo, PlaySource } from '@/types/film'
import { usePlayer, resolutionLabel, isHlsSupported } from '@/composables/usePlayer'
import { setWebWatching } from '@/utils/onlineHeartbeat'
import { usePlayerGestures } from '@/composables/usePlayerGestures'
import { useFilmHistory, buildPlayLink } from '@/composables/useFilmHistory'
import { useSkipSettings } from '@/composables/useSkipSettings'
import type { SkipConfig } from '@/composables/useSkipSettings'
import { useHistoryStore } from '@/stores/history'
import { useSiteStore } from '@/stores/site'
import { toast } from '@/api/http'
import { useViewMode } from '@/composables/useViewMode'
import { measureLine } from '@/composables/usePlaySpeedTest'
import { useNetworkHint } from '@/composables/useNetworkHint'
import { useLocalLikes } from '@/composables/useLocalLikes'
import { useFavoriteStore } from '@/stores/favorite'
import { storeToRefs } from 'pinia'
import { normalizeDpadKey } from '@/utils/dpad'
import EpisodeTabs from '@/components/film/EpisodeTabs.vue'
import RelatedList from '@/components/film/RelatedList.vue'
import DanmakuSettingsPanel from '@/components/film/DanmakuSettingsPanel.vue'
import { useGridRowsLimit } from '@/composables/useGridRowsLimit'
import { useWebSettings } from '@/composables/useWebSettings'
import { useDanmaku, DANMAKU_FONT_STEPS, DANMAKU_AREA_STEPS, DANMAKU_SPEED_STEPS, DANMAKU_LIMIT_STEPS } from '@/composables/useDanmaku'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseDialog from '@/components/base/BaseDialog.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import posterFallback from '@/assets/play.svg'

const route = useRoute()
const router = useRouter()
const { isTV, isDesktop, isMobile } = useViewMode()

/** PC(A方案)按钮"更多"展开态: 一行放不下的低频项收进这里 */
const moreActionsOpen = ref(false)
const historyStore = useHistoryStore()
const siteStore = useSiteStore()
// 弱网感知: 决定 player 初始化参数 + 错误重试策略
const { isSlow: isSlowNetwork } = useNetworkHint()
const favoriteStore = useFavoriteStore()
const { map: favoriteMap } = storeToRefs(favoriteStore)

/* ============ bilibili 三连操作条 ============ */

/** 点赞 — 走 useLocalLikes composable (后端无接口, localStorage 持久化) */
const likes = useLocalLikes()
const liked = computed(() => {
  const id = detail.value?.mid
  return id !== undefined ? likes.isLiked(id).value : false
})
const likeText = computed(() => (liked.value ? '已点赞' : '点赞'))
function toggleLike(): void {
  const id = detail.value?.mid
  if (id === undefined) return
  likes.toggle(id)
}

/** 收藏: 走真后端 (favoriteStore) */
const favorited = computed(() => {
  const id = detail.value?.mid
  return id !== undefined && !!favoriteMap.value[String(id)]
})
function toggleFavorite(): void {
  const d = detail.value
  if (!d) return
  void favoriteStore.toggle({
    id: String(d.mid),
    name: d.name,
    picture: d.cover,
    remarks: d.remarks,
    pid: d.pid,
    cid: d.cid
  })
}

/** 分享: 复制当前 URL 到剪贴板, 短暂展示"已复制"反馈 (优先用后台配置的 domain 拼绝对地址) */
const shareLabel = ref<string>('分享')
async function handleShare(): Promise<void> {
  const origin = siteStore.basic?.domain || (typeof window !== 'undefined' ? window.location.origin : '')
  const url = origin + (typeof window !== 'undefined' ? window.location.pathname + window.location.search : '')
  if (!url) return
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(url)
    } else {
      // fallback: 极老浏览器 / 非 secure context
      const t = document.createElement('textarea')
      t.value = url
      document.body.appendChild(t)
      t.select()
      document.execCommand('copy')
      document.body.removeChild(t)
    }
    shareLabel.value = '已复制'
    window.setTimeout(() => {
      shareLabel.value = '分享'
    }, 1800)
  } catch {
    shareLabel.value = '复制失败'
    window.setTimeout(() => {
      shareLabel.value = '分享'
    }, 1800)
  }
}

/** ---------- 数据状态 ---------- */
const loading = ref(true)
const loadError = ref<string>('')
const detail = ref<PlayInfo['detail'] | null>(null)
const relate = ref<PlayInfo['related']>([])
const { limitToRows } = useGridRowsLimit()
const visibleRelate = computed(() => limitToRows(relate.value))
/** 当前选中的播放源 ID（与 detail.sources[i].id 对应） */
const currentSourceId = ref<string>('')
/** 当前集索引 */
const currentEpisodeIndex = ref<number>(0)
/**
 * 自动连播开关 —— 统一存 useWebSettings(localStorage: jerocine:autoPlayNext),
 * 与 /settings 配置页、安卓设置抽屉(window.__jcConfig)共用同一份数据源。
 */
const { autoPlayNext, setAutoPlayNext } = useWebSettings()
/** 切换自动连播并持久化 */
function toggleAutoPlayNext(): void {
  setAutoPlayNext(!autoPlayNext.value)
}
/** 视频源错误提示文本 */
const videoErrorMsg = ref<string>('')

/* ============ 弹幕（P0）============ */
/** 弹幕开关（用户级）—— 与 /settings 配置页共用 useWebSettings 单一数据源(localStorage jc-danmaku,
    2026-10-26 起默认关闭; 不进 webSettingsSnapshot 安卓契约, 见 useWebSettings 注释)。
    关闭时不调弹幕获取接口。 */
const { danmaku: danmakuEnabled, setDanmaku: setDanmakuEnabled } = useWebSettings()

/** 实际生效 = 用户级开关（2026-10-30 起移动端同步开放：控制条提供弹幕开关/设置入口） */
const danmakuActive = computed(() => danmakuEnabled.value)
function toggleDanmaku(): void {
  setDanmakuEnabled(!danmakuEnabled.value)
  if (danmakuEnabled.value) {
    toast('info', '弹幕已开启')
  } else {
    toast('info', '弹幕已关闭')
  }
}

/** ---------- 派生 ---------- */
const currentSource = computed<PlaySource | null>(() => {
  if (!detail.value) return null
  return (
    detail.value.sources.find((s) => s.id === currentSourceId.value) ??
    detail.value.sources[0] ??
    null
  )
})

const currentEpisode = computed(() => {
  const src = currentSource.value
  if (!src) return null
  return src.episodes[currentEpisodeIndex.value] ?? src.episodes[0] ?? null
})

/** 当前集"原始片源直链"(未过滤); watch(currentSrc) 驱动 resolvePlaySrc 算出可播 playSrc。 */
const currentSrc = ref<string>('')

/**
 * 广告过滤开关 —— 统一存 useWebSettings(localStorage: jc-ad-filter, 默认开)。
 * 开启后, 若当前 src 是 m3u8: 浏览器抓 m3u8 → TS 内核绝对化+剔广告 → blob 清单播放;
 * 非 m3u8 (mp4 / flv 等) 不重写, 走原始 URL。
 * 与 /settings 配置页、安卓设置抽屉共用同一份 useWebSettings 状态。
 */
const { adFilter, setAdFilter } = useWebSettings()
/** 端侧 blob 过滤的前提: 播放器具备 MSE/HLS.js 能力(hls.js 自己抓流)。
 *  iPhone Safari 无 MSE(且无 ManagedMediaSource) → 原生 HLS, blob 清单直接解析失败
 *  → 无 HLS.js 能力设备走原始直链(不过滤)。 */
const hasHlsEngine = isHlsSupported() // 无 MSE 设备(iPhone 原生 HLS)播不了 blob 清单 → 原始直链(不过滤)
function toggleAdFilter(): void {
  setAdFilter(!adFilter.value)
}


/**
 * 广告过滤结果角标 (I-017, 常驻七态; 文案与安卓 AdFilterStatus 逐字对齐):
 *   off         = 过滤开关未开 → 「过滤未开启」
 *   unsupported = 非 m3u8 源(mp4 等无广告清单) → 「该源无需过滤」
 *   proxy       = 走服务端代理(云端取源/全量中转/端侧失败降级), 数量未知 → 「服务端过滤中」
 *   filtering   = 端侧过滤在途(抓清单+POST 未回, 结果未出) → 「广告过滤中」(2026-10-19 起,
 *                 此前该窗口角标整体隐藏, 不反映"正在过滤"的真实状态)
 *   clean       = 端侧过滤跑完但未检出 → 「未检出广告」
 *   filtered    = 端侧过滤检出并剔除 N 段 → 「已过滤 N 段广告」
 *   failed      = 端侧过滤链路整体失败(无结果), 已回退原始直链
 *                 (未过滤, 可能有广告) → 「过滤失败」(2026-10-19 起; 此前该兜底误复用
 *                 unsupported「该源无需过滤」, 与真实语义相反)
 *   nosupport   = 无 MSE 设备(iPhone Safari 等), 端侧 blob 清单不可播 → 「该源无需过滤」
 * 每次集/源切换由 resolvePlaySrc 重置, 常驻播放器右上角。
 */
interface AdFilterBadge {
  kind: AdFilterBadgeKind
  count: number
}
const adFilterBadge = ref<AdFilterBadge | null>(null)
const adBadgeText = computed(() => {
  const b = adFilterBadge.value
  if (!b) return ''
  return adFilterBadgeText(b.kind, b.count)
})

const reM3u8 = /\.m3u8(\?|#|$)/i

/* ============ Web 端侧混合广告过滤 (端侧抓流 → blob 清单播放) ============
 *
 * 2026-10-28 服务端 /v1/m3u8/filter/proxy/stream/stats 全线下线, 此函数**纯本地实现**:
 *   a. 浏览器 fetch(原始 m3u8) —— 浏览器没被地域封, 能拿到
 *   b. 本地 TS 内核 rewriteM3u8 剔广告 + 绝对化 (同 Kotlin PlayerAdFilterHelper + Go golden 夹具)
 *   c. 过滤后文本 → Blob → URL.createObjectURL → blob: URL 喂播放器 (绝对清单可直接播)
 *   d. 降级: fetch 失败 → 直接用原始直链(不过滤, 保证能播)。
 * 无服务端回退路径。
 *
 * effectiveSrc 由同步 computed 改为【异步求出】写入 playSrc ref(下方 resolvePlaySrc),
 * 由 watch(currentSrc + adFilter) 驱动, 用 token 防竞态(切集快时旧异步结果不回写)。
 */

/** 实际喂给 player 的可播 src(异步求出); usePlayer 监听此 ref 切换。 */
const playSrc = ref<string>('')
/** 片源已就绪标志(过滤落地/直链 commit 成功置位; 切集/切源/重算复位)。
 *  就绪后到播放器 canplay 之间是分片下载+起播缓冲窗口(国际链路可达数十秒),
 *  遮罩文案据此降档为"正在缓冲视频"(2026-10-25 用户报文案与阶段错位后新增)。 */
const playSrcReady = ref(false)
/** playSrc 的 mime(blob 清单同样是 hls, 仍报 application/x-mpegURL)。 */
const playSrcType = ref<string>('')
/** 当前持有的 blob: URL, 切集/卸载/重算前需 revoke 释放, 防内存泄漏。 */
let currentBlobUrl = ''
/** resolvePlaySrc 竞态令牌: 仅最新一次的异步结果允许回写 playSrc。 */
let playSrcToken = 0
/**
 * 端侧过滤在途请求控制器: 切集/切源时立即 abort 上一集仍在跑的
 * "抓 m3u8 + POST 过滤"链路(含 500ms 重试等待), 不让旧请求占着带宽拖慢新集。
 * token 负责"丢弃旧结果", abort 负责"终止旧请求" —— 两道都要。
 */
let srcAbortCtl: AbortController | null = null
/** 切集 seek + 续播参数: applyCurrentEpisodeToPlayer 写入, 供 armSeekOnce 读取(新源就绪时)。 */
let pendingResumeAt = 0
let pendingAutoPlay = false
/**
 * 切集互斥标志: selectEpisode 落地时置位, 直到新片源就绪(armSeekOnce 生效 / canplay)才解除。
 * 修复"连跳 2 集": 切集窗口(旧视频暂停→新源提交前)内, 旧视频残余的 timeupdate / ended
 * 事件会以"新集"的 key 再次通过去抖并触发 playNext → 再跳一集; 且第二次跳集会把旧播放头
 * 写进中间那集的独立历史(进度串集的根源)。互斥标志把整个窗口内的自动切集请求全部忽略。
 */
let switchInFlight = false
/** armSeekOnce 令牌: 每次切集递增, 保证只有最新一次 arming 的 seek+续播生效。 */
let seekArmToken = 0
/** 是否正在等待新片源: playSrc 实际变化(watch(playSrc))后清掉并触发一次 armSeekOnce。 */
let awaitingNewSource = false

/**
 * 自动连播冷却: 新片源就绪(切集落地/首次开播)后的 10 秒内, 忽略一切自动切集请求。
 * switchInFlight 只在 loadedmetadata/canplay 就绪事件时解除, 解除后个别源仍可能有
 * 残余 ended/timeupdate 迟到触发, 这 10 秒是就绪后的第二道防线。手动切集不受影响。
 */
const AUTO_NEXT_COOLDOWN_MS = 10000
let autoNextGuardUntil = 0

/**
 * 新片源就绪后 seek 到本集续播点 + 按需自动续播。
 * 由 watch(playSrc) 在每次切集(playSrc 实际变化)时调用一次; token 去重保证仅首个
 * loadedmetadata/canplay 生效, 且监听挂在"新源加载前", 故即便播放内核复用 MSE 不重发事件
 * 也能可靠 seek/续播, 不会卡在暂停态 0(复用播放器切 HLS 源的常见坑)。
 */
function armSeekOnce(): void {
  const myToken = ++seekArmToken
  const resumeAt = pendingResumeAt
  const autoPlay = pendingAutoPlay
  const skip = currentSkipConfig()
  const isIntroSkip = resumeAt <= 0 && skip.intro > 0
  let fired = false
  const offL = onPlayerEvent('loadedmetadata', run)
  const offC = onPlayerEvent('canplay', run)
  // video:playing 是"真正开始播放"的唯一可靠信号(canplay 在移动端 blob URL 上偶发不触发,
  // video:waiting 会在播放中反复触发故不能用作一次性就绪信号)。三选一先到的即触发 run。
  const offP = onPlayerEvent('playing', run)
  function run(): void {
    if (fired || myToken !== seekArmToken) return
    fired = true
    offL()
    offC()
    offP()
    switchInFlight = false // 新片源已就绪, 解除切集互斥
    autoNextGuardUntil = Date.now() + AUTO_NEXT_COOLDOWN_MS // 就绪后 10s 自动连播冷却
    // 就绪信号(loadedmetadata/canplay/playing 任一触发): 立即清除切集时挂起的 loading 态,
    // 露出新集画面。放在 seek/play 之前, 即便 player 此刻为空也不会让 loading 卡住。
    loading.value = false
    const p = player.value
    if (!p) return
    try {
      const duration = p.duration || 0
      if (import.meta.env.DEV) {
        console.debug('[PlayView][切集seek]', { resumeAt, duration, autoPlay })
      }
      // duration 未就绪 / 短视频(<5min) / 跳过位置已超末尾, 都不 seek
      if (duration && duration > 300 && resumeAt >= 0 && resumeAt < duration) {
        p.currentTime = resumeAt
        if (isIntroSkip) toast('info', `已跳过片头 ${skip.intro}s`)
      }
    } catch {
      // ignore
    }
    if (autoPlay) {
      try {
        void playerPlay()
      } catch {
        // ignore
      }
    }
  }
}

function revokeCurrentBlob(): void {
  if (currentBlobUrl) {
    try {
      URL.revokeObjectURL(currentBlobUrl)
    } catch {
      /* ignore */
    }
    currentBlobUrl = ''
  }
}

/**
 * 作废当前在途的播放源解析: 令牌 +1(旧结果一律不回写)并 abort 在途请求。
 * 切集/切源、离开播放页时调用 —— 上一集的过滤链路立刻停, 不再"在原状态等一段时间"。
 */
function cancelPendingResolve(): void {
  playSrcToken++
  srcAbortCtl?.abort()
  srcAbortCtl = null
}

/** 可中断的延时(过滤失败重试的 500ms 等待): abort 时立即 reject, 不白等。 */
function abortableDelay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('aborted', 'AbortError'))
      return
    }
    const t = window.setTimeout(() => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    function onAbort(): void {
      window.clearTimeout(t)
      reject(new DOMException('aborted', 'AbortError'))
    }
    signal.addEventListener('abort', onAbort, { once: true })
  })
}

/**
 * GET 原始清单文本。
 * signal: 切集/切源时 abort。
 */
async function fetchPlaylist(url: string, signal: AbortSignal): Promise<string> {
  const raw = await fetch(url, { cache: 'no-store', signal })
  if (!raw.ok) throw new Error(`fetch m3u8 ${raw.status}`)
  const text = await raw.text()
  if (!text) throw new Error('empty m3u8')
  return text
}

/**
 * 端侧本地过滤主链路: 逐层抓清单, **只有媒体列表(含分片, #EXTINF 判据)才本地过滤**。
 * master/媒体的判据用内容(#EXTINF, utils.hasMediaSegments, 与安卓 PlayerUrls 同款)而非
 * URL 扩展名: 变体地址不带 .m3u8 后缀的源按扩展名会被误判成媒体表, master 层被当媒体层
 * 过滤, 带广告的子表被播放器直连绕过(2026-10-19 修正)。
 * master 主表没有任何分片可剔(过滤的全部产出只是 URI 绝对化, 浏览器本来就会做)
 * → 免过滤直接下钻子表(2026-10-19 加速)。
 * bf 等源对不同 IP 可能返回 master 主表(分片在子表里), 故**跟随 master→子表逐层下钻**
 * (单清晰度 VOD 通常 1 层), 否则只过滤主表、带广告的子表会被播放器直连绕过。
 * 成功返回 { text: 最终(已过滤、已绝对化)清单文本, filtered: 剔除数 };
 * 抓取失败抛错(由 resolvePlaySrc 接住走直链)。服务端过滤已下线(2026-10-28), 无服务端回退。
 */
async function filterPlaylistChain(
  originalUrl: string,
  signal: AbortSignal
): Promise<{ text: string; filtered: number }> {
  let cur = originalUrl
  let lastRaw = ''
  for (let depth = 0; depth < 3; depth++) {
    lastRaw = await fetchPlaylist(cur, signal)
    const child = firstChildPlaylistUrl(lastRaw)
    if (child) {
      // master: 子表地址端侧自行绝对化(以前靠服务端过滤输出顺手做)
      cur = new URL(child, cur).href
      continue
    }
    // 媒体列表: 本地 TS 内核过滤(绝对化+三维判定一次完成)
    const r = rewriteM3u8(lastRaw, cur, { filterAds: true })
    return { text: r.text, filtered: r.filtered }
  }
  // 异常兜底: 3 层全是 master(病态源) → 最后一层也本地过滤
  const r = rewriteM3u8(lastRaw, cur, { filterAds: true })
  return { text: r.text, filtered: r.filtered }
}

/* ============ 端侧取源(client 模式)的下一集过滤结果缓存 ============
 * 片尾窗口把下一集"抓源+过滤"整个提前跑掉, 过滤文本存这里(key=原始清单 URL);
 * 自动切集时 resolvePlaySrc 缓存命中 → 直接 blob, 整个过滤窗口归零。
 * 与安卓 PlayerPrefetchHelper 同款加速器; TTL 10min 与服务端过滤缓存对齐
 * (m3u8 防盗链 token 通常远长于此)。离开播放页清空。 */
const FILTERED_TEXT_TTL_MS = 10 * 60_000
const filteredTextCache = new Map<string, { text: string; filtered: number; at: number }>()

function rememberFilteredText(url: string, text: string, filtered: number): void {
  filteredTextCache.set(url, { text, filtered, at: Date.now() })
}

function takeCachedFilteredText(url: string): { text: string; filtered: number } | null {
  const hit = filteredTextCache.get(url)
  if (!hit) return null
  if (Date.now() - hit.at > FILTERED_TEXT_TTL_MS) {
    filteredTextCache.delete(url)
    return null
  }
  return hit
}

function clearFilteredTextCache(): void {
  filteredTextCache.clear()
}

/** 由(已过滤、已绝对化的)清单文本创建可播 blob: URL —— resolvePlaySrc 的 client 路径终点。 */
function createFilteredBlob(text: string): string {
  return URL.createObjectURL(new Blob([text], { type: 'application/vnd.apple.mpegurl' }))
}

/**
 * 求出当前可播 src 并写入 playSrc(异步)。竞态由 token 保护: 快速切集时旧结果丢弃。
 *   - 非 m3u8 / adFilter 关 → 原始直链(同步路径)
 *   - adFilter 开 + m3u8 → 端侧混合过滤 blob; 失败降级服务端 proxy; 再失败原始直链
 */
async function resolvePlaySrc(originalUrl: string): Promise<void> {
  const token = ++playSrcToken
  // 上一集仍在跑的过滤链路立即停(abort 在途 fetch + 等待), 不与新集抢带宽
  srcAbortCtl?.abort()
  const ctl = new AbortController()
  srcAbortCtl = ctl
  const signal = ctl.signal
  // 旧 blob 先记下, 等新 src 写入后再 revoke(避免提前 revoke 导致正在播的清单失效)
  const prevBlob = currentBlobUrl
  currentBlobUrl = ''

  const commit = (url: string, type: string, isBlob: boolean): void => {
    if (token !== playSrcToken) {
      // 已被更新的请求取代: 若本次产生了 blob, 立即 revoke 防泄漏
      if (isBlob) {
        try {
          URL.revokeObjectURL(url)
        } catch {
          /* ignore */
        }
      }
      return
    }
    if (isBlob) currentBlobUrl = url
    playSrc.value = url
    playSrcReady.value = true
    playSrcType.value = type
    // 写入新 src 后再释放上一集的 blob
    if (prevBlob && prevBlob !== url) {
      try {
        URL.revokeObjectURL(prevBlob)
      } catch {
        /* ignore */
      }
    }
  }

  // 无 URL 不显示角标; 未开过滤/非 m3u8 源也给出原因说明(I-017 常驻反馈)
  if (!originalUrl) {
    adFilterBadge.value = null
    commit(originalUrl, '', false)
    return
  }
  if (!adFilter.value || !reM3u8.test(originalUrl)) {
    adFilterBadge.value = { kind: adFilter.value ? 'unsupported' : 'off', count: 0 }
    commit(originalUrl, '', false)
    return
  }

  // 无 HLS.js 能力的设备(iPhone Safari 等原生 HLS): blob 清单原生播放器解析不了,
  // 端侧过滤整体不可用, 服务端过滤已下线(2026-10-28) → 直接原始直链(不过滤)。
  // (必须在查 blob 缓存之前 —— 缓存的也是 blob, 原生设备一样播不了)
  if (!hasHlsEngine) {
    adFilterBadge.value = { kind: 'nosupport', count: 0 }
    commit(originalUrl, '', false)
    return
  }

  // adFilter 开 + m3u8: 端侧混合过滤 —— 先查预取缓存(片尾窗口提前过滤好的下一集),
  // 命中则直接 blob, 整个过滤窗口归零; 未命中才进入过滤(角标显「广告过滤中」, 结果落地回填)
  const cached = takeCachedFilteredText(originalUrl)
  if (cached) {
    commit(createFilteredBlob(cached.text), 'application/x-mpegURL', true)
    adFilterBadge.value = { kind: cached.filtered > 0 ? 'filtered' : 'clean', count: cached.filtered }
    return
  }
  adFilterBadge.value = { kind: 'filtering', count: 0 }
  try {
    const { text, filtered } = await filterPlaylistChain(originalUrl, signal)
    rememberFilteredText(originalUrl, text, filtered) // 供"切走再切回"与重试复用
    if (token !== playSrcToken) return
    commit(createFilteredBlob(text), 'application/x-mpegURL', true)
    // I-017: 角标常驻反馈过滤结果
    adFilterBadge.value = { kind: filtered > 0 ? 'filtered' : 'clean', count: filtered }
    return
  } catch {
    if (token !== playSrcToken) return
    // 端侧过滤失败重试一次(网络抖动/CORS 偶发失败, 避免直接降级导致过滤失效);
    // 切集 abort 会在 await/延时处立刻抛 AbortError, 由上方 token 检查丢弃。
    // 重试前再查一次缓存 —— 若预取恰好在失败间隙完成, 直接用预取结果, 不再白跑。
    try {
      await abortableDelay(500, signal)
      if (token !== playSrcToken) return
      const retry = takeCachedFilteredText(originalUrl) ?? (await filterPlaylistChain(originalUrl, signal))
      if (token !== playSrcToken) return
      rememberFilteredText(originalUrl, retry.text, retry.filtered)
      commit(createFilteredBlob(retry.text), 'application/x-mpegURL', true)
      adFilterBadge.value = { kind: retry.filtered > 0 ? 'filtered' : 'clean', count: retry.filtered }
      return
    } catch {
      if (token !== playSrcToken) return
    }
    // 重试也失败(抓源失败/源不可达) → 原始直链不过滤(角标「过滤失败」), 不阻塞播放
    adFilterBadge.value = { kind: 'failed', count: 0 }
    commit(originalUrl, '', false)
  }
}

/* ============ 线路测速 (实测各源"当前集" m3u8 首片加载延时, 选最快线路) ============
 * 测速实现在公共模块 usePlaySpeedTest(与后台采集页「测播放」共用), 这里只管结果展示。 */
const lineSpeeds = ref<Record<string, number>>({}) // ms; -1=失败/无 m3u8
const testingLines = ref(false)

const fastestLineId = computed(() => {
  let id = ''
  let best = Infinity
  for (const k in lineSpeeds.value) {
    const v = lineSpeeds.value[k]
    if (v === undefined || v < 0) continue
    if (v < best) {
      best = v
      id = k
    }
  }
  return id
})

async function testLines(): Promise<void> {
  if (!detail.value || testingLines.value) return
  const sources = detail.value.sources
  if (!sources.length) return
  testingLines.value = true
  const epIdx = currentEpisodeIndex.value
  try {
    // 限并发 2 逐线路测: 全并行时各线路争抢用户带宽, 计时互相拖慢, 排名失真。
    const results = await pMap(
      sources,
      async (s) => {
        const ep = s.episodes[epIdx] ?? s.episodes[0]
        if (!ep) return { id: s.id, ms: -1 }
        return { id: s.id, ms: await measureLine(ep.link) }
      },
      2
    )
    const map: Record<string, number> = {}
    for (const r of results) map[r.id] = r.ms
    lineSpeeds.value = map
    const ok = results.filter((r) => r.ms >= 0).sort((a, b) => a.ms - b.ms)
    if (ok.length) {
      const name = sources.find((s) => s.id === ok[0]!.id)?.name ?? ''
      toast('success', `播放测速完成, 最快线路: ${name} (${ok[0]!.ms}ms)`)
    } else {
      toast('warning', '播放测速完成, 当前集无可用 m3u8 线路')
    }
  } finally {
    testingLines.value = false
  }
}

/** 限并发 map(保序): 测速专用小工具 */
async function pMap<T, R>(items: T[], fn: (item: T) => Promise<R>, concurrency: number): Promise<R[]> {
  const out = new Array<R>(items.length)
  const queue = items.map((item, i) => ({ item, i }))
  async function worker(): Promise<void> {
    while (queue.length) {
      const next = queue.shift()
      if (!next) return
      out[next.i] = await fn(next.item)
    }
  }
  await Promise.all(Array.from({ length: Math.min(concurrency, items.length) }, worker))
  return out
}

/** 一键切到最快线路 */
function switchToFastest(): void {
  if (fastestLineId.value && fastestLineId.value !== currentSourceId.value) {
    changeSource(fastestLineId.value)
  }
}

/**
 * Web 端: currentSrc(原始片源直链) 变化 → 异步求出可播 src 写入 playSrc。
 * 端侧过滤是异步的(await fetch), 这里集中触发, 不在 applyCurrentEpisodeToPlayer 里同步算。
 * Native 路径不设 currentSrc(走 jerocine.playPlaylist), 故此 watch 只影响 web。
 */
watch(currentSrc, (url) => {
  void resolvePlaySrc(url)
})

/**
 * 切集: playSrc 实际变化(新片源已提交)后才 arming 一次 seek+续播。
 * 关键: 监听必须挂在"新源加载前" —— 这里在 playSrc 变化时触发, 而 usePlayer 的
 * watch(playSrc) 在其后才 player.src(new), 故下方 armSeekOnce 注册的 loadedmetadata/canplay
 * 必然能捕获新源就绪事件, 避免播放内核复用 MSE 不重发事件导致卡在暂停态 0。
 * 仅切集(awaitingNewSource)才走此路径; adFilter 切换由自身逻辑处理, 不冲突。
 */
watch(playSrc, () => {
  if (awaitingNewSource) {
    awaitingNewSource = false
    armSeekOnce()
  }
})

/**
 * adFilter 切换:
 *  - APK 端: 重新调 applyCurrentEpisodeToPlayer 让 native ExoPlayer 切换到新 URL(会重启 Activity, 位置丢失, 是 trade-off)。
 *  - Web 端: 抓当前 currentTime → 重算 playSrc(端侧过滤/原始切换) → 新 src 起播后 seek 回原位置, 体感无中断。
 */
watch(adFilter, () => {
  if (getNativePlayer()) {
    applyCurrentEpisodeToPlayer(0)
    return
  }
  // 仅 m3u8 源会因开/关过滤实际切换 playSrc(blob/proxy/直链), 非 m3u8 切换是 no-op, 无需补 seek。
  const willChangeSrc = !!currentSrc.value && reM3u8.test(currentSrc.value)
  const resume = playerReady.value && player.value ? playerCurrentTime.value : 0
  // 重算可播 src; playSrc 变化驱动播放器重载
  void resolvePlaySrc(currentSrc.value)
  if (!willChangeSrc || resume <= 0) return
  // 新 src 起播后 seek 回原位置, 体感无中断
  const off = onPlayerEvent('loadedmetadata', () => {
    const p = player.value
    if (!p) return
    try {
      p.currentTime = resume
      void playerPlay()
    } catch {
      /* ignore */
    }
    off()
  })
})

const hasNext = computed(() => {
  const src = currentSource.value
  if (!src) return false
  return currentEpisodeIndex.value < src.episodes.length - 1
})

const hasPrev = computed(() => currentEpisodeIndex.value > 0)

const tagList = computed<string[]>(() => {
  if (!detail.value) return []
  const d = detail.value
  const tags: string[] = []
  if (d.cName) tags.push(d.cName)
  if (d.classTag) {
    for (const t of String(d.classTag).split(',')) {
      const v = t.trim()
      if (v) tags.push(v)
    }
  }
  if (d.year) tags.push(String(d.year))
  if (d.area) tags.push(String(d.area))
  return tags.slice(0, 6)
})

const filmId = computed(() => String(route.query.id ?? ''))

/** 已观看链接：基于 history store 推算（同 source 中 0..episodeIndex 全部视为已观看） */
const watchedLinks = computed<string[]>(() => {
  if (!detail.value) return []
  const id = String(detail.value.mid)
  const rec = historyStore.get(id)
  if (!rec || !rec.source) return []
  const src = detail.value.sources.find((s) => s.id === rec.source)
  if (!src) return []
  const upto = Math.max(0, rec.episodeIndex ?? 0)
  return src.episodes.slice(0, upto + 1).map((e) => e.link)
})

/** ---------- 播放器 ---------- */
const playerEl = ref<HTMLDivElement | null>(null)
const {
  init: initPlayer,
  dispose: disposePlayer,
  play: playerPlay,
  pause: playerPause,
  seekBy: playerSeekBy,
  setVolume: playerSetVolume,
  load: playerLoad,
  enterTempRate,
  exitTempRate,
  on: onPlayerEvent,
  player,
  paused,
  buffering,
  videoSize,
  currentTime: playerCurrentTime,
  ready: playerReady
} = usePlayer({
  src: playSrc,
  type: playSrcType,
  poster: ref<string | undefined>(posterFallback),
  autoplay: false,
  volume: 0.6,
  playbackRates: [0.5, 1.0, 1.25, 1.5, 2.0],
  // 弱网友好默认: preload metadata 而非 auto, 起播更快; 弱网时启用 hls.js 低带宽预设
  preload: 'metadata',
  lowBandwidth: isSlowNetwork.value,
  // I-018: 控制条内常驻「下一集」图标入口(左置, 始终展示; 移动端无文字仅图标)
  nextEpisode: { onClick: () => playNext() },
  // 控制条弹幕设置按钮: 点击打开设置弹窗(移动端/桌面端均显示)
  danmakuSettings: { onClick: () => (danmakuSettingsOpen.value = true) },
  // 控制条播放设置按钮: 点击打开连播/过滤/弹幕入口面板(移动端整合控制)
  playSettings: { onClick: () => (playSettingsOpen.value = !playSettingsOpen.value) },
  // 移动端: 隐藏画中画/网页全屏按钮(屏幕小、手势已覆盖, 节省控制条空间)
  isMobile: isMobile.value
})

// 在线心跳 watching 信号: 播放器播放状态实时置位(暂停/结束即回落)。
watch(paused, (v) => setWebWatching(!v))

/* ============ 弹幕层（P0, 方案 §四之二/§四之三: 独立层叠在播放器容器上, 不换渲染引擎） ============ */
const danmaku = useDanmaku({
  mid: computed(() => detail.value?.mid),
  // 聚合弹幕按标题匹配: 影片名 + 年份 + 集号
  title: computed(() => detail.value?.name ?? ''),
  year: computed(() => detail.value?.year),
  episode: computed(() => currentEpisodeIndex.value + 1),
  player,
  playing: computed(() => !paused.value),
  // 实际生效 = 用户级; 任一变化触发 useDanmaku watch 重载
  enabled: danmakuActive
})
const danmakuStage = danmaku.stage
const danmakuState = danmaku.danmuState
/** 弹幕显示设置（字号/透明度/区域/速度等, 面板实时更新） */
const danmakuSettings = danmaku.settings
const danmakuUpdateSettings = danmaku.updateSettings
/** 弹幕设置弹窗开关 */
const danmakuSettingsOpen = ref(false)
/** 播放设置面板开关(连播/过滤/弹幕入口) */
const playSettingsOpen = ref(false)

/** 弹幕控制条按钮开启态颜色（is-on class; CSS 见样式段 .art-control-danmaku-*.is-on） */
function syncDanmakuBtnState(): void {
  const host = player.value?.template.$player
  const toggle = host?.querySelector('.art-control-danmaku-toggle')
  if (toggle) toggle.classList.toggle('is-on', danmakuActive.value)
  const settingsBtn = host?.querySelector('.art-control-danmaku-settings')
  if (settingsBtn) settingsBtn.classList.toggle('is-on', danmakuActive.value)
}
watch(danmakuActive, syncDanmakuBtnState, { flush: 'post' })
// 按钮由 usePlayer 在控制条初始化时插入, 首次渲染后可能晚一拍 → 挂载后查两次
onMounted(() => {
  nextTick(() => {
    syncDanmakuBtnState()
    setTimeout(syncDanmakuBtnState, 300)
  })
})

/**
 * 弹幕入口三态（用户要求 2026-10-25: 该片不支持 / 没查到 / 加载失败, 都要有显示, 不许静默）:
 *   可用（ready/loading/idle+开关）→ 正常开关;
 *   不可用（unsupported/empty/failed）→ 按钮置灰但仍显示, 点击给原因; failed 可重试。
 * 不用 disabled 属性: TV 遥控器焦点会跳过禁用按钮, 用户看不到原因 —— aria-disabled + 点击反馈。
 */
const danmakuUnavailable = computed(
  () =>
    danmakuState.value === 'unsupported' ||
    danmakuState.value === 'empty'
)
const danmakuHint = computed(() => {
  switch (danmakuState.value) {
    case 'unsupported':
      return '该片暂无弹幕源'
    case 'empty':
      return '未找到弹幕内容'
    case 'failed':
      return '弹幕加载失败, 点击重试'
    default:
      return ''
  }
})
function onDanmakuEntryClick(): void {
  // failed: 直接重试装载（提示文案即「弹幕加载失败, 点击重试」）
  if (danmakuState.value === 'failed') {
    danmaku.reload()
    return
  }
  // 不可用(unsupported/empty): 给原因, 不静默
  if (danmakuUnavailable.value) {
    toast('info', danmakuHint.value)
    return
  }
  toggleDanmaku()
}

/** ---------- 历史记录 ---------- */
/** #4: 倒数 5 分钟内不记录播放记忆 —— 看完/退出在片尾附近时, 不写进度,
 * 避免下次打开直接 resume 到片尾。短视频(<=5min)不适用, 否则永远不记录。 */
const NO_RECORD_TAIL_SEC = 300

const { flush: flushHistory } = useFilmHistory({
  collect: () => {
    if (!detail.value || !currentEpisode.value) {
      return null
    }
    // #4: 临近片尾(倒数 5 分钟内)不写历史
    try {
      const p = player.value
      const duration = p?.duration ?? 0
      if (duration > NO_RECORD_TAIL_SEC) {
        const remaining = duration - (p?.currentTime ?? 0)
        if (remaining <= NO_RECORD_TAIL_SEC) return null
      }
    } catch {
      /* ignore */
    }
    const link = buildPlayLink({
      id: detail.value.mid,
      source: currentSourceId.value,
      episodeIndex: currentEpisodeIndex.value,
      currentTime: playerCurrentTime.value
    })
    return {
      id: String(detail.value.mid),
      name: detail.value.name,
      link,
      episode: currentEpisode.value.episode,
      picture: detail.value.cover,
      source: currentSourceId.value,
      episodeIndex: currentEpisodeIndex.value,
      currentTime: Math.floor(playerCurrentTime.value),
      pid: detail.value.pid,
      cid: detail.value.cid,
      // 影片自身更新状态(HD / 更新至 N 集): 供首页"继续观看"卡左下角像普通影片卡一样展示
      remarks: detail.value.remarks
    }
  }
})

/** ---------- 数据加载 ---------- */
async function loadPlayInfo(): Promise<void> {
  const id = String(route.query.id ?? '')
  const playFrom = String(route.query.source ?? '')
  const episode = String(route.query.episode ?? '0')

  if (!id) {
    loadError.value = '缺少影片 ID'
    loading.value = false
    return
  }

  loading.value = true
  loadError.value = ''
  videoErrorMsg.value = ''

  try {
    const data: PlayInfo = await filmApi.getPlayInfo({
      mid: id,
      source: playFrom,
      episode: Number(episode) || 0
    })
    if (!data || !data.detail) {
      loadError.value = '播放信息为空'
      loading.value = false
      return
    }
    detail.value = data.detail
    // 页面标题带影片名(路由 meta 固定"播放"): 供在线明细后台直接看到"在看什么"
    document.title = `${data.detail.name} - ${siteStore.basic?.siteName || 'Jerocine'}`
    relate.value = data.related ?? []
    currentSourceId.value = data.currentSource || data.detail.sources[0]?.id || ''
    currentEpisodeIndex.value = Number(data.currentEpisode) || 0
    // 续播进度优先级：URL query.currentTime > history store（严格按 影片::源::集 取该集独立进度）
    let resumeAt = Number(route.query.currentTime) || 0
    if (!resumeAt) {
      const rec = historyStore.getEpisode(id, currentSourceId.value, currentEpisodeIndex.value)
      if (rec && (rec.currentTime ?? 0) > 0) {
        resumeAt = rec.currentTime ?? 0
      }
    }
    applyCurrentEpisodeToPlayer(resumeAt)
    // Web: 保持 loading 直到新源就绪(armSeekOnce→loadedmetadata/canplay)或出错(handleVideoError),
    // 覆盖端侧广告过滤解析窗口 —— 此前这里提前关 loading, 过滤期间(可达数秒)点击视频无任何反馈。
    if (isNative() || getNativePlayer() || !currentEpisode.value) loading.value = false
  } catch {
    // http 拦截器已 toast，这里只设页面态
    loadError.value = '播放信息加载失败'
    loading.value = false
  }
}

import { isNative } from '@/utils/jerocineNative'
import { dispatchNativePlaylist } from '@/utils/nativePlay'
import { telemetry } from '@/utils/telemetry'
import {
  adFilterBadgeText,
  firstChildPlaylistUrl,
  loadingHintText,
  type AdFilterBadgeKind
} from '@/utils/adFilter'
import { rewriteM3u8 } from '@/utils/m3u8Filter'

/**
 * Android TV 壳 (Jerocine APK) 注入的 JS Bridge.
 * 检测到原生时, PlayView 不渲染 web 播放器, 直接交给 ExoPlayer 全屏.
 */
function getNativePlayer(): { playPlaylist?: (cfg: string) => void; playVideo?: (u: string, t: string) => void } | undefined {
  if (typeof window === 'undefined') return undefined
  return (window as unknown as { JerocinePlayer?: { playPlaylist?: (c: string) => void; playVideo?: (u: string, t: string) => void } }).JerocinePlayer
}

/** 跳片头片尾按 filmId 持久化, 没存过用默认 90/60 */
const skipSettings = useSkipSettings()
/** 实际生效的跳过秒数: 总开关关闭 → 0/0(不跳), 配置里的秒数原样保留 */
function currentSkipConfig(): { intro: number; outro: number } {
  const id = detail.value?.mid
  if (id === undefined || id === null) {
    return { intro: skipSettings.DEFAULT_INTRO, outro: skipSettings.DEFAULT_OUTRO }
  }
  return skipSettings.effective(id)
}

/** 防止 timeupdate 每秒多次触发 playNext, 用 sourceId/episodeIndex 联合键去抖 */
const outroTriggeredKey = ref('')
/** 片尾跳过前 10s 预告去抖(每集只提示一次) */
const outroPromptKey = ref('')

/** 跳过设置弹窗 */
const skipDialogOpen = ref(false)
const draftIntro = ref(skipSettings.DEFAULT_INTRO)
const draftOutro = ref(skipSettings.DEFAULT_OUTRO)
/** 跳过总开关(草稿): 关闭后本剧不跳, 秒数保留 */
const draftEnabled = ref(true)
function openSkipDialog(): void {
  const id = detail.value?.mid
  const cfg: SkipConfig =
    id === undefined || id === null
      ? { intro: skipSettings.DEFAULT_INTRO, outro: skipSettings.DEFAULT_OUTRO, enabled: true }
      : skipSettings.get(id)
  draftIntro.value = cfg.intro
  draftOutro.value = cfg.outro
  draftEnabled.value = cfg.enabled
  skipDialogOpen.value = true
}
function saveSkip(): void {
  const id = detail.value?.mid
  if (id === undefined || id === null) return
  const cfg: SkipConfig = {
    intro: Number(draftIntro.value) || 0,
    outro: Number(draftOutro.value) || 0,
    enabled: draftEnabled.value
  }
  skipSettings.save(id, cfg)
  skipDialogOpen.value = false
  // 总开关关闭 → 按 0/0 生效(等于不跳), 秒数已存
  applySkipLive(cfg.enabled ? { intro: cfg.intro, outro: cfg.outro } : { intro: 0, outro: 0 })
}
function resetSkip(): void {
  const id = detail.value?.mid
  if (id === undefined || id === null) return
  skipSettings.reset(id)
  // 恢复默认 = 回到用户级全局默认（未设置过则为内置 90/60）
  const g = skipSettings.getDefault()
  draftIntro.value = g.intro
  draftOutro.value = g.outro
  draftEnabled.value = g.enabled
  skipDialogOpen.value = false
  applySkipLive(g.enabled ? { intro: g.intro, outro: g.outro } : { intro: 0, outro: 0 })
}

/**
 * 改跳过设置后立即按新 intro/outro 生效(web 播放器; native 走重启不在此路径)。
 *  - 片尾: 重置去抖 key, 让新 outro 阈值在下次 timeupdate 立即重新判定(onPlayerTimeUpdateForSkipOutro 每帧读最新配置)。
 *  - 片头: 仅当"还在片头区"(当前进度 < 新 intro 且 < 旧+缓冲, 即用户刚进/刚改没主动拖到正片)才前跳, 避免把正在看正片的用户硬拽走。
 */
function applySkipLive(cfg: { intro: number; outro: number }): void {
  if (getNativePlayer()) return // 原生由 applyCurrentEpisodeToPlayer 重启时下发, 不在此处
  const p = player.value
  if (!playerReady.value || !p) return
  // 片尾: 清去抖, 新阈值立即重新判定
  outroTriggeredKey.value = ''
  outroPromptKey.value = ''
  // 片头: 只在"仍处片头区"且新片头更靠后时前跳
  try {
    const duration = p.duration || 0
    const current = p.currentTime || 0
    const INTRO_LIVE_WINDOW = 180 // 当前进度在前 3 分钟内才视为"还在片头区", 可即时前跳
    if (
      cfg.intro > 0 &&
      duration > 300 &&
      current < cfg.intro &&
      current < INTRO_LIVE_WINDOW &&
      cfg.intro < duration
    ) {
      p.currentTime = cfg.intro
      toast('info', `已跳过片头 ${cfg.intro}s`)
    }
  } catch {
    /* ignore */
  }
}

/**
 * 把当前 episode 的 link 写入播放器 src ref（player composable 会响应式切换）。
 * @param resumeAt  续播秒数(该集自身进度; 0=从头/跳片头)
 * @param autoPlay  新片源就绪后是否自动续播。仅当"切集前在播放"才传 true,
 *                  避免用户已暂停时静默自动播放。初始进入播放页用 false。
 */
function applyCurrentEpisodeToPlayer(resumeAt = 0, autoPlay = false): void {
  const ep = currentEpisode.value
  if (!ep) return
  outroTriggeredKey.value = '' // 切集重置跳片尾去抖
  outroPromptKey.value = '' // 切集重置片尾预告
  // 广告过滤成功提示:
  //  - web: 由 watch(currentSrc)→resolvePlaySrc 端侧过滤拿响应头 X-Ad-Filtered 直接提示(不再查 /m3u8/stats, server 抓不到 bf 源)
  //  - native: 原生播放器自身按集弹"已过滤 N 段", web 层不重复
  const skip = currentSkipConfig()
  // Android TV 壳: 整片所有源 + 集列表交 ExoPlayer (全屏 + 缓存 + 自动续集 + 跳片头片尾 + 多源切换)
  // 接通历史: filmId + filmName 让 native 周期回传 playerProgress, App.vue 写历史
  const src = currentSource.value
  if (isNative() && detail.value && src) {
    // 统一派发(与详情页 gotoPlay / 路由守卫共用 dispatchNativePlaylist: 映射源+建历史+skip 一处)
    dispatchNativePlaylist(detail.value, src.id, currentEpisodeIndex.value, resumeAt)
    return
  }
  // 老 v1 bridge 兼容路径
  const native = getNativePlayer()
  if (native?.playPlaylist && src) {
    const movieName = detail.value?.name ?? ''
    // 一律给**原始地址**: 端侧本地过滤成为唯一路径(服务端过滤/代理/中转均已下线),
    // 原生按 adFilter 开关决定是否在 PlayerAdFilterHelper 走本地内核, web 层无需预包装。
    const episodes = src.episodes.map((e) => ({
      url: e.link,
      title: `${movieName} · ${e.episode ?? ''}`.trim()
    }))
    native.playPlaylist(JSON.stringify({
      episodes,
      startIndex: currentEpisodeIndex.value,
      resumeAtSec: resumeAt,
      skipIntroSec: skip.intro,
      skipOutroSec: skip.outro,
      filmId: String(detail.value?.mid ?? ''),
      filmName: movieName
    }))
    return
  }
  // 兼容旧 bridge (只支持单集)
  if (native?.playVideo) {
    const title = `${detail.value?.name ?? ''} · ${ep.episode ?? ''}`.trim()
    native.playVideo(ep.link, title)
    return
  }
  // 切集/切源立即复位(用户要求 2026-10-18): 先作废上一集在途的过滤/加载(abort 全部
  // 在途 fetch + 作废竞态令牌), 界面立刻切到"新集准备中" —— 不再停留在旧集状态等旧请求跑完。
  cancelPendingResolve()
  prefetchAbort?.abort() // 在途的下一集预取一并取消
  prefetchedEpisodeKey.value = '' // 允许新目标集重新预取
  adFilterBadge.value = null // 清掉上一集的过滤结果角标, 新集过滤完成后回填
  currentSrc.value = ep.link
  // 续播目标: 该集自身进度 > 跳片头; 短视频/无记录=0。记录到 pending*, 供新源就绪后 armSeekOnce 使用。
  const seekTo = resumeAt > 0 ? resumeAt : skip.intro
  pendingResumeAt = seekTo
  pendingAutoPlay = autoPlay
  // 旧片源立即回到初始状态: pause + reset(卸掉旧源、清画面与进度), 新源就绪前只露 loading 层;
  // reset 若在个别实现上抛错, 退回仅 pause(旧画面仍会被 loading 遮罩盖住)。
  const pOld = player.value
  if (pOld && playerReady.value) {
    try {
      pOld.pause()
      pOld.reset()
    } catch {
      try {
        pOld.pause()
      } catch {
        // ignore
      }
    }
  }
  // 标记等待新源: playSrc 实际变化(watch(playSrc))后再 arming 一次, 确保监听挂在新源加载之前,
  // 解决"复用播放器切 HLS 源个别内核不重发 loadedmetadata"导致卡在暂停态 0 的问题。
  awaitingNewSource = true
  playSrcReady.value = false // 新片源未就绪, 遮罩文案回到"正在准备视频"
  // 切集/切源立即进入 loading 态: 文案 loadingHint 按过滤开关/取源方式给出"广告过滤中/正在准备视频"。
  loading.value = true
}

/* ============ 下一集预取(自动连播场景, 消除切集过滤窗口) ============
 * 仅当"广告过滤开 + 自动连播开启 + 有下一集 + 距自动切集点 <= 60s"才后台预取(幂等+静默失败):
 *   - 端侧取源(client, 默认): 把下一集"抓源+过滤"整个提前跑掉, 过滤文本存本地缓存
 *     (filteredTextCache) → 自动切集时 resolvePlaySrc 缓存命中, 过滤窗口归零。
 * 切集时在途预取立即 abort。 */
const PREFETCH_LEAD_S = 60
const prefetchedEpisodeKey = ref('')
let prefetchAbort: AbortController | null = null
async function prefetchNextEpisode(): Promise<void> {
  if (!adFilter.value || !autoPlayNext.value || !hasNext.value) return
  if (!hasHlsEngine) return // 无 HLS.js 能力设备: 无端侧过滤形态, 无可预热
  const nextIdx = currentEpisodeIndex.value + 1
  const key = `${currentSourceId.value}/${nextIdx}`
  if (prefetchedEpisodeKey.value === key) return
  prefetchedEpisodeKey.value = key
  try {
    const data: PlayInfo = await filmApi.getPlayInfo({
      mid: String(route.query.id ?? ''),
      source: currentSourceId.value,
      episode: nextIdx
    })
    const ep = data?.detail?.sources
      ?.find((s) => s.id === currentSourceId.value)
      ?.episodes?.[nextIdx]
    if (!ep || !reM3u8.test(ep.link)) return
    // 挂 signal: 切集/切源时立即取消在途预取。
    prefetchAbort?.abort()
    const ctl = new AbortController()
    prefetchAbort = ctl
    // 端侧取源(需 MSE): 提前跑完整过滤链(master 免过滤), 过滤文本入本地缓存。
    const r = await filterPlaylistChain(ep.link, ctl.signal)
    rememberFilteredText(ep.link, r.text, r.filtered)
  } catch {
    /* 预取失败不影响播放(切集时再付一次全价); abort 走这里被静默吞掉 */
  }
}

/** Web 播放器 timeupdate 监听: 接近片尾自动播下一集 (key 去抖) */
function onPlayerTimeUpdateForSkipOutro(): void {
  const p = player.value
  if (!p) return
  const skip = currentSkipConfig()
  // 自动连播关闭 → 不自动跳片尾(跳过片尾=进下一集, 属连播范畴); 跳片头不受影响
  if (!autoPlayNext.value) return
  if (skip.outro <= 0 || !hasNext.value) return
  const duration = p.duration || 0
  const current = p.currentTime || 0
  if (duration <= 300 || current <= 0) return
  const remaining = duration - current
  // 预取下一集: 距自动切集点 <=60s 时后台预热服务器过滤缓存(幂等, 静默, 失败不影响播放)
  if (remaining <= skip.outro + PREFETCH_LEAD_S) {
    prefetchNextEpisode()
  }
  const key = `${currentSourceId.value}/${currentEpisodeIndex.value}`
  // 跳过片尾前 10s 预告(每集一次)
  if (remaining <= skip.outro + 10 && remaining > skip.outro) {
    if (outroPromptKey.value !== key) {
      outroPromptKey.value = key
      toast('info', '10 秒后自动跳过片尾, 播放下一集')
    }
    return
  }
  if (remaining > skip.outro) return
  if (outroTriggeredKey.value === key) return
  outroTriggeredKey.value = key
  playNextAuto()
}

/** ---------- 触屏手势(web 专用; TV/原生播放器不适用) ---------- */
const isPlayerFullscreen = ref(false)

// 全屏进出：隐藏弹幕层，避免已发射弹幕在尺寸跳变期错位/被裁；尺寸稳定后重载（缓存命中、零请求）以新字号重渲染（item 1）
const danmakuResizing = ref(false)
let danmakuResizeTimer: ReturnType<typeof setTimeout> | null = null
watch(isPlayerFullscreen, () => {
  console.log('[danmaku] fullscreen toggle:', isPlayerFullscreen.value)
  // 全屏只短暂隐藏防错位; 尺寸/自适应字号由 ResizeObserver→engine.resize()+setFontSize 处理,
  // 不再 reload(会 clear 清空弹幕导致"全屏后等好久才重新出现")
  danmakuResizing.value = true
  if (danmakuResizeTimer) clearTimeout(danmakuResizeTimer)
  danmakuResizeTimer = setTimeout(() => {
    danmakuResizing.value = false
  }, 250)
})
onScopeDispose(() => {
  if (danmakuResizeTimer) clearTimeout(danmakuResizeTimer)
})

/** loading 遮罩文案(三档纯函数 loadingHintText, 逐档单测覆盖):
 *  未就绪+过滤在途「正在准备视频 · 广告过滤中…」/ 未就绪「正在准备视频…」/
 *  片源已就绪等播放器「正在缓冲视频…」—— 过滤落地到 canplay 之间文案必须反映真实阶段。 */
const loadingHint = computed(() =>
  loadingHintText({
    srcReady: playSrcReady.value,
    filtering: !!(currentEpisode.value && adFilter.value && reM3u8.test(currentEpisode.value.link))
  })
)

/** loading 遮罩兜底: 超过 25s 仍未就绪(极端慢网 / 就绪事件丢失)则撤遮罩 + 提示,
 *  防无限阻塞(正常路径 2~8s; 撤后用户仍可见视频区与控制条, 可手动切源重试)。 */
const LOADING_FALLBACK_MS = 25_000
let loadingFallbackTimer: number | undefined
watch(loading, (v) => {
  if (loadingFallbackTimer !== undefined) {
    window.clearTimeout(loadingFallbackTimer)
    loadingFallbackTimer = undefined
  }
  if (!v) return
  loadingFallbackTimer = window.setTimeout(() => {
    loadingFallbackTimer = undefined
    if (!loading.value) return
    loading.value = false
    // 25s 未就绪上报(区分"正在缓冲"卡死: playSrc 已就绪但 playing 不来 = 视频数据层卡住)
    telemetry.track('error', {
      category: 'video-loading-fallback',
      action: playSrcReady.value ? 'src-ready-stuck' : 'src-not-ready',
      label: currentSrc.value.slice(0, 160),
      value: LOADING_FALLBACK_MS,
      extra: {
        sourceId: currentSourceId.value,
        episodeIndex: currentEpisodeIndex.value,
        filmId: detail.value ? String(detail.value.mid) : undefined
      }
    })
    toast('info', '视频加载较慢, 可尝试切换播放源')
  }, LOADING_FALLBACK_MS)
})

/** loading 阻塞期禁用播放器手势(双击全屏/长按倍速/滑动 seek) */
const gesturesEnabled = computed(() => !loading.value)
/** 移动端 extra timeout: mobile/hls.js blob URL 在 loading 持续 10s 后强制撤遮罩,
 *  作为 video:playing 事件补充兜底(极端情况下 playing 延迟过久, 避免用户无限等待)。
 *  与 25s LOADING_FALLBACK_MS 配合: 先触发的生效, 互不干扰。 */
let mobileExtraTimeout: number | undefined
watch(loading, (v) => {
  if (mobileExtraTimeout !== undefined) {
    window.clearTimeout(mobileExtraTimeout)
    mobileExtraTimeout = undefined
  }
  if (!v || !isMobile.value) return
  mobileExtraTimeout = window.setTimeout(() => {
    mobileExtraTimeout = undefined
    if (!loading.value) return
    loading.value = false
    telemetry.track('action', {
      category: 'video-loading-mobile-extra',
      action: 'fallback-clear',
      label: currentSrc.value.slice(0, 160)
    })
  }, 10_000)
})
/** 控制条可见状态(usePlayer 由 ArtPlayer 'control' 事件派生的 useractive/userinactive 驱动)。
 * 全屏左上角标题层跟随它显隐; 暂停时 ArtPlayer 控制条常显 → 用 paused 兜底强制显示。 */
const playerControlsVisible = ref(true)
/** 覆盖层宿主: 全屏元素是 ArtPlayer 的 .art-video-player 容器, 广告角标/手势提示若留在
 * 外层 wrap(其兄弟节点), 全屏时会随 DOM 落在全屏元素之外而全部不可见。player 初始化后
 * 把宿主指向 art 的根节点, Teleport 即把覆盖层迁进去。 */
const playerOverlayHost = ref<HTMLElement | null>(null)
const {
  onTouchStart: onPlayerTouchStart,
  onTouchMove: onPlayerTouchMove,
  onTouchEnd: onPlayerTouchEnd,
  gestureRateHint,
  gestureSeekHint
} = usePlayerGestures({
  player,
  paused,
  buffering,
  fullscreen: isPlayerFullscreen,
  enterTempRate,
  exitTempRate,
  // loading 阻塞期禁用手势: 遮罩拦得住鼠标, 但 wrapper 上的 touch 事件仍会触发(双击误进全屏)
  enabled: gesturesEnabled
})

/** ---------- 集数 / 源切换 ---------- */
function changeSource(sourceId: string): void {
  if (!detail.value) return
  const next = detail.value.sources.find((s) => s.id === sourceId)
  if (!next) return
  // 切源保留: 集数 (若超新源最大 index 取最后一集)
  const keepEp = Math.max(0, Math.min(currentEpisodeIndex.value, next.episodes.length - 1))
  // 续播: 优先该(源,集)已记录进度; 无记录则沿用当前播放头, 让用户从原位置继续
  const epRec = historyStore.getEpisode(String(detail.value.mid), sourceId, keepEp)
  const resumeAt = epRec?.currentTime ?? Math.floor(playerCurrentTime.value || 0)
  selectEpisode({ sourceId, episodeIndex: keepEp, resumeAt })
}

function selectEpisode(payload: { sourceId: string; episodeIndex: number; resumeAt?: number; autoPlay?: boolean }): void {
  if (!detail.value) return
  const src = detail.value.sources.find((s) => s.id === payload.sourceId)
  if (!src) return
  const ep = src.episodes[payload.episodeIndex]
  if (!ep) return
  // 同 (源,集) 重复选择: 不重走切集流程 —— 否则 currentSrc 不变 → playSrc 不变 →
  // watch(playSrc) 不触发 → armSeekOnce 不执行 → switchInFlight 互斥窗口悬挂, 自动连播被永久卡死。
  if (payload.sourceId === currentSourceId.value && payload.episodeIndex === currentEpisodeIndex.value) {
    return
  }

  // 用户切换源/集, 视为新的尝试, 重置错误重试计数
  resetRetry()
  // 切集互斥: 从这里开始到新片源就绪前, 忽略一切自动切集(见 switchInFlight 注释)。
  // 手动切集总是放行(重新进入互斥窗口), 仅自动入口在窗口内被 playNextAuto 拦截。
  switchInFlight = true
  // 切换前先把当前进度写历史
  flushHistory()

  // 续播: 优先用调用方显式传入的 resumeAt(切源保留当前位置); 否则取该集独立进度(切集回到上次中断处)
  const id = String(detail.value.mid)
  const epRec = historyStore.getEpisode(id, payload.sourceId, payload.episodeIndex)
  const resumeAt = payload.resumeAt ?? (epRec?.currentTime ?? 0)
  if (import.meta.env.DEV) {
    console.debug('[PlayView][selectEpisode]', {
      payload,
      episodeMapHit: epRec ?? null,
      resumeAt,
      pausedBefore: !paused.value
    })
  }
  // 切集后是否自动续播: 默认"切集前在播放"才续播(用户已暂停则不静默自动播放); 显式 autoPlay 覆盖
  const wasPlaying = !paused.value
  const autoPlay = payload.autoPlay ?? wasPlaying

  currentSourceId.value = payload.sourceId
  currentEpisodeIndex.value = payload.episodeIndex
  applyCurrentEpisodeToPlayer(resumeAt, autoPlay)

  // 同步 router query（不刷新页面，仅替换 URL，保证刷新后能恢复）
  void router.replace({
    path: '/play',
    query: {
      id: String(detail.value.mid),
      source: payload.sourceId,
      episode: String(payload.episodeIndex)
    }
  })
}

/** 清理指定集的播放记忆(封装 filmId 空值判断)。本集自然播完/自动切集时调用,
 * 防止"倒数 5 分钟不上报进度"导致的旧进度滞留。 */
function clearEpisodeMemory(filmId: number | string | undefined | null, srcId: string, epIdx: number): void {
  if (filmId === undefined || filmId === null || filmId === '') return
  void historyStore.clearEpisode(String(filmId), srcId, epIdx)
}

/** 自动连播入口(ended / 片尾 timeupdate): 切集互斥窗口内忽略 —— 修"连跳 2 集"。
 * 两道防线: ①互斥窗口 = selectEpisode 落地 → 新片源就绪(armSeekOnce 生效), 窗口内旧视频
 * 残余 timeupdate/ended 会以"新集"身份再次通过去抖, 不拦截就会再跳一集, 且把旧播放头写进
 * 中间集的历史(进度串集); ②就绪后 10s 冷却, 防个别源就绪后仍迟到触发残余事件。
 * 手动切集按钮不受任何一道限制。 */
function playNextAuto(): void {
  if (switchInFlight) return
  if (Date.now() < autoNextGuardUntil) return
  if (!hasNext.value) return
  // 自动切集 = 本集已播完: 先记下本集(源,集)再切(切完 current* 已指向新集), 随后清其记忆
  const filmId = detail.value?.mid
  const srcId = currentSourceId.value
  const epIdx = currentEpisodeIndex.value
  playNext()
  clearEpisodeMemory(filmId, srcId, epIdx)
}

function playNext(): void {
  if (!hasNext.value) return
  selectEpisode({
    sourceId: currentSourceId.value,
    episodeIndex: currentEpisodeIndex.value + 1
  })
}

function playPrev(): void {
  if (!hasPrev.value) return
  selectEpisode({
    sourceId: currentSourceId.value,
    episodeIndex: currentEpisodeIndex.value - 1
  })
}

/** ---------- 键盘 / D-pad ---------- */
/** 回车切换全屏/非全屏(桌面语义, 与触摸双击一致) */
function togglePlayerFullscreen(): void {
  const p = player.value
  if (!p) return
  try {
    p.fullscreen = !p.fullscreen
  } catch {
    /* ignore */
  }
}

/** 桌面/移动: 键盘始终控制播放器, 与焦点无关 —— 点击按钮留下的焦点不再接管按键。
 * 左右=进度 ±10s, 空格=暂停/继续, 回车=全屏切换, 上下=音量, Esc=返回。
 * 全屏与非全屏行为一致(window 级监听)。 */
function handleDesktopKeydown(e: KeyboardEvent, key: string, target: HTMLElement | null): void {
  const tag = target?.tagName?.toLowerCase()
  const onUiButton = tag === 'button' || tag === 'a'
  // 弹层打开时键盘交给弹层自身: Esc 关弹层而非返回, Enter/空格不劫持(防误切全屏等)
  if (moreActionsOpen.value || skipDialogOpen.value) {
    if (key === 'Escape') {
      e.preventDefault()
      moreActionsOpen.value = false
      skipDialogOpen.value = false
    }
    return
  }
  switch (key) {
    case ' ':
    case 'Spacebar':
      e.preventDefault()
      if (onUiButton) target?.blur()
      if (paused.value) {
        void playerPlay()
      } else {
        playerPause()
      }
      break
    case 'Enter':
      e.preventDefault()
      if (onUiButton) target?.blur()
      togglePlayerFullscreen()
      break
    case 'ArrowLeft':
      e.preventDefault()
      playerSeekBy(-10)
      break
    case 'ArrowRight':
      e.preventDefault()
      playerSeekBy(10)
      break
    case 'ArrowUp': {
      e.preventDefault()
      const cur = player.value?.volume ?? 0.6
      playerSetVolume(Math.min(1, cur + 0.05))
      break
    }
    case 'ArrowDown': {
      e.preventDefault()
      const cur = player.value?.volume ?? 0.6
      playerSetVolume(Math.max(0, cur - 0.05))
      break
    }
    case 'Escape':
      // 返回详情页
      e.preventDefault()
      goBackToDetail()
      break
    default:
      break
  }
}

/** TV: 保留 D-pad 焦点导航语义 —— OK/空格激活焦点按钮, 无焦点时控制播放器 */
function handleTvKeydown(e: KeyboardEvent, key: string, tag: string | undefined): void {
  switch (key) {
    case ' ':
    case 'Spacebar':
    case 'Enter': {
      // OK / 空格：暂停 / 播放（仅当焦点不在按钮上）
      if (tag === 'button' || tag === 'a') {
        return
      }
      e.preventDefault()
      if (paused.value) {
        void playerPlay()
      } else {
        playerPause()
      }
      break
    }
    case 'ArrowLeft':
      e.preventDefault()
      playerSeekBy(-10)
      break
    case 'ArrowRight':
      e.preventDefault()
      playerSeekBy(10)
      break
    case 'ArrowUp': {
      e.preventDefault()
      const cur = player.value?.volume ?? 0.6
      playerSetVolume(Math.min(1, cur + 0.05))
      break
    }
    case 'ArrowDown': {
      e.preventDefault()
      const cur = player.value?.volume ?? 0.6
      playerSetVolume(Math.max(0, cur - 0.05))
      break
    }
    case 'Escape':
      // 返回详情页
      e.preventDefault()
      goBackToDetail()
      break
    default:
      break
  }
}

function handleKeydown(e: KeyboardEvent): void {
  // 输入框聚焦时不拦截
  const target = e.target as HTMLElement | null
  const tag = target?.tagName?.toLowerCase()
  if (tag === 'input' || tag === 'textarea' || target?.isContentEditable) {
    return
  }
  const key = normalizeDpadKey(e)
  if (isTV.value) {
    handleTvKeydown(e, key, tag)
  } else {
    handleDesktopKeydown(e, key, target)
  }
}

/** #3: 清理本视频的播放缓存 —— 删除该片全部观看历史(影片级 + 所有源/集的独立进度),
 * 本地与登录态云端同步清。跳过设置/点赞是用户偏好, 不在此列。 */
async function clearFilmCache(): Promise<void> {
  const id = detail.value?.mid
  if (id === undefined || id === null) return
  moreActionsOpen.value = false
  await historyStore.remove(String(id))
  toast('success', '已清理本视频的播放缓存')
}

function goBackToDetail(): void {
  const id = filmId.value
  if (id) {
    void router.push({ path: '/filmDetail', query: { link: id } })
    return
  }
  void router.push('/index')
}

/** ---------- 视频源错误处理 ----------
 * 旧实现见到 error 立刻换源, 但弱网下临时超时也会触发 error,
 * 直接换源用户体验是"莫名其妙跳到下一个源". 现在改:
 *   1. 同 (source, episode) 先原地重试 2 次, 退避 1.5s / 4s
 *   2. 仍失败再换下一个源 (老逻辑)
 *   3. canplay 触发时重置计数, 避免历史错误累积
 */
const MAX_SAME_SOURCE_RETRIES = 2
const RETRY_DELAYS_MS = [1500, 4000]
const retryCount = ref<number>(0)
let retryTimer: number | undefined

function resetRetry(): void {
  retryCount.value = 0
  if (retryTimer !== undefined) {
    window.clearTimeout(retryTimer)
    retryTimer = undefined
  }
}

function handleVideoError(): void {
  if (!detail.value) return
  const p = player.value
  if (!p) return
  // 视频级错误上报(此前无 video error 埋点, iOS 卡"正在缓冲"类问题在遥测里是盲区)
  const mediaErr = p.video?.error ?? null
  telemetry.track('error', {
    category: 'video-error',
    action: `code:${mediaErr?.code ?? '-'}`,
    label: currentSrc.value.slice(0, 160),
    value: currentSourceId.value ? 1 : 0,
    extra: {
      mediaMessage: mediaErr?.message ?? '',
      sourceId: currentSourceId.value,
      sourceName: currentSource.value?.name,
      episodeIndex: currentEpisodeIndex.value,
      filmId: detail.value ? String(detail.value.mid) : undefined,
      adFilter: adFilter.value
    }
  })
  // 出错即解除 loading 遮罩(阻塞态), 后续反馈交给 videoErrorMsg/重试/换源流程, 避免遮罩死锁
  loading.value = false
  // 切集互斥窗口内的 error 是旧源残余事件: 不重试(避免重挂旧 src 与切换打架),
  // 新源就绪(互斥解除)后若仍出错会正常走重试/换源。
  if (switchInFlight) return

  // 首次 error 且 adFilter 在用 → 推测过滤链路(端侧 blob / 服务端 proxy)故障, 关过滤回退原片
  // (Web 端: 关 adFilter → watch(adFilter) 重算 playSrc 为原始直链; APK 端见 native 内 Toast)
  if (
    retryCount.value === 0 &&
    adFilter.value &&
    currentSrc.value &&
    reM3u8.test(currentSrc.value)
  ) {
    adFilter.value = false
    toast('warning', '广告过滤异常, 已回退原片')
    return
  }

  if (retryCount.value < MAX_SAME_SOURCE_RETRIES) {
    const delay = RETRY_DELAYS_MS[retryCount.value] ?? 4000
    retryCount.value += 1
    videoErrorMsg.value = `加载失败, ${Math.round(delay / 1000)} 秒后第 ${retryCount.value} 次重试…`
    if (retryTimer !== undefined) {
      window.clearTimeout(retryTimer)
    }
    retryTimer = window.setTimeout(() => {
      retryTimer = undefined
      const next = playSrc.value
      if (!next || !player.value) return
      const cur = currentTimePersisted.value // 记忆当前时间, 重试后跳回去
      try {
        playerLoad(next, playSrcType.value || guessMime(next))
        if (cur > 0) {
          const off = onPlayerEvent('loadedmetadata', () => {
            try {
              if (player.value) player.value.currentTime = cur
            } catch {
              /* ignore */
            }
            off()
          })
        }
        void playerPlay()
      } catch {
        // load 调用本身失败极少见; 直接落到换源
        fallbackSwitchSource()
      }
    }, delay)
    return
  }

  // 同源重试上限, 换下一个源
  resetRetry()
  fallbackSwitchSource()
}

function fallbackSwitchSource(): void {
  if (!detail.value) return
  // 切集互斥窗口内不做自动换源: 旧源在切换瞬间的 error 属于残余事件, 等新源就绪后再评估
  if (switchInFlight) return
  videoErrorMsg.value = '当前播放源不可用，正在尝试切换…'
  const sources = detail.value.sources
  const curIdx = sources.findIndex((s) => s.id === currentSourceId.value)
  if (curIdx < 0 || sources.length <= 1) return
  const nextIdx = (curIdx + 1) % sources.length
  if (nextIdx === curIdx) return
  const targetSource = sources[nextIdx]
  if (!targetSource) return
  const targetEpisodeIdx = Math.min(
    currentEpisodeIndex.value,
    Math.max(0, targetSource.episodes.length - 1)
  )
  selectEpisode({ sourceId: targetSource.id, episodeIndex: targetEpisodeIdx })
}

/** 记忆错误发生时的播放进度, 重试后用. */
const currentTimePersisted = computed(() => {
  try {
    return player.value?.currentTime ?? 0
  } catch {
    return 0
  }
})

function guessMime(url: string): string {
  if (/\.m3u8(\?|#|$)/i.test(url)) return 'application/x-mpegURL'
  if (/\.mp4(\?|#|$)/i.test(url)) return 'video/mp4'
  if (/\.webm(\?|#|$)/i.test(url)) return 'video/webm'
  return ''
}

/** ---------- 生命周期 ---------- */
onMounted(async () => {
  await loadPlayInfo()
  // Native 环境: applyCurrentEpisodeToPlayer 已经把整集 playlist 交给 ExoPlayer 全屏播放,
  // PlayView 不再需要渲染 web 播放器, 立即回详情页 (BACK 退 ExoPlayer 后用户看到详情页, 不是空 PlayView)
  if (isNative() && detail.value) {
    setTimeout(() => {
      const id = detail.value?.mid
      if (window.history.length > 1) {
        router.back()
      } else if (id !== undefined) {
        void router.replace({ path: '/filmDetail', query: { link: String(id) } })
      }
    }, 300)
    return
  }
  // 等 DOM 渲染完成后再 init player
  await nextTick()
  if (playerEl.value) {
    initPlayer(playerEl.value)
    // ArtPlayer 初始化后会在容器内构建 .art-video-player 根节点 — 它就是全屏元素,
    // 覆盖层宿主指向它, Teleport 即把覆盖层迁进去
    playerOverlayHost.value = (player.value?.template.$player as HTMLElement | null) ?? null
    onPlayerEvent('ended', () => {
      // 自然播完: 清理本集播放记忆(无论是否开自动连播; 开连播时 playNextAuto 内还会清一次, 幂等)
      clearEpisodeMemory(detail.value?.mid, currentSourceId.value, currentEpisodeIndex.value)
      if (autoPlayNext.value && hasNext.value) {
        playNextAuto()
      }
    })
    onPlayerEvent('error', () => {
      handleVideoError()
    })
    onPlayerEvent('canplay', () => {
      videoErrorMsg.value = ''
      resetRetry()
    })
    // 跳片尾自动播下一集 (按 filmId 配置, 默认 60s, 0=关闭)
    onPlayerEvent('timeupdate', onPlayerTimeUpdateForSkipOutro)
    // 诊断日志: 跳秒(seek)时打印位置与弹幕状态, 便于排查"跳秒后弹幕长期不出现"
    onPlayerEvent('seeked', () => {
      const t = player.value?.currentTime
      console.log('[danmaku] seeked: position =', typeof t === 'number' ? t.toFixed(2) : t, ', state =', danmakuState.value)
    })
    // 全屏状态: 触屏手势(长按2x/横滑快进退)仅全屏时启用
    onPlayerEvent('fullscreenchange', () => {
      isPlayerFullscreen.value = !!player.value?.fullscreen
    })
    // 控制条显隐(ArtPlayer 'control' 事件, usePlayer 派生): 驱动全屏左上角标题层同步出现/消失
    onPlayerEvent('useractive', () => {
      playerControlsVisible.value = true
    })
    onPlayerEvent('userinactive', () => {
      playerControlsVisible.value = false
    })
  }
  if (typeof window !== 'undefined') {
    window.addEventListener('keydown', handleKeydown)
    // APK native ExoPlayer 报错回调: 方案B 下原生已自行"本集回退原始源", 仅当原始源也失败才会
    // 走到这里(真·失败)。不再全局关广告过滤(那会让一次抖动后所有集都不过滤), 只上报+提示切源。
    ;(window as unknown as { __jerocineNativeError?: () => void }).__jerocineNativeError = () => {
      toast('error', '播放失败, 请尝试切换其他播放源')
    }
  }
})

onBeforeRouteLeave(() => {
  flushHistory()
  // 卸载播放器（onScopeDispose 也会兜底，但提前 dispose 可避免短暂的画面残留）
  disposePlayer()
  if (loadingFallbackTimer !== undefined) {
    window.clearTimeout(loadingFallbackTimer)
    loadingFallbackTimer = undefined
  }
  cancelPendingResolve() // 在途的过滤/加载请求立即终止
  prefetchAbort?.abort() // 在途的下一集预取一并取消
  clearFilteredTextCache() // 下一集过滤文本缓存一并清空(换片后旧片源的缓存无意义)
  revokeCurrentBlob() // 释放端侧过滤遗留的 blob: URL, 防内存泄漏
  if (typeof window !== 'undefined') {
    window.removeEventListener('keydown', handleKeydown)
    delete (window as unknown as { __jerocineNativeError?: () => void }).__jerocineNativeError
  }
})

/** 监听 query 变化（仅 id / source / episode），仅在 path 仍为 /play 时响应；
 *  selectEpisode 内部会调用 router.replace 主动同步 query —— 此 watch 在那种情况下
 *  仅做一次幂等检查，发现 store state 与 query 已一致则跳过。 */
watch(
  () => [route.query.id, route.query.source, route.query.episode],
  ([qId, qSource, qEpisode]) => {
    if (route.path !== '/play') return
    if (!detail.value) return
    if (String(qId ?? '') !== String(detail.value.mid)) {
      // 影片切换 → 重新拉取
      void loadPlayInfo()
      return
    }
    const wantSource = String(qSource ?? '')
    const wantEpisode = Number(qEpisode ?? 0)
    if (
      wantSource &&
      (wantSource !== currentSourceId.value || wantEpisode !== currentEpisodeIndex.value)
    ) {
      const src = detail.value.sources.find((s) => s.id === wantSource)
      if (!src) return
      const ep = src.episodes[wantEpisode]
      if (!ep) return
      currentSourceId.value = wantSource
      currentEpisodeIndex.value = wantEpisode
      videoErrorMsg.value = ''
      applyCurrentEpisodeToPlayer(Number(route.query.currentTime) || 0)
    }
  }
)

/** TV 模式下首次进入页面把焦点放到播放器（playerReady 后） */
watch(playerReady, (v) => {
  if (v && isTV.value) {
    nextTick(() => {
      playerEl.value?.focus()
    })
  }
})
</script>

<template>
  <div class="jc-play-view container-page py-[var(--jc-space-6)]">
    <!-- 错误：API 失败 -->
    <BaseEmpty
      v-if="loadError && !loading"
      :title="loadError"
      description="影片暂时无法播放，请稍后重试"
    >
      <template #action>
        <BaseButton variant="gradient" size="lg" @click="router.push('/index')">
          返回首页
        </BaseButton>
      </template>
    </BaseEmpty>

    <!-- 主内容 -->
    <template v-if="!loadError">
      <!-- 主栅格: lg+ 左视频/简介 + 右选集; 小屏单栏堆叠 -->
      <div class="jc-play-grid">
        <section class="jc-play-grid__main flex flex-col gap-[var(--jc-space-5)]">
          <!-- 播放器容器 -->
          <div
            class="jc-player-wrap"
            :data-loading="loading ? '1' : '0'"
            @touchstart="onPlayerTouchStart"
            @touchmove="onPlayerTouchMove"
            @touchend="onPlayerTouchEnd"
            @touchcancel="onPlayerTouchEnd"
          >
            <div ref="playerEl" class="jc-player" tabindex="0"></div>
            <!-- 覆盖层(全屏可见的关键): 全屏元素 = ArtPlayer 的 .art-video-player 容器, 覆盖层必须
                 Teleport 进它内部, 否则全屏时 DOM 在全屏元素之外 → 全部不可见
                 (右上角广告角标/手势提示消失的根因)。player 未初始化时宿主为空 → 留在原位。 -->
            <Teleport :to="playerOverlayHost || 'body'" :disabled="!playerOverlayHost">
              <!-- 弹幕层(P0): 独立 DOM 层叠在播放器容器内, 全屏同样可见;
                   不拦截任何指针事件; z-index 45 = 视频画面(art z10)之上、控制条(art z60)之下。
                   **常驻不 v-if**: useDanmaku 的引擎创建依赖 stage 挂载, 而 stage 又曾
                   v-if=providers 非空 ⇒ 鸡生蛋死锁(providers 永远为空, 按钮全不显示,
                   2026-10-25 线上实测)。空层无视觉影响(引擎未装载时无弹幕节点), 可见性
                   由 --off(visibility) 按开关/就绪态控制, 指针事件本就 none。 -->
              <div
                ref="danmakuStage"
                class="jc-danmaku-stage"
                :class="{ 'jc-danmaku-stage--off': !danmakuActive || danmakuState !== 'ready', 'jc-danmaku-stage--resizing': danmakuResizing }"
                aria-hidden="true"
              />
              <!-- 长按临时倍速提示(顶部居中) -->
              <div v-if="gestureRateHint" class="jc-player-gesture jc-player-gesture--rate" aria-hidden="true">
                {{ gestureRateHint }}
              </div>
              <!-- 横滑刮擦预览(底部): 迷你进度条(目标+当前位置) + 文本 -->
              <div v-if="gestureSeekHint" class="jc-player-gesture jc-player-gesture--seek" aria-hidden="true">
                <div class="jc-player-gesture__bar">
                  <div class="jc-player-gesture__fill" :style="{ width: gestureSeekHint.percent + '%' }" />
                  <div class="jc-player-gesture__cur" :style="{ left: gestureSeekHint.curPercent + '%' }" />
                </div>
                <div class="jc-player-gesture__text">{{ gestureSeekHint.text }}</div>
              </div>
              <!-- 右上角角标组: 广告过滤 + 分辨率 同行(间距 8px) -->
              <div class="jc-player-badges">
                <!-- I-017: 广告过滤状态角标(常驻, 右上角, 六态) -->
                <div
                  v-if="adFilterBadge"
                  class="jc-player-adtag"
                  :data-kind="adFilterBadge.kind"
                >
                  {{ adBadgeText }}
                </div>
                <!-- 分辨率打标: 常驻(广告角标右侧), 反映当前实际画质; hls.js 码率切换实时更新,
                     加载中隐藏(切集瞬间旧分辨率会误导) -->
                <div
                  v-if="videoSize && !loading"
                  class="jc-player-res"
                  aria-hidden="true"
                >
                  {{ resolutionLabel(videoSize.width, videoSize.height) }}
                </div>
              </div>
              <!-- 全屏左上角: 影片名 · 该集名称。显隐跟随控制条(useractive/userinactive, 暂停常显),
                   与右上角角标垂直居中对齐、顶部留 safe-area 空隙避开状态栏(样式见 jc-player-title) -->
              <div
                v-if="isPlayerFullscreen && (playerControlsVisible || paused) && detail"
                class="jc-player-title"
                aria-hidden="true"
              >
                {{ detail.name }}<template v-if="currentEpisode"> · {{ currentEpisode.episode }}</template>
              </div>
              <!-- loading(取数据/过滤解析/切集): 阻塞态, 拦截点击(此前 pointer-events:none 会穿透到
                   控制条, 未就绪也能点全屏), 并给出阶段文案; buffering(播放中缓冲): 仅 spinner, 保持可点暂停。
                   必须放在 Teleport 内: 全屏元素是 .art-video-player 容器, 留在外面则全屏时不可见 -->
              <div
                v-if="loading || buffering"
                class="jc-player-loading"
                :class="{ 'jc-player-loading--blocking': loading }"
                role="status"
                aria-live="polite"
              >
                <span class="jc-player-loading__spinner" />
                <span v-if="loading" class="jc-player-loading__text">{{ loadingHint }}</span>
              </div>
              <div v-if="videoErrorMsg" class="jc-player-error" role="alert">
                {{ videoErrorMsg }}
              </div>
              <!-- 播放设置面板(连播/过滤/弹幕入口): 放 Teleport 内确保全屏可见;
                   z-index 65 = 控制条(art z60)之上、loading(art z70)之下 -->
              <div
                v-if="playSettingsOpen"
                class="jc-play-settings-panel"
                @click.stop
              >
                <div class="jc-play-settings-panel__header">播放设置</div>
                <div class="jc-play-settings-panel__list">
                  <button type="button" class="jc-play-settings-panel__row" :class="{ 'is-on': autoPlayNext }" @click="toggleAutoPlayNext">
                    <BaseIcon name="autoplay" size="18px" class="jc-play-settings-panel__icon" />
                    <span class="jc-play-settings-panel__label">自动连播</span>
                    <span class="jc-play-settings-panel__check" aria-hidden="true">{{ autoPlayNext ? '✓' : '' }}</span>
                  </button>
                  <button type="button" class="jc-play-settings-panel__row" :class="{ 'is-on': adFilter }" @click="toggleAdFilter">
                    <BaseIcon name="shield" size="18px" class="jc-play-settings-panel__icon" />
                    <span class="jc-play-settings-panel__label">过滤广告</span>
                    <span class="jc-play-settings-panel__check" aria-hidden="true">{{ adFilter ? '✓' : '' }}</span>
                  </button>
                  <button
                    type="button"
                    class="jc-play-settings-panel__row"
                    :class="{ 'is-on': danmakuEnabled && !danmakuUnavailable, 'is-disabled': danmakuUnavailable }"
                    @click="onDanmakuEntryClick"
                  >
                    <BaseIcon name="chat" size="18px" class="jc-play-settings-panel__icon" />
                    <span class="jc-play-settings-panel__label">
                      弹幕
                      <template v-if="danmakuUnavailable"> · {{ danmakuHint }}</template>
                    </span>
                    <span class="jc-play-settings-panel__check" aria-hidden="true">{{ danmakuEnabled && !danmakuUnavailable ? '✓' : '' }}</span>
                  </button>
                </div>
                <button type="button" class="jc-play-settings-panel__close" @click="playSettingsOpen = false">关闭</button>
              </div>
            </Teleport>
          </div>

          <!-- 当前播放信息 + 控件: 桌面端 操作按钮组与标题同行(靠右), 移动端按钮组另起一行 -->
          <header
            v-if="detail"
            class="jc-play-info"
          >
            <!-- 标题行: 影片名 + 当前集名称(超长单行省略); 桌面端按钮组由 CSS 落到本行右侧 -->
            <h1 class="jc-play-info__title text-[length:var(--jc-fs-lg)] font-[var(--jc-fw-bold)] text-primary leading-[var(--jc-lh-snug)]">
              <span class="jc-play-info__name">{{ detail.name }}</span>
              <span v-if="currentEpisode" class="jc-play-info__episode text-secondary text-[length:var(--jc-fs-sm)]">
                · {{ currentEpisode.episode }}
              </span>
            </h1>
            <!-- 标签行 -->
            <div class="jc-play-info__tags flex flex-wrap items-center gap-[var(--jc-space-2)]">
              <BaseTag
                v-for="t in tagList"
                :key="t"
                variant="default"
                size="sm"
              >
                {{ t }}
              </BaseTag>
              <RouterLink
                :to="{ path: '/filmDetail', query: { link: String(detail.mid) } }"
                class="jc-play-info__detail-link"
              >
                查看完整介绍 ›
              </RouterLink>
            </div>

        <!-- TV 端 (C 方案): 图标工具条, 一排等宽图标+小字 -->
        <div
          v-if="isTV"
          class="jc-play-toolbar jc-play-toolbar--tv"
          role="toolbar"
          aria-label="播放操作"
        >
          <button type="button" class="jc-pt-btn jc-pt-btn--primary" :disabled="!hasNext" data-focusable="true" @click="playNext">
            <BaseIcon name="skip-next" size="26px" /><span>下一集</span>
          </button>
          <button type="button" class="jc-pt-btn" :class="autoPlayNext ? 'is-on' : ''" :aria-pressed="autoPlayNext" data-focusable="true" @click="toggleAutoPlayNext">
            <BaseIcon name="autoplay" size="26px" /><span>连播</span>
          </button>
          <button type="button" class="jc-pt-btn" :class="testingLines ? 'is-loading' : ''" data-focusable="true" @click="testLines">
            <BaseIcon name="refresh" size="26px" /><span>测速</span>
          </button>
          <button v-if="fastestLineId && fastestLineId !== currentSourceId" type="button" class="jc-pt-btn jc-pt-btn--accent" data-focusable="true" @click="switchToFastest">
            <BaseIcon name="skip-next" size="26px" /><span>最快</span>
          </button>
          <button type="button" class="jc-pt-btn" :class="adFilter ? 'is-on' : ''" :aria-pressed="adFilter" data-focusable="true" @click="toggleAdFilter">
            <BaseIcon name="shield" size="26px" /><span>广告</span>
          </button>
          <button
            type="button"
            class="jc-pt-btn"
            :class="[{ 'is-on': danmakuEnabled && !danmakuUnavailable }, danmakuUnavailable ? 'is-disabled' : '']"
            :aria-pressed="danmakuEnabled"
            :aria-disabled="danmakuUnavailable"
            :title="danmakuUnavailable ? danmakuHint : danmakuEnabled ? '关闭弹幕' : '开启弹幕'"
            data-focusable="true"
            @click="onDanmakuEntryClick"
          >
            <BaseIcon name="chat" size="26px" /><span>弹幕</span>
          </button>
          <button type="button" class="jc-pt-btn" :class="favorited ? 'is-on' : ''" :aria-pressed="favorited" data-focusable="true" @click="toggleFavorite">
            <BaseIcon name="star" size="26px" /><span>{{ favorited ? '已收藏' : '收藏' }}</span>
          </button>
          <button type="button" class="jc-pt-btn" data-focusable="true" @click="openSkipDialog">
            <BaseIcon name="settings" size="26px" /><span>跳过</span>
          </button>
          <button type="button" class="jc-pt-btn" data-focusable="true" @click="router.push('/settings')">
            <BaseIcon name="settings" size="26px" /><span>设置</span>
          </button>
        </div>

        <!-- PC / 移动: 主操作区只留高频(下集 / 收藏 / 切到最快);
             自动连播 / 过滤广告 / 线路测速 / 跳过设置 默认收进"更多"下拉, 减少拥挤。
             桌面端由 .jc-play-info 的栅格放到标题行右侧(与影片名同行), 移动端留在标签行下方。 -->
        <div v-else class="jc-play-toolbar jc-play-toolbar--main">
          <!-- 组1 播放控制 -->
          <div class="jc-pt-group jc-pt-group--primary">
            <BaseButton variant="gradient" size="sm" :disabled="!hasNext" @click="playNext">
              <template #icon><BaseIcon name="skip-next" size="18px" /></template>
              下集
            </BaseButton>
          </div>
          <!-- 组2 操作 -->
          <div class="jc-pt-group">
            <!-- 收藏: 纯图标(空心=未收藏 / 实心+品牌色=已收藏), 文案走 aria-label + title -->
            <BaseButton
              variant="outline"
              size="sm"
              class="jc-pt-fav"
              :class="favorited ? 'jc-toggle--on' : ''"
              :aria-pressed="favorited"
              :aria-label="favorited ? '已收藏' : '收藏'"
              :title="favorited ? '已收藏' : '收藏'"
              @click="toggleFavorite"
            >
              <template #icon>
                <BaseIcon :name="favorited ? 'star' : 'star-outline'" size="18px" />
              </template>
            </BaseButton>
            <!-- 切到最快: 仅在有更快线路时出现 -->
            <BaseButton v-if="fastestLineId && fastestLineId !== currentSourceId" variant="ghost" size="sm" @click="switchToFastest">
              <template #icon><BaseIcon name="skip-next" size="18px" /></template>
              切到最快
            </BaseButton>
            <!-- 分享: TV 无复制场景不显示; 桌面/移动/Pad 显示, 点击复制链接 -->
            <BaseButton v-if="!isTV" variant="ghost" size="sm" :aria-label="shareLabel" :title="shareLabel" @click="handleShare">
              <template #icon><BaseIcon name="share" size="18px" /></template>
            </BaseButton>
            <!-- 更多: 收纳 自动连播 / 过滤广告 / 线路测速 / 跳过设置 -->
            <div class="jc-pt-more">
              <BaseButton variant="outline" size="sm" :aria-expanded="moreActionsOpen" @click="moreActionsOpen = !moreActionsOpen">
                <template #icon><BaseIcon name="menu" size="18px" /></template>
                更多
              </BaseButton>
              <!-- 点击空白处关闭 (开关项不关菜单, 故需此遮罩兜底) -->
              <div v-if="moreActionsOpen" class="jc-pt-more__backdrop" @click="moreActionsOpen = false" />
              <Transition name="jc-fade">
                <div v-if="moreActionsOpen" class="jc-pt-more__panel">
                  <!-- 开关项: 点击切换, 不关菜单, 用对勾显示当前态 -->
                  <button type="button" class="jc-pt-more__item" :class="autoPlayNext ? 'is-on' : ''" :aria-pressed="autoPlayNext" @click="toggleAutoPlayNext">
                    <BaseIcon name="autoplay" size="16px" />
                    <span class="jc-pt-more__item-label">自动连播</span>
                    <span v-if="autoPlayNext" class="jc-pt-more__check" aria-hidden="true">✓</span>
                  </button>
                  <button type="button" class="jc-pt-more__item" :class="adFilter ? 'is-on' : ''" :aria-pressed="adFilter" @click="toggleAdFilter">
                    <BaseIcon name="shield" size="16px" />
                    <span class="jc-pt-more__item-label">过滤广告</span>
                    <span v-if="adFilter" class="jc-pt-more__check" aria-hidden="true">✓</span>
                  </button>
                  <!-- 弹幕开关: 常驻显示(不可用时置灰+原因, 点击给反馈) -->
                  <button type="button" class="jc-pt-more__item" :class="[danmakuEnabled && !danmakuUnavailable ? 'is-on' : '', danmakuUnavailable ? 'is-disabled' : '']" :aria-pressed="danmakuEnabled" :aria-disabled="danmakuUnavailable" @click="onDanmakuEntryClick">
                    <BaseIcon name="chat" size="16px" />
                    <span class="jc-pt-more__item-label">弹幕<template v-if="danmakuUnavailable"> · {{ danmakuHint }}</template></span>
                    <span v-if="danmakuEnabled && !danmakuUnavailable" class="jc-pt-more__check" aria-hidden="true">✓</span>
                  </button>
                  <div class="jc-pt-more__sep" aria-hidden="true" />
                  <!-- 动作项: 点击后关菜单 -->
                  <button type="button" class="jc-pt-more__item" :class="testingLines ? 'is-loading' : ''" @click="testLines(); moreActionsOpen = false">
                    <BaseIcon name="refresh" size="16px" />
                    <span class="jc-pt-more__item-label">播放测速</span>
                  </button>
                  <button type="button" class="jc-pt-more__item" @click="router.push('/settings'); moreActionsOpen = false">
                    <BaseIcon name="settings" size="16px" />
                    <span class="jc-pt-more__item-label">播放设置</span>
                  </button>
                  <button type="button" class="jc-pt-more__item" @click="openSkipDialog(); moreActionsOpen = false">
                    <BaseIcon name="settings" size="16px" />
                    <span class="jc-pt-more__item-label">跳过设置</span>
                  </button>
                  <button type="button" class="jc-pt-more__item" @click="clearFilmCache">
                    <BaseIcon name="trash" size="16px" />
                    <span class="jc-pt-more__item-label">清理本视频播放缓存</span>
                  </button>
                </div>
              </Transition>
            </div>
          </div>
        </div>
      </header>

      <!-- 跳片头片尾设置 (按 filmId 持久化在 localStorage / 登录后随账号同步) -->
      <BaseDialog v-model:visible="skipDialogOpen" title="跳过片头/片尾" width="380px">
        <div class="flex flex-col gap-[var(--jc-space-4)]">
          <p class="text-sm text-secondary">
            仅对本剧生效。保存后<strong>立即生效</strong>(片尾阈值即时更新; 若仍在片头区会自动跳过新片头)。
          </p>
          <!-- 总开关: 关掉 = 本剧不跳, 秒数原样保留(不用把秒数改成 0 丢原值) -->
          <button
            type="button"
            class="jc-skip-switch"
            :class="{ 'is-on': draftEnabled }"
            role="switch"
            :aria-checked="draftEnabled"
            data-focusable="true"
            @click="draftEnabled = !draftEnabled"
          >
            <span class="jc-skip-switch__track" aria-hidden="true">
              <span class="jc-skip-switch__thumb" />
            </span>
            <span class="jc-skip-switch__label">启用跳过</span>
            <span class="jc-skip-switch__state">{{ draftEnabled ? '已开启' : '已关闭' }}</span>
          </button>
          <label
            class="flex items-center gap-[var(--jc-space-3)]"
            :class="{ 'opacity-40': !draftEnabled }"
          >
            <span class="w-[80px] text-sm">片头 (秒)</span>
            <input
              v-model.number="draftIntro"
              type="number"
              min="0"
              max="600"
              step="10"
              :disabled="!draftEnabled"
              class="flex-1 bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-2)] text-base"
              data-focusable="true"
            />
          </label>
          <label
            class="flex items-center gap-[var(--jc-space-3)]"
            :class="{ 'opacity-40': !draftEnabled }"
          >
            <span class="w-[80px] text-sm">片尾 (秒)</span>
            <input
              v-model.number="draftOutro"
              type="number"
              min="0"
              max="600"
              step="10"
              :disabled="!draftEnabled"
              class="flex-1 bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-2)] text-base"
              data-focusable="true"
            />
          </label>
          <p class="text-xs text-muted">
            默认 90 / 60。设为 0 = 不跳; 关闭上方开关 = 本剧不跳(秒数仍保留)。
          </p>
        </div>
        <template #footer>
          <BaseButton variant="ghost" @click="resetSkip">恢复默认</BaseButton>
          <BaseButton variant="ghost" @click="skipDialogOpen = false">取消</BaseButton>
          <BaseButton variant="gradient" @click="saveSkip">保存</BaseButton>
        </template>
      </BaseDialog>

      <!-- 弹幕设置面板(控制条齿轮按钮打开; 所有项实时生效, 持久化见 useDanmaku)
           放 Teleport 内确保全屏可见, 与播放设置面板同款内嵌样式 -->
      <div
        v-if="danmakuSettingsOpen"
        class="jc-play-settings-panel jc-play-settings-panel--danmaku"
        @click.stop
      >
        <div class="jc-play-settings-panel__header">弹幕设置</div>
        <div class="jc-play-settings-panel__list">
          <!-- 弹幕总开关 -->
          <button
            type="button"
            class="jc-play-settings-panel__row"
            :class="{ 'is-on': danmakuEnabled && !danmakuUnavailable, 'is-disabled': danmakuUnavailable }"
            @click="onDanmakuEntryClick"
          >
            <BaseIcon name="chat" size="18px" class="jc-play-settings-panel__icon" />
            <span class="jc-play-settings-panel__label">
              弹幕
              <template v-if="danmakuUnavailable"> · {{ danmakuHint }}</template>
            </span>
            <span class="jc-play-settings-panel__check" aria-hidden="true">{{ danmakuEnabled && !danmakuUnavailable ? '✓' : '' }}</span>
          </button>
        </div>
        <!-- 字号 -->
        <div class="jc-play-settings-panel__section">
          <div class="jc-play-settings-panel__section-label">字号</div>
          <div class="jc-play-settings-panel__segs">
            <button
              v-for="(px, i) in DANMAKU_FONT_STEPS"
              :key="px"
              type="button"
              class="jc-play-settings-panel__seg"
              :class="{ 'is-active': danmakuSettings.fontSize === px }"
              @click="danmakuUpdateSettings({ fontSize: px })"
            >
              {{ ['自适应', '极小', '小', '中', '大', '特大'][i] }}
            </button>
          </div>
        </div>
        <!-- 透明度 -->
        <div class="jc-play-settings-panel__section">
          <div class="jc-play-settings-panel__section-label">透明度 {{ Math.round(danmakuSettings.opacity * 100) }}%</div>
          <input
            type="range"
            min="10"
            max="100"
            step="5"
            class="jc-play-settings-panel__range"
            :value="Math.round(danmakuSettings.opacity * 100)"
            @input="danmakuUpdateSettings({ opacity: Number(($event.target as HTMLInputElement).value) / 100 })"
          />
        </div>
        <!-- 显示区域 -->
        <div class="jc-play-settings-panel__section">
          <div class="jc-play-settings-panel__section-label">显示区域</div>
          <div class="jc-play-settings-panel__segs">
            <button
              v-for="(a, i) in DANMAKU_AREA_STEPS"
              :key="a"
              type="button"
              class="jc-play-settings-panel__seg"
              :class="{ 'is-active': danmakuSettings.area === a }"
              @click="danmakuUpdateSettings({ area: a })"
            >
              {{ ['1/4屏', '半屏', '3/4屏', '全屏'][i] }}
            </button>
          </div>
        </div>
        <!-- 速度 -->
        <div class="jc-play-settings-panel__section">
          <div class="jc-play-settings-panel__section-label">速度</div>
          <div class="jc-play-settings-panel__segs">
            <button
              v-for="(s, i) in DANMAKU_SPEED_STEPS"
              :key="s"
              type="button"
              class="jc-play-settings-panel__seg"
              :class="{ 'is-active': danmakuSettings.speed === s }"
              @click="danmakuUpdateSettings({ speed: s })"
            >
              {{ ['极快', '快', '正常', '较慢', '慢'][i] }}
            </button>
          </div>
        </div>
        <!-- 同屏数量 -->
        <div class="jc-play-settings-panel__section">
          <div class="jc-play-settings-panel__section-label">同屏数量</div>
          <div class="jc-play-settings-panel__segs">
            <button
              v-for="(n, i) in DANMAKU_LIMIT_STEPS"
              :key="n"
              type="button"
              class="jc-play-settings-panel__seg"
              :class="{ 'is-active': danmakuSettings.limit === n }"
              @click="danmakuUpdateSettings({ limit: n })"
            >
              {{ ['少', '中', '多', '不限'][i] }}
            </button>
          </div>
        </div>
        <!-- 模式开关 -->
        <div class="jc-play-settings-panel__grid">
          <button type="button" class="jc-play-settings-panel__toggle" :class="{ 'is-on': danmakuSettings.topMode }" @click="danmakuUpdateSettings({ topMode: !danmakuSettings.topMode })">
            顶部
          </button>
          <button type="button" class="jc-play-settings-panel__toggle" :class="{ 'is-on': danmakuSettings.bottomMode }" @click="danmakuUpdateSettings({ bottomMode: !danmakuSettings.bottomMode })">
            底部
          </button>
          <button type="button" class="jc-play-settings-panel__toggle" :class="{ 'is-on': danmakuSettings.colored }" @click="danmakuUpdateSettings({ colored: !danmakuSettings.colored })">
            彩色
          </button>
          <button type="button" class="jc-play-settings-panel__toggle" :class="{ 'is-on': danmakuSettings.outline }" @click="danmakuUpdateSettings({ outline: !danmakuSettings.outline })">
            描边
          </button>
        </div>
        <button type="button" class="jc-play-settings-panel__close" @click="danmakuSettingsOpen = false">关闭</button>
      </div>

      <!-- 播放设置面板已移入 Teleport (见上方)，全屏可见。此处不再重复渲染。 -->

        </section>

        <!-- 右侧选集 (PC: 视频右边, 与左列等高, 两层 tab 固定仅集列表滚动; 小屏堆叠到下方). -->
        <aside v-if="detail" class="jc-play-grid__aside">
          <div class="jc-play-grid__aside-inner">
            <EpisodeTabs
              :sources="detail.sources"
              :current-source-id="currentSourceId"
              :current-episode="currentEpisode?.link ?? ''"
              :watched-links="watchedLinks"
              :speeds="lineSpeeds"
              :film-name="detail.name"
              @change-source="changeSource"
              @select="selectEpisode"
            />
          </div>
        </aside>
      </div>

      <!-- 相关推荐: 移到栅格下方整行展示 -->
      <section v-if="visibleRelate.length" class="jc-play-relate mt-[var(--jc-space-8)]">
        <RelatedList :items="visibleRelate" title="相关推荐" />
      </section>
    </template>
  </div>
</template>

<style scoped>
.jc-play-view {
  min-height: 60vh;
}

/* 播放器容器：16:9 自适应 */
.jc-player-wrap {
  position: relative;
  width: 100%;
  background-color: #000;
  border-radius: var(--jc-radius-lg);
  overflow: hidden;
  aspect-ratio: 16 / 9;
  box-shadow: var(--jc-shadow-xl);
}

.jc-player {
  position: absolute;
  inset: 0;
  width: 100% !important;
  height: 100% !important;
  outline: none;
}

.jc-player:focus,
.jc-player:focus-visible {
  outline: none;
}

/* 弹幕层（P0）: 覆盖视频画面、不拦截指针事件。
 * z-index 按 ArtPlayer 内部标尺排布(video=10/poster=11/subtitle=20/layers=40/mask=50/
 * bottom=60/loading=70): 弹幕 45 = 视频与字幕之上、居中状态图标(50)与控制条(60)之下。
 * 渲染由 danmaku-lite（Canvas 引擎）负责：引擎自身按 fontSize/fontFamily/strokeWidth
 * 写行内样式并驱动 transform 动画，无需 .cmt 类规则，字号自适应由引擎 resolveFontSize 推算。
 * 容器仅负责定位与裁剪。 */
.jc-danmaku-stage {
  position: absolute;
  inset: 0;
  z-index: 45;
  overflow: hidden;
  pointer-events: none;
}
.jc-danmaku-stage--off {
  visibility: hidden;
}
.jc-danmaku-stage--resizing {
  visibility: hidden;
}

/* 桌面端最大宽度（>= 1280 居中） */
@media (min-width: 1280px) {
  .jc-player-wrap {
    max-width: 1280px;
    margin: 0 auto;
  }
}

/* 加载占位：暗色遮罩 + 胶囊容器内 3 个跳动圆点 + 文字
   (原版仅半透明圆点、无遮罩无 z-index, 亮画面上几乎看不见) */
/* 触屏手势提示(全屏, Teleport 在 .art-video-player 内): 长按3x = 顶部居中胶囊; 刮擦 = 底部进度条面板。
   共用胶囊底样式, 位置/尺寸由修饰类覆盖; 不拦截触摸。
   z-index 65: ArtPlayer 控制条 .art-bottom 是 60, 提示需盖过它。 */
.jc-player-gesture {
  position: absolute;
  z-index: 65;
  padding: 8px 16px;
  border-radius: 999px;
  background-color: rgba(0, 0, 0, 0.65);
  color: #fff;
  font-size: 14px;
  letter-spacing: 0.5px;
  white-space: nowrap;
  pointer-events: none;
}
/* 长按 3x: 顶部居中(避开右上角广告角标) */
.jc-player-gesture--rate {
  left: 50%;
  top: calc(var(--jc-space-4) + env(safe-area-inset-top, 0px));
  transform: translateX(-50%);
}
/* 刮擦预览: 底部居中(悬在控制条上方), 面板含迷你进度条 + 文本行 */
.jc-player-gesture--seek {
  left: 50%;
  bottom: calc(72px + env(safe-area-inset-bottom, 0px));
  transform: translateX(-50%);
  width: min(420px, 72%);
  padding: 10px 14px;
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.jc-player-gesture__bar {
  position: relative;
  height: 4px;
  border-radius: 999px;
  background-color: rgba(255, 255, 255, 0.25);
}
/* 目标位置 = 白色填充; 当前位置 = 灰点(对照) */
.jc-player-gesture__fill {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  border-radius: 999px;
  background-color: #fff;
}
.jc-player-gesture__cur {
  position: absolute;
  top: 50%;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: rgba(255, 255, 255, 0.55);
  transform: translate(-50%, -50%);
}
.jc-player-gesture__text {
  text-align: center;
  font-size: 13px;
}

.jc-player-loading {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
  align-items: center;
  justify-content: center;
  /* 遮罩: 压暗旧画面, 保证 loading 在任何视频底色上都可辨识
     z-index 75: ArtPlayer loading=70/bottom=60, 阻塞态需盖过控制条 */
  background-color: rgba(0, 0, 0, 0.55);
  backdrop-filter: blur(2px);
  z-index: 75;
  pointer-events: none;
}
/* loading(非 buffering)为阻塞态: 拦截点击, 不穿透到播放器控制条(未就绪不可暂停/全屏) */
.jc-player-loading--blocking {
  pointer-events: auto;
  cursor: progress;
}
.jc-player-loading__text {
  color: rgba(255, 255, 255, 0.92);
  font-size: 13px;
  text-shadow: 0 1px 4px rgba(0, 0, 0, 0.6);
}
/* 圆形 spinner: 用显式 px 而非 em —— 本层在播放器根节点之外, em 按根字号算不准。 */
.jc-player-loading__spinner {
  width: 44px;
  height: 44px;
  border-radius: 9999px;
  border: 4px solid rgba(255, 255, 255, 0.25);
  border-top-color: #fff;
  box-shadow: 0 0 12px rgba(255, 255, 255, 0.45);
  animation: jc-play-spin 0.8s linear infinite;
}
@keyframes jc-play-spin {
  to {
    transform: rotate(360deg);
  }
}

.jc-player-error {
  position: absolute;
  left: var(--jc-space-3);
  bottom: var(--jc-space-3);
  /* 盖过 jc-player-loading(z-75): 阻塞期错误提示(重试倒计时等)必须可见 */
  z-index: 78;
  padding: var(--jc-space-2) var(--jc-space-3);
  background-color: rgba(0, 0, 0, 0.65);
  color: #fff;
  font-size: var(--jc-fs-sm);
  border-radius: var(--jc-radius-md);
  pointer-events: none;
}

/* 当前播放信息块: 移动端竖排(标题 / 标签 / 按钮条 顺序堆叠) */
.jc-play-info {
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-2);
}

.jc-play-info__title :deep(a) {
  color: inherit;
  text-decoration: none;
}

/* 移动端: "· 当前集"与影片名同行, 用 margin 拉开间距(桌面端改用 flex gap) */
.jc-play-info__episode {
  margin-left: var(--jc-space-2);
}

/* 平板及以上(≥768px): 首行 = 标题(可单行省略) + 操作按钮组(靠右), 标签行独占第二行。
   断点对齐 useViewMode 的 tablet 档(768-1023) —— 900px 左右也能把按钮收到标题行右侧;
   仅移动端(<768)保持"按钮另起一行"。
   仅在渲染了 PC/移动按钮组(非 TV)时切换为栅格 —— TV 工具条保持整行在下方。 */
@media (min-width: 768px) {
  .jc-play-info:has(.jc-play-toolbar--main) {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    column-gap: var(--jc-space-3);
    row-gap: var(--jc-space-2);
  }
  .jc-play-info:has(.jc-play-toolbar--main) .jc-play-info__title {
    grid-area: 1 / 1;
    display: flex;
    align-items: baseline;
    gap: var(--jc-space-2);
    min-width: 0;
  }
  /* 影片名 / 当前集名称: 各自单行 + 溢出省略号。
     集名优先保持可读(不参与收缩, 最多占半行), 影片名负责让位收缩 →
     影片名过长时先截影片名, 集名过长时才轮到它自己截断 */
  .jc-play-info__name,
  .jc-play-info__episode {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .jc-play-info__name {
    flex: 0 1 auto;
  }
  .jc-play-info__episode {
    flex: 0 0 auto;
    max-width: 50%;
    margin-left: 0;
  }
  .jc-play-info:has(.jc-play-toolbar--main) .jc-play-info__tags {
    grid-area: 2 / 1 / 3 / -1;
  }
  .jc-play-info:has(.jc-play-toolbar--main) .jc-play-toolbar--main {
    grid-area: 1 / 2;
  }
}

/* 自动连播开关激活态 */
.jc-toggle--on {
  color: var(--jc-brand-primary) !important;
  border-color: var(--jc-brand-primary) !important;
}

/* 主体栅格：移动 / 平板 单列；桌面 1024+ 双列 */
/* 标题旁"查看完整介绍"链接 */
.jc-play-info__detail-link {
  display: inline-flex;
  align-items: center;
  color: var(--jc-text-link);
  font-size: var(--jc-fs-xs);
  text-decoration: none;
  padding: 2px 8px;
  border-radius: var(--jc-radius-sm);
  transition: color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-play-info__detail-link:hover,
.jc-play-info__detail-link:focus-visible {
  color: var(--jc-text-link-hover);
  background-color: rgba(74, 209, 229, 0.08);
  outline: none;
}

/* ===== 播放页操作工具条 (A: PC/移动单排分组) ===== */
.jc-play-toolbar {
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  flex-wrap: wrap;
  /* 操作按钮整体靠右 */
  justify-content: flex-end;
}
/* 按钮宽度随文字自适应 + 内边距更小 */
.jc-play-toolbar :deep(.jc-btn) {
  width: auto;
  flex: 0 0 auto;
  padding-inline: var(--jc-space-3);
}
/* 收藏: 纯图标 → 收成正方形命中区(状态由空心/实心星 + 品牌色区分) */
.jc-play-toolbar :deep(.jc-btn.jc-pt-fav) {
  width: 32px;
  padding-inline: 0;
  gap: 0;
}
.jc-pt-group {
  display: flex;
  align-items: center;
  gap: var(--jc-space-2);
  flex-wrap: wrap;
}
/* "更多操作" 下拉 */
.jc-pt-more {
  position: relative;
}
/* 透明全屏遮罩: 点击空白关闭下拉 (开关项不自关, 故需此兜底) */
.jc-pt-more__backdrop {
  position: fixed;
  inset: 0;
  z-index: calc(var(--jc-z-dropdown) - 1);
}
.jc-pt-more__panel {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: var(--jc-z-dropdown);
  min-width: 184px;
  padding: var(--jc-space-2);
  background-color: var(--jc-bg-surface);
  border: 1px solid var(--jc-border-subtle);
  border-radius: var(--jc-radius-md);
  box-shadow: var(--jc-shadow-lg);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.jc-pt-more__item {
  display: flex;
  align-items: center;
  gap: var(--jc-space-2);
  width: 100%;
  padding: var(--jc-space-2) var(--jc-space-3);
  background: transparent;
  border: none;
  border-radius: var(--jc-radius-sm);
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  cursor: pointer;
  text-align: left;
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
/* 文字占满中间, 把对勾推到最右 */
.jc-pt-more__item-label {
  flex: 1 1 auto;
}
/* 开关项已开启: 主题色 + 右侧对勾 */
.jc-pt-more__item.is-on {
  color: var(--jc-brand-primary);
}
/* 弹幕不可用(无源/查无/失败): 置灰但仍可点击(点击给原因, failed 重试) */
.jc-pt-more__item.is-disabled {
  color: var(--jc-text-tertiary, var(--jc-text-secondary));
  opacity: 0.55;
  cursor: default;
}
.jc-pt-more__check {
  flex: 0 0 auto;
  color: var(--jc-brand-primary);
  font-weight: var(--jc-fw-bold);
}
/* 开关项与动作项之间的分隔线 */
.jc-pt-more__sep {
  height: 1px;
  margin: var(--jc-space-1) var(--jc-space-2);
  background-color: var(--jc-border-subtle);
}
.jc-pt-more__item:hover,
.jc-pt-more__item:focus-visible {
  background-color: rgba(255, 255, 255, 0.06);
  color: var(--jc-text-primary);
  outline: none;
}
.jc-pt-more__item.is-loading {
  opacity: 0.6;
  pointer-events: none;
}

.jc-play-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--jc-space-6);
}

/* 大屏: 左视频/影片信息, 右选集。
 * 选集栏用「绝对定位 + 栅格定位」放进第 2 列: 它不进流 → 不参与行高计算,
 * 因此高度严格等于左列高度(播放器 + 影片信息), 集数再多也不会把整行撑高、
 * 拖到左列下方去; 超出部分由集网格内部纵向滚动消化。
 * 面板自身仍 sticky: 左列比视口高时(超大屏/矮窗口)整块保持可见, 高度上限取二者较小值。 */
@media (min-width: 1024px) {
  .jc-play-grid {
    grid-template-columns: minmax(0, 2.6fr) minmax(300px, 1fr);
    align-items: start;
    /* 绝对定位的选集栏以本容器为包含块, 才能用 grid-column/row 定位到第 2 列 */
    position: relative;
  }
  .jc-play-grid__aside {
    /* 绝对定位的栅格子项: 第 2 列 × 第 1 行, 四边贴合该栅格区域(宽=右栏宽, 高=左列高) */
    position: absolute;
    grid-column: 2;
    grid-row: 1;
    inset: 0;
    min-width: 0;
    min-height: 0;
  }
  .jc-play-grid__aside-inner {
    /* sticky: 随页面滚动停在视口上方; 高度 = 左列高度, 但不超过视口可用高 */
    position: sticky;
    top: var(--jc-space-6);
    height: 100%;
    max-height: calc(100vh - var(--jc-space-6) * 2);
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
  }
  .jc-play-grid__aside :deep(.jc-episodes) {
    height: 100%;
    min-height: 0;
  }
  /* 两层 tab(播放源条 + 集数分段)固定, 不进滚动区 */
  .jc-play-grid__aside :deep(.jc-source-bar),
  .jc-play-grid__aside :deep(.jc-episode-segments) {
    flex: 0 0 auto;
  }
  /* 选源条内的横向滚动容器必须可收缩(否则 flex:0 0 auto 会让它撑满溢出被 aside 裁掉,
     最右的源滚不到). 由其自身 min-width:0 + overflow-x:auto 接管内部横向滚动 */
  .jc-play-grid__aside :deep(.jc-source-tabs) {
    flex: 1 1 0;
    min-width: 0;
  }
  /* 集网格: 占满剩余高度 + 纵向滚动 + 滚轮顺畅. 右侧窄栏每行 2 个(用户指定), 细滚动条 */
  .jc-play-grid__aside :deep(.jc-episode-grid) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    /* clip 另一轴, 避免 CSS 规范把 overflow-y 强制成 auto 造成嵌套滚动 */
    overflow-x: clip;
    overscroll-behavior: contain;
    align-content: start;
    padding-right: var(--jc-space-1);
    scrollbar-width: thin;
    scrollbar-color: rgba(255, 255, 255, 0.2) transparent;
  }
  .jc-play-grid__aside :deep(.jc-episode-grid)::-webkit-scrollbar {
    width: 8px;
  }
  .jc-play-grid__aside :deep(.jc-episode-grid)::-webkit-scrollbar-thumb {
    background-color: rgba(255, 255, 255, 0.2);
    border-radius: 4px;
  }
  .jc-play-grid__aside :deep(.jc-episode-grid)::-webkit-scrollbar-thumb:hover {
    background-color: rgba(255, 255, 255, 0.35);
  }
}

.jc-play-grid__main {
  min-width: 0;
}
.jc-play-grid__aside {
  min-width: 0;
}

.jc-play-synopsis {
  background-color: var(--jc-bg-surface);
  border-radius: var(--jc-radius-md);
  padding: var(--jc-space-4);
}

/* ArtPlayer 皮肤: 官方 CSS 变量接入主题色/进度条配色, 其余沿用 ArtPlayer 默认外观 */
:deep(.art-video-player) {
  --art-theme: var(--jc-brand-primary);
  --art-loaded-color: rgba(255, 255, 255, 0.45);
}
:deep(video) {
  outline: none !important;
}
/* 居中播放按钮(art-state, 暂停时显示/点击播放): 加投影保证亮画面可辨识 */
:deep(.art-state) {
  color: rgba(255, 255, 255, 0.92);
  filter: drop-shadow(0 1px 6px rgb(0 0 0 / 0.5));
}
/* loading 期隐藏居中播放按钮: 与 loading 遮罩的 spinner 双圆圈叠显不美观
   (loading 结束后自动恢复) */
.jc-player-wrap[data-loading='1'] :deep(.art-state) {
  display: none;
}
/* ArtPlayer 自带 loading spinner 与我们自己的 jc-player-loading(带文案/阻塞态)重复, 隐藏 */
:deep(.art-loading) {
  display: none !important;
}

/* I-017: 广告过滤结果角标(播放器右上角, 常驻不遮操作)
   top 加 safe-area: 全屏时移动端可能出现状态栏, 顶部留出空隙; 与左上角标题同基线对齐 */
.jc-player-adtag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 999px;
  background-color: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  color: rgba(255, 255, 255, 0.92);
  font-size: var(--jc-fs-xs);
  line-height: 1;
  pointer-events: none;
  white-space: nowrap;
}
.jc-player-adtag::before {
  content: '';
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: #34d399;
  box-shadow: 0 0 6px rgba(52, 211, 153, 0.8);
}
/* I-017 角标配色: filtered/clean=绿(默认), off/unsupported/failed/nosupport=灰, proxy/filtering=蓝(处理中) */
.jc-player-adtag[data-kind='off']::before,
.jc-player-adtag[data-kind='unsupported']::before,
.jc-player-adtag[data-kind='failed']::before,
.jc-player-adtag[data-kind='nosupport']::before {
  background-color: #9ca3af;
  box-shadow: none;
}
.jc-player-adtag[data-kind='proxy']::before,
.jc-player-adtag[data-kind='filtering']::before {
  background-color: #60a5fa;
  box-shadow: 0 0 6px rgba(96, 165, 250, 0.8);
}

/* 右上角角标组(广告过滤 + 分辨率): absolute 定位于播放器右上角, 内部 flex 同行, 间距 8px
   z-index 55: ArtPlayer 居中状态层(mask)是 50, 角标需在其上、控制条(.art-bottom 60)之下 */
.jc-player-badges {
  position: absolute;
  top: calc(env(safe-area-inset-top, 0px) + 10px);
  right: var(--jc-space-2);
  z-index: 55;
  display: flex;
  align-items: center;
  gap: 8px;
  pointer-events: none;
}

/* 分辨率打标(广告角标右侧, 同行): 同款玻璃胶囊, 反映当前实际画质 */
.jc-player-res {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 999px;
  background-color: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  color: rgba(255, 255, 255, 0.92);
  font-size: var(--jc-fs-xs);
  line-height: 1;
  pointer-events: none;
  white-space: nowrap;
}

/* 全屏左上角标题(影片名 · 集名): 与右上角广告角标同 top 基线且同高(20px) → 垂直中心线对齐;
   字号比角标(fs-xs)大一号(fs-sm); top 含 safe-area 避开状态栏; 显隐由模板 v-if 跟随控制条,
   此处仅做淡入过渡。 */
.jc-player-title {
  position: absolute;
  top: calc(env(safe-area-inset-top, 0px) + 10px);
  left: var(--jc-space-3, 12px);
  z-index: 55;
  display: flex;
  align-items: center;
  height: 20px;
  max-width: 68%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--jc-fs-sm);
  font-weight: 600;
  color: rgba(255, 255, 255, 0.95);
  text-shadow: 0 1px 4px rgba(0, 0, 0, 0.9), 0 0 2px rgba(0, 0, 0, 0.7);
  pointer-events: none;
  animation: jc-player-title-in 0.25s ease;
}
@keyframes jc-player-title-in {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

/* 控制条自定义按钮(下一集/弹幕开关/弹幕设置): 统一图标样式 + hover 微变色 */
:deep(.art-control-next-episode),
:deep(.art-control-danmaku-toggle),
:deep(.art-control-danmaku-settings) {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  cursor: pointer;
  opacity: 0.82;
  transition: opacity var(--jc-dur-fast) var(--jc-ease-standard),
              color var(--jc-dur-fast) var(--jc-ease-standard);
}
:deep(.art-control-next-episode:hover),
:deep(.art-control-danmaku-toggle:hover),
:deep(.art-control-danmaku-settings:hover),
:deep(.art-control-next-episode:focus-visible),
:deep(.art-control-danmaku-toggle:focus-visible),
:deep(.art-control-danmaku-settings:focus-visible) {
  opacity: 1;
}
:deep(.art-control-next-episode svg),
:deep(.art-control-danmaku-toggle svg),
:deep(.art-control-danmaku-settings svg) {
  display: block;
  filter: drop-shadow(0 1px 2px rgb(0 0 0 / 0.5));
}
/* 开启态颜色: 弹幕有效启用时按钮着品牌色 */
:deep(.art-control-danmaku-toggle.is-on),
:deep(.art-control-danmaku-settings.is-on) {
  color: var(--jc-brand-primary) !important;
}
/* 下一集(左置): 与播放按钮间隔, TV 远距离加大 */
:deep(.art-control-next-episode) {
  margin-right: 4px;
}
[data-mode='tv'] :deep(.art-control-next-episode) {
  width: 52px;
  height: 52px;
}

/* 移动端：标题块换行 + 按钮组靠右 */
@media (max-width: 767px) {
  .jc-play-info {
    align-items: flex-start;
  }
}
</style>

<style>
/* TV 模式覆盖 */
[data-mode='tv'] .jc-player-wrap {
  border-radius: 16px;
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.7);
}
[data-mode='tv'] .jc-play-view {
  padding-block: var(--jc-tv-safe);
  padding-inline: var(--jc-tv-safe);
}
/* TV 远距离观看: 控制条/居中播放按钮放大(ArtPlayer 官方 CSS 变量) */
[data-mode='tv'] .jc-player-wrap .art-video-player {
  --art-control-height: 64px;
  --art-control-icon-size: 28px;
  --art-state-size: 120px;
}
/* TV 远距离观看: spinner 对齐 TV 下放大后的播放按钮 */
[data-mode='tv'] .jc-player-loading__spinner {
  width: 64px;
  height: 64px;
  border-width: 6px;
}
/* TV: 广告过滤角标放大(远距可读) */
[data-mode='tv'] .jc-player-adtag {
  font-size: 15px;
  padding: 8px 16px;
  gap: 9px;
}
[data-mode='tv'] .jc-player-adtag::before {
  width: 9px;
  height: 9px;
}
/* TV: 控制条「下一集」图标放大 */
[data-mode='tv'] .art-control-next-episode {
  width: 52px;
  height: 52px;
}
/* TV: 居中播放按钮(art-state)放大 —— 官方 CSS 变量已在上方统一调整 */

/* ===== TV 工具条 (C: 图标 + 小字, 一排等宽) ===== */
.jc-play-toolbar--tv {
  display: flex;
  flex-wrap: wrap;
  gap: var(--jc-space-3);
  /* 焦点环不被裁 */
  padding-block: 8px;
}
.jc-pt-btn {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 88px;
  padding: var(--jc-space-3) var(--jc-space-3);
  background-color: var(--jc-bg-elevated);
  border: none;
  border-radius: var(--jc-radius-md);
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-medium);
  cursor: pointer;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-pt-btn span {
  font-size: var(--jc-fs-sm);
  white-space: nowrap;
}
.jc-pt-btn.is-on {
  color: var(--jc-brand-cyan);
}
/* 弹幕不可用(无源/查无/失败): 置灰但仍可聚焦可点击(TV 遥控器也要能选中看原因) */
.jc-pt-btn.is-disabled {
  color: var(--jc-text-tertiary, var(--jc-text-secondary));
  opacity: 0.55;
  cursor: default;
}
.jc-pt-btn--primary {
  background-image: var(--jc-brand-gradient);
  color: #fff;
}
.jc-pt-btn--accent {
  color: var(--jc-brand-cyan);
  border: 1px solid var(--jc-brand-cyan);
}
.jc-pt-btn:disabled {
  opacity: 0.4;
  pointer-events: none;
}
.jc-pt-btn.is-loading {
  opacity: 0.6;
  pointer-events: none;
}
/* TV 焦点: 细环(不被裁), 统一 token */
[data-mode='tv'] .jc-pt-btn:focus,
[data-mode='tv'] .jc-pt-btn:focus-visible {
  outline: none;
  box-shadow: var(--jc-tv-focus-ring);
  background-color: rgba(255, 255, 255, 0.1);
  color: var(--jc-text-primary);
}

/* 桌面/移动(非 TV): 点击按钮产生的焦点框去掉 —— 键盘已全局接管为播放器控制
   (左右=进度/空格=暂停/回车=全屏), 焦点导航无意义; TV 模式保留焦点环(D-pad 定位依赖)。 */
[data-mode]:not([data-mode='tv']) .jc-play-view button:focus,
[data-mode]:not([data-mode='tv']) .jc-play-view button:focus-visible,
[data-mode]:not([data-mode='tv']) .jc-play-view .art-video-player .art-control:focus,
[data-mode]:not([data-mode='tv']) .jc-play-view .art-video-player .art-control:focus-visible {
  outline: none;
  box-shadow: none;
}

/* ============ 跳过设置弹窗: 启用跳过总开关 ============ */
.jc-skip-switch {
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  width: 100%;
  padding: var(--jc-space-2) var(--jc-space-3);
  background-color: var(--jc-bg-elevated);
  border: 1px solid var(--jc-border-default);
  border-radius: var(--jc-radius-md);
  cursor: pointer;
  transition: border-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-skip-switch__track {
  position: relative;
  flex: none;
  width: 36px;
  height: 20px;
  border-radius: 999px;
  background-color: var(--jc-text-muted);
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-skip-switch.is-on .jc-skip-switch__track {
  background-color: var(--jc-brand-primary);
}
.jc-skip-switch__thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background-color: #fff;
  transition: transform var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-skip-switch.is-on .jc-skip-switch__thumb {
  transform: translateX(16px);
}
.jc-skip-switch__label {
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-primary);
}
.jc-skip-switch__state {
  margin-left: auto;
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-secondary);
}
[data-mode='tv'] .jc-skip-switch:focus,
[data-mode='tv'] .jc-skip-switch:focus-visible {
  outline: none;
  box-shadow: var(--jc-tv-focus-ring);
}
/* TV/安卓壳: 倍速/设置面板由 ArtPlayer settings 组件提供, 默认样式已适配触屏, 不再特调 */

/* ===== 播放设置内嵌面板 (控制条齿轮按钮打开) ===== */
.jc-play-settings-panel {
  position: absolute;
  bottom: calc(env(safe-area-inset-bottom, 0px) + 72px);
  right: 8px;
  z-index: 80;
  background: rgba(16, 18, 24, 0.92);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--jc-radius-lg);
  padding: 12px 0;
  min-width: 200px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.6);
  color: rgba(255, 255, 255, 0.92);
}
.jc-play-settings-panel__header {
  font-size: 12px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.5);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  padding: 0 14px 8px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  margin-bottom: 4px;
}
.jc-play-settings-panel__list {
  display: flex;
  flex-direction: column;
}
.jc-play-settings-panel__row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  width: 100%;
  background: none;
  border: none;
  color: rgba(255, 255, 255, 0.85);
  font-size: 14px;
  cursor: pointer;
  text-align: left;
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-play-settings-panel__row:hover {
  background: rgba(255, 255, 255, 0.07);
}
.jc-play-settings-panel__row.is-on {
  color: var(--jc-brand-primary);
}
.jc-play-settings-panel__row.is-disabled {
  opacity: 0.45;
  cursor: default;
}
.jc-play-settings-panel__icon {
  flex: none;
  opacity: 0.85;
}
.jc-play-settings-panel__label {
  flex: 1;
  font-size: 14px;
}
.jc-play-settings-panel__check {
  font-size: 13px;
  opacity: 0.7;
}
.jc-play-settings-panel__close {
  margin-top: 6px;
  padding: 8px 14px;
  width: 100%;
  background: none;
  border: none;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.5);
  font-size: 13px;
  cursor: pointer;
  text-align: center;
  transition: color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-play-settings-panel__close:hover {
  color: rgba(255, 255, 255, 0.85);
}
/* 弹幕设置扩展样式(字号/透明度/区域/速度/同屏/模式开关) */
.jc-play-settings-panel__section {
  padding: 8px 14px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}
.jc-play-settings-panel__section-label {
  font-size: 11px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.45);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  margin-bottom: 8px;
}
.jc-play-settings-panel__segs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.jc-play-settings-panel__seg {
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid rgba(255, 255, 255, 0.15);
  background: rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.75);
  font-size: 12px;
  cursor: pointer;
  transition: all var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-play-settings-panel__seg:hover {
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.95);
}
.jc-play-settings-panel__seg.is-active {
  border-color: var(--jc-brand-primary);
  background: color-mix(in srgb, var(--jc-brand-primary) 22%, transparent);
  color: var(--jc-brand-primary);
}
.jc-play-settings-panel__range {
  width: 100%;
  accent-color: var(--jc-brand-primary);
  height: 4px;
}
.jc-play-settings-panel__grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  padding: 8px 14px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}
.jc-play-settings-panel__toggle {
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.12);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.7);
  font-size: 13px;
  cursor: pointer;
  transition: all var(--jc-dur-fast) var(--jc-ease-standard);
  text-align: center;
}
.jc-play-settings-panel__toggle:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.9);
}
.jc-play-settings-panel__toggle.is-on {
  border-color: color-mix(in srgb, var(--jc-brand-primary) 60%, transparent);
  color: var(--jc-brand-primary);
  background: color-mix(in srgb, var(--jc-brand-primary) 15%, transparent);
}
</style>

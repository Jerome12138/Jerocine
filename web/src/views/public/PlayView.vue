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
  ref,
  watch
} from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { filmApi } from '@/api'
import type { PlayInfo, PlaySource } from '@/types/film'
import { usePlayer } from '@/composables/usePlayer'
import videojs from 'video.js'
import { useFilmHistory, buildPlayLink } from '@/composables/useFilmHistory'
import { useSkipSettings } from '@/composables/useSkipSettings'
import { useHistoryStore } from '@/stores/history'
import { toast } from '@/api/http'
import { useViewMode } from '@/composables/useViewMode'
import { useNetworkHint } from '@/composables/useNetworkHint'
import { useLocalLikes } from '@/composables/useLocalLikes'
import { useFavoriteStore } from '@/stores/favorite'
import { storeToRefs } from 'pinia'
import { normalizeDpadKey } from '@/utils/dpad'
import EpisodeTabs from '@/components/film/EpisodeTabs.vue'
import RelatedList from '@/components/film/RelatedList.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseDialog from '@/components/base/BaseDialog.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import posterFallback from '@/assets/play.svg'

const route = useRoute()
const router = useRouter()
const { isTV, isDesktop } = useViewMode()

/** PC(A方案)按钮"更多"展开态: 一行放不下的低频项收进这里 */
const moreActionsOpen = ref(false)
const historyStore = useHistoryStore()
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

/** 分享: 复制当前 URL 到剪贴板, 短暂展示"已复制"反馈 */
const shareLabel = ref<string>('分享')
async function handleShare(): Promise<void> {
  const url = typeof window !== 'undefined' ? window.location.href : ''
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
/** 当前选中的播放源 ID（与 detail.sources[i].id 对应） */
const currentSourceId = ref<string>('')
/** 当前集索引 */
const currentEpisodeIndex = ref<number>(0)
/** 是否自动播放下一集 */
const autoPlayNext = ref<boolean>(true)
/** 视频源错误提示文本 */
const videoErrorMsg = ref<string>('')

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
 * 广告过滤开关 (localStorage 持久化).
 * 开启后, 若当前 src 是 m3u8, 重写为 `${API}/m3u8/proxy?src=<encoded>`,
 * 由后端拉源后剔除疑似广告 segment 再回吐, video.js 透明消费.
 * 非 m3u8 (mp4 / flv 等) 不重写, 走原始 URL.
 */
const AD_FILTER_LS_KEY = 'gf-ad-filter'
const adFilter = ref<boolean>(
  (() => {
    try {
      const v = localStorage.getItem(AD_FILTER_LS_KEY)
      // 未设置时默认开启 (用户没主动关过, 用最佳体验)
      if (v === null) return true
      return v === '1'
    } catch {
      return true
    }
  })()
)
function toggleAdFilter(): void {
  adFilter.value = !adFilter.value
  if (adFilter.value) {
    toast('success', '广告过滤已开启')
  } else {
    toast('info', '广告过滤已关闭')
  }
}
watch(adFilter, (v) => {
  try {
    localStorage.setItem(AD_FILTER_LS_KEY, v ? '1' : '0')
  } catch {
    /* 隐私模式忽略 */
  }
})

/**
 * 广告过滤结果角标 (I-017, 常驻五态):
 *   off         = 过滤开关未开 → 「过滤未开启」
 *   unsupported = 非 m3u8 源(mp4 等无广告清单) → 「该源无需过滤」
 *   proxy       = 端侧抓流失败(如 CORS)降级服务端代理, 数量未知 → 「服务端过滤中」
 *   clean       = 端侧过滤跑完但未检出 → 「未检出广告」
 *   filtered    = 端侧过滤检出并剔除 N 段 → 「已过滤 N 段广告」
 * 每次集/源切换由 resolvePlaySrc 重置, 常驻播放器右上角。
 */
interface AdFilterBadge {
  kind: 'off' | 'unsupported' | 'proxy' | 'clean' | 'filtered'
  count: number
}
const adFilterBadge = ref<AdFilterBadge | null>(null)
const adBadgeText = computed(() => {
  const b = adFilterBadge.value
  if (!b) return ''
  switch (b.kind) {
    case 'off':
      return '过滤未开启'
    case 'unsupported':
      return '该源无需过滤'
    case 'proxy':
      return '服务端过滤中'
    case 'clean':
      return '未检出广告'
    case 'filtered':
      return `已过滤 ${b.count} 段广告`
  }
})

const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) || '/api'
const reM3u8 = /\.m3u8(\?|#|$)/i

/**
 * 把原始视频 URL 包成"广告过滤代理" URL.
 *   - adFilter=false 或非 m3u8 → 原样返回
 *   - APK 经过 native 调用, 必须返回绝对 URL (data scheme 不识相对)
 */
function wrapAdFilterUrl(originalUrl: string): string {
  if (!originalUrl || !adFilter.value || !reM3u8.test(originalUrl)) return originalUrl
  // 注意补 /v1: API_BASE 仅到 /api, 真实接口在 /api/v1 下(与 http 实例 baseURL 一致)
  return `${absApiBase()}/v1/m3u8/proxy?src=${encodeURIComponent(originalUrl)}`
}

/** 绝对 API base(到 /api). 方案B 下传给原生, 由原生自行拼 /v1/m3u8/proxy. */
function absApiBase(): string {
  const isAbs = /^https?:\/\//i.test(API_BASE)
  return isAbs ? API_BASE : (typeof window !== 'undefined' ? window.location.origin : '') + API_BASE
}

/* ============ Web 端侧混合广告过滤 (端侧抓流 → 服务器仅过滤文本 → blob 清单播放) ============
 *
 * 背景: 旧方案 adFilter 开时把 src 包成 `${API}/v1/m3u8/proxy?src=...`, 由【服务器】抓原始
 * m3u8 再过滤。但 bf 等源被地域封, 服务器抓不到 → 500。改为端侧混合:
 *   a. 浏览器 fetch(原始 m3u8)  ——浏览器没被地域封, 能拿到
 *   b. POST /v1/m3u8/filter?src=<原始url>  body=该文本 → 服务器只剔广告 + 把分片绝对化(不联网)
 *   c. 过滤后文本 → Blob → URL.createObjectURL → blob: URL 喂 video.js (绝对清单可直接播)
 *   d. 降级链: b 步任一失败 → 退回服务端 /m3u8/proxy; 浏览器 fetch 原始失败 → 用原始直链(不过滤)
 *
 * effectiveSrc 由同步 computed 改为【异步求出】写入 playSrc ref(下方 resolvePlaySrc),
 * 由 watch(currentSrc + adFilter) 驱动, 用 token 防竞态(切集快时旧异步结果不回写)。
 */

/** 实际喂给 player 的可播 src(异步求出); video.js 监听此 ref 切换。 */
const playSrc = ref<string>('')
/** playSrc 的 mime(blob 清单同样是 hls, 仍报 application/x-mpegURL)。 */
const playSrcType = ref<string>('')
/** 当前持有的 blob: URL, 切集/卸载/重算前需 revoke 释放, 防内存泄漏。 */
let currentBlobUrl = ''
/** resolvePlaySrc 竞态令牌: 仅最新一次的异步结果允许回写 playSrc。 */
let playSrcToken = 0
/** 上次已提示过"已过滤 N 段"的 url, 防同一集重复弹(web 端由端侧过滤拿响应头后提示)。 */
let lastAdFilterToastUrl = ''
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
 * 新片源就绪后 seek 到本集续播点 + 按需自动续播。
 * 由 watch(playSrc) 在每次切集(playSrc 实际变化)时调用一次; token 去重保证仅首个
 * loadedmetadata/canplay 生效, 且监听挂在"新源加载前", 故即便 VHS 复用 MSE 不重发事件
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
  function run(): void {
    if (fired || myToken !== seekArmToken) return
    fired = true
    offL()
    offC()
    switchInFlight = false // 新片源已就绪, 解除切集互斥
    // 新源就绪事件(loadedmetadata/canplay)已触发: 立即清除切集时挂起的 loading 态,
    // 露出新集画面。放在 seek/play 之前, 即便 player 此刻为空也不会让 loading 卡住。
    loading.value = false
    const p = player.value
    if (!p) return
    try {
      const duration = p.duration() ?? 0
      if (import.meta.env.DEV) {
        console.debug('[PlayView][切集seek]', { resumeAt, duration, autoPlay })
      }
      // duration 未就绪 / 短视频(<5min) / 跳过位置已超末尾, 都不 seek
      if (duration && duration > 300 && resumeAt >= 0 && resumeAt < duration) {
        p.currentTime(resumeAt)
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

/** 服务端代理 URL(降级用): 由服务器抓源 + 过滤。 */
function proxyAdFilterUrl(originalUrl: string): string {
  return `${absApiBase()}/v1/m3u8/proxy?src=${encodeURIComponent(originalUrl)}`
}

/** 端侧混合过滤接口 URL。 */
function clientFilterUrl(originalUrl: string): string {
  return `${absApiBase()}/v1/m3u8/filter?src=${encodeURIComponent(originalUrl)}`
}

/**
 * 抓单张 m3u8 → 送服务器过滤文本 → 返回过滤后文本 + 剔除数。
 * (浏览器抓源无地域封; 服务器只过滤文本不联网, 把分片/子表绝对化。)
 */
async function fetchAndFilter(url: string): Promise<{ text: string; filtered: number }> {
  const raw = await fetch(url, { cache: 'no-store' })
  if (!raw.ok) throw new Error(`fetch m3u8 ${raw.status}`)
  const text = await raw.text()
  if (!text) throw new Error('empty m3u8')
  const resp = await fetch(clientFilterUrl(url), {
    method: 'POST',
    headers: { 'Content-Type': 'application/vnd.apple.mpegurl' },
    body: text,
    cache: 'no-store'
  })
  if (!resp.ok) throw new Error(`filter ${resp.status}`)
  const filteredText = await resp.text()
  if (!filteredText) throw new Error('empty filtered')
  return { text: filteredText, filtered: Number(resp.headers.get('X-Ad-Filtered') ?? '0') || 0 }
}

/**
 * 从(已绝对化的)m3u8 文本取第一个子播放列表(.m3u8)绝对 URL;
 * 若第一个非注释 URI 是分片(.ts 等)则说明这是媒体表, 返回 null。
 */
function firstChildPlaylist(text: string): string | null {
  for (const line of text.split('\n')) {
    const t = line.trim()
    if (!t || t.startsWith('#')) continue
    return /\.m3u8(\?|#|$)/i.test(t) ? t : null // 首个 URI: 子表→跟进; 分片→已是媒体表
  }
  return null
}

/**
 * 端侧混合过滤: 浏览器抓 m3u8 → 服务器只过滤文本 → blob: 清单。
 * bf 等源对不同 IP 可能返回 master 主表(分片在子表里), 故**跟随 master→子表逐层过滤**
 * (单清晰度 VOD 通常 1 层), 否则只过滤主表、带广告的子表会被播放器直连绕过。
 * 成功返回 { url, filtered }; 任一步失败抛错(由 resolvePlaySrc 接住做降级)。
 */
async function clientSideFilter(originalUrl: string): Promise<{ url: string; filtered: number }> {
  let cur = originalUrl
  let text = ''
  let filtered = 0
  for (let depth = 0; depth < 3; depth++) {
    const r = await fetchAndFilter(cur)
    text = r.text
    filtered += r.filtered
    const child = firstChildPlaylist(text)
    if (!child) break // 已是媒体表(含分片), 停止
    cur = child // 是 master, 跟进子表继续过滤(子表才含广告分片)
  }
  // blob 清单(分片已绝对化, 浏览器可直接播)
  const url = URL.createObjectURL(new Blob([text], { type: 'application/vnd.apple.mpegurl' }))
  return { url, filtered }
}

/**
 * 求出当前可播 src 并写入 playSrc(异步)。竞态由 token 保护: 快速切集时旧结果丢弃。
 *   - 非 m3u8 / adFilter 关 → 原始直链(同步路径)
 *   - adFilter 开 + m3u8 → 端侧混合过滤 blob; 失败降级服务端 proxy; 再失败原始直链
 */
async function resolvePlaySrc(originalUrl: string): Promise<void> {
  const token = ++playSrcToken
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

  // adFilter 开 + m3u8: 端侧混合过滤
  try {
    const { url, filtered } = await clientSideFilter(originalUrl)
    if (token !== playSrcToken) {
      try {
        URL.revokeObjectURL(url)
      } catch {
        /* ignore */
      }
      return
    }
    commit(url, 'application/x-mpegURL', true)
    // I-017: 角标常驻反馈过滤结果
    adFilterBadge.value = { kind: filtered > 0 ? 'filtered' : 'clean', count: filtered }
    // 过滤成功提示: 沿用 adFilterSuccessMessage 文案, 数量取响应头 X-Ad-Filtered(不再二次查 stats)
    if (filtered > 0 && originalUrl !== lastAdFilterToastUrl) {
      lastAdFilterToastUrl = originalUrl
      toast('success', adFilterSuccessMessage(filtered))
    }
    return
  } catch {
    if (token !== playSrcToken) return
    // 降级 1: 退回服务端 proxy(服务器能抓的源仍可过滤); 端侧拿不到过滤数, 角标转「服务端过滤中」
    adFilterBadge.value = { kind: 'proxy', count: 0 }
    commit(proxyAdFilterUrl(originalUrl), 'application/x-mpegURL', false)
    // 降级 2(再失败)由 video.js 首错回退兜底(handleVideoError 关过滤走原始直链)
  }
}

/* ============ 线路测速 (实测各源"当前集" m3u8 经代理的加载延时, 选最快线路) ============ */
const lineSpeeds = ref<Record<string, number>>({}) // ms; -1=失败/无 m3u8
const testingLines = ref(false)

/** 强制走 m3u8 代理(同源, 规避跨域), 用于测速真实播放链路 */
function proxyM3u8Url(link: string): string {
  const isAbs = /^https?:\/\//i.test(API_BASE)
  const base = isAbs ? API_BASE : (typeof window !== 'undefined' ? window.location.origin : '') + API_BASE
  return `${base}/v1/m3u8/proxy?src=${encodeURIComponent(link)}`
}

function originAbs(path: string): string {
  return (typeof window !== 'undefined' ? window.location.origin : '') + path
}

/** 限时读取(代理后的) m3u8 文本(同源代理可读) */
async function fetchText(url: string, timeoutMs = 8000): Promise<string> {
  const ctrl = new AbortController()
  const timer = window.setTimeout(() => ctrl.abort(), timeoutMs)
  try {
    const resp = await fetch(url, { signal: ctrl.signal, cache: 'no-store' })
    if (!resp.ok) return ''
    return await resp.text()
  } catch {
    return ''
  } finally {
    window.clearTimeout(timer)
  }
}

/**
 * 从(代理后的) m3u8 解析出第一条真实分片的绝对 URL.
 * 代理重写后: 子播放列表是 `/api/v1/m3u8/proxy?src=...`(同源可读), 分片是 CDN 绝对直链.
 * 遇子播放列表下钻一层(最多 2 层)直到拿到分片.
 */
async function resolveFirstSegment(playlistUrl: string, depth = 0): Promise<string> {
  if (depth > 2) return ''
  const text = await fetchText(playlistUrl)
  if (!text) return ''
  const uris = text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith('#'))
  const first = uris[0]
  if (!first) return ''
  const isProxy = first.includes('/m3u8/proxy?')
  if (isProxy || /\.m3u8(\?|#|$)/i.test(first)) {
    const next = isProxy ? originAbs(first) : proxyM3u8Url(first)
    return resolveFirstSegment(next, depth + 1)
  }
  return first // 绝对 CDN 分片 URL
}

/**
 * 端侧抓"第一片"计时: 浏览器直连 CDN 拉首个分片(这正是真实播放的瓶颈链路).
 * 盗版 CDN 无 CORS → 用 no-cors 不透明请求, 计时到响应可用(首字节往返), 拿到即 abort 不下整片.
 * 注: no-cors 看不到状态码, 测的是"可达 + 首字节延时", 不保证 200.
 */
async function measureSegment(segUrl: string, timeoutMs = 8000): Promise<number> {
  const ctrl = new AbortController()
  const timer = window.setTimeout(() => ctrl.abort(), timeoutMs)
  const t0 = performance.now()
  try {
    await fetch(segUrl, { mode: 'no-cors', cache: 'no-store', signal: ctrl.signal })
    const ms = Math.round(performance.now() - t0)
    ctrl.abort() // 拿到响应即止, 不下整片
    return ms
  } catch {
    return -1
  } finally {
    window.clearTimeout(timer)
  }
}

/** 单线路测速: 解析首片 → 直连 CDN 计时 */
async function measureLine(link: string): Promise<number> {
  if (!reM3u8.test(link)) return -1
  const seg = await resolveFirstSegment(proxyM3u8Url(link))
  if (!seg) return -1
  return measureSegment(seg)
}

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
    const results = await Promise.all(
      sources.map(async (s) => {
        const ep = s.episodes[epIdx] ?? s.episodes[0]
        if (!ep) return { id: s.id, ms: -1 }
        return { id: s.id, ms: await measureLine(ep.link) }
      })
    )
    const map: Record<string, number> = {}
    for (const r of results) map[r.id] = r.ms
    lineSpeeds.value = map
    const ok = results.filter((r) => r.ms >= 0).sort((a, b) => a.ms - b.ms)
    if (ok.length) {
      const name = sources.find((s) => s.id === ok[0]!.id)?.name ?? ''
      toast('success', `测速完成, 最快线路: ${name} (${ok[0]!.ms}ms)`)
    } else {
      toast('warning', '测速完成, 当前集无可用 m3u8 线路')
    }
  } finally {
    testingLines.value = false
  }
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
 * 必然能捕获新源就绪事件, 避免 VHS 复用 MSE 不重发事件导致卡在暂停态 0。
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
  // 重算可播 src; playSrc 变化驱动 video.js 重载
  void resolvePlaySrc(currentSrc.value)
  if (!willChangeSrc || resume <= 0) return
  // 新 src 起播后 seek 回原位置, 体感无中断
  const off = onPlayerEvent('loadedmetadata', () => {
    const p = player.value
    if (!p) return
    try {
      p.currentTime(resume)
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
const videoEl = ref<HTMLVideoElement | null>(null)
const {
  init: initPlayer,
  dispose: disposePlayer,
  play: playerPlay,
  pause: playerPause,
  seekBy: playerSeekBy,
  setVolume: playerSetVolume,
  on: onPlayerEvent,
  player,
  paused,
  buffering,
  currentTime: playerCurrentTime,
  ready: playerReady
} = usePlayer({
  src: playSrc,
  type: playSrcType,
  poster: ref<string | undefined>(posterFallback),
  autoplay: false,
  volume: 0.6,
  playbackRates: [0.5, 1.0, 1.25, 1.5, 2.0],
  // 弱网友好默认: preload metadata 而非 auto, 起播更快; 弱网时启用 VHS 低带宽预设
  preload: 'metadata',
  lowBandwidth: isSlowNetwork.value,
  // I-018: 控制条内常驻「下一集」入口(点击走 playNext, 内部有 hasNext guard)
  nextEpisode: { onClick: () => playNext() }
})

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
      const duration = p?.duration() ?? 0
      if (duration > NO_RECORD_TAIL_SEC) {
        const remaining = duration - (p?.currentTime() ?? 0)
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
      cid: detail.value.cid
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
    loading.value = false
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
  adFilterSuccessMessage,
  buildAdFilterFailureEvent
} from '@/utils/adFilter'

/**
 * 广告过滤代理播放失败 → 上报埋点存链接, 便于后台分析过滤为何挂.
 * 由 web(video.js)/native(ExoPlayer) 两处"回退原片"逻辑调用.
 */
function reportAdFilterFailure(channel: 'web' | 'native'): void {
  const ep = currentEpisode.value
  const originalUrl = ep?.link ?? currentSrc.value
  if (!originalUrl) return
  telemetry.track(
    'error',
    buildAdFilterFailureEvent({
      channel,
      originalUrl,
      proxyUrl: wrapAdFilterUrl(originalUrl),
      sourceId: currentSourceId.value,
      sourceName: currentSource.value?.name,
      episodeIndex: currentEpisodeIndex.value,
      filmId: detail.value ? String(detail.value.mid) : undefined,
      filmName: detail.value?.name
    })
  )
}

/**
 * Android TV 壳 (Jerocine APK) 注入的 JS Bridge.
 * 检测到原生时, PlayView 不渲染 video.js 播放器, 直接交给 ExoPlayer 全屏.
 */
function getNativePlayer(): { playPlaylist?: (cfg: string) => void; playVideo?: (u: string, t: string) => void } | undefined {
  if (typeof window === 'undefined') return undefined
  return (window as unknown as { JerocinePlayer?: { playPlaylist?: (c: string) => void; playVideo?: (u: string, t: string) => void } }).JerocinePlayer
}

/** 跳片头片尾按 filmId 持久化, 没存过用默认 90/60 */
const skipSettings = useSkipSettings()
function currentSkipConfig(): { intro: number; outro: number } {
  const id = detail.value?.mid
  if (id === undefined || id === null) {
    return { intro: skipSettings.DEFAULT_INTRO, outro: skipSettings.DEFAULT_OUTRO }
  }
  return skipSettings.get(id)
}

/** 防止 timeupdate 每秒多次触发 playNext, 用 sourceId/episodeIndex 联合键去抖 */
const outroTriggeredKey = ref('')
/** 片尾跳过前 10s 预告去抖(每集只提示一次) */
const outroPromptKey = ref('')

/** 跳过设置弹窗 */
const skipDialogOpen = ref(false)
const draftIntro = ref(skipSettings.DEFAULT_INTRO)
const draftOutro = ref(skipSettings.DEFAULT_OUTRO)
function openSkipDialog(): void {
  const cfg = currentSkipConfig()
  draftIntro.value = cfg.intro
  draftOutro.value = cfg.outro
  skipDialogOpen.value = true
}
function saveSkip(): void {
  const id = detail.value?.mid
  if (id === undefined || id === null) return
  const cfg = { intro: Number(draftIntro.value) || 0, outro: Number(draftOutro.value) || 0 }
  skipSettings.save(id, cfg)
  skipDialogOpen.value = false
  applySkipLive(cfg) // 正在播则立即生效, 不必等切集
}
function resetSkip(): void {
  const id = detail.value?.mid
  if (id === undefined || id === null) return
  skipSettings.reset(id)
  draftIntro.value = skipSettings.DEFAULT_INTRO
  draftOutro.value = skipSettings.DEFAULT_OUTRO
  skipDialogOpen.value = false
  applySkipLive({ intro: skipSettings.DEFAULT_INTRO, outro: skipSettings.DEFAULT_OUTRO })
}

/**
 * 改跳过设置后立即按新 intro/outro 生效(web video.js; native 走重启不在此路径)。
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
    const duration = p.duration() ?? 0
    const current = p.currentTime() ?? 0
    const INTRO_LIVE_WINDOW = 180 // 当前进度在前 3 分钟内才视为"还在片头区", 可即时前跳
    if (
      cfg.intro > 0 &&
      duration > 300 &&
      current < cfg.intro &&
      current < INTRO_LIVE_WINDOW &&
      cfg.intro < duration
    ) {
      p.currentTime(cfg.intro)
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
    // 统一派发(与详情页 gotoPlay / 路由守卫共用 dispatchNativePlaylist: 映射源+建历史+skip+proxyBase 一处)
    dispatchNativePlaylist(detail.value, src.id, currentEpisodeIndex.value, resumeAt)
    return
  }
  // 老 v1 bridge 兼容路径
  const native = getNativePlayer()
  if (native?.playPlaylist && src) {
    const movieName = detail.value?.name ?? ''
    const episodes = src.episodes.map((e) => ({
      url: wrapAdFilterUrl(e.link),
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
    native.playVideo(wrapAdFilterUrl(ep.link), title)
    return
  }
  currentSrc.value = ep.link
  // 续播目标: 该集自身进度 > 跳片头; 短视频/无记录=0。记录到 pending*, 供新源就绪后 armSeekOnce 使用。
  const seekTo = resumeAt > 0 ? resumeAt : skip.intro
  pendingResumeAt = seekTo
  pendingAutoPlay = autoPlay
  // 旧片源先 pause: 避免切换瞬间旧视频继续播 + 闪旧画面; 新源就绪后 armSeekOnce 负责 seek+续播
  const pOld = player.value
  if (pOld && playerReady.value) {
    try {
      pOld.pause()
    } catch {
      // ignore
    }
  }
  // 标记等待新源: playSrc 实际变化(watch(playSrc))后再 arming 一次, 确保监听挂在新源加载之前,
  // 解决"复用播放器 + VHS 切 HLS 源不重发 loadedmetadata"导致卡在暂停态 0 的问题。
  awaitingNewSource = true
  // 切集/切源立即进入 loading 态: 复用播放器时旧画面会停留到新源就绪, 先盖 loading 层,
  // 待 armSeekOnce 新源就绪(loadedmetadata/canplay)后再清除, 避免"卡在旧画面"的观感。
  loading.value = true
}

/** Web 播放器 timeupdate 监听: 接近片尾自动播下一集 (key 去抖) */
function onPlayerTimeUpdateForSkipOutro(): void {
  const p = player.value
  if (!p) return
  const skip = currentSkipConfig()
  if (skip.outro <= 0 || !hasNext.value) return
  const duration = p.duration() ?? 0
  const current = p.currentTime() ?? 0
  if (duration <= 300 || current <= 0) return
  const remaining = duration - current
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

/** 自动连播入口(ended / 片尾 timeupdate): 切集互斥窗口内忽略 —— 修"连跳 2 集"。
 * 窗口 = selectEpisode 落地 → 新片源就绪(armSeekOnce 生效)。窗口内旧视频残余的
 * timeupdate/ended 事件会以"新集"身份再次通过去抖, 不拦截就会再跳一集,
 * 且把旧播放头写进中间那集的历史(进度串集)。手动按钮不受此限制。 */
function playNextAuto(): void {
  if (switchInFlight) return
  playNext()
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
function handleKeydown(e: KeyboardEvent): void {
  // 输入框聚焦时不拦截
  const target = e.target as HTMLElement | null
  const tag = target?.tagName?.toLowerCase()
  if (tag === 'input' || tag === 'textarea' || target?.isContentEditable) {
    return
  }
  const key = normalizeDpadKey(e)
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
      const cur = player.value?.volume() ?? 0.6
      playerSetVolume(Math.min(1, cur + 0.05))
      break
    }
    case 'ArrowDown': {
      e.preventDefault()
      const cur = player.value?.volume() ?? 0.6
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
    reportAdFilterFailure('web') // 存链接备查
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
        player.value.src({ src: next, type: playSrcType.value || guessMime(next) })
        if (cur > 0) {
          const off = onPlayerEvent('loadedmetadata', () => {
            try {
              player.value?.currentTime(cur)
            } catch {
              /* ignore */
            }
            off()
          })
        }
        void playerPlay()
      } catch {
        // src 调用本身失败极少见; 直接落到换源
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
    return player.value?.currentTime() ?? 0
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
  // PlayView 不再需要渲染 video.js, 立即回详情页 (BACK 退 ExoPlayer 后用户看到详情页, 不是空 PlayView)
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
  if (videoEl.value) {
    initPlayer(videoEl.value)
    onPlayerEvent('ended', () => {
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
  }
  if (typeof window !== 'undefined') {
    window.addEventListener('keydown', handleKeydown)
    // APK native ExoPlayer 报错回调: 方案B 下原生已自行"本集回退原始源", 仅当原始源也失败才会
    // 走到这里(真·失败)。不再全局关广告过滤(那会让一次抖动后所有集都不过滤), 只上报+提示切源。
    ;(window as unknown as { __jerocineNativeError?: () => void }).__jerocineNativeError = () => {
      reportAdFilterFailure('native') // 存链接备查
      toast('error', '播放失败, 请尝试切换其他播放源')
    }
  }
})

onBeforeRouteLeave(() => {
  flushHistory()
  // 卸载播放器（onScopeDispose 也会兜底，但提前 dispose 可避免短暂的画面残留）
  disposePlayer()
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
      videoEl.value?.focus()
    })
  }
})
</script>

<template>
  <div class="gf-play-view container-page py-[var(--gf-space-6)]">
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
      <div class="gf-play-grid">
        <section class="gf-play-grid__main flex flex-col gap-[var(--gf-space-5)]">
          <!-- 播放器容器 -->
          <div class="gf-player-wrap" :data-loading="loading ? '1' : '0'">
            <video
              ref="videoEl"
              class="video-js vjs-default-skin gf-player"
              playsinline
              tabindex="0"
            />
            <!-- I-017: 广告过滤状态角标(常驻, 右上角, 五态) -->
            <div
              v-if="adFilterBadge"
              class="gf-player-adtag"
              :data-kind="adFilterBadge.kind"
            >
              {{ adBadgeText }}
            </div>
            <div v-if="loading || buffering" class="gf-player-loading" role="status" aria-live="polite">
              <span class="gf-player-loading__spinner" />
            </div>
            <div v-if="videoErrorMsg" class="gf-player-error" role="alert">
              {{ videoErrorMsg }}
            </div>
          </div>

          <!-- 当前播放信息 + 控件: 标题单独一行(小字); 标签 + 操作按钮同一行 -->
          <header
            v-if="detail"
            class="gf-play-info flex flex-col gap-[var(--gf-space-2)]"
          >
        <div class="flex flex-col gap-[var(--gf-space-2)] min-w-0">
          <!-- 返回按钮已移除: "查看完整介绍"链接即可回详情页. 影片名小一档(xl→lg) -->
          <h1 class="gf-play-info__title text-[var(--gf-fs-lg)] font-[var(--gf-fw-bold)] text-primary leading-[var(--gf-lh-snug)]">
            {{ detail.name }}
            <span v-if="currentEpisode" class="gf-play-info__episode ml-[var(--gf-space-2)] text-secondary text-[var(--gf-fs-sm)]">
              · {{ currentEpisode.episode }}
            </span>
          </h1>
          <!-- 标签行 + 操作按钮(非TV): 同一行, 标签在左, 按钮靠右 -->
          <div class="flex flex-wrap items-center gap-[var(--gf-space-2)]">
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
              class="gf-play-info__detail-link"
            >
              查看完整介绍 ›
            </RouterLink>
            <!-- 非 TV: 操作按钮放标签行右侧(随 toolbar 块) -->
          </div>
        </div>

        <!-- TV 端 (C 方案): 图标工具条, 一排等宽图标+小字 -->
        <div
          v-if="isTV"
          class="gf-play-toolbar gf-play-toolbar--tv"
          role="toolbar"
          aria-label="播放操作"
        >
          <button type="button" class="gf-pt-btn gf-pt-btn--primary" :disabled="!hasNext" data-focusable="true" @click="playNext">
            <BaseIcon name="skip-next" size="26px" /><span>下一集</span>
          </button>
          <button type="button" class="gf-pt-btn" :class="autoPlayNext ? 'is-on' : ''" :aria-pressed="autoPlayNext" data-focusable="true" @click="autoPlayNext = !autoPlayNext">
            <BaseIcon name="autoplay" size="26px" /><span>连播</span>
          </button>
          <button type="button" class="gf-pt-btn" :class="testingLines ? 'is-loading' : ''" data-focusable="true" @click="testLines">
            <BaseIcon name="refresh" size="26px" /><span>测速</span>
          </button>
          <button v-if="fastestLineId && fastestLineId !== currentSourceId" type="button" class="gf-pt-btn gf-pt-btn--accent" data-focusable="true" @click="switchToFastest">
            <BaseIcon name="skip-next" size="26px" /><span>最快</span>
          </button>
          <button type="button" class="gf-pt-btn" :class="adFilter ? 'is-on' : ''" :aria-pressed="adFilter" data-focusable="true" @click="adFilter = !adFilter">
            <BaseIcon name="shield" size="26px" /><span>广告</span>
          </button>
          <button type="button" class="gf-pt-btn" :class="favorited ? 'is-on' : ''" :aria-pressed="favorited" data-focusable="true" @click="toggleFavorite">
            <BaseIcon name="star" size="26px" /><span>{{ favorited ? '已收藏' : '收藏' }}</span>
          </button>
          <button type="button" class="gf-pt-btn" data-focusable="true" @click="openSkipDialog">
            <BaseIcon name="settings" size="26px" /><span>跳过</span>
          </button>
        </div>

        <!-- PC / 移动 (A 方案): 主操作区只留高频(下一集 / 收藏 / 切到最快);
             自动连播 / 过滤广告 / 线路测速 / 跳过设置 默认收进"更多操作"下拉, 减少拥挤 -->
        <div v-else class="gf-play-toolbar">
          <!-- 组1 播放控制 -->
          <div class="gf-pt-group gf-pt-group--primary">
            <BaseButton variant="gradient" size="sm" :disabled="!hasNext" @click="playNext">
              <template #icon><BaseIcon name="skip-next" size="18px" /></template>
              下一集
            </BaseButton>
          </div>
          <!-- 组2 操作 -->
          <div class="gf-pt-group">
            <BaseButton variant="outline" size="sm" :class="favorited ? 'gf-toggle--on' : ''" :aria-pressed="favorited" @click="toggleFavorite">
              <template #icon><BaseIcon name="star" size="18px" /></template>
              {{ favorited ? '已收藏' : '收藏' }}
            </BaseButton>
            <!-- 切到最快: 仅在有更快线路时出现 -->
            <BaseButton v-if="fastestLineId && fastestLineId !== currentSourceId" variant="ghost" size="sm" @click="switchToFastest">
              <template #icon><BaseIcon name="skip-next" size="18px" /></template>
              切到最快
            </BaseButton>
            <!-- 更多操作: 收纳 自动连播 / 过滤广告 / 线路测速 / 跳过设置 (+移动端分享) -->
            <div class="gf-pt-more">
              <BaseButton variant="outline" size="sm" :aria-expanded="moreActionsOpen" @click="moreActionsOpen = !moreActionsOpen">
                <template #icon><BaseIcon name="menu" size="18px" /></template>
                更多操作
              </BaseButton>
              <!-- 点击空白处关闭 (开关项不关菜单, 故需此遮罩兜底) -->
              <div v-if="moreActionsOpen" class="gf-pt-more__backdrop" @click="moreActionsOpen = false" />
              <Transition name="gf-fade">
                <div v-if="moreActionsOpen" class="gf-pt-more__panel">
                  <!-- 开关项: 点击切换, 不关菜单, 用对勾显示当前态 -->
                  <button type="button" class="gf-pt-more__item" :class="autoPlayNext ? 'is-on' : ''" :aria-pressed="autoPlayNext" @click="autoPlayNext = !autoPlayNext">
                    <BaseIcon name="autoplay" size="16px" />
                    <span class="gf-pt-more__item-label">自动连播</span>
                    <span v-if="autoPlayNext" class="gf-pt-more__check" aria-hidden="true">✓</span>
                  </button>
                  <button type="button" class="gf-pt-more__item" :class="adFilter ? 'is-on' : ''" :aria-pressed="adFilter" @click="adFilter = !adFilter">
                    <BaseIcon name="shield" size="16px" />
                    <span class="gf-pt-more__item-label">过滤广告</span>
                    <span v-if="adFilter" class="gf-pt-more__check" aria-hidden="true">✓</span>
                  </button>
                  <div class="gf-pt-more__sep" aria-hidden="true" />
                  <!-- 动作项: 点击后关菜单 -->
                  <button type="button" class="gf-pt-more__item" :class="testingLines ? 'is-loading' : ''" @click="testLines(); moreActionsOpen = false">
                    <BaseIcon name="refresh" size="16px" />
                    <span class="gf-pt-more__item-label">线路测速</span>
                  </button>
                  <button type="button" class="gf-pt-more__item" @click="openSkipDialog(); moreActionsOpen = false">
                    <BaseIcon name="settings" size="16px" />
                    <span class="gf-pt-more__item-label">跳过设置</span>
                  </button>
                  <button type="button" class="gf-pt-more__item" @click="clearFilmCache">
                    <BaseIcon name="trash" size="16px" />
                    <span class="gf-pt-more__item-label">清理本视频播放缓存</span>
                  </button>
                  <button v-if="!isDesktop" type="button" class="gf-pt-more__item" @click="handleShare(); moreActionsOpen = false">
                    <BaseIcon name="share" size="16px" />
                    <span class="gf-pt-more__item-label">{{ shareLabel }}</span>
                  </button>
                </div>
              </Transition>
            </div>
          </div>
        </div>
      </header>

      <!-- 跳片头片尾设置 (按 filmId 持久化在 localStorage) -->
      <BaseDialog v-model:visible="skipDialogOpen" title="跳过片头/片尾" width="380px">
        <div class="flex flex-col gap-[var(--gf-space-4)]">
          <p class="text-sm text-secondary">
            仅对本剧生效。保存后<strong>立即生效</strong>(片尾阈值即时更新; 若仍在片头区会自动跳过新片头)。
          </p>
          <label class="flex items-center gap-[var(--gf-space-3)]">
            <span class="w-[80px] text-sm">片头 (秒)</span>
            <input
              v-model.number="draftIntro"
              type="number"
              min="0"
              max="600"
              step="10"
              class="flex-1 bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-3)] py-[var(--gf-space-2)] text-base"
              data-focusable="true"
            />
          </label>
          <label class="flex items-center gap-[var(--gf-space-3)]">
            <span class="w-[80px] text-sm">片尾 (秒)</span>
            <input
              v-model.number="draftOutro"
              type="number"
              min="0"
              max="600"
              step="10"
              class="flex-1 bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-3)] py-[var(--gf-space-2)] text-base"
              data-focusable="true"
            />
          </label>
          <p class="text-xs text-muted">默认 90 / 60。设为 0 = 不跳。</p>
        </div>
        <template #footer>
          <BaseButton variant="ghost" @click="resetSkip">恢复默认</BaseButton>
          <BaseButton variant="ghost" @click="skipDialogOpen = false">取消</BaseButton>
          <BaseButton variant="gradient" @click="saveSkip">保存</BaseButton>
        </template>
      </BaseDialog>

        </section>

        <!-- 右侧选集 (PC: 视频右边, 与左列等高, 两层 tab 固定仅集列表滚动; 小屏堆叠到下方). -->
        <aside v-if="detail" class="gf-play-grid__aside">
          <div class="gf-play-grid__aside-inner">
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
      <section v-if="relate.length" class="gf-play-relate mt-[var(--gf-space-8)]">
        <RelatedList :items="relate" title="相关推荐" />
      </section>
    </template>
  </div>
</template>

<style scoped>
.gf-play-view {
  min-height: 60vh;
}

/* 播放器容器：16:9 自适应 */
.gf-player-wrap {
  position: relative;
  width: 100%;
  background-color: #000;
  border-radius: var(--gf-radius-lg);
  overflow: hidden;
  aspect-ratio: 16 / 9;
  box-shadow: var(--gf-shadow-xl);
}

.gf-player {
  position: absolute;
  inset: 0;
  width: 100% !important;
  height: 100% !important;
  outline: none;
}

.gf-player:focus,
.gf-player:focus-visible {
  outline: none;
}

/* 桌面端最大宽度（>= 1280 居中） */
@media (min-width: 1280px) {
  .gf-player-wrap {
    max-width: 1280px;
    margin: 0 auto;
  }
}

/* 加载占位：暗色遮罩 + 胶囊容器内 3 个跳动圆点 + 文字
   (原版仅半透明圆点、无遮罩无 z-index, 亮画面上几乎看不见) */
.gf-player-loading {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  /* 遮罩: 压暗旧画面, 保证 loading 在任何视频底色上都可辨识 */
  background-color: rgba(0, 0, 0, 0.55);
  backdrop-filter: blur(2px);
  z-index: 5;
  pointer-events: none;
}
/* 圆形 spinner: 用显式 px 而非 em —— 本层在 video 外, em 按根字号算不准
   (video.js 的 .video-js{font-size:10px} 只对播放器内部元素生效)。 */
.gf-player-loading__spinner {
  width: 44px;
  height: 44px;
  border-radius: 9999px;
  border: 4px solid rgba(255, 255, 255, 0.25);
  border-top-color: #fff;
  box-shadow: 0 0 12px rgba(255, 255, 255, 0.45);
  animation: gf-play-spin 0.8s linear infinite;
}
@keyframes gf-play-spin {
  to {
    transform: rotate(360deg);
  }
}

.gf-player-error {
  position: absolute;
  left: var(--gf-space-3);
  bottom: var(--gf-space-3);
  padding: var(--gf-space-2) var(--gf-space-3);
  background-color: rgba(0, 0, 0, 0.65);
  color: #fff;
  font-size: var(--gf-fs-sm);
  border-radius: var(--gf-radius-md);
  z-index: 6;
  pointer-events: none;
}

/* 当前播放信息块 */
.gf-play-info__title :deep(a) {
  color: inherit;
  text-decoration: none;
}

/* 自动连播开关激活态 */
.gf-toggle--on {
  color: var(--gf-brand-primary) !important;
  border-color: var(--gf-brand-primary) !important;
}

/* 主体栅格：移动 / 平板 单列；桌面 1024+ 双列 */
/* 标题旁"查看完整介绍"链接 */
.gf-play-info__detail-link {
  display: inline-flex;
  align-items: center;
  color: var(--gf-text-link);
  font-size: var(--gf-fs-xs);
  text-decoration: none;
  padding: 2px 8px;
  border-radius: var(--gf-radius-sm);
  transition: color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-play-info__detail-link:hover,
.gf-play-info__detail-link:focus-visible {
  color: var(--gf-text-link-hover);
  background-color: rgba(74, 209, 229, 0.08);
  outline: none;
}

/* ===== 播放页操作工具条 (A: PC/移动单排分组) ===== */
.gf-play-toolbar {
  display: flex;
  align-items: center;
  gap: var(--gf-space-3);
  flex-wrap: wrap;
  /* 操作按钮整体靠右 */
  justify-content: flex-end;
}
/* 按钮宽度随文字自适应 + 内边距更小 */
.gf-play-toolbar :deep(.gf-btn) {
  width: auto;
  flex: 0 0 auto;
  padding-inline: var(--gf-space-3);
}
.gf-pt-group {
  display: flex;
  align-items: center;
  gap: var(--gf-space-2);
  flex-wrap: wrap;
}
/* "更多操作" 下拉 */
.gf-pt-more {
  position: relative;
}
/* 透明全屏遮罩: 点击空白关闭下拉 (开关项不自关, 故需此兜底) */
.gf-pt-more__backdrop {
  position: fixed;
  inset: 0;
  z-index: calc(var(--gf-z-dropdown) - 1);
}
.gf-pt-more__panel {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: var(--gf-z-dropdown);
  min-width: 184px;
  padding: var(--gf-space-2);
  background-color: var(--gf-bg-surface);
  border: 1px solid var(--gf-border-subtle);
  border-radius: var(--gf-radius-md);
  box-shadow: var(--gf-shadow-lg);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.gf-pt-more__item {
  display: flex;
  align-items: center;
  gap: var(--gf-space-2);
  width: 100%;
  padding: var(--gf-space-2) var(--gf-space-3);
  background: transparent;
  border: none;
  border-radius: var(--gf-radius-sm);
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  cursor: pointer;
  text-align: left;
  transition: background-color var(--gf-dur-fast) var(--gf-ease-standard);
}
/* 文字占满中间, 把对勾推到最右 */
.gf-pt-more__item-label {
  flex: 1 1 auto;
}
/* 开关项已开启: 主题色 + 右侧对勾 */
.gf-pt-more__item.is-on {
  color: var(--gf-brand-primary);
}
.gf-pt-more__check {
  flex: 0 0 auto;
  color: var(--gf-brand-primary);
  font-weight: var(--gf-fw-bold);
}
/* 开关项与动作项之间的分隔线 */
.gf-pt-more__sep {
  height: 1px;
  margin: var(--gf-space-1) var(--gf-space-2);
  background-color: var(--gf-border-subtle);
}
.gf-pt-more__item:hover,
.gf-pt-more__item:focus-visible {
  background-color: rgba(255, 255, 255, 0.06);
  color: var(--gf-text-primary);
  outline: none;
}
.gf-pt-more__item.is-loading {
  opacity: 0.6;
  pointer-events: none;
}

.gf-play-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--gf-space-6);
}

/* 大屏: 左视频, 右选集. 右栏 sticky 跟随视口, 自身固定高度(随视口自适应),
 * 两层 tab 固定不滚, 仅集网格内部纵向滚动 + 鼠标滚轮顺畅. 不再依赖"左列等高"约束,
 * 即使左列(短片/无简介)很矮, 右栏仍有充裕滚动区. */
@media (min-width: 1024px) {
  .gf-play-grid {
    grid-template-columns: minmax(0, 2.6fr) minmax(300px, 1fr);
    align-items: start;
  }
  .gf-play-grid__aside {
    min-width: 0;
    /* sticky: 随页面滚动停在视口上方; 固定高度 = 视口高 - 上下留白, 随视口自适应 */
    position: sticky;
    top: var(--gf-space-6);
    align-self: start;
    height: calc(100vh - var(--gf-space-6) * 2);
    max-height: calc(100vh - var(--gf-space-6) * 2);
    min-height: 0;
    overflow: hidden;
  }
  .gf-play-grid__aside-inner {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  .gf-play-grid__aside :deep(.gf-episodes) {
    height: 100%;
    min-height: 0;
  }
  /* 两层 tab(播放源条 + 集数分段)固定, 不进滚动区 */
  .gf-play-grid__aside :deep(.gf-source-bar),
  .gf-play-grid__aside :deep(.gf-episode-segments) {
    flex: 0 0 auto;
  }
  /* 选源条内的横向滚动容器必须可收缩(否则 flex:0 0 auto 会让它撑满溢出被 aside 裁掉,
     最右的源滚不到). 由其自身 min-width:0 + overflow-x:auto 接管内部横向滚动 */
  .gf-play-grid__aside :deep(.gf-source-tabs) {
    flex: 1 1 0;
    min-width: 0;
  }
  /* 集网格: 占满剩余高度 + 纵向滚动 + 滚轮顺畅. 右侧窄栏每行 2 个(用户指定), 细滚动条 */
  .gf-play-grid__aside :deep(.gf-episode-grid) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    /* clip 另一轴, 避免 CSS 规范把 overflow-y 强制成 auto 造成嵌套滚动 */
    overflow-x: clip;
    overscroll-behavior: contain;
    align-content: start;
    padding-right: var(--gf-space-1);
    scrollbar-width: thin;
    scrollbar-color: rgba(255, 255, 255, 0.2) transparent;
  }
  .gf-play-grid__aside :deep(.gf-episode-grid)::-webkit-scrollbar {
    width: 8px;
  }
  .gf-play-grid__aside :deep(.gf-episode-grid)::-webkit-scrollbar-thumb {
    background-color: rgba(255, 255, 255, 0.2);
    border-radius: 4px;
  }
  .gf-play-grid__aside :deep(.gf-episode-grid)::-webkit-scrollbar-thumb:hover {
    background-color: rgba(255, 255, 255, 0.35);
  }
}

.gf-play-grid__main {
  min-width: 0;
}
.gf-play-grid__aside {
  min-width: 0;
}

.gf-play-synopsis {
  background-color: var(--gf-bg-surface);
  border-radius: var(--gf-radius-md);
  padding: var(--gf-space-4);
}

/* video.js 控件按钮去除白边 */
:deep(video) {
  outline: none !important;
}
:deep(.vjs-tech) {
  border-radius: var(--gf-radius-lg);
}
:deep(.vjs-control-bar) {
  background-color: rgba(0, 0, 0, 0.55);
  font-size: 14px;
}
/* 居中播放按钮只保留 video.js 这个小圆圈 (I-006):
   页面中心原本叠了两个 —— 大的那个来自 poster 占位图 assets/play.svg 里画的圆圈+三角,
   已从 svg 中移除; 这里保留小圆圈作为中心播放入口, 与左下角 playToggle 并存不重复。 */
:deep(.vjs-big-play-button) {
  height: 2em;
  width: 2em;
  line-height: 2em;
  border-radius: 50%;
  border: none;
  background-color: rgba(0, 0, 0, 0.6);
  top: 50%;
  left: 50%;
  margin-top: -1em;
  margin-left: -1em;
}
:deep(.vjs-play-progress) {
  background-color: var(--gf-brand-primary);
}
:deep(.vjs-load-progress div) {
  background-color: rgba(255, 255, 255, 0.45);
}
:deep(.vjs-slider) {
  background-color: rgba(255, 255, 255, 0.18);
}

/* I-017: 广告过滤结果角标(播放器右上角, 常驻不遮操作) */
.gf-player-adtag {
  position: absolute;
  top: var(--gf-space-2);
  right: var(--gf-space-2);
  z-index: 3;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 999px;
  background-color: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  color: rgba(255, 255, 255, 0.92);
  font-size: var(--gf-fs-xs);
  line-height: 1;
  pointer-events: none;
  white-space: nowrap;
}
.gf-player-adtag::before {
  content: '';
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: #34d399;
  box-shadow: 0 0 6px rgba(52, 211, 153, 0.8);
}
/* I-017 五态配色: filtered/clean=绿(默认), off/unsupported=灰, proxy=蓝 */
.gf-player-adtag[data-kind='off']::before,
.gf-player-adtag[data-kind='unsupported']::before {
  background-color: #9ca3af;
  box-shadow: none;
}
.gf-player-adtag[data-kind='proxy']::before {
  background-color: #60a5fa;
  box-shadow: 0 0 6px rgba(96, 165, 250, 0.8);
}

/* I-018: 控制条「下一集」文字按钮(插在倍速与全屏之间) */
:deep(.vjs-next-episode-button) {
  width: auto;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 1em;
  cursor: pointer;
  opacity: 0.85;
}
:deep(.vjs-next-episode-button:hover),
:deep(.vjs-next-episode-button:focus-visible) {
  opacity: 1;
}
:deep(.vjs-next-episode-button__label) {
  font-size: 0.8em;
  font-weight: 600;
  letter-spacing: 0.05em;
  color: #fff;
  user-select: none;
  white-space: nowrap;
}

/* 移动端：标题块换行 + 按钮组靠右 */
@media (max-width: 767px) {
  .gf-play-info {
    align-items: flex-start;
  }
}
</style>

<style>
/* TV 模式覆盖 */
[data-mode='tv'] .gf-player-wrap {
  border-radius: 16px;
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.7);
}
[data-mode='tv'] .gf-play-view {
  padding-block: var(--gf-tv-safe);
  padding-inline: var(--gf-tv-safe);
}
[data-mode='tv'] .gf-play-view .video-js .vjs-control-bar {
  font-size: 18px;
  height: 4em;
}
/* TV 远距离观看: spinner 对齐 TV 下的播放按钮(3em @ font-size:10px = 30px) */
[data-mode='tv'] .gf-player-loading__spinner {
  width: 64px;
  height: 64px;
  border-width: 6px;
}
/* TV: 广告过滤角标放大(远距可读) */
[data-mode='tv'] .gf-player-adtag {
  font-size: 15px;
  padding: 8px 16px;
  gap: 9px;
}
[data-mode='tv'] .gf-player-adtag::before {
  width: 9px;
  height: 9px;
}
/* TV: 控制条「下一集」按钮放大 */
[data-mode='tv'] .video-js .vjs-next-episode-button {
  padding: 0 1.2em;
}
[data-mode='tv'] .video-js .vjs-next-episode-button__label {
  font-size: 15px;
}
[data-mode='tv'] .gf-play-view .vjs-big-play-button {
  height: 3em;
  width: 3em;
  line-height: 3em;
  margin-top: -1.5em;
  margin-left: -1.5em;
}

/* ===== TV 工具条 (C: 图标 + 小字, 一排等宽) ===== */
.gf-play-toolbar--tv {
  display: flex;
  flex-wrap: wrap;
  gap: var(--gf-space-3);
  /* 焦点环不被裁 */
  padding-block: 8px;
}
.gf-pt-btn {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 88px;
  padding: var(--gf-space-3) var(--gf-space-3);
  background-color: var(--gf-bg-elevated);
  border: none;
  border-radius: var(--gf-radius-md);
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-medium);
  cursor: pointer;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-pt-btn span {
  font-size: var(--gf-fs-sm);
  white-space: nowrap;
}
.gf-pt-btn.is-on {
  color: var(--gf-brand-cyan);
}
.gf-pt-btn--primary {
  background-image: var(--gf-brand-gradient);
  color: #fff;
}
.gf-pt-btn--accent {
  color: var(--gf-brand-cyan);
  border: 1px solid var(--gf-brand-cyan);
}
.gf-pt-btn:disabled {
  opacity: 0.4;
  pointer-events: none;
}
.gf-pt-btn.is-loading {
  opacity: 0.6;
  pointer-events: none;
}
/* TV 焦点: 细环(不被裁), 统一 token */
[data-mode='tv'] .gf-pt-btn:focus,
[data-mode='tv'] .gf-pt-btn:focus-visible {
  outline: none;
  box-shadow: var(--gf-tv-focus-ring);
  background-color: rgba(255, 255, 255, 0.1);
  color: var(--gf-text-primary);
}
</style>

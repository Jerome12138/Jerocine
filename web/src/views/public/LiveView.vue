<script setup lang="ts">
/**
 * 直播页：直播源切换 + 分组频道列表 + 直播播放。
 *  - 播放复用 usePlayer（isLive 模式：隐藏进度/时长，hls.js 直播跟随）
 *  - 频道数据 = 服务端解析 m3u（带缓存），切源 = 切 liveSourceId 重新拉频道
 *  - TV 模式：频道/源/分组均为 data-focusable，遥控器空间导航可用
 */
import { computed, onBeforeUnmount, onMounted, ref, watch, nextTick } from 'vue'
import { liveApi } from '@/api'
import type { LiveChannel, LiveSource } from '@/types/live'
import { groupChannels } from '@/utils/live'
import { usePlayer } from '@/composables/usePlayer'
import { useViewMode } from '@/composables/useViewMode'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'

const { isTV, isMobile } = useViewMode()

/* ---------- 数据 ---------- */
const sources = ref<LiveSource[]>([])
const activeSourceId = ref<number>(0)
const channels = ref<LiveChannel[]>([])
const channelsLoading = ref(false)
const channelsError = ref('')
const sourcesLoading = ref(true)
const sourcesError = ref('')

/** 活跃源（未选中时取第一个） */
const activeSource = computed<LiveSource | null>(() => {
  if (activeSourceId.value) {
    return sources.value.find((s) => s.id === activeSourceId.value) ?? null
  }
  return sources.value[0] ?? null
})

/** 分组视图（保序聚合） */
const groups = computed(() => groupChannels(channels.value))

/** 频道可用性判定（端侧探源结果; 未探测=null 不标记）:
 *  仅 cors===false 是硬约束(浏览器跨域必拒)。
 *  alive 等端侧探测结果后才有意义，不单独作为禁用条件。 */
function channelUnavailable(ch: LiveChannel): boolean {
  if (ch.cors === false) return true
  return false
}
/** 角标文案: 端侧探源标记 */
function channelBadgeText(ch: LiveChannel): string {
  if (ch.cors === false) return '跨域受限'
  if (ch.alive === false) return '不可达'
  return ''
}
/** 当前选中频道（回显高亮 + 记忆恢复用 localStorage） */
const activeUrl = ref('')
const LAST_CHANNEL_KEY = 'jc-live-last-channel'
/** 端侧探源结果缓存（sourceId → { urlHash → ProbeResult }），过期 24h */
interface ProbeResult { url: string; alive: boolean; cors: boolean }
const PROBE_CACHE_KEY = 'jc-live-probe-cache'
const PROBE_CACHE_TTL_MS = 24 * 60 * 60 * 1000
const PROBE_TIMEOUT_MS = 5000
function fnv1aHash(s: string): string {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return (h >>> 0).toString(36)
}
function readProbeCache(): Record<number, Record<string, ProbeResult & { ts: number }>> {
  try {
    const raw = localStorage.getItem(PROBE_CACHE_KEY)
    if (!raw) return {}
    return JSON.parse(raw) as Record<number, Record<string, ProbeResult & { ts: number }>>
  } catch {
    return {}
  }
}
function writeProbeCache(cache: ReturnType<typeof readProbeCache>): void {
  try {
    localStorage.setItem(PROBE_CACHE_KEY, JSON.stringify(cache))
  } catch {
    /* ignore */
  }
}
async function probeChannels(sourceId: number, chs: LiveChannel[]): Promise<void> {
  // 全部已探过且未过期 → 跳过
  const cache = readProbeCache()
  const srcCache = cache[sourceId] ?? {}
  const dirty = chs.filter((c) => {
    const stored = srcCache[fnv1aHash(c.url)]
    return !stored || Date.now() - stored.ts > PROBE_CACHE_TTL_MS
  })
  if (dirty.length === 0) return
  // 并发探源: fetch(no-cors) 成功 = 网络可达; TypeError = CORS 拦截/协议拒绝; AbortError = 超时
  const results = await Promise.allSettled(
    dirty.map((ch) => {
      const ctrl = new AbortController()
      const timer = setTimeout(() => ctrl.abort(), PROBE_TIMEOUT_MS)
      return fetch(ch.url, { mode: 'no-cors', signal: ctrl.signal, redirect: 'follow' })
        .then(() => {
          clearTimeout(timer)
          return { url: ch.url, alive: true, cors: true } as ProbeResult
        })
        .catch((err): ProbeResult => {
          clearTimeout(timer)
          if (err.name === 'AbortError') return { url: ch.url, alive: false, cors: false }
          // no-cors 模式下 fetch 只会在网络错误/CORS 拦截时抛 TypeError
          // 区分 CORS 拦截(SecurityError) 和其他网络错误
          const isCors = err instanceof DOMException && err.name === 'SecurityError'
          return { url: ch.url, alive: !isCors, cors: false }
        })
    }),
  )
  // 合并结果到缓存 + 更新 channels 数组
  const merged = { ...srcCache }
  for (const ch of dirty) {
    delete merged[fnv1aHash(ch.url)] // 清除旧条目
  }
  for (const r of results) {
    if (r.status === 'fulfilled') {
      const result = r.value
      const key = fnv1aHash(result.url)
      merged[key] = { ...result, ts: Date.now() }
      const ch = chs.find((c) => c.url === result.url)
      if (ch) {
        ch.alive = result.alive
        ch.cors = result.cors
      }
    }
  }
  cache[sourceId] = merged
  writeProbeCache(cache)
}

/** 频道按 HLS 装载：IPTV 地址大量无 .m3u8 后缀（带 token/查询串），显式声明 mime
 *  让 usePlayer 走 hls.js 而非 video.src 直链 */
const HLS_TYPE = 'application/x-mpegURL'

function selectSource(id: number): void {
  if (id === activeSourceId.value) return
  activeSourceId.value = id
  activeUrl.value = ''
  // 真正停播: usePlayer 的 src watcher 对空串 early-return(仅清值是 no-op), 旧 hls 若不
  // 显式卸载会以直播跟随模式继续后台拉流 —— ArtPlayer reset() = removeAttribute src + load。
  playerPause()
  try {
    player.value?.reset()
  } catch {
    /* ignore */
  }
  playSrc.value = ''
  void loadChannels(id)
}

/** 点击频道后的待起播标志: 用户点击 = 有手势, 但自动播放需等装载完成(autoplay:false 时
 *  ArtPlayer 装载新源不会自动起播), 等 canplay 后补一次 play(见 onPlayerEvent('canplay'))。 */
let pendingPlay = false

function selectChannel(ch: LiveChannel): void {
  activeUrl.value = ch.url
  pendingPlay = true
  // 同 URL 重复点击(hls fatal 自毁后重试): playSrc watcher 对同值不触发 —— 强制走 load
  if (playSrc.value === ch.url && errored.value) {
    playerLoad(ch.url, HLS_TYPE)
    return
  }
  playSrc.value = ch.url
  try {
    localStorage.setItem(LAST_CHANNEL_KEY, JSON.stringify({ sourceId: activeSourceId.value, url: ch.url }))
  } catch {
    /* 隐私模式忽略 */
  }
}

/* ---------- 播放器（isLive） ---------- */
const playerEl = ref<HTMLDivElement | null>(null)
const playSrc = ref('')
const buffering = ref(false)
const {
  init: initPlayer,
  dispose: disposePlayer,
  pause: playerPause,
  load: playerLoad,
  on: onPlayerEvent,
  player,
  errored
} = usePlayer({
  src: playSrc,
  type: HLS_TYPE,
  // 不传 poster: ArtPlayer option 校验对显式 undefined 会抛 Type Error(播放器构造失败 → 点击频道无反应)
  autoplay: false,
  volume: 0.6,
  // 直播模式: 隐藏进度/时长, hls.js 直播跟随
  isLive: true,
  // 移动端隐藏画中画/网页全屏(与 PlayView 同口径)
  isMobile: isMobile.value,
  preload: 'metadata'
})

onPlayerEvent('waiting', () => {
  buffering.value = true
})
onPlayerEvent('playing', () => {
  pendingPlay = false
  buffering.value = false
})
onPlayerEvent('canplay', () => {
  // 点击频道后装载完成 → 补起播(见 selectChannel pendingPlay 注释)。
  // 用 art 原生 play(): usePlayer.playerPlay 会吞掉 NotAllowedError, 无法感知失败降级。
  if (!pendingPlay) return
  pendingPlay = false
  const p = player.value
  if (!p) return
  const ret = p.play() as Promise<void> | undefined
  if (ret && typeof ret.catch === 'function') {
    ret.catch(() => {
      // 手势激活窗口已过(慢流装载超时) → 浏览器拒绝有声播放 → 静音降级起播,
      // 用户点击控制条音量按钮(手势)即可恢复声音
      p.muted = true
      const r2 = p.play() as Promise<void> | undefined
      void r2?.catch(() => undefined)
      try {
        p.notice.show = '已静音起播，点击音量按钮开启声音'
      } catch {
        /* ignore */
      }
    })
  }
})
onPlayerEvent('error', () => {
  // 直播源失效常见：频道离线/区域限制 —— 提示换频道
  console.warn('[live] player error, url =', playSrc.value)
  buffering.value = false
})

async function loadChannels(sourceId: number): Promise<void> {
  if (!sourceId) return
  channelsLoading.value = true
  channelsError.value = ''
  channels.value = []
  try {
    const resp = await liveApi.getLiveChannels(sourceId)
    channels.value = resp.channels
    if (resp.channels.length === 0) {
      channelsError.value = '该直播源暂无可用频道'
    } else {
      // 端侧探源: 仅对未缓存/过期条目探测, 不阻塞频道列表展示
      void probeChannels(sourceId, channels.value)
    }
  } catch {
    channelsError.value = '频道列表加载失败，请稍后重试'
  } finally {
    channelsLoading.value = false
  }
}

/** 读取上次观看记忆（源 + 频道地址）。 */
function readLast(): { sourceId?: number; url?: string } | null {
  try {
    const raw = localStorage.getItem(LAST_CHANNEL_KEY)
    return raw ? (JSON.parse(raw) as { sourceId?: number; url?: string }) : null
  } catch {
    return null
  }
}

/** 频道列表就绪后恢复上次频道（只装载画面不自动播放 —— 浏览器自动播放策略不允许） */
function restoreUrlIn(list: LiveChannel[]): void {
  const saved = readLast()
  if (saved?.url && list.some((c) => c.url === saved.url)) {
    activeUrl.value = saved.url
    playSrc.value = saved.url
  }
}

onMounted(async () => {
  // 先读记忆源（loadSources 内在 sources 落地后校正 activeSourceId），
  // 再按该源拉频道列表，频道就绪后恢复上次频道
  const savedSourceId = readLast()?.sourceId
  sourcesLoading.value = true
  sourcesError.value = ''
  try {
    const resp = await liveApi.getLiveSources()
    sources.value = resp.sources
    if (resp.sources.length > 0) {
      // 记忆源优先；无记忆(或不存在的源)落到第一个源 —— activeSourceId 必须显式落值,
      // 否则 selectChannel 写入的记忆 sourceId 恒为 0
      if (savedSourceId && resp.sources.some((s) => s.id === savedSourceId)) {
        activeSourceId.value = savedSourceId
      } else {
        activeSourceId.value = resp.sources[0]!.id
      }
      await loadChannels(activeSourceId.value)
      restoreUrlIn(channels.value)
    }
  } catch {
    sourcesError.value = '直播源加载失败，请稍后重试'
  } finally {
    sourcesLoading.value = false
  }
  await nextTick()
  if (playerEl.value) {
    initPlayer(playerEl.value)
  }
})

onBeforeUnmount(() => {
  disposePlayer()
})

const pageTitle = '直播'
</script>

<template>
  <div class="jc-live-view container-page py-[var(--jc-space-6)]">
    <h1 class="jc-live-title text-[length:var(--jc-fs-lg)] font-[var(--jc-fw-bold)]">{{ pageTitle }}</h1>

    <BaseEmpty
      v-if="!sourcesLoading && sources.length === 0"
      :title="sourcesError || '暂无直播源'"
      description="可在管理后台「直播源」中添加"
    />

    <div v-else class="jc-live-grid">
      <!-- 播放器区 -->
      <section class="jc-live-main">
        <div class="jc-live-player-wrap">
          <div ref="playerEl" class="jc-live-player" tabindex="0"></div>
          <div v-if="channelsLoading || !playSrc" class="jc-live-loading" role="status" aria-live="polite">
            <BaseSkeleton variant="rect" class="jc-live-loading__box" />
            <span class="jc-live-loading__text">{{ channelsLoading ? '正在加载频道…' : '选择一个频道开始观看' }}</span>
          </div>
          <!-- 播放中缓冲 indicator (直播流加载慢时给用户反馈) -->
          <div v-if="buffering" class="jc-live-buffering" role="status" aria-live="polite">
            <span class="jc-live-buffering__spinner" />
            <span class="jc-live-buffering__text">缓冲中…</span>
          </div>
        </div>
        <!-- 源切换（多源时展示） -->
        <div v-if="sources.length > 1" class="jc-live-sources" role="tablist" aria-label="直播源切换">
          <button
            v-for="s in sources"
            :key="s.id"
            type="button"
            class="jc-live-source-chip"
            :class="{ 'is-active': s.id === activeSource?.id }"
            :data-focusable="isTV ? 'true' : undefined"
            role="tab"
            :aria-selected="s.id === activeSource?.id"
            @click="selectSource(s.id)"
          >
            {{ s.name }}
          </button>
        </div>
        <p v-if="errored" class="jc-live-error" role="alert">当前频道播放失败，请尝试其他频道</p>
      </section>

      <!-- 频道区 -->
      <aside class="jc-live-side" aria-label="频道列表">
        <BaseSkeleton v-if="channelsLoading" variant="rect" class="jc-live-side-skeleton" />
        <template v-else>
          <p v-if="channelsError" class="jc-live-side__empty">{{ channelsError }}</p>
          <div v-for="g in groups" :key="g.name" class="jc-live-group">
            <h2 class="jc-live-group__title">{{ g.name }}<span class="jc-live-group__count">{{ g.channels.length }}</span></h2>
            <div class="jc-live-group__list">
              <button
                v-for="ch in g.channels"
                :key="ch.url"
                type="button"
                class="jc-live-channel"
                :class="{ 'is-active': ch.url === activeUrl, 'is-unavailable': channelUnavailable(ch) }"
                :title="channelUnavailable(ch) ? channelBadgeText(ch) : undefined"
                :data-focusable="isTV ? 'true' : undefined"
                @click="selectChannel(ch)"
              >
                <img v-if="ch.logo" :src="ch.logo" alt="" class="jc-live-channel__logo" loading="lazy" />
                <span class="jc-live-channel__name">{{ ch.name }}</span>
                <!-- 探源标记: CORS 不通(浏览器拉不到) / 流离线 —— 仍可点击强试, 置灰+角标提示 -->
                <span v-if="channelUnavailable(ch)" class="jc-live-channel__badge">{{ channelBadgeText(ch) }}</span>
              </button>
            </div>
          </div>
        </template>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.jc-live-title {
  margin-bottom: var(--jc-space-4);
}

/* 主栅格: lg+ 左播放器/右频道列表; 小屏堆叠 */
.jc-live-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: var(--jc-space-5);
}
@media (min-width: 1024px) {
  .jc-live-grid {
    grid-template-columns: minmax(0, 1fr) 320px;
    align-items: start;
  }
}

.jc-live-main {
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-3);
  min-width: 0;
}

.jc-live-player-wrap {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  background-color: #000;
  border-radius: var(--jc-radius-lg);
  overflow: hidden;
  box-shadow: var(--jc-shadow-xl);
}
.jc-live-player {
  position: absolute;
  inset: 0;
  width: 100% !important;
  height: 100% !important;
  outline: none;
}
/* ArtPlayer 主题接入品牌色（直播页同款皮肤） */
:deep(.art-video-player) {
  --art-theme: var(--jc-brand-primary);
}
:deep(.art-loading) {
  display: none !important;
}

.jc-live-loading {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  background-color: rgba(0, 0, 0, 0.55);
  z-index: 70;
  pointer-events: none;
}
.jc-live-loading__box {
  width: 160px;
  height: 12px;
}
.jc-live-loading__text {
  color: rgba(255, 255, 255, 0.92);
  font-size: 13px;
}

.jc-live-sources {
  display: flex;
  flex-wrap: wrap;
  gap: var(--jc-space-2);
}
.jc-live-source-chip {
  padding: 6px 14px;
  border-radius: 999px;
  background-color: var(--jc-bg-surface);
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  cursor: pointer;
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard), color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-live-source-chip.is-active {
  background-image: var(--jc-brand-gradient);
  color: #fff;
}
.jc-live-error {
  color: var(--jc-danger, #ef4444);
  font-size: var(--jc-fs-sm);
}
/* 直播缓冲中 overlay (播放中 rebuffer 时显示 spinner + 文案) */
.jc-live-buffering {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  background-color: rgba(0, 0, 0, 0.45);
  z-index: 70;
  pointer-events: none;
}
.jc-live-buffering__spinner {
  width: 36px;
  height: 36px;
  border: 3px solid rgba(255, 255, 255, 0.2);
  border-top-color: rgba(255, 255, 255, 0.85);
  border-radius: 50%;
  animation: jc-live-spin 0.8s linear infinite;
}
@keyframes jc-live-spin {
  to { transform: rotate(360deg); }
}
.jc-live-buffering__text {
  color: rgba(255, 255, 255, 0.85);
  font-size: 13px;
}

/* 频道列表 */
.jc-live-side {
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-4);
  max-height: none;
}
@media (min-width: 1024px) {
  .jc-live-side {
    max-height: calc(56.25vw * 0.62 + 220px);
    overflow-y: auto;
    padding-right: 4px;
  }
}
.jc-live-side-skeleton {
  height: 320px;
}
.jc-live-side__empty {
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  padding: var(--jc-space-4);
}
.jc-live-group__title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-semibold);
  color: var(--jc-text-secondary);
  margin-bottom: var(--jc-space-2);
}
.jc-live-group__count {
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-tertiary, var(--jc-text-secondary));
}
.jc-live-group__list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: var(--jc-space-2);
}
.jc-live-channel {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: var(--jc-radius-md);
  background-color: var(--jc-bg-surface);
  border: 1px solid transparent;
  color: var(--jc-text-primary);
  font-size: var(--jc-fs-sm);
  cursor: pointer;
  text-align: left;
  overflow: hidden;
  transition: border-color var(--jc-dur-fast) var(--jc-ease-standard), background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-live-channel:hover {
  background-color: var(--jc-bg-elevated);
}
/* 探源不可用频道: 置灰 + 角标(仍可点击强试, 用户可能有自己的代理环境) */
.jc-live-channel.is-unavailable {
  opacity: 0.45;
  filter: grayscale(0.9);
}
.jc-live-channel.is-unavailable:hover {
  opacity: 0.7;
}
.jc-live-channel__badge {
  margin-left: auto;
  flex-shrink: 0;
  font-size: 11px;
  line-height: 1;
  padding: 3px 6px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  color: var(--jc-text-tertiary, rgba(255, 255, 255, 0.5));
  white-space: nowrap;
}
.jc-live-channel.is-active {
  border-color: var(--jc-brand-primary);
  color: var(--jc-brand-primary);
}
.jc-live-channel__logo {
  width: 28px;
  height: 28px;
  object-fit: contain;
  flex: none;
  border-radius: 4px;
}
.jc-live-channel__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* TV: 焦点环 */
[data-mode='tv'] .jc-live-channel:focus,
[data-mode='tv'] .jc-live-channel:focus-visible,
[data-mode='tv'] .jc-live-source-chip:focus,
[data-mode='tv'] .jc-live-source-chip:focus-visible {
  outline: none;
  box-shadow: var(--jc-tv-focus-ring);
}
</style>

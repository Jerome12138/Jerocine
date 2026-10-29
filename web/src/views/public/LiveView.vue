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

const { isTV } = useViewMode()

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
/** 当前选中频道（回显高亮 + 记忆恢复用 localStorage） */
const activeUrl = ref('')
const LAST_CHANNEL_KEY = 'jc-live-last-channel'
/** 频道按 HLS 装载：IPTV 地址大量无 .m3u8 后缀（带 token/查询串），显式声明 mime
 *  让 usePlayer 走 hls.js 而非 video.src 直链 */
const HLS_TYPE = 'application/x-mpegURL'

function selectSource(id: number): void {
  if (id === activeSourceId.value) return
  activeSourceId.value = id
  activeUrl.value = ''
  playSrc.value = '' // 停掉旧源画面（无高亮的"僵尸播放"）
  void loadChannels(id)
}

function selectChannel(ch: LiveChannel): void {
  activeUrl.value = ch.url
  playSrc.value = ch.url
  // 用户点击 = 已有手势，自动起播
  void playerPlay()
  try {
    localStorage.setItem(LAST_CHANNEL_KEY, JSON.stringify({ sourceId: activeSourceId.value, url: ch.url }))
  } catch {
    /* 隐私模式忽略 */
  }
}

/* ---------- 播放器（isLive） ---------- */
const playerEl = ref<HTMLDivElement | null>(null)
const playSrc = ref('')
const {
  init: initPlayer,
  dispose: disposePlayer,
  play: playerPlay,
  on: onPlayerEvent,
  errored
} = usePlayer({
  src: playSrc,
  type: HLS_TYPE,
  poster: undefined,
  autoplay: false,
  volume: 0.6,
  // 直播模式: 隐藏进度/时长, hls.js 直播跟随
  isLive: true,
  preload: 'metadata'
})

onPlayerEvent('error', () => {
  // 直播源失效常见：频道离线/区域限制 —— 提示换频道
  console.warn('[live] player error, url =', playSrc.value)
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
                :class="{ 'is-active': ch.url === activeUrl }"
                :data-focusable="isTV ? 'true' : undefined"
                @click="selectChannel(ch)"
              >
                <img v-if="ch.logo" :src="ch.logo" alt="" class="jc-live-channel__logo" loading="lazy" />
                <span class="jc-live-channel__name">{{ ch.name }}</span>
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

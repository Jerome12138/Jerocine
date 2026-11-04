<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useHistoryStore, useUserStore } from '@/stores'
import type { HistoryRecord } from '@/stores/history'
import { buildPlayLink, recordToCard } from '@/stores/history'
import { clearLiveLast, liveLastLabel, readLiveLast, LIVE_COVER_SVG, LIVE_HISTORY_ID } from '@/utils/live'
import { useViewMode } from '@/composables/useViewMode'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import EpisodeBadge from '@/components/base/EpisodeBadge.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import { confirm } from '@/composables/useConfirm'
import {
  groupByTimeBucket,
  progressPercent,
  episodeLabel,
  formatRelativeTime
} from '@/composables/useTimeBucket'

const historyStore = useHistoryStore()
const userStore = useUserStore()
const { list, remoteMode, remoteLoading } = storeToRefs(historyStore)
const { isLoggedIn } = storeToRefs(userStore)
const { isTV } = useViewMode()

/** TV「管理」模式: 开启后点卡片=删除该条 (遥控器点不中海报上的小✕角标, 改整卡删除) */
const manageMode = ref(false)

/**
 * 观看历史里的「直播」置顶条目：把整个直播当一个影片，**只记最新一条**（本地专用，不入后端历史）。
 * 数据来自上次直播记忆；点击回 /live（原生壳由路由守卫转原生播放器）。
 */
const liveRecord = ref<HistoryRecord | null>(null)
function syncLiveRecord(): void {
  const last = readLiveLast()
  liveRecord.value = last
    ? {
        id: LIVE_HISTORY_ID,
        name: '直播',
        link: '/live',
        episode: liveLastLabel(last),
        // 用真实播放时间，让它按时间参与排序（不置顶）
        timeStamp: last.ts ?? Date.now(),
        picture: LIVE_COVER_SVG
      }
    : null
}
syncLiveRecord()

/** 影片历史 + 「直播」条目，统一按播放时间倒序（直播不置顶） */
const items = computed(() => {
  const live = liveRecord.value
  if (!live) return list.value
  return [...list.value, live].sort((a, b) => (b.timeStamp ?? 0) - (a.timeStamp ?? 0))
})

/** 按时间分桶 (今天/本周/本月/更早) */
const groups = computed(() => groupByTimeBucket(items.value))
const sourceLabel = computed(() =>
  remoteMode.value ? '云端历史 · 跨设备同步' : '本地历史 · 仅当前浏览器'
)

function formatTime(ts: number): string {
  if (!ts) return ''
  const d = new Date(ts)
  const now = Date.now()
  const diff = now - ts
  const day = 24 * 60 * 60 * 1000
  if (diff < day) {
    return `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
  }
  if (diff < 7 * day) {
    return `${Math.floor(diff / day)} 天前`
  }
  return `${d.getMonth() + 1}/${d.getDate()}`
}

function formatProgress(seconds?: number): string {
  if (!seconds || seconds < 1) return ''
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

async function handleClear(): Promise<void> {
  if (!items.value.length) return
  const ok = await confirm({
    title: '确认清空全部观看历史？',
    desc: '此操作不可恢复',
    okText: '清空',
    danger: true
  })
  if (ok) {
    historyStore.clear()
    // 「直播」条目不属后端历史，是本地专用 —— 清空历史时一并清掉
    clearLiveLast()
    syncLiveRecord()
  }
}

function handleRemove(id: string, e: Event): void {
  e.preventDefault()
  e.stopPropagation()
  historyStore.remove(id)
}
</script>

<template>
  <!-- ============ TV (雷鸟卡片式) 分支 ============ -->
  <section v-if="isTV" class="jc-tv-history">
    <!-- 操作条: 标题 + 来源标签 + 共N条 + 登录同步 + 清空 -->
    <!-- 横排样式内联写死(不依赖 [data-mode=tv] 全局块, 保证 TV 壳任何环境生效):
         bar=flex 行, head-left(标题+meta)=nowrap 横排, ops 靠右 -->
    <div
      class="jc-tv-history__bar"
      style="display: flex; align-items: center; gap: 14px; flex-wrap: wrap;"
    >
      <div
        class="jc-tv-history__head-left"
        style="display: flex; align-items: center; gap: 12px; flex: 1 1 auto; min-width: 0; flex-wrap: nowrap;"
      >
        <div
          class="jc-tv-history__title"
          style="white-space: nowrap; flex-shrink: 0;"
        >观看历史</div>
        <div
          class="jc-tv-history__meta"
          style="display: flex; align-items: center; gap: 10px; flex-wrap: nowrap; min-width: 0; overflow: hidden; white-space: nowrap;"
        >
          <span class="jc-tv-chip sel jc-tv-history__srcchip" style="flex-shrink: 0;">
            {{ remoteMode ? '云端' : '本地' }}
          </span>
          <span style="min-width: 0; overflow: hidden; text-overflow: ellipsis;">{{ sourceLabel }}</span>
          <span style="flex-shrink: 0;">·</span>
          <span style="flex-shrink: 0;">共 {{ items.length }} 条</span>
          <span v-if="remoteLoading" class="jc-tv-history__syncing" style="flex-shrink: 0;">同步中…</span>
        </div>
      </div>
      <div
        class="jc-tv-history__ops"
        style="margin-left: auto; display: flex; align-items: center; gap: 10px; flex-shrink: 0;"
      >
        <RouterLink
          v-if="!isLoggedIn"
          to="/login"
          class="jc-tv-chip"
          data-focusable="true"
          tabindex="0"
        >
          登录以云端同步
        </RouterLink>
        <button
          v-if="items.length"
          type="button"
          class="jc-tv-btn jc-tv-history__manage"
          :class="{ cyan: manageMode }"
          data-focusable="true"
          tabindex="0"
          @click="manageMode = !manageMode"
        >
          {{ manageMode ? '✓ 完成' : '管理删除' }}
        </button>
        <button
          v-if="items.length"
          type="button"
          class="jc-tv-btn jc-tv-history__clear"
          data-focusable="true"
          tabindex="0"
          @click="handleClear"
        >
          <BaseIcon name="trash" size="18px" />
          清空
        </button>
      </div>
    </div>

    <BaseEmpty
      v-if="!items.length"
      title="还没有观看记录"
      description="去首页找一部喜欢的影片开始观看吧"
    />

    <template v-else>
      <section
        v-for="group in groups"
        :key="group.bucket"
        class="jc-tv-history__group"
      >
        <div class="jc-tv-sec">
          <span class="t">{{ group.label }}</span>
          <span class="s">{{ group.items.length }} 条</span>
        </div>
        <div class="jc-tv-grid">
          <RouterLink
            v-for="record in group.items"
            :key="record.id"
            v-slot="{ navigate }"
            :to="buildPlayLink(record)"
            custom
          >
            <div
              class="jc-tv-card jc-tv-hcard"
              :class="{ 'is-manage': manageMode }"
              data-focusable="true"
              tabindex="0"
              :aria-label="manageMode ? `删除 ${record.name}` : record.name"
              @click="manageMode ? handleRemove(record.id, $event) : navigate()"
              @keydown.enter="manageMode ? handleRemove(record.id, $event) : navigate()"
            >
            <div class="poster h">
              <BaseImage
                :src="record.picture || ''"
                :alt="record.name"
                ratio="16/9"
                fit="cover"
              />
              <span
                v-if="episodeLabel(record.episode, record.episodeIndex)"
                class="ep"
              >
                {{ episodeLabel(record.episode, record.episodeIndex) }}
              </span>
              <button
                type="button"
                class="del"
                :aria-label="`从历史中移除 ${record.name}`"
                @click="handleRemove(record.id, $event)"
              >
                ✕
              </button>
              <span v-if="formatProgress(record.currentTime)" class="ptime">
                {{ formatProgress(record.currentTime) }}
              </span>
              <span
                v-if="progressPercent(record.currentTime, record.duration) > 0"
                class="pbar"
              >
                <i :style="{ width: progressPercent(record.currentTime, record.duration) + '%' }" />
              </span>
              <span v-if="manageMode" class="jc-tv-hcard__delmark" aria-hidden="true">✕ 删除</span>
            </div>
            <div class="name">{{ record.name }}</div>
            <div class="sub">{{ formatTime(record.timeStamp) }}</div>
            </div>
          </RouterLink>
        </div>
      </section>
    </template>
  </section>

  <!-- ============ 桌面 / 移动 分支 (页头/删除钮已随 W2c 收编) ============ -->
  <section v-else class="container-page py-[var(--jc-space-6)]">
    <ListPageHeader
      title="观看历史"
      :source-label="sourceLabel"
      :remote-mode="remoteMode"
      :count="items.length"
      unit="条"
      :remote-loading="remoteLoading"
      :logged-in="isLoggedIn"
    >
      <template #actions>
        <BaseButton
          v-if="items.length"
          variant="ghost"
          size="sm"
          @click="handleClear"
        >
          <BaseIcon name="trash" size="16px" />
          清空
        </BaseButton>
      </template>
    </ListPageHeader>

    <BaseEmpty
      v-if="!items.length"
      title="还没有观看记录"
      description="去首页找一部喜欢的影片开始观看吧"
    />

    <div v-else class="flex flex-col gap-[var(--jc-space-8)]">
      <section
        v-for="group in groups"
        :key="group.bucket"
        class="flex flex-col gap-[var(--jc-space-4)]"
      >
        <h2 class="jc-history-group__title flex items-baseline gap-[var(--jc-space-2)]">
          <span class="text-[length:var(--jc-fs-lg)] font-[var(--jc-fw-bold)] text-primary">
            {{ group.label }}
          </span>
          <span class="text-[length:var(--jc-fs-xs)] text-muted">
            {{ group.items.length }} 条
          </span>
        </h2>
        <div class="jc-card-grid">
          <FilmCard
            v-for="record in group.items"
            :key="record.id"
            :item="recordToCard(record)"
            :to="buildPlayLink(record)"
            :progress="progressPercent(record.currentTime, record.duration)"
            :sub-text="formatRelativeTime(record.timeStamp)"
          >
            <template #poster-overlay>
              <!-- 左上角"看到第 N 集"(角标随 W1d 收敛进 EpisodeBadge, 位置与截断不变); 左下角 remarks 由
                   recordToCard 提供, 与普通影片卡一致 -->
              <EpisodeBadge
                v-if="episodeLabel(record.episode, record.episodeIndex)"
                variant="page"
              >
                {{ episodeLabel(record.episode, record.episodeIndex) }}
              </EpisodeBadge>
              <CardCornerRemove
                z-class="z-4"
                :label="`从历史中移除 ${record.name}`"
                @click="handleRemove(record.id, $event)"
              />

              <div
                v-if="formatProgress(record.currentTime)"
                class="absolute bottom-[8px] right-[var(--jc-space-2)] px-[6px] py-[2px] rounded-[var(--jc-radius-sm)] bg-[rgba(0,0,0,0.7)] text-white text-[length:var(--jc-fs-xs)] z-3"
              >
                {{ formatProgress(record.currentTime) }}
              </div>
            </template>
          </FilmCard>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
/* .jc-link-btn 视觉已随 W2c 收敛进 ListPageHeader */

/* 左上角"看到第 N 集"角标样式已随 W1d 收敛进 EpisodeBadge(variant=page, 值不变) */
</style>

<!-- ============ TV (雷鸟) 专属样式: 非 scoped, 仅 [data-mode=tv] 作用域 ============ -->
<style>
[data-mode='tv'] .jc-tv-history {
  padding: 6px var(--jc-space-8, 30px) 26px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 操作条 */
[data-mode='tv'] .jc-tv-history__bar {
  display: flex;
  align-items: flex-end;
  gap: 14px;
  flex-wrap: wrap;
}
[data-mode='tv'] .jc-tv-history__head-left {
  flex: 1 1 auto;
  min-width: 0;
  /* 标题 + 来源/同步/计数/标签 强制同一行左右排布(原块级上下堆叠; 禁 wrap 防窄视口换行) */
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  flex-wrap: nowrap;
}
[data-mode='tv'] .jc-tv-history__title {
  font-size: clamp(22px, 2vw, 30px);
  font-weight: 800;
  color: var(--jc-text-primary);
  white-space: nowrap;
}
[data-mode='tv'] .jc-tv-history__meta {
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-muted);
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 10px;
  flex-wrap: nowrap;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
}
[data-mode='tv'] .jc-tv-history__meta > span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
[data-mode='tv'] .jc-tv-history__srcchip {
  height: 22px;
  padding: 0 10px;
  font-size: 11px;
  flex-shrink: 0;
}
[data-mode='tv'] .jc-tv-history__syncing {
  color: var(--jc-brand-cyan);
}
[data-mode='tv'] .jc-tv-history__ops {
  margin-left: auto;
  display: flex;
  gap: 10px;
}
[data-mode='tv'] .jc-tv-history__clear,
[data-mode='tv'] .jc-tv-history__manage {
  height: 44px;
  padding: 0 18px;
  font-size: 14px;
}

/* 历史进度卡 (横图 16:9 + 角标 + 进度条 + 下方片名/时间) */
[data-mode='tv'] .jc-tv-hcard {
  display: block;
  text-decoration: none;
  color: var(--jc-text-primary);
}
[data-mode='tv'] .jc-tv-hcard .poster {
  aspect-ratio: 16 / 9;
  border-radius: var(--jc-radius-md, 11px);
  border: 1px solid var(--jc-tv-stroke, rgba(255, 255, 255, 0.13));
  position: relative;
  overflow: hidden;
  background-color: var(--jc-bg-elevated, #1c1d22);
}
[data-mode='tv'] .jc-tv-hcard .ep {
  position: absolute;
  top: 6px;
  left: 6px;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 5px;
  background: var(--jc-brand-gradient);
  color: #fff;
  z-index: 2;
}
[data-mode='tv'] .jc-tv-hcard .del {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 22px;
  height: 22px;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.6);
  color: #fff;
  font-size: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 3;
  cursor: pointer;
  border: none;
}
[data-mode='tv'] .jc-tv-hcard .ptime {
  position: absolute;
  right: 7px;
  bottom: 8px;
  font-size: 10px;
  color: #fff;
  background: rgba(0, 0, 0, 0.7);
  padding: 1px 6px;
  border-radius: 5px;
  z-index: 2;
}
[data-mode='tv'] .jc-tv-hcard .pbar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 3px;
  background: rgba(255, 255, 255, 0.25);
  z-index: 2;
}
[data-mode='tv'] .jc-tv-hcard .pbar i {
  display: block;
  height: 100%;
  background: var(--jc-brand-gradient);
}
[data-mode='tv'] .jc-tv-hcard .name {
  margin-top: 7px;
  font-size: 13px;
  font-weight: 600;
  color: var(--jc-text-primary);
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
[data-mode='tv'] .jc-tv-hcard .sub {
  font-size: 11px;
  color: var(--jc-text-muted);
  margin-top: 1px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 焦点: 只放大海报, 下方片名/时间静止 (与 web TV FilmCard 行为一致) */
[data-mode='tv'] .jc-tv-hcard[data-focusable='true']:focus,
[data-mode='tv'] .jc-tv-hcard[data-focusable='true']:focus-visible {
  transform: none;
  box-shadow: none;
  outline: none;
}
[data-mode='tv'] .jc-tv-hcard[data-focusable='true']:focus .poster,
[data-mode='tv'] .jc-tv-hcard[data-focusable='true']:focus-visible .poster {
  transform: scale(var(--jc-tv-focus-scale-card, 1.05));
  box-shadow: var(--jc-tv-focus-ring);
  z-index: 5;
  transform-origin: center;
  transition:
    transform var(--jc-dur-fast) var(--jc-ease-spring),
    box-shadow var(--jc-dur-fast) var(--jc-ease-standard);
}

/* 管理模式: 海报上覆盖红色"✕ 删除"提示, 点整卡(OK 键)即删该条 */
[data-mode='tv'] .jc-tv-hcard__delmark {
  position: absolute;
  inset: 0;
  z-index: 4;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  font-size: var(--jc-fs-base);
  font-weight: 800;
  letter-spacing: 1px;
  color: #fff;
  background: rgba(220, 38, 38, 0.42);
}
[data-mode='tv'] .jc-tv-hcard.is-manage .name {
  color: var(--jc-danger, #ff4757);
}
</style>

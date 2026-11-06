<script setup lang="ts">
import { computed, defineAsyncComponent, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useHistoryStore, useUserStore } from '@/stores'
import type { HistoryRecord } from '@/stores/history'
import { buildPlayLink, recordToCard } from '@/stores/history'
import { clearLiveLast, liveLastLabel, readLiveLast, LIVE_COVER_SVG, LIVE_HISTORY_ID } from '@/utils/live'
import { useViewMode } from '@/composables/useViewMode'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import EpisodeBadge from '@/components/base/EpisodeBadge.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import { confirm } from '@/composables/useConfirm'
import {
  formatProgress,
  formatRelativeTime,
  groupByTimeBucket,
  progressPercent,
  episodeLabel
} from '@/composables/useTimeBucket'

/**
 * 观看历史 — 壳(数据/状态/路由) + 双分支渲染
 * - 数据: historyStore(本地/云端) + 「直播」本地记忆条目, 合并后按播放时间倒序分组
 * - 桌面 / 移动: 本文件内模板
 * - TV(雷鸟卡片式): 渲染组件 HistoryTv.vue(方案 B, defineAsyncComponent 异步分片),
 *   本壳只做数据获取/派生与删除/清空逻辑, 经 props + 回调单向传入; 不另起接口。
 */

/** TV 渲染组件(方案 B 拆分): TV 模板/页级 TV CSS 全部在 HistoryTv.vue, 仅 isTV 时挂载 */
const HistoryTv = defineAsyncComponent(() => import('./HistoryTv.vue'))

const historyStore = useHistoryStore()
const userStore = useUserStore()
const { list, remoteMode, remoteLoading } = storeToRefs(historyStore)
const { isLoggedIn } = storeToRefs(userStore)
const { isTV } = useViewMode()

/** TV「管理」模式: 开启后点卡片=删除该条 (遥控器点不中海报上的小✕角标, 改整卡删除) */
const manageMode = ref(false)

/** TV 操作条「管理删除 / ✓ 完成」切换(状态留在壳内, 由渲染组件回调触发) */
function toggleManage(): void {
  manageMode.value = !manageMode.value
}

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
  <!-- ============ TV (雷鸟卡片式) 分支: 渲染组件(方案 B) ============ -->
  <!-- TV 模板/页级 TV CSS 全部在 HistoryTv.vue; 此处只做数据供给与删除/清空/管理模式回调。 -->
  <HistoryTv
    v-if="isTV"
    :groups="groups"
    :count="items.length"
    :source-label="sourceLabel"
    :remote-mode="remoteMode"
    :remote-loading="remoteLoading"
    :is-logged-in="isLoggedIn"
    :manage-mode="manageMode"
    :on-remove="handleRemove"
    :on-clear="handleClear"
    :on-toggle-manage="toggleManage"
  />

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

/* 页级 TV 样式块(jc-tv-history* / jc-tv-hcard*)已随渲染组件迁入 HistoryTv.vue(scoped) */
</style>

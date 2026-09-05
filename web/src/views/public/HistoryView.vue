<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useHistoryStore, useUserStore } from '@/stores'
import { buildPlayLink } from '@/stores/history'
import { useViewMode } from '@/composables/useViewMode'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import { confirm } from '@/composables/useConfirm'
import {
  groupByTimeBucket,
  progressPercent,
  episodeLabel
} from '@/composables/useTimeBucket'

const historyStore = useHistoryStore()
const userStore = useUserStore()
const { list, remoteMode, remoteLoading } = storeToRefs(historyStore)
const { isLoggedIn } = storeToRefs(userStore)
const { isTV } = useViewMode()

/** TV「管理」模式: 开启后点卡片=删除该条 (遥控器点不中海报上的小✕角标, 改整卡删除) */
const manageMode = ref(false)

const items = computed(() => list.value)

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
  <section v-if="isTV" class="gf-tv-history">
    <!-- 操作条: 标题 + 来源标签 + 共N条 + 登录同步 + 清空 -->
    <div class="gf-tv-history__bar">
      <div class="gf-tv-history__head-left">
        <div class="gf-tv-history__title">观看历史</div>
        <div class="gf-tv-history__meta">
          <span class="gf-tv-chip sel gf-tv-history__srcchip">
            {{ remoteMode ? '云端' : '本地' }}
          </span>
          <span>{{ sourceLabel }}</span>
          <span>·</span>
          <span>共 {{ items.length }} 条</span>
          <span v-if="remoteLoading" class="gf-tv-history__syncing">同步中…</span>
        </div>
      </div>
      <div class="gf-tv-history__ops">
        <RouterLink
          v-if="!isLoggedIn"
          to="/login"
          class="gf-tv-chip"
          data-focusable="true"
          tabindex="0"
        >
          登录以云端同步
        </RouterLink>
        <button
          v-if="items.length"
          type="button"
          class="gf-tv-btn gf-tv-history__manage"
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
          class="gf-tv-btn gf-tv-history__clear"
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
        class="gf-tv-history__group"
      >
        <div class="gf-tv-sec">
          <span class="t">{{ group.label }}</span>
          <span class="s">{{ group.items.length }} 条</span>
        </div>
        <div class="gf-tv-grid">
          <RouterLink
            v-for="record in group.items"
            :key="record.id"
            v-slot="{ navigate }"
            :to="buildPlayLink(record)"
            custom
          >
            <div
              class="gf-tv-card gf-tv-hcard"
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
              <span v-if="manageMode" class="gf-tv-hcard__delmark" aria-hidden="true">✕ 删除</span>
            </div>
            <div class="name">{{ record.name }}</div>
            <div class="sub">{{ formatTime(record.timeStamp) }}</div>
            </div>
          </RouterLink>
        </div>
      </section>
    </template>
  </section>

  <!-- ============ 桌面 / 移动 分支 (原样保留) ============ -->
  <section v-else class="px-[var(--gf-space-4)] md:px-[var(--gf-space-6)] py-[var(--gf-space-6)]">
    <header class="flex items-center justify-between mb-[var(--gf-space-5)] flex-wrap gap-[var(--gf-space-3)]">
      <div>
        <h1 class="text-[var(--gf-fs-2xl)] font-[var(--gf-fw-bold)]">观看历史</h1>
        <p class="text-sm text-muted mt-[var(--gf-space-1)] flex items-center gap-[var(--gf-space-2)] flex-wrap">
          <BaseTag :variant="remoteMode ? 'purple' : 'default'" size="xs">
            {{ remoteMode ? '云端' : '本地' }}
          </BaseTag>
          <span>{{ sourceLabel }}</span>
          <span>·</span>
          <span>共 {{ items.length }} 条</span>
          <span v-if="remoteLoading" class="text-link">同步中…</span>
        </p>
      </div>
      <div class="flex items-center gap-[var(--gf-space-2)]">
        <RouterLink
          v-if="!isLoggedIn"
          to="/login"
          class="gf-link-btn"
        >
          登录以云端同步
        </RouterLink>
        <BaseButton
          v-if="items.length"
          variant="ghost"
          size="sm"
          @click="handleClear"
        >
          <BaseIcon name="trash" size="16px" />
          清空
        </BaseButton>
      </div>
    </header>

    <BaseEmpty
      v-if="!items.length"
      title="还没有观看记录"
      description="去首页找一部喜欢的影片开始观看吧"
    />

    <div v-else class="flex flex-col gap-[var(--gf-space-8)]">
      <section
        v-for="group in groups"
        :key="group.bucket"
        class="flex flex-col gap-[var(--gf-space-4)]"
      >
        <h2 class="gf-history-group__title flex items-baseline gap-[var(--gf-space-2)]">
          <span class="text-[var(--gf-fs-lg)] font-[var(--gf-fw-bold)] text-primary">
            {{ group.label }}
          </span>
          <span class="text-[var(--gf-fs-xs)] text-muted">
            {{ group.items.length }} 条
          </span>
        </h2>
        <div
          class="grid gap-[var(--gf-space-2)] sm:gap-[var(--gf-space-4)] grid-cols-3 md:grid-cols-[repeat(auto-fill,minmax(180px,1fr))]"
        >
          <RouterLink
            v-for="record in group.items"
            :key="record.id"
            :to="buildPlayLink(record)"
            class="gf-history-card group block"
            data-focusable="true"
            tabindex="0"
            :aria-label="record.name"
          >
            <div class="relative overflow-hidden rounded-[var(--gf-radius-lg)] shadow-card aspect-[3/4] bg-elevated">
              <BaseImage
                :src="record.picture || ''"
                :alt="record.name"
                ratio="3/4"
                fit="cover"
              />

              <BaseTag
                v-if="episodeLabel(record.episode, record.episodeIndex)"
                variant="brand"
                size="xs"
                class="absolute top-[var(--gf-space-2)] left-[var(--gf-space-2)] z-2"
              >
                {{ episodeLabel(record.episode, record.episodeIndex) }}
              </BaseTag>

              <button
                type="button"
                class="absolute top-[var(--gf-space-2)] right-[var(--gf-space-2)] z-2 w-[24px] h-[24px] rounded-full bg-[rgba(0,0,0,0.6)] hover:bg-[rgba(0,0,0,0.85)] flex-center text-white transition-colors"
                :aria-label="`从历史中移除 ${record.name}`"
                @click="handleRemove(record.id, $event)"
              >
                <BaseIcon name="close" size="14px" />
              </button>

              <div
                v-if="formatProgress(record.currentTime)"
                class="absolute bottom-[8px] right-[var(--gf-space-2)] px-[6px] py-[2px] rounded-[var(--gf-radius-sm)] bg-[rgba(0,0,0,0.7)] text-white text-[var(--gf-fs-xs)] z-2"
              >
                {{ formatProgress(record.currentTime) }}
              </div>

              <!-- 进度条 (基于 currentTime / duration), 卡片底部 4px 横条 -->
              <div
                v-if="progressPercent(record.currentTime, record.duration) > 0"
                class="gf-history-progress"
                :aria-label="`已观看 ${progressPercent(record.currentTime, record.duration)}%`"
              >
                <span
                  class="gf-history-progress__fill"
                  :style="{ width: progressPercent(record.currentTime, record.duration) + '%' }"
                />
              </div>

              <div class="absolute inset-0 bg-[linear-gradient(180deg,transparent_50%,rgba(0,0,0,0.85)_100%)] opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 transition-opacity" />

              <div class="absolute inset-x-0 bottom-0 px-[var(--gf-space-3)] pb-[var(--gf-space-3)] pt-[var(--gf-space-5)] z-2 opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 transition-opacity">
                <div class="text-white text-[var(--gf-fs-sm)] font-[var(--gf-fw-semibold)] flex items-center gap-[var(--gf-space-1)]">
                  <BaseIcon name="play" size="14px" />
                  继续观看
                </div>
              </div>
            </div>

            <div class="mt-[var(--gf-space-2)]">
              <h3 class="text-[var(--gf-fs-sm)] font-[var(--gf-fw-medium)] text-primary line-clamp-1">
                {{ record.name }}
              </h3>
              <p class="text-[var(--gf-fs-xs)] text-muted mt-[2px]">
                {{ formatTime(record.timeStamp) }}
              </p>
            </div>
          </RouterLink>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.gf-link-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: var(--gf-radius-sm);
  background-color: rgba(155, 73, 231, 0.16);
  color: var(--gf-text-link);
  font-size: var(--gf-fs-sm);
  text-decoration: none;
  transition: background-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-link-btn:hover,
.gf-link-btn:focus-visible {
  background-color: rgba(155, 73, 231, 0.28);
  outline: none;
}

.gf-history-card {
  text-decoration: none;
  outline: none;
  transition: transform var(--gf-dur-base) var(--gf-ease-spring);
}
.gf-history-card:focus-visible {
  outline: none;
}
.gf-history-card:focus-visible > div:first-child {
  box-shadow: var(--gf-shadow-focus-ring), var(--gf-shadow-hover);
}
@media (hover: hover) and (pointer: fine) {
  .gf-history-card:hover > div:first-child {
    transform: scale(1.04);
    box-shadow: var(--gf-shadow-hover);
  }
}
.line-clamp-1 {
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 进度条: 卡片底部 4px 横条 */
.gf-history-progress {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 3px;
  background-color: var(--gf-progress-bg);
  z-index: 2;
  overflow: hidden;
}
.gf-history-progress__fill {
  display: block;
  height: 100%;
  background-image: var(--gf-progress-fg);
  border-top-right-radius: 2px;
  border-bottom-right-radius: 2px;
  transition: width var(--gf-dur-base) var(--gf-ease-standard);
}
</style>

<!-- ============ TV (雷鸟) 专属样式: 非 scoped, 仅 [data-mode=tv] 作用域 ============ -->
<style>
[data-mode='tv'] .gf-tv-history {
  padding: 6px var(--gf-space-8, 30px) 26px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 操作条 */
[data-mode='tv'] .gf-tv-history__bar {
  display: flex;
  align-items: flex-end;
  gap: 14px;
  flex-wrap: wrap;
}
[data-mode='tv'] .gf-tv-history__head-left {
  flex: 1 1 auto;
  min-width: 0;
  /* 标题 + 来源/同步/计数/标签 左右排布(原块级上下堆叠) */
  display: flex;
  align-items: center;
  gap: var(--gf-space-3);
  flex-wrap: wrap;
}
[data-mode='tv'] .gf-tv-history__title {
  font-size: clamp(22px, 2vw, 30px);
  font-weight: 800;
  color: var(--gf-text-primary);
}
[data-mode='tv'] .gf-tv-history__meta {
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-muted);
  margin-top: 6px;
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
[data-mode='tv'] .gf-tv-history__srcchip {
  height: 22px;
  padding: 0 10px;
  font-size: 11px;
}
[data-mode='tv'] .gf-tv-history__syncing {
  color: var(--gf-brand-cyan);
}
[data-mode='tv'] .gf-tv-history__ops {
  margin-left: auto;
  display: flex;
  gap: 10px;
}
[data-mode='tv'] .gf-tv-history__clear,
[data-mode='tv'] .gf-tv-history__manage {
  height: 44px;
  padding: 0 18px;
  font-size: 14px;
}

/* 历史进度卡 (横图 16:9 + 角标 + 进度条 + 下方片名/时间) */
[data-mode='tv'] .gf-tv-hcard {
  display: block;
  text-decoration: none;
  color: var(--gf-text-primary);
}
[data-mode='tv'] .gf-tv-hcard .poster {
  aspect-ratio: 16 / 9;
  border-radius: var(--gf-radius-md, 11px);
  border: 1px solid var(--gf-tv-stroke, rgba(255, 255, 255, 0.13));
  position: relative;
  overflow: hidden;
  background-color: var(--gf-bg-elevated, #1c1d22);
}
[data-mode='tv'] .gf-tv-hcard .ep {
  position: absolute;
  top: 6px;
  left: 6px;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 5px;
  background: var(--gf-brand-gradient);
  color: #fff;
  z-index: 2;
}
[data-mode='tv'] .gf-tv-hcard .del {
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
[data-mode='tv'] .gf-tv-hcard .ptime {
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
[data-mode='tv'] .gf-tv-hcard .pbar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 3px;
  background: rgba(255, 255, 255, 0.25);
  z-index: 2;
}
[data-mode='tv'] .gf-tv-hcard .pbar i {
  display: block;
  height: 100%;
  background: var(--gf-brand-gradient);
}
[data-mode='tv'] .gf-tv-hcard .name {
  margin-top: 7px;
  font-size: 13px;
  font-weight: 600;
  color: var(--gf-text-primary);
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
[data-mode='tv'] .gf-tv-hcard .sub {
  font-size: 11px;
  color: var(--gf-text-muted);
  margin-top: 1px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 焦点: 只放大海报, 下方片名/时间静止 (与 web TV FilmCard 行为一致) */
[data-mode='tv'] .gf-tv-hcard[data-focusable='true']:focus,
[data-mode='tv'] .gf-tv-hcard[data-focusable='true']:focus-visible {
  transform: none;
  box-shadow: none;
  outline: none;
}
[data-mode='tv'] .gf-tv-hcard[data-focusable='true']:focus .poster,
[data-mode='tv'] .gf-tv-hcard[data-focusable='true']:focus-visible .poster {
  transform: scale(var(--gf-tv-focus-scale-card, 1.05));
  box-shadow: var(--gf-tv-focus-ring);
  z-index: 5;
  transform-origin: center;
  transition:
    transform var(--gf-dur-fast) var(--gf-ease-spring),
    box-shadow var(--gf-dur-fast) var(--gf-ease-standard);
}

/* 管理模式: 海报上覆盖红色"✕ 删除"提示, 点整卡(OK 键)即删该条 */
[data-mode='tv'] .gf-tv-hcard__delmark {
  position: absolute;
  inset: 0;
  z-index: 4;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  font-size: var(--gf-fs-base);
  font-weight: 800;
  letter-spacing: 1px;
  color: #fff;
  background: rgba(220, 38, 38, 0.42);
}
[data-mode='tv'] .gf-tv-hcard.is-manage .name {
  color: var(--gf-danger, #ff4757);
}
</style>

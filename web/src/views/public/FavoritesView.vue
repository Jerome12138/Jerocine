<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useFavoriteStore, useUserStore } from '@/stores'
import { useViewMode } from '@/composables/useViewMode'
import type { Card } from '@/types/film'
import type { FavoriteRecord } from '@/stores/favorite'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import FilmCard from '@/components/film/FilmCard.vue'

const favoriteStore = useFavoriteStore()
const userStore = useUserStore()
const { isTV } = useViewMode()
const { list, remoteMode, remoteLoading } = storeToRefs(favoriteStore)
const { isLoggedIn } = storeToRefs(userStore)

const items = computed(() => list.value)
const sourceLabel = computed(() =>
  remoteMode.value ? '云端收藏 · 跨设备同步' : '本地收藏 · 仅当前浏览器'
)

function detailLink(id: string): { path: string; query: { link: string } } {
  return { path: '/filmDetail', query: { link: id } }
}

function handleRemove(id: string, e: Event): void {
  e.preventDefault()
  e.stopPropagation()
  void favoriteStore.remove(id)
}

/** 把收藏记录映射成 FilmCard 需要的 Card (FavoriteRecord 字段与 Card 不同) */
function toCard(record: FavoriteRecord): Card {
  return {
    mid: Number(record.id) || 0,
    name: record.name,
    cover: record.picture || '',
    cid: record.cid ?? 0,
    pid: record.pid ?? 0,
    cName: '',
    subTitle: '',
    area: '',
    year: 0,
    state: '',
    remarks: record.remarks || '',
    dbScore: 0
  }
}
</script>

<template>
  <!-- ===================== TV(雷鸟卡片式)分支 ===================== -->
  <section
    v-if="isTV"
    class="gf-fav-tv container-page"
  >
    <!-- 标题区: 我的收藏 + 云端/本地来源标识 + 共 N 部 + 登录入口 -->
    <header class="gf-fav-tv__head">
      <div class="gf-fav-tv__head-left">
        <h1 class="gf-fav-tv__title">
          <BaseIcon name="heart" size="1em" />
          我的收藏
          <BaseTag :variant="remoteMode ? 'purple' : 'default'" size="sm">
            {{ remoteMode ? '云端' : '本地' }}
          </BaseTag>
        </h1>
        <p class="gf-fav-tv__sub">
          {{ sourceLabel }} · 共 {{ items.length }} 部
          <span v-if="remoteLoading" class="text-link">· 同步中…</span>
        </p>
      </div>
      <RouterLink
        v-if="!isLoggedIn"
        to="/login"
        class="gf-tv-btn cyan"
        data-focusable="true"
        tabindex="0"
      >
        登录以云端同步
      </RouterLink>
    </header>

    <!-- 空态: 大字 + 提示 -->
    <div
      v-if="!items.length"
      class="gf-tv-glass-card gf-fav-tv__empty"
    >
      <div class="gf-fav-tv__empty-glyph" aria-hidden="true">
        <BaseIcon name="heart" size="1em" />
      </div>
      <div class="gf-fav-tv__empty-title">还没有收藏内容</div>
      <div class="gf-fav-tv__empty-desc">在影片详情页点击「收藏」即可加入这里</div>
    </div>

    <!-- 收藏网格: 2:3 竖海报 (FilmCard) + 卡角常驻"取消收藏"角标 -->
    <div v-else class="gf-tv-grid">
      <div
        v-for="record in items"
        :key="record.id"
        class="gf-fav-tv__cell"
      >
        <FilmCard :item="toCard(record)" :show-title-below="true" />
        <button
          type="button"
          class="gf-fav-tv__remove"
          :aria-label="`取消收藏 ${record.name}`"
          data-focusable="true"
          tabindex="0"
          @click="handleRemove(record.id, $event)"
        >
          <BaseIcon name="close" size="20px" />
        </button>
      </div>
    </div>
  </section>

  <!-- ===================== 桌面 / 移动分支 (原样保留) ===================== -->
  <section
    v-else
    class="px-[var(--gf-space-4)] md:px-[var(--gf-space-6)] py-[var(--gf-space-6)]"
  >
    <header class="flex items-center justify-between mb-[var(--gf-space-5)] flex-wrap gap-[var(--gf-space-3)]">
      <div>
        <h1 class="text-[var(--gf-fs-2xl)] font-[var(--gf-fw-bold)]">我的收藏</h1>
        <p class="text-sm text-muted mt-[var(--gf-space-1)] flex items-center gap-[var(--gf-space-2)] flex-wrap">
          <BaseTag :variant="remoteMode ? 'purple' : 'default'" size="xs">
            {{ remoteMode ? '云端' : '本地' }}
          </BaseTag>
          <span>{{ sourceLabel }}</span>
          <span>·</span>
          <span>共 {{ items.length }} 部</span>
          <span v-if="remoteLoading" class="text-link">同步中…</span>
        </p>
      </div>
      <RouterLink
        v-if="!isLoggedIn"
        to="/login"
        class="gf-link-btn"
      >
        登录以云端同步
      </RouterLink>
    </header>

    <BaseEmpty
      v-if="!items.length"
      title="还没有收藏内容"
      description="在影片详情页点击「收藏」即可加入这里"
    />

    <div
      v-else
      class="grid gap-[var(--gf-space-2)] sm:gap-[var(--gf-space-4)] grid-cols-3 md:grid-cols-[repeat(auto-fill,minmax(180px,1fr))]"
    >
      <RouterLink
        v-for="record in items"
        :key="record.id"
        :to="detailLink(record.id)"
        class="gf-fav-card group block"
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

          <button
            type="button"
            class="absolute top-[var(--gf-space-2)] right-[var(--gf-space-2)] z-2 w-[24px] h-[24px] rounded-full bg-[rgba(0,0,0,0.6)] hover:bg-[rgba(0,0,0,0.85)] flex-center text-white transition-colors"
            :aria-label="`取消收藏 ${record.name}`"
            @click="handleRemove(record.id, $event)"
          >
            <BaseIcon name="close" size="14px" />
          </button>

          <div class="absolute inset-0 bg-[linear-gradient(180deg,transparent_50%,rgba(0,0,0,0.85)_100%)] opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 transition-opacity" />
        </div>

        <div class="mt-[var(--gf-space-2)]">
          <h3 class="text-[var(--gf-fs-sm)] font-[var(--gf-fw-medium)] text-primary line-clamp-1">
            {{ record.name }}
          </h3>
          <p v-if="record.remarks" class="text-[var(--gf-fs-xs)] text-muted mt-[2px] line-clamp-1">
            {{ record.remarks }}
          </p>
        </div>
      </RouterLink>
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
.gf-fav-card {
  text-decoration: none;
  outline: none;
  transition: transform var(--gf-dur-base) var(--gf-ease-spring);
}
.gf-fav-card:focus-visible > div:first-child {
  box-shadow: var(--gf-shadow-focus-ring), var(--gf-shadow-hover);
}
@media (hover: hover) and (pointer: fine) {
  .gf-fav-card:hover > div:first-child {
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
</style>

<!-- TV 专属样式 (非 scoped, 但全部以 [data-mode='tv'] 限定, 不污染桌面/移动) -->
<style>
[data-mode='tv'] .gf-fav-tv {
  padding-block: var(--gf-tv-safe-y) var(--gf-space-16);
}
[data-mode='tv'] .gf-fav-tv__head {
  display: flex;
  align-items: flex-end;
  gap: var(--gf-space-4);
  flex-wrap: wrap;
  margin-bottom: var(--gf-space-8);
}
[data-mode='tv'] .gf-fav-tv__head-left {
  min-width: 0;
}
[data-mode='tv'] .gf-fav-tv__title {
  display: flex;
  align-items: center;
  gap: 10px;
  /* 2xl 太大 → xl(对齐历史页观感) */
  font-size: var(--gf-fs-xl);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
}

/* 收藏卡片偏大 → 缩小 card-min, 网格更密(参考历史页) */
[data-mode='tv'] .gf-fav-tv .gf-tv-grid {
  --gf-tv-card-min: clamp(150px, 13vw, 200px);
}
[data-mode='tv'] .gf-fav-tv__sub {
  margin-top: 8px;
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-muted);
}
[data-mode='tv'] .gf-fav-tv__head .gf-tv-btn.cyan {
  margin-left: auto;
  text-decoration: none;
}

/* 网格单元: FilmCard + 右上角取消收藏角标 */
[data-mode='tv'] .gf-fav-tv__cell {
  position: relative;
}
[data-mode='tv'] .gf-fav-tv__remove {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 4;
  width: 40px;
  height: 40px;
  min-height: 40px;
  border-radius: 999px;
  border: none;
  background: rgba(0, 0, 0, 0.62);
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background var(--gf-dur-fast) var(--gf-ease-standard);
}
[data-mode='tv'] .gf-fav-tv__remove:hover,
[data-mode='tv'] .gf-fav-tv__remove:focus,
[data-mode='tv'] .gf-fav-tv__remove:focus-visible {
  background: rgba(0, 0, 0, 0.88);
  outline: none;
}

/* 空态 */
[data-mode='tv'] .gf-fav-tv__empty {
  text-align: center;
  padding: 56px 40px;
}
[data-mode='tv'] .gf-fav-tv__empty-glyph {
  font-size: 56px;
  line-height: 1;
  color: var(--gf-text-muted);
  opacity: 0.6;
}
[data-mode='tv'] .gf-fav-tv__empty-title {
  margin-top: 16px;
  font-size: var(--gf-fs-lg);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
}
[data-mode='tv'] .gf-fav-tv__empty-desc {
  margin-top: 8px;
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-muted);
}
</style>

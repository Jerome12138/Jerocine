<script setup lang="ts">
import type { Card } from '@/types/film'
import type { PageMeta } from '@/types/api'
import FilmFilterBar, {
  type FilterGroup
} from '../film/FilmFilterBar.vue'
import FilmGrid from '../film/FilmGrid.vue'
import SkeletonGrid from './SkeletonGrid.vue'
import BaseEmpty from './BaseEmpty.vue'
import BasePagination from './BasePagination.vue'

/**
 * 桌面/移动结果流水线(W2b): ClassifySearchView / HotBoardView 两页桌面分支的
 * 纯模板收编 —— 筛选栏(吸边 wrap) → 结果统计 → 骨架 → 网格 → 分页 → 空态。
 *
 * 视觉契约:
 * - 纯模板收编: 本件为 **fragment 根(无 wrapper)**, 各块与原模板一样互为兄弟节点,
 *   直接落进调用方容器(加包裹层会破坏 jc-classify-search 的负 margin 吸边几何)。
 * - DOM 类名逐字保留: jc-classify-search__filter-wrap / jc-classify-search__count
 *   (吸边负 margin 样式随本件迁入, HotBoard 的 .jc-hot-board 前缀抄写副本删除;
 *   ClassifySearch 原有的第二个 @media 768 块与前块净效果完全相同, 收敛为单块)。
 * - BasePagination / FilmFilterBar 原样内嵌, change 事件原样上抛为
 *   filter-change / paginate, 由调用方 push query + 回顶。
 * - 空态条件 `!errorMsg` 与原模板逐字一致(errorMsg 属调用方状态, 经 prop 传入)。
 */
defineProps<{
  loading: boolean
  loaded: boolean
  errorMsg: string
  films: Card[]
  page: PageMeta
  filterGroups: FilterGroup[]
  /** 空态标题(无符合条件的影片 / 该榜单暂无影片) */
  emptyTitle: string
}>()

const emit = defineEmits<{
  (e: 'filter-change', payload: { key: string; value: string | number }): void
  (e: 'paginate', page: number): void
}>()
</script>

<template>
  <!-- 筛选栏 (sticky 吸顶, bilibili 风格保持滚动时可见) -->
  <div
    v-if="filterGroups.length > 0"
    class="jc-classify-search__filter-wrap"
  >
    <FilmFilterBar
      :groups="filterGroups"
      @change="emit('filter-change', $event)"
    />
  </div>

  <!-- 结果统计 -->
  <div
    v-if="page.total > 0"
    class="jc-classify-search__count flex items-baseline justify-between mb-[var(--jc-space-4)] mt-[var(--jc-space-5)]"
  >
    <p class="text-secondary text-[length:var(--jc-fs-sm)]">
      共 <strong class="text-primary">{{ page.total }}</strong> 部影片
    </p>
    <span class="text-muted text-[length:var(--jc-fs-xs)]">
      第 {{ page.current || 1 }} / {{ Math.max(1, page.pageCount || 1) }} 页
    </span>
  </div>

  <!-- 骨架 -->
  <SkeletonGrid
    v-if="loading && films.length === 0"
    :count="12"
    layout="poster"
    height="auto"
    ratio="3/4"
  />

  <!-- 网格列表 -->
  <FilmGrid v-else-if="films.length > 0" :items="films" />

  <!-- 分页 -->
  <div
    v-if="!loading && page.total > 0"
    class="mt-[var(--jc-space-8)] flex justify-center"
  >
    <BasePagination
      :current="page.current || 1"
      :page-size="page.size || 49"
      :total="page.total"
      @change="emit('paginate', $event)"
    />
  </div>

  <!-- 空状态 -->
  <BaseEmpty
    v-if="!loading && loaded && films.length === 0 && !errorMsg"
    :title="emptyTitle"
    description="试试切换其他筛选条件"
  />
</template>

<style scoped>
/* 自 ClassifySearchView.vue 逐字迁入(值不变); HotBoardView 的
   .jc-hot-board 前缀抄写副本随 DOM 收编一并删除 */
/* 筛选栏: mobile 不 sticky (避免占满半屏), md+ 才吸顶 */
.jc-classify-search__filter-wrap {
  background-color: rgba(11, 11, 15, 0.92);
  margin-inline: calc(-1 * var(--jc-gutter-mobile));
  padding: var(--jc-space-2) var(--jc-gutter-mobile);
  border-bottom: 1px solid var(--jc-border-subtle);
}
@media (min-width: 768px) {
  .jc-classify-search__filter-wrap {
    /* 取消 sticky 吸顶: 滚动后固定在顶部太占位置, 改为随页面滚走 */
    padding: var(--jc-space-3) var(--jc-gutter-tablet);
    margin-inline: calc(-1 * var(--jc-gutter-tablet));
  }
}

@media (min-width: 1024px) {
  .jc-classify-search__filter-wrap {
    margin-inline: calc(-1 * var(--jc-gutter-desktop));
    padding-inline: var(--jc-gutter-desktop);
  }
}
</style>

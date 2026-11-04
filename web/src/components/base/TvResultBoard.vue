<script setup lang="ts">
import { computed } from 'vue'
import type { Card } from '@/types/film'
import type { PageMeta } from '@/types/api'
import type { FilterGroup } from '@/components/film/FilmFilterBar.vue'
import BaseEmpty from './BaseEmpty.vue'
import SkeletonGrid from './SkeletonGrid.vue'
import FilmCard from '../film/FilmCard.vue'

/**
 * TV 分屏结果板(W2a): ClassifySearchView / HotBoardView 两页 TV 分支的整版收编
 * (筛选竖栏 + 结果网格 + 翻页器)。原两页模板逐字同构(HotBoard 抄写自 ClassifySearch),
 * 差异只在页头 —— 页头由调用方经 #head 插槽提供(两页只留数据与头部), 插槽内容在
 * 调用方作用域编译, 各页 scoped 页头样式(jc-cs-tv__head/__count、jc-hb-tv__title)继续生效。
 *
 * 视觉契约(TV 铁律):
 * - DOM 类名逐字保留: jc-cs-tv__split/__filter/__grp(__title/__chips)/__chip/__main/
 *   __pager(__pager-info); chip 仍叠加全局 jc-tv-chip, 翻页按钮仍用全局 jc-tv-btn(+-cyan)。
 * - data-focusable 顺序不变: 竖栏 chips(组序×选项序) → 翻页 上一页/下一页; TV 焦点
 *   由全局 CSS(tv-cards.css + [data-mode='tv'])接管, 本件不新增焦点样式。
 * - 根节点为无类名 <section>, 调用方透传原根类(jc-cs-tv [+jc-hb-tv] container-page py-…)
 *   保证类属性逐字一致。原 HotBoard 竖栏无 v-if、ClassifySearch 有 v-if="filterGroups.length>0"
 *   —— HotBoard 的 filterGroups 恒含"榜单"组(永非空), 统一按 v-if 渲染 DOM 等价。
 * - 翻页边界保护与两页原 tvGoPage 逐字同逻辑, 越界不上抛, 调用方收 paginate 后自行
 *   push query + 回顶。
 */
const props = defineProps<{
  loading: boolean
  loaded: boolean
  errorMsg: string
  /** 错误态 BaseEmpty 描述(ClassifySearch="请检查链接中的 Pid 参数" / HotBoard="请稍后重试") */
  errorDescription: string
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

/** TV 分页计数(与两页原 tvPageCount/tvCurrent 计算逐字一致) */
const tvPageCount = computed(() => Math.max(1, props.page.pageCount || 1))
const tvCurrent = computed(() => props.page.current || 1)
function goPage(delta: number): void {
  const next = tvCurrent.value + delta
  if (next < 1 || next > tvPageCount.value) return
  emit('paginate', next)
}
</script>

<template>
  <section>
    <!-- 页头: 标题切换(分类库)/榜单名 + 结果计数, 由调用方提供 -->
    <slot name="head" />

    <!-- 错误态 -->
    <BaseEmpty
      v-if="!loading && errorMsg"
      :title="errorMsg"
      :description="errorDescription"
    />

    <div v-else class="jc-cs-tv__split">
      <!-- ===== 左: 竖向筛选分区栏 ===== -->
      <aside
        v-if="filterGroups.length > 0"
        class="jc-cs-tv__filter"
      >
        <div
          v-for="group in filterGroups"
          :key="group.key"
          class="jc-cs-tv__grp"
        >
          <div class="jc-cs-tv__grp-title">{{ group.title }}</div>
          <div class="jc-cs-tv__grp-chips">
            <button
              v-for="opt in group.options"
              :key="String(opt.value)"
              type="button"
              class="jc-tv-chip jc-cs-tv__chip"
              :class="String(opt.value) === String(group.current) ? 'sel' : ''"
              data-focusable="true"
              tabindex="0"
              :aria-pressed="String(opt.value) === String(group.current)"
              @click="emit('filter-change', { key: group.key, value: opt.value })"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>
      </aside>

      <!-- ===== 右: 结果网格 ===== -->
      <main class="jc-cs-tv__main">
        <!-- 骨架 -->
        <SkeletonGrid
          v-if="loading && films.length === 0"
          :count="12"
          layout="poster-tv"
          height="auto"
          ratio="3/4"
        />

        <!-- 网格 -->
        <div v-else-if="films.length > 0" class="jc-tv-grid">
          <FilmCard
            v-for="item in films"
            :key="item.mid"
            :item="item"
            :show-title-below="true"
          />
        </div>

        <!-- 空态 -->
        <BaseEmpty
          v-else-if="!loading && loaded"
          :title="emptyTitle"
          description="试试切换其他筛选条件"
        />

        <!-- 翻页 -->
        <div
          v-if="!loading && page.total > 0 && tvPageCount > 1"
          class="jc-cs-tv__pager"
        >
          <button
            type="button"
            class="jc-tv-btn"
            :disabled="tvCurrent <= 1"
            data-focusable="true"
            tabindex="0"
            @click="goPage(-1)"
          >
            上一页
          </button>
          <span class="jc-cs-tv__pager-info">{{ tvCurrent }} / {{ tvPageCount }}</span>
          <button
            type="button"
            class="jc-tv-btn cyan"
            :disabled="tvCurrent >= tvPageCount"
            data-focusable="true"
            tabindex="0"
            @click="goPage(1)"
          >
            下一页
          </button>
        </div>
      </main>
    </div>
  </section>
</template>

<style scoped>
/* 以下规则自 ClassifySearchView.vue TV 分支样式逐字迁入(值不变);
   HotBoardView 的 .jc-hb-tv 前缀抄写副本随 DOM 收编一并删除。
   页头(__head/__title-*/__count)仍在各视图 —— 那部分样式留在调用方。 */

/* 左右分屏: 左固定宽筛选栏 + 右弹性结果网格 */
.jc-cs-tv__split {
  display: grid;
  /* 左筛选占 2 列宽 / 右影片占 4 列(用户指定 6 等分: 2 + 4) */
  grid-template-columns: 2fr 4fr;
  gap: var(--jc-space-6);
  align-items: start;
}

/* 左: 玻璃面板 + 分区 */
.jc-cs-tv__filter {
  border-radius: var(--jc-radius-xl, 20px);
  border: 1px solid var(--jc-tv-stroke, rgba(255, 255, 255, 0.13));
  background: var(--jc-tv-glass, rgba(21, 21, 28, 0.88));
  padding: var(--jc-space-4) var(--jc-space-4) var(--jc-space-5);
}
.jc-cs-tv__grp {
  padding: var(--jc-space-3) 0;
}
.jc-cs-tv__grp + .jc-cs-tv__grp {
  border-top: 1px solid var(--jc-tv-stroke, rgba(255, 255, 255, 0.13));
}
.jc-cs-tv__grp-title {
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-semibold);
  color: var(--jc-text-muted);
  margin-bottom: var(--jc-space-2);
}
.jc-cs-tv__grp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--jc-space-2);
}
/* 竖栏空间窄, chip 再收一档(高 40→34 / 内边距 16→10 / 字号 sm→xs):
   左栏只有 2/6 屏宽, 原来的尺寸一屏放不下几个选项 */
.jc-cs-tv__chip {
  height: 34px;
  padding: 0 10px;
  font-size: var(--jc-fs-xs);
}

/* 右: 结果主区 — 影片固定 4 列(用户指定) */
.jc-cs-tv__main {
  min-width: 0;
}
.jc-cs-tv__main .jc-tv-grid {
  grid-template-columns: repeat(4, 1fr);
}

/* 翻页 */
.jc-cs-tv__pager {
  margin-top: var(--jc-space-8);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--jc-space-4);
}
.jc-cs-tv__pager-info {
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-base);
  font-weight: var(--jc-fw-semibold);
  min-width: 72px;
  text-align: center;
}
.jc-cs-tv__pager .jc-tv-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>

<script setup lang="ts">
import type { Card } from '@/types/film'
import FilmCard from './FilmCard.vue'

interface Props {
  items: Card[]
  /** 间距（CSS 值），默认按断点 */
  gap?: string
  /** 每个 item 的 key 字段，默认 id */
  itemKey?: keyof Card
}

const props = withDefaults(defineProps<Props>(), {
  gap: '',
  itemKey: 'mid'
})

function getItemKey(item: Card, idx: number): string | number {
  const v = item[props.itemKey as keyof Card]
  if (typeof v === 'string' || typeof v === 'number') return v
  return idx
}
</script>

<template>
  <div
    class="jc-film-grid"
    :style="gap ? { gap } : undefined"
  >
    <div
      v-for="(item, idx) in items"
      :key="getItemKey(item, idx)"
      class="jc-film-grid__cell"
    >
      <slot name="item" :item="item" :index="idx">
        <FilmCard :item="item" :show-title-below="true" />
      </slot>
    </div>
  </div>
</template>

<style scoped>
.jc-film-grid {
  display: grid;
  /* 列数/间距取自 theme.css 的全站统一阶梯(与首页横滚行、各列表页同阶梯):
     移动 3 → ≥480 4 → ≥768 5 → ≥1024 及以上恒 6。改列数只需改 theme.css 一处。 */
  grid-template-columns: repeat(var(--jc-list-cols), minmax(0, 1fr));
  gap: var(--jc-list-gap);
}
</style>

<style>
[data-mode='tv'] .jc-film-grid {
  /* P0: 列数自适应 — 960 视口 ~5 列, 1920 ~7 列, 不再写死 8(大屏卡太小/小屏挤) */
  grid-template-columns: repeat(auto-fit, minmax(var(--jc-tv-card-min, 200px), 1fr));
  gap: var(--jc-space-6);
}
</style>

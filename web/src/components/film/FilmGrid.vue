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
    class="gf-film-grid"
    :style="gap ? { gap } : undefined"
  >
    <div
      v-for="(item, idx) in items"
      :key="getItemKey(item, idx)"
      class="gf-film-grid__cell"
    >
      <slot name="item" :item="item" :index="idx">
        <FilmCard :item="item" :show-title-below="true" />
      </slot>
    </div>
  </div>
</template>

<style scoped>
.gf-film-grid {
  display: grid;
  /* 移动端默认 3 列, 与 bilibili/腾讯视频移动端密度一致 */
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--gf-space-2);
}
@media (min-width: 480px) {
  .gf-film-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--gf-space-3);
  }
}
@media (min-width: 768px) {
  .gf-film-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--gf-space-4);
  }
}
@media (min-width: 1024px) {
  .gf-film-grid {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }
}
@media (min-width: 1440px) {
  .gf-film-grid {
    grid-template-columns: repeat(6, minmax(0, 1fr));
  }
}
@media (min-width: 1920px) {
  .gf-film-grid {
    grid-template-columns: repeat(8, minmax(0, 1fr));
    gap: var(--gf-space-6);
  }
}

</style>

<style>
[data-mode='tv'] .gf-film-grid {
  /* P0: 列数自适应 — 960 视口 ~5 列, 1920 ~7 列, 不再写死 8(大屏卡太小/小屏挤) */
  grid-template-columns: repeat(auto-fit, minmax(var(--gf-tv-card-min, 200px), 1fr));
  gap: var(--gf-space-6);
}
</style>

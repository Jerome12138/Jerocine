<script setup lang="ts">
import type { Card } from '@/types/film'
import FilmCard from './FilmCard.vue'

interface Props {
  items: Card[]
  title?: string
}

withDefaults(defineProps<Props>(), {
  title: '相关推荐'
})
</script>

<template>
  <section
    v-if="items.length"
    class="jc-related-list flex flex-col gap-[var(--jc-space-4)]"
  >
    <h2
      v-if="title"
      class="text-[length:var(--jc-fs-lg)] font-[var(--jc-fw-bold)] text-primary leading-[var(--jc-lh-snug)]"
    >
      {{ title }}
    </h2>
    <div class="jc-related-list__grid">
      <FilmCard
        v-for="(item, idx) in items"
        :key="(item.mid ?? idx) + '-' + idx"
        :item="item"
        :show-title-below="true"
      />
    </div>
  </section>
</template>

<style scoped>
.jc-related-list__grid {
  display: grid;
  /* 列数/间距取自 theme.css 的全站统一阶梯(与首页"猜你喜欢"网格、FilmGrid 同阶梯):
     移动 3 → ≥480 4 → ≥768 5 → ≥1024 及以上恒 6; TV 由 theme.css 接管为恒 6。
     改列数只需改 theme.css 一处。 */
  grid-template-columns: repeat(var(--jc-list-cols), minmax(0, 1fr));
  gap: var(--jc-list-gap);
}
</style>

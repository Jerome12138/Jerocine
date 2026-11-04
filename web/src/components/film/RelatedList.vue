<script setup lang="ts">
import type { Card } from '@/types/film'
import FilmCard from './FilmCard.vue'
import SectionHeader from '@/components/base/SectionHeader.vue'

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
    <!-- 区块标题(W1e): 裸 h2 形态(无 header 包装/更多链接), 默认类与原逐字同 -->
    <SectionHeader v-if="title" :title="title" bare />
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

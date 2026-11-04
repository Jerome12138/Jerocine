<script setup lang="ts">
import BaseSkeleton from './BaseSkeleton.vue'

/**
 * 骨架网格(W1c): BaseSkeleton 的版式组合(props: count/layout, 内部仍用 BaseSkeleton)。
 * layout:
 *   - poster(默认): 桌面海报网格 —— jc-card-grid(theme.css 全局) 包裹 N 个海报骨架;
 *     ClassifySearch/HotBoard 桌面骨架。
 *   - poster-tv: TV 网格 —— jc-tv-grid(tv-cards.css 全局) 包裹; ClassifySearch/HotBoard/
 *     Classify 的 TV 骨架。
 *   - items: 裸条目 fragment(无 wrapper) —— 作页面既有网格容器的直接子元素(Classify 段式
 *     骨架 / Home 横滚行骨架的 grid 直接子元素, 包 wrapper 会破坏页面网格)。
 *   - lines: 单根 BaseSkeleton(count) 纵向行 —— Search 列表骨架等。
 * height/ratio/width/shape 原样透传 BaseSkeleton(不传 = 用 BaseSkeleton 自身默认);
 * 各调用点保持原几何, 本件不新增视觉。
 */
withDefaults(
  defineProps<{
    /** 骨架条目数 */
    count?: number
    /** 版式 */
    layout?: 'poster' | 'poster-tv' | 'items' | 'lines'
    /** 透传 BaseSkeleton */
    height?: string
    ratio?: string
    width?: string
    shape?: 'rect' | 'circle' | 'text'
  }>(),
  { count: 12, layout: 'poster' }
)
</script>

<template>
  <div v-if="layout === 'poster'" class="jc-card-grid">
    <BaseSkeleton
      v-for="i in count"
      :key="i"
      :height="height"
      :ratio="ratio"
      :width="width"
      :shape="shape"
    />
  </div>
  <div v-else-if="layout === 'poster-tv'" class="jc-tv-grid">
    <BaseSkeleton
      v-for="i in count"
      :key="i"
      :height="height"
      :ratio="ratio"
      :width="width"
      :shape="shape"
    />
  </div>
  <template v-else-if="layout === 'items'">
    <BaseSkeleton
      v-for="i in count"
      :key="i"
      :height="height"
      :ratio="ratio"
      :width="width"
      :shape="shape"
    />
  </template>
  <BaseSkeleton v-else :count="count" :height="height" :ratio="ratio" :width="width" :shape="shape" />
</template>

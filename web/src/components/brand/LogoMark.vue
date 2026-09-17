<script setup lang="ts">
/**
 * LogoMark - 站点品牌 logo
 *
 * 品牌来自后台 site_config（品牌中立化）：
 * - 配了 logo 图 URL → 渲染图片（高度随 size，object-fit: contain）
 * - 未配 logo → 渲染 siteName 文字（渐变色斜体字标，适合 TV 远观）
 * - siteName 兜底为中性词「影视」，不硬编码任何个人品牌
 *
 * 字号 / 整体大小用 size prop 控制（字号 = size * 0.7）
 */
import { computed } from 'vue'
import { useSiteStore } from '@/stores/site'

interface Props {
  size?: number
  showText?: boolean
}
const props = withDefaults(defineProps<Props>(), {
  size: 36,
  showText: true
})

const site = useSiteStore()
const siteName = computed(() => site.basic?.siteName || '影视')
const logoUrl = computed(() => site.basic?.logo || '')
</script>

<template>
  <span class="jc-logo" :style="{ height: size + 'px' }">
    <img
      v-if="logoUrl"
      :src="logoUrl"
      :alt="siteName"
      class="jc-logo__img"
      :style="{ height: size + 'px' }"
      loading="lazy"
    />
    <span
      v-else-if="showText"
      class="jc-logo__text"
      :style="{ fontSize: size * 0.7 + 'px' }"
    >
      {{ siteName }}
    </span>
  </span>
</template>

<style scoped>
.jc-logo {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  line-height: 1;
}
.jc-logo__img {
  display: block;
  width: auto;
  max-width: 220px;
  object-fit: contain;
  border-radius: 4px;
}
.jc-logo__text {
  display: inline-block;
  font-weight: 800;
  font-style: italic;
  letter-spacing: 0.5px;
  /* 斜体字形右倾, background-clip:text 的绘制区域以文字盒为界,
     最右字符的右缘会被裁掉一角 —— 加右内边距把渐变背景铺到斜体溢出处 */
  padding-inline-end: 6px;
  background-image: linear-gradient(135deg, #9b49e7 0%, #ff5cc4 100%);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  color: transparent;
  white-space: nowrap;
}
</style>

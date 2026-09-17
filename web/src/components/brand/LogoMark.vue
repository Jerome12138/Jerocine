<script setup lang="ts">
/**
 * LogoMark - 站点品牌字标（纯文字，品牌中立化）
 *
 * - 显示后台 site_config.siteName（渐变色斜体字标，适合 TV 远观视场）
 * - siteName 兜底为中性词「影视」，不硬编码任何个人品牌
 * - 后台 logo 字段不在此渲染图片（用户明确：顶栏保持文字字标，
 *   logo 仅用于浏览器标签页 icon，见 App.vue applyFavicon）
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
</script>

<template>
  <span class="jc-logo" :style="{ height: size + 'px' }">
    <span
      v-if="showText"
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

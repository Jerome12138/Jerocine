<script setup lang="ts">
/**
 * 「看到第 N 集」渐变角标(W1d): 海报左上角品牌渐变胶囊 + 白字 + 超长省略。
 * 两处桌面视觉逐字收敛(规格 §1 W1d):
 *   - rail(默认): 首页继续观看行原款(原 .jc-continue__ep: z-2, max-width 全宽减 2×space-2);
 *   - page: 历史页原款(原 .jc-history-ep: z-4, max-width 右侧再留 28px 删除钮位)。
 * 空文案的显隐由调用方 v-if 控制(无集数信息时不渲染), 本件只负责视觉与 slot。
 * TV 版(jc-tv-card .ep)不在本件范围(TV DOM 与全局样式不动)。
 */
withDefaults(defineProps<{ variant?: 'rail' | 'page' }>(), { variant: 'rail' })
</script>

<template>
  <span class="jc-ep-badge" :class="variant === 'page' ? 'jc-ep-badge--page' : 'jc-ep-badge--rail'">
    <slot />
  </span>
</template>

<style scoped>
/* 公共: 品牌渐变胶囊(两处原款逐字相同的部分) */
.jc-ep-badge {
  position: absolute;
  top: var(--jc-space-2);
  left: var(--jc-space-2);
  padding: 2px 8px;
  border-radius: var(--jc-radius-sm);
  background-image: var(--jc-brand-gradient);
  color: #fff;
  font-size: var(--jc-fs-xs);
  font-weight: var(--jc-fw-semibold);
  line-height: 1.4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
/* rail: 首页继续观看行(原 .jc-continue__ep) */
.jc-ep-badge--rail {
  z-index: 2;
  max-width: calc(100% - var(--jc-space-2) * 2);
}
/* page: 历史页(原 .jc-history-ep; z-4 + 右侧预留删除按钮(24px)的位置, 过长集名截断) */
.jc-ep-badge--page {
  z-index: 4;
  max-width: calc(100% - var(--jc-space-2) * 2 - 28px);
}
</style>

<script setup lang="ts">
import BaseIcon from './BaseIcon.vue'

/**
 * 卡角删除钮(W2c): 收藏卡/历史卡海报右上角的圆形 ✕ 小钮收编
 * (FavoritesView:174-181 / HistoryView:327-334, 公共类逐字相同, 差异经 props 承载):
 * - label: aria-label 全文(取消收藏 X / 从历史中移除 Y);
 * - zClass: 海报覆盖层内的 z 层级(收藏卡 z-2 / 历史卡 z-4, 字面量留在调用方供 UnoCSS 提取);
 * - click: 上抛原生事件, 由调用方 preventDefault/stopPropagation + 移除(行为不变)。
 */
defineProps<{
  label: string
  zClass: string
}>()

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()
</script>

<template>
  <button
    type="button"
    class="absolute top-[var(--jc-space-2)] right-[var(--jc-space-2)] w-[24px] h-[24px] rounded-full bg-[rgba(0,0,0,0.6)] hover:bg-[rgba(0,0,0,0.85)] flex-center text-white transition-colors"
    :class="zClass"
    :aria-label="label"
    @click="emit('click', $event)"
  >
    <BaseIcon name="close" size="14px" />
  </button>
</template>

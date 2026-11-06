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
    class="jc-corner-remove jc-hotarea absolute top-[var(--jc-space-2)] right-[var(--jc-space-2)] w-[24px] h-[24px] rounded-full bg-[rgba(0,0,0,0.6)] hover:bg-[rgba(0,0,0,0.85)] flex-center text-white transition-colors"
    :class="zClass"
    :aria-label="label"
    @click="emit('click', $event)"
  >
    <BaseIcon name="close" size="14px" />
  </button>
</template>

<style scoped>
/* 本钮的本职就是「绝对定位到卡角」—— 显式锁死 position, 避免被 .jc-hotarea 的
   position 声明(legacy 内核 :where() 失效时)或其它全局样式压成 relative。
   2026-11-06 修复: 曾被压成 relative → 历史页漂到左下角 / 收藏页被 overflow:hidden 裁掉。 */
.jc-corner-remove {
  position: absolute;
}
</style>

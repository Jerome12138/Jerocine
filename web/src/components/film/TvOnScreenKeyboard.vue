<script setup lang="ts">
/**
 * TV 字母网格虚拟键盘(遥控器输入) —— 搜索 / 登录共用。
 * 输入字母即可做"首字母快搜"(拼音首字母, 如 FRXXZ→凡人修仙传)或完整字母匹配。
 * 受控: v-model(modelValue) + emit。键位走 data-focusable 供空间导航聚焦。
 */
const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'enter'): void
}>()

const KEYS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'.split('')

function press(k: string): void {
  emit('update:modelValue', props.modelValue + k)
}
function backspace(): void {
  emit('update:modelValue', props.modelValue.slice(0, -1))
}
function clear(): void {
  emit('update:modelValue', '')
}
</script>

<template>
  <div class="gf-tv-kbd" role="group" aria-label="字母键盘">
    <button
      v-for="k in KEYS"
      :key="k"
      type="button"
      class="gf-tv-key"
      data-focusable="true"
      tabindex="0"
      @click="press(k)"
    >
      {{ k }}
    </button>
    <button type="button" class="gf-tv-key gf-tv-key--wide" data-focusable="true" tabindex="0" @click="backspace">⌫ 退格</button>
    <button type="button" class="gf-tv-key gf-tv-key--wide" data-focusable="true" tabindex="0" @click="press(' ')">␣ 空格</button>
    <button type="button" class="gf-tv-key gf-tv-key--wide" data-focusable="true" tabindex="0" @click="clear">✕ 清空</button>
  </div>
</template>

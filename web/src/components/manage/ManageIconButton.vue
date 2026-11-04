<script setup lang="ts">
import BaseIcon from '@/components/base/BaseIcon.vue'

/**
 * ManageIconButton — manage 区「裸 button + hover 变 danger」删除类小按钮（§6 组合组件，R1 评审裁决实施）。
 * 统一契约：默认 muted（文字形态继承父级色）、hover 文字变 danger、focus-visible 焦点环、
 * data-focusable 恒挂（disabled 摘除）、type=button —— 三处调用点（FilmClassView 父/子分类删除钮、
 * FileGalleryView 缩略图删除钮）原先各写一份裸 button + hover 类，现收口于此。
 *
 * - icon 形态（传 icon prop）：36px 定值方块。刻意不走 --jc-ctrl-h 令牌：TV 档会放大到 48px，
 *   改变 FilmClassView 既有布局（D⑨「按原有 UI」），常规档同为 36px 像素不变。
 * - 文字形态（默认槽）：行内文字钮（如 FileGalleryView「删除」），无障碍名由槽文字提供。
 */
interface Props {
  /** icon 形态的图标名（BaseIcon name，如 trash）；不传 = 文字形态（默认槽内容） */
  icon?: string
  /** 无障碍名：icon 形态渲染为 aria-label（必传）；文字形态可省（槽文字即名称） */
  label?: string
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  icon: '',
  label: '',
  disabled: false
})

const emit = defineEmits<{ (e: 'click', evt: MouseEvent): void }>()

function onClick(e: MouseEvent): void {
  // disabled 原生不触发 click，无需额外守卫
  emit('click', e)
}
</script>

<template>
  <button
    type="button"
    class="jc-icon-btn inline-flex items-center justify-center outline-none"
    :class="icon ? 'jc-icon-btn--icon w-[36px] h-[36px] shrink-0 rounded-[var(--jc-radius-md)] text-muted' : ''"
    :aria-label="icon ? props.label : undefined"
    :disabled="props.disabled"
    :data-focusable="props.disabled ? undefined : 'true'"
    @click="onClick"
  >
    <BaseIcon v-if="icon" :name="icon" size="16px" />
    <slot v-else />
  </button>
</template>

<style scoped>
/* 文字/图标色过渡走令牌时长，与 FilmClassView 原 .jc-cat__del 同款 */
.jc-icon-btn {
  transition:
    color var(--jc-dur-fast) var(--jc-ease-standard),
    background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
/* 统一契约核心：hover 变 danger（icon 形态另有浅色底，与原 .jc-cat__del:hover 一致） */
.jc-icon-btn:hover:not(:disabled) {
  color: var(--jc-danger);
}
.jc-icon-btn--icon:hover:not(:disabled) {
  background-color: rgba(255, 255, 255, 0.06);
}
/* 键盘焦点环画在按钮上，与全局 --jc-shadow-focus-ring 令牌同源 */
.jc-icon-btn:focus-visible {
  outline: none;
  box-shadow: var(--jc-shadow-focus-ring);
}
/* disabled：沿用 FileGalleryView 原款（半透明 + 摘除指针事件） */
.jc-icon-btn:disabled {
  opacity: 0.4;
  pointer-events: none;
}
</style>

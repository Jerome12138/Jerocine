<script setup lang="ts">
/**
 * ManageSheet — manage 区统一弹窗组件, 按视口自适应:
 *   desktop / tablet → 居中 modal (复用 BaseDialog 视觉, width 可调)
 *   mobile + mobileMode=sheet → 底部 sheet (max 75vh, grabber 装饰条)
 *   mobile + mobileMode=fullsheet → 全屏 sheet (顶部 ← 返回 / 标题 / 操作 slot)
 *
 * 关闭方式 = 遮罩点击 / ESC / footer 按钮 (touch 下滑关闭手势不做)。
 *
 * 选用建议 (mobile):
 *   ≤5 字段或确认型 → sheet
 *   >5 字段或富文本/多步 → fullsheet
 */
import { computed, watch, onBeforeUnmount } from 'vue'
import { useViewMode } from '@/composables/useViewMode'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { acquireOverlay, releaseOverlay, isTopOverlay } from '@/composables/useOverlayStack'

const props = withDefaults(defineProps<{
  modelValue: boolean
  title?: string
  /** desktop/tablet 形态, 当前只支持 modal */
  desktopMode?: 'modal'
  /** mobile 形态: sheet (底部) 或 fullsheet (全屏) */
  mobileMode?: 'sheet' | 'fullsheet'
  /** 点击遮罩关闭 */
  closeOnOverlay?: boolean
  /** modal 形态宽度 (CSS 值), 对齐 BaseDialog.width */
  width?: string
}>(), {
  desktopMode: 'modal',
  mobileMode: 'sheet',
  closeOnOverlay: true,
  width: '480px'
})

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'close'): void
}>()

const { isMobile } = useViewMode()
const variant = computed<'modal' | 'sheet' | 'fullsheet'>(() => {
  if (!isMobile.value) return 'modal'
  return props.mobileMode
})

/** 本实例在弹层栈中的层序; null = 未打开 */
let overlaySeq: number | null = null

function releaseOverlaySlot(): void {
  if (overlaySeq !== null) {
    releaseOverlay(overlaySeq)
    overlaySeq = null
  }
}

function close(): void {
  emit('update:modelValue', false)
  emit('close')
}

function onOverlayClick(): void {
  if (props.closeOnOverlay) close()
}

// ESC 只归属最顶层弹层(叠层时底层不响应, 复核 #7); 归属判断由栈承担, 不再 stopPropagation
function onKey(e: KeyboardEvent): void {
  if (e.key === 'Escape' && props.modelValue && overlaySeq !== null && isTopOverlay(overlaySeq)) {
    close()
  }
}
watch(() => props.modelValue, (v) => {
  if (v) {
    document.addEventListener('keydown', onKey)
    // 打开 = 申请一层: 首层给 body 加 data-jc-modal-open(空间导航 Esc 让位)+ 滚动锁;
    // immediate: 挂载即开的弹层(modelValue 初始 true)同样生效(复核 #8)
    overlaySeq = acquireOverlay()
  } else {
    document.removeEventListener('keydown', onKey)
    releaseOverlaySlot()
  }
}, { immediate: true })
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKey)
  releaseOverlaySlot()
})
</script>

<template>
  <Teleport to="body">
    <Transition name="jc-sheet-fade">
      <div
        v-if="modelValue"
        class="jc-sheet__overlay fixed inset-0 z-[var(--jc-z-overlay,90)] bg-black/60"
        @click.self="onOverlayClick"
      >
        <!-- modal (desktop/tablet) -->
        <div
          v-if="variant === 'modal'"
          class="jc-sheet--modal absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 bg-elevated rounded-[var(--jc-radius-md)] shadow-card-lg overflow-hidden flex flex-col max-h-[90vh]"
          :style="{ width: '90%', maxWidth: width }"
          @click.stop
        >
          <!-- header 恒渲染: 无 title 也显示关闭按钮(复核 #8) -->
          <header class="px-[var(--jc-space-5)] py-[var(--jc-space-4)] border-b border-subtle flex items-center justify-between">
            <h3 v-if="title" class="font-[var(--jc-fw-semibold)] text-primary">{{ title }}</h3>
            <button
              type="button"
              class="text-muted hover:text-primary min-h-[44px] min-w-[44px] flex items-center justify-center"
              :class="title ? '' : 'ml-auto'"
              data-focusable="true"
              @click="close"
            >
              <BaseIcon name="close" size="18px" />
              <span class="sr-only">关闭</span>
            </button>
          </header>
          <div class="p-[var(--jc-space-5)] overflow-y-auto flex-1">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="px-[var(--jc-space-5)] py-[var(--jc-space-4)] border-t border-subtle flex justify-end gap-[var(--jc-space-3)]">
            <slot name="footer" />
          </footer>
        </div>

        <!-- sheet (mobile 底部弹起) -->
        <div
          v-else-if="variant === 'sheet'"
          class="jc-sheet--sheet absolute left-0 right-0 bottom-0 bg-elevated rounded-t-[var(--jc-radius-lg)] max-h-[75vh] flex flex-col"
          @click.stop
        >
          <div class="w-[40px] h-[4px] bg-white/30 rounded-full mx-auto mt-[var(--jc-space-2)]"></div>
          <header v-if="title" class="px-[var(--jc-space-5)] py-[var(--jc-space-3)] text-center font-[var(--jc-fw-semibold)] text-primary">
            {{ title }}
          </header>
          <div class="px-[var(--jc-space-5)] pb-[var(--jc-space-4)] overflow-y-auto flex-1">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="px-[var(--jc-space-5)] py-[var(--jc-space-4)] border-t border-subtle flex gap-[var(--jc-space-3)]">
            <slot name="footer" />
          </footer>
        </div>

        <!-- fullsheet (mobile 全屏) -->
        <div
          v-else
          class="jc-sheet--fullsheet absolute inset-0 bg-base flex flex-col"
          @click.stop
        >
          <header class="px-[var(--jc-space-4)] py-[var(--jc-space-3)] border-b border-subtle flex items-center justify-between min-h-[56px]">
            <button
              type="button"
              class="flex items-center gap-[var(--jc-space-2)] text-link min-h-[44px] min-w-[44px]"
              data-focusable="true"
              @click="close"
            >
              <BaseIcon name="chevron-left" size="20px" />
              <span>返回</span>
            </button>
            <h3 v-if="title" class="font-[var(--jc-fw-semibold)] text-primary flex-1 text-center truncate">{{ title }}</h3>
            <div class="min-w-[44px]">
              <slot name="header-action" />
            </div>
          </header>
          <div class="flex-1 overflow-y-auto p-[var(--jc-space-5)]">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="px-[var(--jc-space-5)] py-[var(--jc-space-4)] border-t border-subtle flex gap-[var(--jc-space-3)]">
            <slot name="footer" />
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.jc-sheet-fade-enter-active,
.jc-sheet-fade-leave-active { transition: opacity var(--jc-dur-fast) ease; }
.jc-sheet-fade-enter-from,
.jc-sheet-fade-leave-to { opacity: 0; }
</style>

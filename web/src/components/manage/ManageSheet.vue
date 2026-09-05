<script setup lang="ts">
/**
 * ManageSheet — manage 区统一弹窗组件, 按视口自适应:
 *   desktop / tablet → 居中 modal (复用 BaseDialog 视觉)
 *   mobile + mobileMode=sheet → 底部 sheet (max 75vh, grabber, 下拉/点遮罩关)
 *   mobile + mobileMode=fullsheet → 全屏 sheet (顶部 ← 返回 / 标题 / 操作 slot)
 *
 * 选用建议 (mobile):
 *   ≤5 字段或确认型 → sheet
 *   >5 字段或富文本/多步 → fullsheet
 */
import { computed, watch, onBeforeUnmount } from 'vue'
import { useViewMode } from '@/composables/useViewMode'
import BaseIcon from '@/components/base/BaseIcon.vue'

const props = withDefaults(defineProps<{
  modelValue: boolean
  title?: string
  /** desktop/tablet 形态, 当前只支持 modal */
  desktopMode?: 'modal'
  /** mobile 形态: sheet (底部) 或 fullsheet (全屏) */
  mobileMode?: 'sheet' | 'fullsheet'
  /** 点击遮罩关闭 (sheet 模式总是允许下拉关) */
  closeOnOverlay?: boolean
}>(), {
  desktopMode: 'modal',
  mobileMode: 'sheet',
  closeOnOverlay: true
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

function close(): void {
  emit('update:modelValue', false)
  emit('close')
}

function onOverlayClick(): void {
  if (props.closeOnOverlay) close()
}

// ESC 关闭 (与 BaseDialog 一致)
function onKey(e: KeyboardEvent): void {
  if (e.key === 'Escape' && props.modelValue) close()
}
// 给 body 加 data-gf-modal-open, 让空间导航 Esc 让位给本组件 close (TV 弹窗里按 Esc 不应整页回退)
function setModalFlag(open: boolean): void {
  if (typeof document === 'undefined') return
  if (open) document.body.setAttribute('data-gf-modal-open', '1')
  else document.body.removeAttribute('data-gf-modal-open')
}
watch(() => props.modelValue, (v) => {
  if (typeof document === 'undefined') return
  if (v) {
    document.addEventListener('keydown', onKey)
    // 锁滚动
    document.body.style.overflow = 'hidden'
    setModalFlag(true)
  } else {
    document.removeEventListener('keydown', onKey)
    document.body.style.overflow = ''
    setModalFlag(false)
  }
})
onBeforeUnmount(() => {
  if (typeof document !== 'undefined') {
    document.removeEventListener('keydown', onKey)
    document.body.style.overflow = ''
    setModalFlag(false)
  }
})
</script>

<template>
  <Teleport to="body">
    <Transition name="gf-sheet-fade">
      <div
        v-if="modelValue"
        class="gf-sheet__overlay fixed inset-0 z-[var(--gf-z-overlay,90)] bg-black/60"
        @click.self="onOverlayClick"
      >
        <!-- modal (desktop/tablet) -->
        <div
          v-if="variant === 'modal'"
          class="gf-sheet--modal absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[90%] max-w-[480px] bg-elevated rounded-[var(--gf-radius-md)] shadow-card-lg overflow-hidden flex flex-col max-h-[90vh]"
          @click.stop
        >
          <header v-if="title" class="px-[var(--gf-space-5)] py-[var(--gf-space-4)] border-b border-subtle flex items-center justify-between">
            <h3 class="font-[var(--gf-fw-semibold)] text-primary">{{ title }}</h3>
            <button
              type="button"
              class="text-muted hover:text-primary min-h-[44px] min-w-[44px] flex items-center justify-center"
              data-focusable="true"
              @click="close"
            >
              <BaseIcon name="close" size="18px" />
              <span class="sr-only">关闭</span>
            </button>
          </header>
          <div class="p-[var(--gf-space-5)] overflow-y-auto flex-1">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="px-[var(--gf-space-5)] py-[var(--gf-space-4)] border-t border-subtle flex justify-end gap-[var(--gf-space-3)]">
            <slot name="footer" />
          </footer>
        </div>

        <!-- sheet (mobile 底部弹起) -->
        <div
          v-else-if="variant === 'sheet'"
          class="gf-sheet--sheet absolute left-0 right-0 bottom-0 bg-elevated rounded-t-[var(--gf-radius-lg)] max-h-[75vh] flex flex-col"
          @click.stop
        >
          <div class="w-[40px] h-[4px] bg-white/30 rounded-full mx-auto mt-[var(--gf-space-2)]"></div>
          <header v-if="title" class="px-[var(--gf-space-5)] py-[var(--gf-space-3)] text-center font-[var(--gf-fw-semibold)] text-primary">
            {{ title }}
          </header>
          <div class="px-[var(--gf-space-5)] pb-[var(--gf-space-4)] overflow-y-auto flex-1">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="px-[var(--gf-space-5)] py-[var(--gf-space-4)] border-t border-subtle flex gap-[var(--gf-space-3)]">
            <slot name="footer" />
          </footer>
        </div>

        <!-- fullsheet (mobile 全屏) -->
        <div
          v-else
          class="gf-sheet--fullsheet absolute inset-0 bg-base flex flex-col"
          @click.stop
        >
          <header class="px-[var(--gf-space-4)] py-[var(--gf-space-3)] border-b border-subtle flex items-center justify-between min-h-[56px]">
            <button
              type="button"
              class="flex items-center gap-[var(--gf-space-2)] text-link min-h-[44px] min-w-[44px]"
              data-focusable="true"
              @click="close"
            >
              <BaseIcon name="chevron-left" size="20px" />
              <span>返回</span>
            </button>
            <h3 v-if="title" class="font-[var(--gf-fw-semibold)] text-primary flex-1 text-center truncate">{{ title }}</h3>
            <div class="min-w-[44px]">
              <slot name="header-action" />
            </div>
          </header>
          <div class="flex-1 overflow-y-auto p-[var(--gf-space-5)]">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="px-[var(--gf-space-5)] py-[var(--gf-space-4)] border-t border-subtle flex gap-[var(--gf-space-3)]">
            <slot name="footer" />
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.gf-sheet-fade-enter-active,
.gf-sheet-fade-leave-active { transition: opacity var(--gf-dur-fast) ease; }
.gf-sheet-fade-enter-from,
.gf-sheet-fade-leave-to { opacity: 0; }
</style>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

interface Props {
  visible: boolean
  title?: string
  /** 点击遮罩是否关闭 */
  closeOnOverlay?: boolean
  /** ESC 是否关闭 */
  closeOnEsc?: boolean
  /** 宽度（CSS 值），默认 480px */
  width?: string
  /** 是否显示关闭按钮 */
  showClose?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  title: '',
  closeOnOverlay: true,
  closeOnEsc: true,
  width: '480px',
  showClose: true
})

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'close'): void
}>()

const dialogEl = ref<HTMLElement | null>(null)

function handleClose(): void {
  emit('update:visible', false)
  emit('close')
}

function onOverlay(): void {
  if (props.closeOnOverlay) {
    handleClose()
  }
}

function onKeydown(e: KeyboardEvent): void {
  if (!props.visible) return
  if (e.key === 'Escape' && props.closeOnEsc) {
    e.stopPropagation()
    handleClose()
  }
}

// 给 body 加 data-jc-modal-open, 让空间导航 Esc 让位给本对话框 (TV 弹窗里按 Esc 不应整页回退)
function setModalFlag(open: boolean): void {
  if (typeof document === 'undefined') return
  if (open) document.body.setAttribute('data-jc-modal-open', '1')
  else document.body.removeAttribute('data-jc-modal-open')
}

watch(
  () => props.visible,
  async (v) => {
    if (v) {
      window.addEventListener('keydown', onKeydown)
      document.body.style.overflow = 'hidden'
      setModalFlag(true)
      await nextTick()
      // 自动聚焦到对话框，便于键盘 / 遥控器
      dialogEl.value?.focus()
    } else {
      window.removeEventListener('keydown', onKeydown)
      document.body.style.overflow = ''
      setModalFlag(false)
    }
  },
  { immediate: true }
)

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  document.body.style.overflow = ''
  setModalFlag(false)
})

const wrapStyle = computed(() => ({
  width: props.width,
  maxWidth: 'calc(100vw - 32px)'
}))
</script>

<template>
  <Teleport to="body">
    <Transition name="jc-dialog">
      <div
        v-if="visible"
        class="jc-dialog-mask fixed inset-0 flex items-center justify-center"
        :style="{ zIndex: 'var(--jc-z-modal)' }"
        @click.self="onOverlay"
      >
        <div
          ref="dialogEl"
          class="jc-dialog bg-elevated rounded-[var(--jc-radius-2xl)] shadow-card-xl flex flex-col max-h-[calc(100vh-64px)]"
          :style="wrapStyle"
          role="dialog"
          aria-modal="true"
          tabindex="-1"
          @click.stop
        >
          <header
            v-if="title || $slots.header || showClose"
            class="flex items-center justify-between gap-[var(--jc-space-4)] px-[var(--jc-space-6)] py-[var(--jc-space-4)] border-b border-subtle"
          >
            <slot name="header">
              <h3
                class="text-[var(--jc-fs-lg)] font-[var(--jc-fw-semibold)] text-primary truncate"
              >
                {{ title }}
              </h3>
            </slot>
            <button
              v-if="showClose"
              class="jc-dialog-close shrink-0 inline-flex items-center justify-center text-muted hover:text-primary"
              data-focusable="true"
              tabindex="0"
              aria-label="close dialog"
              @click="handleClose"
            >
              <BaseIcon name="close" size="20px" />
            </button>
          </header>

          <div
            class="px-[var(--jc-space-6)] py-[var(--jc-space-5)] overflow-auto text-[var(--jc-fs-md)] text-secondary"
          >
            <slot />
          </div>

          <footer
            v-if="$slots.footer"
            class="flex items-center justify-end gap-[var(--jc-space-3)] px-[var(--jc-space-6)] py-[var(--jc-space-4)] border-t border-subtle"
          >
            <slot name="footer" />
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.jc-dialog-mask {
  background-color: var(--jc-bg-overlay);
  backdrop-filter: blur(4px);
}
.jc-dialog-close {
  width: 36px;
  height: 36px;
  border-radius: var(--jc-radius-md);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: color var(--jc-dur-fast) var(--jc-ease-standard),
    background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-dialog-close:hover {
  background-color: rgba(255, 255, 255, 0.06);
}

.jc-dialog-enter-active,
.jc-dialog-leave-active {
  transition:
    opacity var(--jc-dur-base) var(--jc-ease-standard),
    transform var(--jc-dur-base) var(--jc-ease-standard);
}
.jc-dialog-enter-active .jc-dialog,
.jc-dialog-leave-active .jc-dialog {
  transition:
    opacity var(--jc-dur-base) var(--jc-ease-standard),
    transform var(--jc-dur-base) var(--jc-ease-standard);
}

.jc-dialog-enter-from,
.jc-dialog-leave-to {
  opacity: 0;
}
.jc-dialog-enter-from .jc-dialog,
.jc-dialog-leave-to .jc-dialog {
  opacity: 0;
  transform: scale(0.96);
}
</style>

<style>
/* TV 模式：Dialog 居中加大 + 关闭按钮焦点环 */
/* P0: 弱 WebView 上 backdrop blur 掉帧 → 降级为纯半透明压暗 */
[data-mode='tv'] .jc-dialog-mask {
  backdrop-filter: none;
  background-color: rgba(0, 0, 0, 0.72);
}
[data-mode='tv'] .jc-dialog-close {
  width: 56px;
  height: 56px;
}
[data-mode='tv'] .jc-dialog-close:focus,
[data-mode='tv'] .jc-dialog-close:focus-visible {
  outline: none;
  box-shadow: var(--jc-tv-focus-ring);
  background-color: rgba(255, 255, 255, 0.08);
}
</style>

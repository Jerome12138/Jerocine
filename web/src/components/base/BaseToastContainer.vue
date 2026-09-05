<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { registerToast } from '@/api/http'

interface ToastItem {
  id: number
  type: 'success' | 'error' | 'info' | 'warning'
  msg: string
}

const items = ref<ToastItem[]>([])
let nextId = 1

function push(type: ToastItem['type'], msg: string): void {
  const id = nextId++
  items.value.push({ id, type, msg })
  window.setTimeout(() => {
    items.value = items.value.filter((it) => it.id !== id)
  }, 3000)
}

/** 全屏期间把 toast 宿主切到全屏元素 —— 全屏元素的 DOM 子树之外的任何内容
 * (包括 body 下的 fixed 元素) 都不会被渲染, 不中转则"已跳过片头"等提示在全屏时全部丢失。 */
const toastHost = ref<HTMLElement | null>(null)
function syncFullscreenHost(): void {
  toastHost.value = (document.fullscreenElement as HTMLElement | null) ?? null
}

onMounted(() => {
  registerToast({ push })
  document.addEventListener('fullscreenchange', syncFullscreenHost)
  document.addEventListener('webkitfullscreenchange', syncFullscreenHost)
})

onBeforeUnmount(() => {
  document.removeEventListener('fullscreenchange', syncFullscreenHost)
  document.removeEventListener('webkitfullscreenchange', syncFullscreenHost)
})

function tone(type: ToastItem['type']): string {
  switch (type) {
    case 'success':
      return 'border-l-[3px] border-[var(--gf-success)] bg-[var(--gf-success-soft)]'
    case 'warning':
      return 'border-l-[3px] border-[var(--gf-warning)] bg-[var(--gf-warning-soft)]'
    case 'info':
      return 'border-l-[3px] border-[var(--gf-info)] bg-[var(--gf-info-soft)]'
    case 'error':
    default:
      return 'border-l-[3px] border-[var(--gf-danger)] bg-[var(--gf-danger-soft)]'
  }
}
</script>

<template>
  <!-- 无全屏时挂 body; 有全屏元素时挂进全屏元素内部(fixed 定位以全屏元素为包含块) -->
  <Teleport :to="toastHost || 'body'" :disabled="!toastHost">
    <div
      class="fixed top-[var(--gf-space-4)] right-[var(--gf-space-4)] flex flex-col gap-[var(--gf-space-2)] z-[var(--gf-z-toast)] pointer-events-none"
    >
      <TransitionGroup name="toast">
        <div
          v-for="t in items"
          :key="t.id"
          class="px-[var(--gf-space-4)] py-[var(--gf-space-3)] rounded-[var(--gf-radius-md)] shadow-card-lg text-primary text-sm pointer-events-auto bg-elevated"
          :class="tone(t.type)"
          role="status"
          aria-live="polite"
        >
          {{ t.msg }}
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style>
.toast-enter-active,
.toast-leave-active {
  transition:
    opacity var(--gf-dur-base) var(--gf-ease-standard),
    transform var(--gf-dur-base) var(--gf-ease-standard);
}
.toast-enter-from {
  opacity: 0;
  transform: translateY(-8px);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(16px);
}
</style>

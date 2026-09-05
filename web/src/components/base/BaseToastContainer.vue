<script setup lang="ts">
import { onMounted, ref } from 'vue'
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

onMounted(() => {
  registerToast({ push })
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
  <Teleport to="body">
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

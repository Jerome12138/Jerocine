<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue'
import BaseIcon from '@/components/base/BaseIcon.vue'

/**
 * 图片点击放大(lightbox) —— 全屏遮罩 + 原图等比 contain + 点击空白/ESC 关闭。
 *
 * 用法:
 *   <BaseLightbox v-model:src="previewSrc" :alt="previewAlt" />
 *   点击缩略图时 previewSrc = url 即打开, 置空字符串即关闭。
 *
 * 设计为单图预览(管理页图片核对场景); 不引入左右切换/缩放等重逻辑。
 */
interface Props {
  /** 当前预览的图片地址; 空串/undefined = 关闭 */
  src: string
  /** 无障碍/标题 alt */
  alt?: string
}
const props = withDefaults(defineProps<Props>(), { alt: '' })

const emit = defineEmits<{
  (e: 'update:src', v: string): void
}>()

const visible = computed(() => !!props.src)

function close(): void {
  emit('update:src', '')
}

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') {
    e.stopPropagation()
    close()
  }
}

function setModalFlag(open: boolean): void {
  if (typeof document === 'undefined') return
  if (open) document.body.setAttribute('data-jc-modal-open', '1')
  else document.body.removeAttribute('data-jc-modal-open')
}

watch(
  visible,
  (v) => {
    if (typeof document === 'undefined') return
    if (v) {
      window.addEventListener('keydown', onKeydown)
      document.body.style.overflow = 'hidden'
      setModalFlag(true)
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
  if (typeof document !== 'undefined') {
    document.body.style.overflow = ''
    setModalFlag(false)
  }
})
</script>

<template>
  <Teleport to="body">
    <Transition name="jc-lightbox">
      <div
        v-if="visible"
        class="jc-lightbox fixed inset-0 flex items-center justify-center"
        :style="{ zIndex: 'var(--jc-z-modal)' }"
        role="dialog"
        aria-modal="true"
        aria-label="图片预览"
        @click.self="close"
      >
        <img
          :src="src"
          :alt="alt"
          class="jc-lightbox__img"
          @click.stop
        />
        <button
          type="button"
          class="jc-lightbox__close"
          aria-label="关闭预览"
          data-focusable="true"
          tabindex="0"
          @click="close"
        >
          <BaseIcon name="close" size="22px" />
        </button>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.jc-lightbox {
  background-color: rgba(0, 0, 0, 0.82);
  backdrop-filter: blur(4px);
  padding: var(--jc-space-6);
}
.jc-lightbox__img {
  max-width: min(96vw, 1400px);
  max-height: 92vh;
  object-fit: contain;
  border-radius: var(--jc-radius-sm);
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.6);
  cursor: default;
}
.jc-lightbox__close {
  position: absolute;
  top: var(--jc-space-4);
  right: var(--jc-space-4);
  width: 44px;
  height: 44px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 9999px;
  border: none;
  background-color: rgba(255, 255, 255, 0.12);
  color: #fff;
  cursor: pointer;
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-lightbox__close:hover {
  background-color: rgba(255, 255, 255, 0.24);
}
.jc-lightbox-enter-active,
.jc-lightbox-leave-active {
  transition: opacity var(--jc-dur-base) var(--jc-ease-standard);
}
.jc-lightbox-enter-from,
.jc-lightbox-leave-to {
  opacity: 0;
}
</style>

<style>
[data-mode='tv'] .jc-lightbox {
  backdrop-filter: none;
  background-color: rgba(0, 0, 0, 0.88);
}
[data-mode='tv'] .jc-lightbox__close {
  width: 56px;
  height: 56px;
}
[data-mode='tv'] .jc-lightbox__close:focus,
[data-mode='tv'] .jc-lightbox__close:focus-visible {
  outline: none;
  box-shadow: var(--jc-tv-focus-ring);
  background-color: rgba(255, 255, 255, 0.18);
}
</style>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

interface Props {
  src?: string
  alt?: string
  /** 宽高比，例如 '2/3' '16/9' */
  ratio?: string
  /** 跳过懒加载，直接立即加载（首屏 hero） */
  eager?: boolean
  /** 圆角类（透传），默认无 */
  rounded?: string
  /** object-fit */
  fit?: 'cover' | 'contain' | 'fill' | 'scale-down'
}

const props = withDefaults(defineProps<Props>(), {
  src: '',
  alt: '',
  ratio: '2/3',
  eager: false,
  rounded: '',
  fit: 'cover'
})

const wrapEl = ref<HTMLElement | null>(null)
const visible = ref(false)
const loaded = ref(false)
const errored = ref(false)
const currentSrc = ref<string>('')

let observer: IntersectionObserver | null = null
let blurClearTimer: ReturnType<typeof setTimeout> | null = null
const blurCleared = ref(false)

const aspectStyle = computed(() => {
  if (!props.ratio) {
    // 空 ratio 表示由父容器控制尺寸（hero 全屏背景等场景）
    return { width: '100%', height: '100%' }
  }
  return { aspectRatio: props.ratio }
})

/**
 * 基于 src + alt 算稳定 hash（FNV-1a 32-bit），输出 HSL 暗色主调
 * - hue 全谱 (0-360) 保证多样性
 * - saturation 固定在 18%-28%（低饱和，不刺眼）
 * - lightness 固定在 12%-20%（暗色，与卡片背景调性一致）
 */
function hashStringFNV1a(input: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < input.length; i += 1) {
    h ^= input.charCodeAt(i)
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return h >>> 0
}

const placeholderColor = computed<string>(() => {
  const seed = `${props.src}|${props.alt}` || 'gf-placeholder'
  const h = hashStringFNV1a(seed)
  const hue = h % 360
  const sat = 18 + ((h >> 9) % 11) // 18-28
  const light = 12 + ((h >> 17) % 9) // 12-20
  return `hsl(${hue}, ${sat}%, ${light}%)`
})

const placeholderStyle = computed(() => ({
  backgroundColor: placeholderColor.value
}))

/**
 * 把 http:// 升级成 https:// — 当前页面是 HTTPS 时, 浏览器 (含 WebView) 会
 * 因为 Mixed Content 直接拒掉 http 图片. 多数 CDN 双协议都开, 升级一下能救回
 * 80%+ 的图片. 升级失败的图片走 onerror 回退到占位符.
 */
function safeSrc(raw: string): string {
  if (!raw) return ''
  if (typeof window === 'undefined') return raw
  if (window.location.protocol === 'https:' && raw.startsWith('http://')) {
    return 'https://' + raw.slice('http://'.length)
  }
  return raw
}

function startLoad(): void {
  if (!props.src) {
    errored.value = true
    return
  }
  currentSrc.value = safeSrc(props.src)
}

function onLoaded(): void {
  loaded.value = true
  // 200ms 后清除模糊，形成 blur(8px) → blur(0) 的渐进过渡
  if (blurClearTimer) clearTimeout(blurClearTimer)
  blurClearTimer = setTimeout(() => {
    blurCleared.value = true
  }, 200)
}
function onError(): void {
  errored.value = true
}

onMounted(() => {
  if (props.eager) {
    visible.value = true
    startLoad()
    return
  }
  if (typeof IntersectionObserver === 'undefined' || !wrapEl.value) {
    visible.value = true
    startLoad()
    return
  }
  // 大屏 / TV 视口预加载更远，移动端更近
  const dataMode =
    typeof document !== 'undefined' ? document.documentElement.getAttribute('data-mode') : ''
  const rootMargin =
    dataMode === 'tv' ? '600px 0px' : dataMode === 'desktop' ? '400px 0px' : '200px 0px'
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          visible.value = true
          startLoad()
          observer?.disconnect()
          observer = null
          break
        }
      }
    },
    { rootMargin }
  )
  observer.observe(wrapEl.value)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
  if (blurClearTimer) {
    clearTimeout(blurClearTimer)
    blurClearTimer = null
  }
})

watch(
  () => props.src,
  (next) => {
    loaded.value = false
    errored.value = false
    blurCleared.value = false
    if (blurClearTimer) {
      clearTimeout(blurClearTimer)
      blurClearTimer = null
    }
    if (visible.value && next) {
      currentSrc.value = safeSrc(next)
    }
  }
)
</script>

<template>
  <div
    ref="wrapEl"
    class="gf-base-image relative overflow-hidden"
    :class="rounded"
    :style="{ ...aspectStyle, ...placeholderStyle }"
  >
    <!-- 骨架/占位（shimmer 叠加在确定性颜色之上） -->
    <div
      v-if="!loaded && !errored"
      class="absolute inset-0 gf-base-image__skeleton"
      aria-hidden="true"
    />
    <!-- 真实图片 -->
    <img
      v-if="visible && currentSrc && !errored"
      :src="currentSrc"
      :alt="alt"
      :loading="eager ? 'eager' : 'lazy'"
      :fetchpriority="eager ? 'high' : undefined"
      decoding="async"
      class="gf-base-image__img absolute inset-0 w-full h-full"
      :class="[
        loaded ? 'opacity-100' : 'opacity-0',
        loaded && !blurCleared && 'gf-base-image__img--blurred',
        fit === 'cover' && 'object-cover',
        fit === 'contain' && 'object-contain',
        fit === 'fill' && 'object-fill',
        fit === 'scale-down' && 'object-scale-down'
      ]"
      @load="onLoaded"
      @error="onError"
    />
    <!-- 错误回退（无外部 404.png 时使用渐变） -->
    <div
      v-if="errored"
      class="absolute inset-0 flex items-center justify-center text-muted text-sm gf-base-image__fallback"
      role="img"
      :aria-label="alt || 'image failed to load'"
    >
      <BaseIcon name="image" size="32px" />
    </div>
  </div>
</template>

<style scoped>
.gf-base-image__img {
  transition:
    opacity var(--gf-dur-base) var(--gf-ease-standard),
    filter var(--gf-dur-base) var(--gf-ease-standard);
}

.gf-base-image__img--blurred {
  filter: blur(8px);
}

.gf-base-image__skeleton {
  /* 透明度 ≤ 0.12，让确定性背景色"穿透"可见 */
  background: linear-gradient(
    90deg,
    rgba(255, 255, 255, 0.03) 0%,
    rgba(255, 255, 255, 0.1) 50%,
    rgba(255, 255, 255, 0.03) 100%
  );
  background-size: 200% 100%;
  animation: gf-shimmer 1.4s linear infinite;
}

.gf-base-image__fallback {
  background: linear-gradient(135deg, #1c1d22 0%, #2a2b32 50%, #1c1d22 100%);
}

@keyframes gf-shimmer {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}
</style>

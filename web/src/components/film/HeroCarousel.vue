<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { Card } from '@/types/film'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import { useViewMode } from '@/composables/useViewMode'

interface Props {
  items: Card[]
  /** 自动切换间隔（ms），默认根据 mode：tv 6000 / 其他 4000 */
  interval?: number
  /** 是否显示左右箭头 */
  showArrows?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  interval: 0,
  showArrows: true
})

const router = useRouter()
const { isTV } = useViewMode()

const current = ref(0)
const total = computed(() => props.items.length)
const active = computed(() => props.items[current.value])

// 进度条重启 key: current 变化时 ++, 让 CSS animation 重新挂载
const progressTick = ref(0)
watch(current, () => {
  progressTick.value += 1
})

const effectiveInterval = computed(() => {
  if (props.interval && props.interval > 0) return props.interval
  return isTV.value ? 6000 : 4000
})

let timer: number | null = null
const paused = ref(false)

function go(idx: number): void {
  if (total.value === 0) return
  const next = (idx + total.value) % total.value
  current.value = next
}

function next(): void {
  go(current.value + 1)
}
function prev(): void {
  go(current.value - 1)
}

function startTimer(): void {
  stopTimer()
  if (total.value <= 1) return
  timer = window.setInterval(() => {
    if (!paused.value) next()
  }, effectiveInterval.value)
}
function stopTimer(): void {
  if (timer !== null) {
    window.clearInterval(timer)
    timer = null
  }
}

onMounted(startTimer)
onBeforeUnmount(stopTimer)

watch(effectiveInterval, () => startTimer())
watch(() => props.items.length, () => {
  current.value = 0
  startTimer()
})

// 触摸滑动
let touchStartX = 0
let touchDx = 0
function onTouchStart(e: TouchEvent): void {
  touchStartX = e.touches[0]?.clientX ?? 0
  touchDx = 0
  paused.value = true
}
function onTouchMove(e: TouchEvent): void {
  const x = e.touches[0]?.clientX ?? 0
  touchDx = x - touchStartX
}
function onTouchEnd(): void {
  if (Math.abs(touchDx) > 60) {
    if (touchDx < 0) next()
    else prev()
  }
  paused.value = false
}

// 详情跳转
function gotoDetail(item: Card | undefined): void {
  if (!item) return
  router.push({ path: '/filmDetail', query: { link: String(item.mid) } })
}

// 键盘导航：左右箭头切换
function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'ArrowLeft') {
    prev()
  } else if (e.key === 'ArrowRight') {
    next()
  } else if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    gotoDetail(active.value)
  }
}

const tags = computed<string[]>(() => {
  const it = active.value
  if (!it) return []
  const res: string[] = []
  if (it.year) res.push(String(it.year))
  if (it.cName) res.push(String(it.cName))
  if (it.area) res.push(String(it.area))
  return res
})
</script>

<template>
  <section
    class="gf-hero relative w-full overflow-hidden cursor-pointer"
    data-focus-zone="hero"
    role="button"
    :aria-label="active ? `查看《${active.name}》详情` : undefined"
    aria-roledescription="carousel"
    @click="gotoDetail(active)"
    @mouseenter="paused = true"
    @mouseleave="paused = false"
    @touchstart="onTouchStart"
    @touchmove="onTouchMove"
    @touchend="onTouchEnd"
    @keydown="onKeydown"
    tabindex="0"
    data-focusable="true"
  >
    <!-- 背景图层 -->
    <div class="gf-hero__layers absolute inset-0">
      <div
        v-for="(it, i) in items"
        :key="(it.mid ?? i) + '-' + i"
        class="gf-hero__slide absolute inset-0"
        :class="i === current ? 'opacity-100' : 'opacity-0 pointer-events-none'"
        :aria-hidden="i !== current"
      >
        <BaseImage
          :src="it.cover"
          :alt="it.name"
          ratio=""
          :eager="i === 0"
          fit="cover"
          class="gf-hero__image"
        />
        <!-- 竖海报: TV 模糊铺底 + 右侧清晰竖海报; Web 宽屏同理在右侧展示完整竖海报(避免封面被裁)
        （仅当前 slide 的封面, 与下方热门榜单不重复） -->
        <div
          v-if="it.cover"
          class="gf-hero__poster-tv"
          :style="{ backgroundImage: `url(${JSON.stringify(it.cover)})` }"
          aria-hidden="true"
        />
      </div>
    </div>

    <!-- 蒙版 -->
    <div class="gf-hero__mask-bottom absolute inset-0 pointer-events-none" />
    <div class="gf-hero__mask-left absolute inset-0 pointer-events-none hidden md:block" />

    <!-- 左下信息层 -->
    <div
      v-if="active"
      class="gf-hero__content absolute inset-x-0 bottom-0 container-page"
    >
      <div class="gf-hero__info">
        <div
          v-if="tags.length"
          class="flex flex-wrap gap-[var(--gf-space-2)] mb-[var(--gf-space-3)]"
        >
          <BaseTag
            v-for="(t, i) in tags"
            :key="i"
            variant="purple"
            size="md"
          >
            {{ t }}
          </BaseTag>
        </div>
        <h2 class="gf-hero__title text-primary">
          {{ active.name }}
        </h2>
        <p
          v-if="active.remarks"
          class="gf-hero__desc text-secondary mt-[var(--gf-space-3)] line-clamp-2"
        >
          {{ active.remarks }}
        </p>
      </div>
    </div>

    <!-- 左右箭头 (.stop 阻止冒泡到 section 触发跳转) -->
    <template v-if="showArrows && total > 1">
      <button
        class="gf-hero__arrow gf-hero__arrow--left"
        data-focusable="true"
        tabindex="0"
        aria-label="prev slide"
        @click.stop="prev"
      >
        <BaseIcon name="chevron-left" size="24px" />
      </button>
      <button
        class="gf-hero__arrow gf-hero__arrow--right"
        data-focusable="true"
        tabindex="0"
        aria-label="next slide"
        @click.stop="next"
      >
        <BaseIcon name="chevron-right" size="24px" />
      </button>
    </template>

    <!-- 指示器 -->
    <!-- 指示条 (bilibili 风格底部横条; 当前条带 4s 自动推进进度填充) -->
    <div
      v-if="total > 1"
      class="gf-hero__bars absolute bottom-[var(--gf-space-4)] left-1/2 -translate-x-1/2 flex items-center gap-[var(--gf-space-2)]"
    >
      <button
        v-for="(_, i) in items"
        :key="i"
        class="gf-hero__bar"
        :class="i === current ? 'gf-hero__bar--active' : ''"
        :aria-label="`go to slide ${i + 1}`"
        :aria-current="i === current ? 'true' : 'false'"
        data-focusable="true"
        tabindex="0"
        @click.stop="go(i)"
      >
        <span
          v-if="i === current"
          :key="progressTick"
          class="gf-hero__bar-progress"
          :style="{ animationDuration: effectiveInterval + 'ms', animationPlayState: paused ? 'paused' : 'running' }"
        />
      </button>
    </div>
  </section>
</template>

<style scoped>
/**
 * Hero 容器尺寸策略 —— 各档屏幕都用 aspect-ratio 主导 + 安全区兜底，
 * 避免单纯 vh 在窄竖屏 / 超宽屏 / 横屏小高度下变形：
 *
 *  ┌──────────────────────────────────────────────────────────────────┐
 *  │ 视口            纵横比         min-height   max-height          │
 *  │ < 480 (mobile)  4 / 5         320px        66vh                 │
 *  │ ≥ 480           16 / 10       360px        62vh                 │
 *  │ ≥ 768 (tablet)  16 / 9        420px        70vh                 │
 *  │ ≥ 1024 (PC)     21 / 9        480px        720px                │
 *  │ ≥ 1600 (大屏)   21 / 9        clamp(560,55vh,820)               │
 *  └──────────────────────────────────────────────────────────────────┘
 */
.gf-hero {
  width: 100%;
  /* 手机竖屏：用 16/10 而不是 4/5，避免大图占满半屏 */
  aspect-ratio: 16 / 10;
  min-height: 200px;
  max-height: 42vh;
  background-color: var(--gf-bg-base);
  outline: none;
}

@media (min-width: 480px) {
  .gf-hero {
    aspect-ratio: 16 / 9;
    min-height: 240px;
    max-height: 46vh;
  }
}

@media (min-width: 768px) {
  .gf-hero {
    aspect-ratio: 16 / 9;
    min-height: 300px;
    max-height: 44vh;
  }
}

@media (min-width: 1024px) {
  .gf-hero {
    aspect-ratio: 21 / 9;
    min-height: 340px;
    max-height: 420px;
  }
}

@media (min-width: 1600px) {
  .gf-hero {
    aspect-ratio: 21 / 9;
    min-height: 380px;
    max-height: clamp(360px, 38vh, 480px);
  }
}

/* 横屏小高度设备（手机横屏 / 平板横屏低分辨率）：限制 max-height 防 hero 过高顶走列表 */
@media (orientation: landscape) and (max-height: 600px) {
  .gf-hero {
    max-height: 88vh;
    min-height: 280px;
  }
}

/* TV 模式 hero 的尺寸与图层(沉浸 Banner: 竖图模糊铺底 + 右侧清晰竖海报)
 * 统一在文件底部非 scoped [data-mode='tv'] 块定义, 避免本处与其冲突(曾两处 max-height 打架). */

.gf-hero__image,
.gf-hero__image :deep(img) {
  width: 100%;
  height: 100%;
  border-radius: 0;
}

.gf-hero__slide {
  transition: opacity var(--gf-dur-slow) var(--gf-ease-out);
}

/* Web: 默认隐藏竖海报(移动/平板轮播更矮, 不需); 宽屏(≥1024)在右侧展示完整竖海报,
 * 规避封面被 21/9 横幅裁掉竖图信息的问题. TV 模式尺寸在底部非 scoped 块单独定义. */
.gf-hero__poster-tv {
  display: none;
}
@media (min-width: 1024px) {
  .gf-hero__poster-tv {
    display: block;
    position: absolute;
    top: 50%;
    right: clamp(32px, 5vw, 96px);
    transform: translateY(-50%);
    height: 80%;
    aspect-ratio: 2 / 3;
    border-radius: var(--gf-radius-lg);
    background-color: var(--gf-bg-elevated);
    background-size: cover;
    background-position: center;
    box-shadow: 0 16px 48px rgba(0, 0, 0, 0.5);
    z-index: 1;
  }
}

.gf-hero__mask-bottom {
  background-image: var(--gf-mask-hero-bottom);
}
.gf-hero__mask-left {
  background-image: var(--gf-mask-hero-left);
}

.gf-hero__content {
  padding-top: var(--gf-space-8);
  padding-bottom: var(--gf-space-12);
  z-index: 2;
}
@media (min-width: 1024px) {
  .gf-hero__content {
    padding-bottom: 80px;
  }
}

.gf-hero__info {
  max-width: min(640px, 100%);
}

@media (min-width: 768px) {
  .gf-hero__info {
    max-width: min(640px, 60%);
  }
}

.gf-hero__title {
  font-size: var(--gf-fs-hero);
  font-weight: var(--gf-fw-black);
  line-height: var(--gf-lh-tight);
  letter-spacing: var(--gf-tracking-tight);
}

.gf-hero__desc {
  font-size: var(--gf-fs-md);
  line-height: var(--gf-lh-relaxed);
}

.gf-hero__arrow {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 48px;
  height: 64px;
  display: none;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--gf-radius-md);
  background-color: rgba(0, 0, 0, 0.55);
  color: var(--gf-text-primary);
  cursor: pointer;
  z-index: 3;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    opacity var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-hero__arrow:hover {
  background-color: rgba(0, 0, 0, 0.8);
}
.gf-hero__arrow--left {
  left: var(--gf-space-4);
}
.gf-hero__arrow--right {
  right: var(--gf-space-4);
}

@media (min-width: 768px) {
  .gf-hero__arrow {
    display: inline-flex;
  }
}

/* 指示条 (横条 + 当前条进度填充) */
.gf-hero__bars {
  z-index: 3;
}

.gf-hero__bar {
  position: relative;
  width: 36px;
  height: 3px;
  border-radius: 2px;
  background-color: rgba(255, 255, 255, 0.3);
  border: none;
  padding: 0;
  cursor: pointer;
  overflow: hidden;
  transition: width var(--gf-dur-base) var(--gf-ease-standard);
}

.gf-hero__bar--active {
  width: 56px;
}

.gf-hero__bar:hover {
  background-color: rgba(255, 255, 255, 0.45);
}

.gf-hero__bar:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px rgba(74, 209, 229, 0.8);
}

.gf-hero__bar-progress {
  position: absolute;
  inset: 0;
  background-image: var(--gf-brand-gradient);
  transform: scaleX(0);
  transform-origin: left center;
  animation-name: gf-hero-progress;
  animation-timing-function: linear;
  animation-fill-mode: forwards;
  animation-iteration-count: 1;
}

@keyframes gf-hero-progress {
  from { transform: scaleX(0); }
  to { transform: scaleX(1); }
}

.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>

<style>
/* TV 默认显示箭头（不依赖 hover），加大尺寸 + 安全区缩进 */
[data-mode='tv'] .gf-hero__arrow {
  display: inline-flex;
  width: 64px;
  height: 80px;
}
[data-mode='tv'] .gf-hero__arrow--left {
  left: var(--gf-tv-safe);
}
[data-mode='tv'] .gf-hero__arrow--right {
  right: var(--gf-tv-safe);
}
[data-mode='tv'] .gf-hero__arrow:focus,
[data-mode='tv'] .gf-hero__arrow:focus-visible {
  outline: none;
  background-color: rgba(0, 0, 0, 0.85);
  box-shadow: var(--gf-tv-focus-ring);
}
/* TV 沉浸 Banner —— 占屏 ~55%; 采集源仅竖海报: 同图模糊放大铺底 + 右侧清晰竖海报兜底 */
[data-mode='tv'] .gf-hero {
  aspect-ratio: auto;
  min-height: var(--gf-tv-hero-h, 55vh);
  max-height: var(--gf-tv-hero-h, 55vh);
}
/* 背景层: 静态模糊(非 backdrop-blur)放大压暗, 把竖海报铺满宽幅不露裁切边 */
[data-mode='tv'] .gf-hero__image,
[data-mode='tv'] .gf-hero__image :deep(img) {
  object-fit: cover;
  object-position: center;
  filter: blur(28px) brightness(0.5) saturate(1.1);
  transform: scale(1.18);
}
/* 右侧清晰竖海报(2:3), 真正展示该片封面 */
[data-mode='tv'] .gf-hero__poster-tv {
  position: absolute;
  top: 50%;
  right: clamp(48px, 8vw, 160px);
  transform: translateY(-50%);
  height: 74%;
  aspect-ratio: 2 / 3;
  border-radius: var(--gf-radius-lg);
  background-color: var(--gf-bg-elevated);
  background-size: cover;
  background-position: center;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.65);
  z-index: 1;
}
[data-mode='tv'] .gf-hero__content {
  padding-inline: var(--gf-tv-safe);
  padding-bottom: var(--gf-space-12);
}
[data-mode='tv'] .gf-hero__info {
  max-width: min(720px, 55%);
}
[data-mode='tv'] .gf-hero__desc {
  font-size: var(--gf-fs-lg);
}
[data-mode='tv'] .gf-hero__bar {
  width: 48px;
  height: 4px;
}
[data-mode='tv'] .gf-hero__bar--active {
  width: 72px;
}
</style>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Card } from '@/types/film'
import FilmCard from './FilmCard.vue'

interface Props {
  title?: string
  /** "更多" 链接 router-link to */
  moreLink?: { path: string; query?: Record<string, string | number> } | string
  items: Card[]
  /** key 字段名，默认 id */
  itemKey?: keyof Card
}

const props = withDefaults(defineProps<Props>(), {
  title: '',
  moreLink: '',
  itemKey: 'mid'
})

const scrollEl = ref<HTMLElement | null>(null)
const canScrollLeft = ref(false)
const canScrollRight = ref(false)

function updateArrows(): void {
  const el = scrollEl.value
  if (!el) return
  const left = el.scrollLeft
  const max = el.scrollWidth - el.clientWidth
  // Math.round 消除亚像素: 否则最右时 left 可能永远差零点几px < max-4, 右箭头不消失
  canScrollLeft.value = Math.round(left) > 1
  canScrollRight.value = Math.round(left) < Math.round(max) - 1
}

function scrollByDir(dir: 1 | -1): void {
  const el = scrollEl.value
  if (!el) return
  const delta = el.clientWidth * 0.8 * dir
  el.scrollBy({ left: delta, behavior: 'smooth' })
}

let resizeObserver: ResizeObserver | null = null

/** TV / 桌面键盘焦点：把获得焦点的子项滚到视口居中（仅在 row 内部） */
function onFocusIn(e: FocusEvent): void {
  const target = e.target as HTMLElement | null
  if (!target) return
  // 只在 row 内部 scroll 容器内的目标才接管
  const scroll = scrollEl.value
  if (!scroll || !scroll.contains(target)) return
  // 只对 focusable 子项生效，避免每个内部按钮都触发
  const focusable = target.closest<HTMLElement>('[data-focusable="true"]')
  if (!focusable) return
  // 行内居中滚动（不影响竖向）
  try {
    focusable.scrollIntoView({ inline: 'center', block: 'nearest', behavior: 'smooth' })
  } catch {
    /* ignore */
  }
}

onMounted(() => {
  nextTick(updateArrows)
  scrollEl.value?.addEventListener('scroll', updateArrows, { passive: true })
  scrollEl.value?.addEventListener('focusin', onFocusIn)
  if (typeof ResizeObserver !== 'undefined' && scrollEl.value) {
    resizeObserver = new ResizeObserver(updateArrows)
    resizeObserver.observe(scrollEl.value)
  }
})
onBeforeUnmount(() => {
  scrollEl.value?.removeEventListener('scroll', updateArrows)
  scrollEl.value?.removeEventListener('focusin', onFocusIn)
  resizeObserver?.disconnect()
  resizeObserver = null
})

// 数据异步加载/变化后(首页/分类页 items 来自 API), 重算边界, 否则 canScrollLeft/Right
// 停留在初始 false, 箭头不显示或边界状态不准。
watch(
  () => props.items,
  () => nextTick(updateArrows),
  { flush: 'post' }
)

const moreTo = computed(() => {
  if (!props.moreLink) return null
  return typeof props.moreLink === 'string'
    ? { path: props.moreLink }
    : props.moreLink
})

function getItemKey(item: Card, idx: number): string | number {
  const k = props.itemKey
  const v = item[k as keyof Card]
  if (typeof v === 'string' || typeof v === 'number') return v
  return idx
}
</script>

<template>
  <section class="gf-film-row">
    <header
      v-if="title || moreTo"
      class="container-page flex items-end justify-between gap-[var(--gf-space-4)] mb-[var(--gf-space-3)]"
    >
      <h2
        v-if="title"
        class="gf-film-row__title text-[var(--gf-fs-lg)] font-[var(--gf-fw-bold)] text-primary leading-[var(--gf-lh-snug)]"
      >
        {{ title }}
      </h2>
      <RouterLink
        v-if="moreTo"
        :to="moreTo"
        class="text-link text-[var(--gf-fs-sm)] inline-flex items-center gap-[var(--gf-space-1)] shrink-0"
        data-focusable="true"
        tabindex="0"
      >
        更多
        <BaseIcon name="chevron-right" size="16px" />
      </RouterLink>
    </header>

    <div class="gf-film-row__viewport relative group">
      <!-- 左右遮罩（桌面）：跟随滚动边界显隐，避免常驻遮挡边缘卡片与标题 -->
      <div v-show="canScrollLeft" class="gf-film-row__mask-left absolute inset-y-0 left-0 pointer-events-none" />
      <div v-show="canScrollRight" class="gf-film-row__mask-right absolute inset-y-0 right-0 pointer-events-none" />

      <!-- 横向滚动容器 -->
      <div
        ref="scrollEl"
        class="gf-film-row__scroll flex gap-[var(--gf-space-3)] md:gap-[var(--gf-space-4)] overflow-x-auto scroll-smooth"
        data-focus-zone="rail"
      >
        <!-- 左侧缩进（与页面 gutter 对齐） -->
        <div class="gf-film-row__edge shrink-0" aria-hidden="true" />
        <div
          v-for="(item, idx) in items"
          :key="getItemKey(item, idx)"
          class="gf-film-row__item shrink-0"
        >
          <slot name="item" :item="item" :index="idx">
            <FilmCard :item="item" :show-title-below="true" />
          </slot>
        </div>
        <div class="gf-film-row__edge shrink-0" aria-hidden="true" />
      </div>

      <!-- 左箭头 -->
      <button
        v-show="canScrollLeft"
        class="gf-film-row__arrow gf-film-row__arrow--left"
        data-focusable="true"
        tabindex="0"
        aria-label="scroll left"
        @click="scrollByDir(-1)"
      >
        <BaseIcon name="chevron-left" size="24px" />
      </button>
      <button
        v-show="canScrollRight"
        class="gf-film-row__arrow gf-film-row__arrow--right"
        data-focusable="true"
        tabindex="0"
        aria-label="scroll right"
        @click="scrollByDir(1)"
      >
        <BaseIcon name="chevron-right" size="24px" />
      </button>
    </div>
  </section>
</template>

<style scoped>
.gf-film-row {
  /* row 之间间距由父级或 grid 控制 */
}

.gf-film-row__scroll {
  scroll-snap-type: x mandatory;
  scrollbar-width: none;
  -webkit-overflow-scrolling: touch;
  /* 关键: 横向滚动容器 overflow-x:auto 会按 CSS 规范把 overflow-y 强制计算成 auto,
     导致卡片 hover scale(1.04) 上下溢出的部分被纵向裁切(顶部被截断)。
     加 padding-block 让放大溢出的上下部分落在 padding 区(属 padding box, 不裁),
     与 TV 模式 padding-block:16px 同理。TV 全局覆盖为 16px, 此处为 web 端值。 */
  padding-block: 12px;
}
.gf-film-row__scroll::-webkit-scrollbar {
  display: none;
}

.gf-film-row__edge {
  /* 与页面 gutter 对齐 */
  width: var(--gf-gutter-mobile);
}
@media (min-width: 768px) {
  .gf-film-row__edge {
    width: var(--gf-gutter-tablet);
  }
}
@media (min-width: 1024px) {
  .gf-film-row__edge {
    width: var(--gf-gutter-desktop);
  }
}

.gf-film-row__item {
  scroll-snap-align: start;
  /* 列宽：移动 4.2 (用户反馈 3.2 卡片太大太空), 大屏手机 5, 平板 5.5,
     桌面 6, >=1440 7, >=1920 8. */
  width: calc((100vw - 32px) / 4.2);
}
@media (min-width: 480px) {
  .gf-film-row__item {
    width: calc((100vw - 32px) / 5);
  }
}
@media (min-width: 768px) {
  .gf-film-row__item {
    width: calc((100vw - 48px) / 5.5);
  }
}
@media (min-width: 1024px) {
  .gf-film-row__item {
    width: calc((100vw - 80px) / 6);
  }
}
@media (min-width: 1440px) {
  .gf-film-row__item {
    width: calc(min(100vw - 80px, 1280px) / 7);
  }
}
@media (min-width: 1920px) {
  .gf-film-row__item {
    width: calc(min(100vw - 80px, 1600px) / 8);
  }
}

.gf-film-row__mask-left {
  width: 40px;
  background-image: var(--gf-mask-row-left);
  z-index: var(--gf-z-row);
  /* 默认隐藏；仅桌面(lg)且对应方向仍可滚动时(v-show)显示，避免遮挡边缘卡片 */
  display: none;
}
.gf-film-row__mask-right {
  width: 40px;
  background-image: var(--gf-mask-row-right);
  z-index: var(--gf-z-row);
  display: none;
}
@media (min-width: 1024px) {
  .gf-film-row__mask-left,
  .gf-film-row__mask-right {
    display: block;
  }
}

.gf-film-row__arrow {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  height: 44px;
  width: 44px;
  display: none;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 9999px;
  background-color: rgba(0, 0, 0, 0.45);
  color: var(--gf-text-primary);
  cursor: pointer;
  z-index: calc(var(--gf-z-row) + 1);
  opacity: 0;
  transition:
    opacity var(--gf-dur-fast) var(--gf-ease-standard),
    background-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-film-row__arrow:hover {
  background-color: rgba(0, 0, 0, 0.7);
}
.gf-film-row__arrow--left {
  left: 8px;
}
.gf-film-row__arrow--right {
  right: 8px;
}

@media (hover: hover) and (min-width: 1024px) {
  .gf-film-row__arrow {
    display: inline-flex;
  }
  .gf-film-row__viewport:hover .gf-film-row__arrow,
  .gf-film-row__viewport:focus-within .gf-film-row__arrow {
    opacity: 1;
  }
}

</style>

<style>
/* TV 默认显示箭头（不依赖 hover），加大尺寸.
 * 注意: 用 v-show 控制边界隐藏(canScrollLeft/Right), 这里只给"显示时"的样式;
 * v-show=false 会加 display:none(行内 style 优先级高于本规则), 故边界自动隐藏. */
[data-mode='tv'] .gf-film-row__arrow {
  display: inline-flex;
  opacity: 1;
  width: 56px;
  height: 56px;
}
/* 焦点环不被横向滚动容器上下裁切.
 * overflow-x:auto(需保留横向滚动) 会把 overflow-y 计算成 auto → 纵向裁切.
 * 不能改 clip(会断横滚). 改为: 在滚动容器内加足够 padding-block, 让焦点框
 * (outline 3px + offset 2px + scale~6px ≈ 11px) 落在 padding 内不被裁. */
[data-mode='tv'] .gf-film-row__scroll {
  padding-block: 16px;
}
[data-mode='tv'] .gf-film-row__arrow:focus,
[data-mode='tv'] .gf-film-row__arrow:focus-visible {
  background-color: rgba(0, 0, 0, 0.85);
  outline: none;
  box-shadow: var(--gf-tv-focus-ring);
}

/* TV 卡片间距 +50%（基础 16px → 24px；large 断点 →） */
[data-mode='tv'] .gf-film-row__scroll {
  gap: 24px;
}
@media (min-width: 768px) {
  [data-mode='tv'] .gf-film-row__scroll {
    gap: var(--gf-space-6);
  }
}

/* TV 列宽：1920 视口默认 8 列 */
[data-mode='tv'] .gf-film-row__item {
  width: calc(min(100vw - 96px, 1600px) / 6);
}

/* TV title 字号: 2xl 在大屏偏大, 降到 xl (行标题不需要那么抢眼) */
[data-mode='tv'] .gf-film-row__title {
  font-size: var(--gf-fs-xl);
}

/* TV header 安全区缩进 */
[data-mode='tv'] .gf-film-row > header.container-page {
  padding-inline: var(--gf-tv-safe);
}
[data-mode='tv'] .gf-film-row__edge {
  width: var(--gf-tv-safe);
}
</style>

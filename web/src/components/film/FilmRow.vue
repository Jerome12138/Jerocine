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
const viewportEl = ref<HTMLElement | null>(null)
const canScrollLeft = ref(false)
const canScrollRight = ref(false)

function updateArrows(): void {
  const el = scrollEl.value
  if (!el) return
  updateArrowTop()
  const left = el.scrollLeft
  const max = el.scrollWidth - el.clientWidth
  // Math.round 消除亚像素: 否则最右时 left 可能永远差零点几px < max-4, 右箭头不消失
  canScrollLeft.value = Math.round(left) > 1
  canScrollRight.value = Math.round(left) < Math.round(max) - 1
}

/**
 * 箭头对准海报图片竖直中心(非"海报+下方标题"整卡中心)。
 * 用 JS 实测: top = 滚动容器 padding-top + 列宽×2/3(海报 3:4 → 高=列宽×4/3)。
 * 不用 CSS 公式的原因: ① top 的百分比按容器"高度"解析, 列宽里的 % 会算错;
 * ② 此前各断点 top 规则写在 base 规则之前被覆盖, 桌面实际套用了移动端公式(越宽偏越多)。
 * 实测自动覆盖 web(12px pad) / TV(16px pad) 与全部断点, 改列宽无需同步这里。
 */
function updateArrowTop(): void {
  const vp = viewportEl.value
  const el = scrollEl.value
  if (!vp || !el) return
  const item = el.querySelector<HTMLElement>('.gf-film-row__item')
  if (!item) return
  const padTop = parseFloat(getComputedStyle(el).paddingTop) || 0
  vp.style.setProperty('--row-arrow-top', `${(padTop + (item.offsetWidth * 2) / 3).toFixed(1)}px`)
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

    <div
      ref="viewportEl"
      class="gf-film-row__viewport relative group"
    >
      <!-- 左右遮罩（桌面）：跟随滚动边界显隐，避免常驻遮挡边缘卡片与标题 -->
      <div v-show="canScrollLeft" class="gf-film-row__mask-left absolute inset-y-0 left-0 pointer-events-none" />
      <div v-show="canScrollRight" class="gf-film-row__mask-right absolute inset-y-0 right-0 pointer-events-none" />

      <!-- 横向滚动容器 -->
      <div
        ref="scrollEl"
        class="gf-film-row__scroll flex overflow-x-auto scroll-smooth"
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
/* 列数 / 缩进 / 卡间距全部取自 theme.css 的全站统一阶梯(--gf-rail-*),
   这里只引用不定义 —— 与 ContinueWatchingRow 及各网格页同阶梯(3.2 → 4.2 → 5.2 → 6),
   改列数只需改 theme.css 一处。 */

.gf-film-row__scroll {
  scroll-snap-type: x mandatory;
  scrollbar-width: none;
  -webkit-overflow-scrolling: touch;
  gap: var(--gf-rail-gap);
  /* 关键: 横向滚动容器 overflow-x:auto 会按 CSS 规范把 overflow-y 强制计算成 auto,
     导致卡片 hover scale(1.04) 上下溢出的部分被纵向裁切(顶部被截断)。
     加 padding-block 让放大溢出的上下部分落在 padding 区(属 padding box, 不裁)。 */
  padding-block: 12px;
}
.gf-film-row__scroll::-webkit-scrollbar {
  display: none;
}

.gf-film-row__edge {
  /* 首尾缩进（web 按页面 gutter; TV 用安全区, 均由变量给出） */
  width: var(--gf-rail-edge);
}

.gf-film-row__item {
  scroll-snap-align: start;
  /* 列宽基准 = 滚动容器宽度(100%), 不用 100vw。
     公式 = (100% - 1×edge - 可见卡间 gap 道数 × 卡间距) / 列数

     为什么只扣 1 个 edge（而非左右各一个）:
       初始 scrollLeft=0 时视口内只有"左侧 edge 占位", 右侧没有东西占位,
       可用宽 = 100% - edge。要让末尾正好露出列数的小数部分 f,
       即 (m+f)×c + m×gap = 100% - edge（m=整数部分, m 也正好是可见卡间的 gap 道数）。
       早期误扣 2×edge, 实际露出变成 edge + f×c —— 多出一个 gutter, 且该 gutter 是固定
       px 而卡片宽随屏宽变化, 于是"露出比例"看起来在 0.3~0.5 张之间飘。已修正。

     整数档(列数=6)时 6×c + 5×gap = 100% - edge, 第 6 张右边缘正好落在视口右边界,
       即一行恰好 6 张完整卡片。 */
  width: calc(
    (100% - var(--gf-rail-edge) - var(--gf-rail-gaps) * var(--gf-rail-gap)) /
      var(--gf-rail-cols)
  );
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
  /* top 由 JS 实测写入 --row-arrow-top(见 updateArrowTop): 滚动容器 padding-top + 列宽×2/3。
     不用 CSS 公式 —— top 的百分比按容器"高"解析, 且断点规则曾被 base 覆盖导致位置错误。 */
  top: var(--row-arrow-top, 50%);
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

/* TV 的列数(6) / 卡间距(space-6) / 缩进(安全区) 已在 theme.css 的 [data-mode="tv"] 里
   覆盖 --gf-rail-*, 组件内不再重复定义宽度规则 —— 避免两处公式不同步。 */

/* TV title 字号: 2xl 在大屏偏大, 降到 xl (行标题不需要那么抢眼) */
[data-mode='tv'] .gf-film-row__title {
  font-size: var(--gf-fs-xl);
}

/* TV header 安全区缩进 */
[data-mode='tv'] .gf-film-row > header.container-page {
  padding-inline: var(--gf-tv-safe);
}
</style>

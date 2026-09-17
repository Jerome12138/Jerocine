<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { HeroItem } from '@/types/film'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import { useViewMode } from '@/composables/useViewMode'
import { isExternalLink } from '@/utils/url'
import { formatHotBadge } from '@/utils/format'

interface Props {
  /**
   * 轮播项。兼容两种来源：影片卡片(Card) 与 后台配置的轮播图(只有标题/图/跳转)。
   * Card 的字段是 HeroItem 的超集，所以直接传 Card[] 也是合法的。
   */
  items: HeroItem[]
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

// 详情跳转：自定义链接优先（外链新开页），否则按关联影片进详情
function gotoDetail(item: HeroItem | undefined): void {
  if (!item) return
  const link = item.link?.trim()
  if (link) {
    if (isExternalLink(link)) window.open(link, '_blank', 'noopener,noreferrer')
    else router.push(link)
    return
  }
  if (item.mid) router.push({ path: '/filmDetail', query: { link: String(item.mid) } })
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

/**
 * 标签行 = 影片类型标签(classTag)。
 *
 * 2026-09-14 用户要求: 标签内容只用 classTag, 不再显示 年份(date)/分类(cName)/地区(area) ——
 * 那三项占位多且对"要不要点进去看"帮助有限, 类型标签信息量更高。
 * classTag 是逗号/顿号/斜杠分隔的串(如 "动作,冒险"), 拆开最多展示 4 个, 避免首屏铺满。
 * 没有 classTag 的影片不渲染标签行(不回退到年份/分类/地区)。
 */
const tags = computed<string[]>(() => {
  const raw = active.value?.classTag?.trim()
  if (!raw) return []
  return raw
    .split(/[,，、/|]+/)
    .map((t) => t.trim())
    .filter(Boolean)
    .slice(0, 4)
})

/** 评分(1 位小数)。<1 视为"无评分" —— 与详情页 score 同口径。 */
const score = computed<string>(() => {
  const n = active.value?.dbScore
  if (n === undefined || n === null || !Number.isFinite(n) || n < 1) return ''
  return n.toFixed(1)
})

/** 豆瓣热度榜位 —— 与详情页共用 formatHotBadge, 两处显示必然一致: 「豆瓣·热门电影 No.1」。
 *  榜单名由后端 douban.HotBoardLabel 兜底(缺 hot_board 时按分类热榜补全, pid 4 → 热门动漫),
 *  正常都有值; 万一缺失则该行少这一段, 不会输出没有分类的「热门」。 */
const hotBadge = computed<string>(() =>
  formatHotBadge(active.value?.hotRank, active.value?.hotBoard)
)

/** 描述行是否有内容 —— 全空时不渲染, 避免留一行空白。 */
const hasMeta = computed<boolean>(
  () => !!(score.value || hotBadge.value || active.value?.remarks)
)
</script>

<template>
  <section
    class="jc-hero relative w-full overflow-hidden cursor-pointer"
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
    <div class="jc-hero__layers absolute inset-0">
      <div
        v-for="(it, i) in items"
        :key="(it.mid ?? i) + '-' + i"
        class="jc-hero__slide absolute inset-0"
        :class="i === current ? 'opacity-100' : 'opacity-0 pointer-events-none'"
        :aria-hidden="i !== current"
      >
        <BaseImage
          :src="it.poster || it.cover"
          :alt="it.name"
          ratio=""
          :eager="i === 0"
          fit="cover"
          class="jc-hero__image"
          :class="{ 'jc-hero__image--wide': !!it.poster }"
        />
        <!-- 竖海报: TV 模糊铺底 + 右侧清晰竖海报; Web 宽屏同理在右侧展示完整竖海报(避免封面被裁)
        （仅当前 slide 的封面, 与下方热门榜单不重复） -->
        <div
          v-if="it.cover"
          class="jc-hero__poster-tv"
          :style="{ backgroundImage: `url(${JSON.stringify(it.cover)})` }"
          aria-hidden="true"
        />
      </div>
    </div>

    <!-- 蒙版 -->
    <div class="jc-hero__mask-bottom absolute inset-0 pointer-events-none" />
    <div class="jc-hero__mask-left absolute inset-0 pointer-events-none hidden md:block" />

    <!-- 左下信息层 -->
    <div
      v-if="active"
      class="jc-hero__content absolute inset-x-0 bottom-0 container-page"
    >
      <div class="jc-hero__info">
        <div
          v-if="tags.length"
          class="flex flex-wrap gap-[var(--jc-space-2)] mb-[var(--jc-space-2)]"
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
        <h2 class="jc-hero__title text-primary">
          {{ active.name }}
        </h2>
        <!-- 描述行: 评分 · 豆瓣榜位 · 状态(片源给的 remarks)。
             评分/榜位由后端按 mid 补齐, 缺哪项就少哪项, 不再单独占一行。
             类型标签在上一行的标签行(tags = classTag), 此处不重复。 -->
        <p
          v-if="hasMeta"
          class="jc-hero__desc jc-hero__meta mt-[var(--jc-space-3)]"
        >
          <span v-if="score" class="jc-hero__score">
            <BaseIcon name="star" size="0.85em" class="jc-hero__score-icon" />
            {{ score }}
          </span>
          <span v-if="hotBadge" class="jc-hero__hot">{{ hotBadge }}</span>
          <span v-if="active.remarks" class="jc-hero__remarks">{{ active.remarks }}</span>
        </p>
      </div>
    </div>

    <!-- 左右箭头 (.stop 阻止冒泡到 section 触发跳转) -->
    <template v-if="showArrows && total > 1">
      <button
        class="jc-hero__arrow jc-hero__arrow--left"
        data-focusable="true"
        tabindex="0"
        aria-label="prev slide"
        @click.stop="prev"
      >
        <BaseIcon name="chevron-left" size="24px" />
      </button>
      <button
        class="jc-hero__arrow jc-hero__arrow--right"
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
      class="jc-hero__bars absolute bottom-[var(--jc-space-4)] left-1/2 -translate-x-1/2 flex items-center gap-[var(--jc-space-2)]"
    >
      <button
        v-for="(_, i) in items"
        :key="i"
        class="jc-hero__bar"
        :class="i === current ? 'jc-hero__bar--active' : ''"
        :aria-label="`go to slide ${i + 1}`"
        :aria-current="i === current ? 'true' : 'false'"
        data-focusable="true"
        tabindex="0"
        @click.stop="go(i)"
      >
        <span
          v-if="i === current"
          :key="progressTick"
          class="jc-hero__bar-progress"
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
 *  │ 视口            纵横比         min-height   max-height           │
 *  │ < 480 (mobile)  16 / 9        190px        38vh                 │
 *  │ ≥ 480           16 / 9        220px        40vh                 │
 *  │ ≥ 768 (tablet)  16 / 9        280px        38vh                 │
 *  │ ≥ 1024 (PC)     21 / 9        320px        380px                │
 *  │ ≥ 1600 (大屏)   21 / 9        clamp(340,30vh,400)  clamp(340,34vh,440) │
 *  └──────────────────────────────────────────────────────────────────┘
 *
 * 2026-09-14: 整体下调约一档(手机 16/10→16/9、各档 max-height 收 4~6vh、
 * PC 420→380) —— 描述行补了评分/标签/榜位后信息更密, 高度反而可以更省,
 * 首屏也能多露出下方列表。内容侧的内边距同步收紧(见 .jc-hero__content)。
 */
.jc-hero {
  width: 100%;
  /* 手机竖屏：用 16/9 而不是 4/5，避免大图占满半屏 */
  aspect-ratio: 16 / 9;
  min-height: 190px;
  max-height: 38vh;
  background-color: var(--jc-bg-base);
  outline: none;
}

@media (min-width: 480px) {
  .jc-hero {
    aspect-ratio: 16 / 9;
    min-height: 220px;
    max-height: 40vh;
  }
}

@media (min-width: 768px) {
  .jc-hero {
    aspect-ratio: 16 / 9;
    min-height: 280px;
    max-height: 38vh;
  }
}

@media (min-width: 1024px) {
  .jc-hero {
    aspect-ratio: 21 / 9;
    min-height: 320px;
    max-height: 380px;
  }
}

@media (min-width: 1600px) {
  .jc-hero {
    aspect-ratio: 21 / 9;
    min-height: clamp(340px, 30vh, 400px);
    max-height: clamp(340px, 34vh, 440px);
  }
}

/* 横屏小高度设备（手机横屏 / 平板横屏低分辨率）：限制 max-height 防 hero 过高顶走列表 */
@media (orientation: landscape) and (max-height: 600px) {
  .jc-hero {
    max-height: 88vh;
    min-height: 280px;
  }
}

/* TV 模式 hero 的尺寸与图层(沉浸 Banner: 竖图模糊铺底 + 右侧清晰竖海报)
 * 统一在文件底部非 scoped [data-mode='tv'] 块定义, 避免本处与其冲突(曾两处 max-height 打架). */

.jc-hero__image,
.jc-hero__image :deep(img) {
  width: 100%;
  height: 100%;
  border-radius: 0;
}

.jc-hero__slide {
  transition: opacity var(--jc-dur-slow) var(--jc-ease-out);
}

/* Web: 默认隐藏竖海报(移动/平板轮播更矮, 不需); 宽屏(≥1024)在右侧展示完整竖海报,
 * 规避封面被 21/9 横幅裁掉竖图信息的问题. TV 模式尺寸在底部非 scoped 块单独定义. */
.jc-hero__poster-tv {
  display: none;
}
@media (min-width: 1024px) {
  .jc-hero__poster-tv {
    display: block;
    position: absolute;
    top: 50%;
    right: clamp(32px, 5vw, 96px);
    transform: translateY(-50%);
    height: 80%;
    aspect-ratio: 2 / 3;
    border-radius: var(--jc-radius-lg);
    background-color: var(--jc-bg-elevated);
    background-size: cover;
    background-position: center;
    box-shadow: 0 16px 48px rgba(0, 0, 0, 0.5);
    z-index: 1;
  }
}

.jc-hero__mask-bottom {
  background-image: var(--jc-mask-hero-bottom);
}
.jc-hero__mask-left {
  background-image: var(--jc-mask-hero-left);
}

.jc-hero__content {
  padding-top: var(--jc-space-6);
  padding-bottom: var(--jc-space-8);
  z-index: 2;
}
@media (min-width: 1024px) {
  .jc-hero__content {
    padding-bottom: 64px;
  }
}

.jc-hero__info {
  max-width: min(640px, 100%);
}

@media (min-width: 768px) {
  .jc-hero__info {
    max-width: min(640px, 60%);
  }
}

.jc-hero__title {
  font-size: var(--jc-fs-hero);
  font-weight: var(--jc-fw-black);
  line-height: var(--jc-lh-tight);
  letter-spacing: var(--jc-tracking-tight);
}

/* 手机档片名再收一档: 令牌是 clamp(2.5rem, 4vw + 1rem, 4.5rem), 在 390px 屏上 4vw+1rem
 * 只有 31.6px, 直接卡到下限 40px —— 配上 16/9 的矮横幅头重脚轻, 挤掉下方列表。
 * 用户反馈"移动端轮播图的片名可以再小点", 故此处只覆盖 <768 档, 不动 --jc-fs-hero 令牌
 * (令牌还被 TV 档整体放大覆盖, 改令牌会连带影响 TV)。
 * 显式排除 TV: TV 是独立 mode(data-mode 挂在 <html>, 不随宽度走), 万一大屏设备上报的
 * 视口宽度 <768, 这条宽度规则会把 TV 的片名一并收小 —— 那不是我们要的。 */
@media (max-width: 767px) {
  html:not([data-mode='tv']) .jc-hero__title {
    font-size: clamp(1.5rem, 6.5vw, 1.75rem);
  }
}

/* 描述行: 评分 / 类型标签 / 豆瓣榜位 / 状态 并排一行, 窄屏自动换行(最多两行)。
 * 各段靠颜色与字重区分(评分暖色、榜位品牌色、状态弱化), 不再插入分隔符。
 * 保留 .jc-hero__desc 类名是为了不透传破坏 TV 下的字号覆盖([data-mode='tv'] .jc-hero__desc)。 */
.jc-hero__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px var(--jc-space-3);
  font-size: var(--jc-fs-sm);
  line-height: var(--jc-lh-snug);
  color: var(--jc-text-secondary);
  max-height: calc(2em * var(--jc-lh-snug));
  overflow: hidden;
}

@media (min-width: 768px) {
  .jc-hero__meta {
    font-size: var(--jc-fs-base);
  }
}

.jc-hero__score {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  color: var(--jc-warning);
  font-weight: var(--jc-fw-bold);
}

.jc-hero__score-icon {
  margin-bottom: 1px;
}

.jc-hero__hot {
  color: var(--jc-brand-cyan);
  font-weight: var(--jc-fw-semibold);
}

.jc-hero__remarks {
  color: var(--jc-text-muted);
}

.jc-hero__arrow {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 48px;
  height: 64px;
  display: none;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--jc-radius-md);
  background-color: rgba(0, 0, 0, 0.55);
  color: var(--jc-text-primary);
  cursor: pointer;
  z-index: 3;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    opacity var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-hero__arrow:hover {
  background-color: rgba(0, 0, 0, 0.8);
}
.jc-hero__arrow--left {
  left: var(--jc-space-4);
}
.jc-hero__arrow--right {
  right: var(--jc-space-4);
}

@media (min-width: 768px) {
  .jc-hero__arrow {
    display: inline-flex;
  }
}

/* 指示条 (横条 + 当前条进度填充) */
.jc-hero__bars {
  z-index: 3;
}

.jc-hero__bar {
  position: relative;
  width: 36px;
  height: 3px;
  border-radius: 2px;
  background-color: rgba(255, 255, 255, 0.3);
  border: none;
  padding: 0;
  cursor: pointer;
  overflow: hidden;
  transition: width var(--jc-dur-base) var(--jc-ease-standard);
}

.jc-hero__bar--active {
  width: 56px;
}

.jc-hero__bar:hover {
  background-color: rgba(255, 255, 255, 0.45);
}

.jc-hero__bar:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px rgba(74, 209, 229, 0.8);
}

.jc-hero__bar-progress {
  position: absolute;
  inset: 0;
  background-image: var(--jc-brand-gradient);
  transform: scaleX(0);
  transform-origin: left center;
  animation-name: jc-hero-progress;
  animation-timing-function: linear;
  animation-fill-mode: forwards;
  animation-iteration-count: 1;
}

@keyframes jc-hero-progress {
  from { transform: scaleX(0); }
  to { transform: scaleX(1); }
}
</style>

<style>
/* TV 默认显示箭头（不依赖 hover），加大尺寸 + 安全区缩进 */
[data-mode='tv'] .jc-hero__arrow {
  display: inline-flex;
  width: 64px;
  height: 80px;
}
[data-mode='tv'] .jc-hero__arrow--left {
  left: var(--jc-tv-safe);
}
[data-mode='tv'] .jc-hero__arrow--right {
  right: var(--jc-tv-safe);
}
[data-mode='tv'] .jc-hero__arrow:focus,
[data-mode='tv'] .jc-hero__arrow:focus-visible {
  outline: none;
  background-color: rgba(0, 0, 0, 0.85);
  box-shadow: var(--jc-tv-focus-ring);
}
/* TV 沉浸 Banner —— 占屏 ~55%; 采集源仅竖海报: 同图模糊放大铺底 + 右侧清晰竖海报兜底 */
[data-mode='tv'] .jc-hero {
  aspect-ratio: auto;
  min-height: var(--jc-tv-hero-h, 55vh);
  max-height: var(--jc-tv-hero-h, 55vh);
}
/* 背景层: 静态模糊(非 backdrop-blur)放大压暗, 把竖海报铺满宽幅不露裁切边 */
[data-mode='tv'] .jc-hero__image,
[data-mode='tv'] .jc-hero__image :deep(img) {
  object-fit: cover;
  object-position: center;
  filter: blur(28px) brightness(0.5) saturate(1.1);
  transform: scale(1.18);
}
/* 有真横图时不做模糊放大: 那套"模糊铺底"是采集源只给竖海报时的兜底 */
[data-mode='tv'] .jc-hero__image--wide {
  filter: none;
  transform: none;
}
/* 右侧清晰竖海报(2:3), 真正展示该片封面 */
[data-mode='tv'] .jc-hero__poster-tv {
  position: absolute;
  top: 50%;
  right: clamp(48px, 8vw, 160px);
  transform: translateY(-50%);
  height: 74%;
  aspect-ratio: 2 / 3;
  border-radius: var(--jc-radius-lg);
  background-color: var(--jc-bg-elevated);
  background-size: cover;
  background-position: center;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.65);
  z-index: 1;
}
[data-mode='tv'] .jc-hero__content {
  padding-inline: var(--jc-tv-safe);
  padding-bottom: var(--jc-space-12);
}
[data-mode='tv'] .jc-hero__info {
  max-width: min(720px, 55%);
}
[data-mode='tv'] .jc-hero__desc {
  font-size: var(--jc-fs-lg);
}
[data-mode='tv'] .jc-hero__bar {
  width: 48px;
  height: 4px;
}
[data-mode='tv'] .jc-hero__bar--active {
  width: 72px;
}
</style>

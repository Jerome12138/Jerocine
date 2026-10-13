<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
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
  /**
   * 沉浸式(2026-10-10 评审定稿): 盒子几何与盒内一切(文字/按钮/右侧竖海报/箭头/
   * 指示条)完全维持原版; 只把图片从"圆角框内显示"改为铺满页面顶部全宽背景 ——
   * 图片元素 breakout 出盒子(铺满视口宽、顶到页头下缘、高度=盒高+下探量,
   * 下探尾压在继续观看区背后, 底部渐隐入页面底色)。仅 ≥1024 档生效。
   */
  immersive?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  interval: 0,
  showArrows: true,
  immersive: false
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

/**
 * 沉浸式背景图真实宽高比(2026-10-13 用户定稿): src → ratio, img load 时量
 * naturalWidth/naturalHeight 写入, slide 经 CSS 变量 --jc-hero-ratio 注入
 * 图片盒与蒙版盒的 aspect-ratio —— 高度 = 盒宽 ÷ 比例, 实现宽度优先等比缩放。
 * 竖图(naturalW < naturalHeight)比例 ÷0.6: 用户要求裁去上下各 20% 只显示中段,
 * 盒比例放宽后 object-fit:cover 居中恰好纵向裁掉 40%(上下各 20%)。
 * 加载前/量不到时由 CSS 回落 16/9。
 */
const imgRatios = reactive<Record<string, number>>({})
function onHeroImgLoad(e: Event, src: string): void {
  const img = e.target as HTMLImageElement | null
  if (!img || !img.naturalWidth || !img.naturalHeight) return
  const raw = img.naturalWidth / img.naturalHeight
  imgRatios[src] = img.naturalWidth < img.naturalHeight ? raw / 0.6 : raw
}
</script>

<template>
  <section
    class="jc-hero relative w-full overflow-hidden cursor-pointer"
    :class="{ 'jc-hero--immersive': immersive }"
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
        :class="[
          i === current ? 'opacity-100' : 'opacity-0 pointer-events-none',
          /* 竖图 slide(无 poster 即 cover 竖图): 比例变量挂 slide,
           * 图片与下方蒙版共同继承, 各 slide 按自己的比例取矩形 */
          { 'jc-hero__slide--portrait': !it.poster },
        ]"
        :aria-hidden="i !== current"
        :style="{ '--jc-hero-ratio': imgRatios[it.poster || it.cover || ''] }"
        @load.capture="onHeroImgLoad($event, it.poster || it.cover || '')"
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
        <!-- 蒙版在 slide 内(每张自带一套, 2026-10-11 用户反馈重构): 此前是 layers 外
             的一套共享蒙版、矩形跟激活 slide 的方向跳变 —— 切换时上一张先失阴影、
             当前图后补阴影, 且图片硬边与蒙版渐隐边错位成"两套细线"。移入后阴影随
             slide 一起交叉淡入淡出, 永远与自己的图片同向同矩形。(desktop 沉浸式另给
             顶/右两侧渐隐, 见沉浸式块; 其余档无背景不可见) -->
        <div class="jc-hero__mask-top absolute inset-0 pointer-events-none" />
        <div class="jc-hero__mask-right absolute inset-0 pointer-events-none" />
        <div class="jc-hero__mask-bottom absolute inset-0 pointer-events-none" />
        <div class="jc-hero__mask-left absolute inset-0 pointer-events-none hidden md:block" />
        <!-- 竖海报在蒙版之后: 恒定处于阴影上方。此前 z-index:1 在过渡期间被 slide 的
             opacity(<1) 层叠上下文困在蒙版下、过渡结束又逃逸到外层蒙版上 —— 即用户
             看到的"先压到阴影下方、再被挪到上方"的闪变; 移入 slide 内靠 DOM 顺序
             定层序, 与过渡状态无关 -->
        <div
          v-if="it.cover"
          class="jc-hero__poster-tv"
          :style="{ backgroundImage: `url(${JSON.stringify(it.cover)})` }"
          aria-hidden="true"
        />
      </div>
    </div>

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
@media (min-width: 768px) {
  /* 左箭头 768 起显示且已靠边(宽 48px), 文字/标签左边距 64px = 箭头右缘
   * + 16px 间隙防遮挡(2026-10-10: 箭头去 16px 偏移后 80→64 同步收窄);
   * 右侧沿用容器 gutter */
  .jc-hero__content {
    padding-left: 64px;
  }
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
  /* 对齐分类区行箭头(FilmRow)的圆形规格: 44x44 圆形, 底色同 0.45 */
  width: 44px;
  height: 44px;
  display: none;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 9999px;
  background-color: rgba(0, 0, 0, 0.45);
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
  left: 0;
}
.jc-hero__arrow--right {
  right: 0;
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

/* ===== 沉浸式(除 TV 外全尺寸, 2026-10-10 评审定稿, 未 push) =====
 * 用户定稿口径: 原有轮播区域不变、高度不变, 盒内按钮/文字原位;
 * 把图从"圆角框内显示"改为页面顶部背景 —— 宽度铺满视口(body 已有
 * overflow-x:hidden 不出横向滚动条), 高度向上覆盖到顶部导航栏背后, 向下透出
 * 压到继续观看区背后。原仅桌面 ≥1024, 后按用户要求放开到 mobile/tablet/desktop
 * 全尺寸(除 TV)。
 *
 * 实现口径 —— 只 breakout 图片元素本身, DOM 不动:
 *   · 盒子(.jc-hero)几何沿用原版(21/9 + max-height 380 等), 仅关闭圆角裁剪
 *     放行溢出、盒底色透明(图不落在盒内而在背景上);
 *   · .jc-hero__image(BaseImage 根)绝对定位: top 上提(--jc-hero-toplift =
 *     页头高 mobile 56 / md+ 64 + wrapper pt-6, 顶到页面最顶、垫在 sticky
 *     页头之下)。尺寸为宽度优先等比模型(2026-10-13 用户定稿, 取代旧
 *     "盒高+上提+bleed 下探" vh 口径 —— 那套高度不随宽度缩, 窄窗下图片
 *     被 cover 放大裁切):
 *       盒宽 = min(内容区宽(=轮播盒宽, 2026-10-13 定稿不按视口), 1000px × 真实比例)
 *       盒高 = 盒宽 ÷ 真实比例(img load 时 JS 量 naturalWidth/Height 注入
 *              --jc-hero-ratio; 竖图 ÷0.6 = 裁上下各 20%), 下限 = 轮播盒高
 *              (min-height:100%; 更扁的图托底后 cover 轻裁 —— 用户允许的
 *              "高度已缩到盒底"分支)
 *     垂直锚点自适应: 等比高够从页顶盖到盒底 → top:-toplift 铺满+下探;
 *     不够(窄窗) → 下沉底边贴盒底(top:0), 模块区域完整覆盖、零裁切。
 *     正常态整图零裁切等比缩放; 高度超出 1000px 上限时宽度同步收窄居中,
 *     两侧露页面底色由蒙版贴图缘渐隐。定位包含块是 slide(=盒子), 盒内其它
 *     元素不受影响(右侧竖海报仍锚定 slide, ≥1024 才显示, 位置与原版一致);
 *   · breakout 图必须 pointer-events:none —— 图层覆盖到页头区域后会截胡
 *     nav 的悬停/点击(点击会落到 section 的 gotoDetail);
 *   · 页头可读性: PublicHeader 的 jc-header--top 自带 rgba(0,0,0,.55)→透明
 *     渐变阴影背景(z-index 高于本层), 图垫在其下正好透出渐变, 无需另加;
 *   · 四层蒙版(顶/右/底/左)同步拉伸到同一矩形做四边渐隐, 顶部条带最小, 底部
 *     渐隐尾融入页面底色;
 *   · HomeView 侧 .jc-home__rows 抬 z-index:1, 继续观看卡片压在下探尾之上。
 *   · width/height 必须 !important 覆盖 BaseImage 根的内联 100%(ratio="" 时
 *     aspectStyle 内联输出, 内联样式压过一切选择器, 不加则纵向拉伸不生效, 踩过);
 *   · 选择器统一加 html:not([data-mode='tv']) 前缀 —— TV 模式完全不匹配这些
 *     规则, 沿用原有 TV 沉浸 Banner(55vh 模糊铺底)样式。 */
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive {
  /* 顶部上提量 = 页头高(mobile 56 / md+ 64) + wrapper pt-6 */
  --jc-hero-toplift: 80px;
}
@media (min-width: 768px) {
  html:not([data-mode='tv']) .jc-hero.jc-hero--immersive {
    --jc-hero-toplift: 88px;
  }
}
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive {
  overflow: visible;
  border-radius: 0;
  background-color: transparent;
}
/* 水平尺寸基准 = 内容区(2026-10-13 用户定稿:"大图宽度最大值限制为内容区宽度,
 * 不按视口"): 图片/蒙版宽度以 slide 盒宽(=布局内容区宽)为 100% 基准, 宽度上限
 * = 内容区宽; 高度封顶分支(1000px×比例 < 盒宽)时按高度等比收窄并在盒内居中。
 * 定位 left:50% + translateX(-50%) 对盒居中 —— 与 container-page 内容同一中心线。
 * 旧的"铺满视口 breakout"逻辑(100vw 宽 + side-pad/2 偏移 + inset 边距变量)整套
 * 废除。top 公式里的盒宽用 (100vw - side-pad) 作精确等价代理: 内容区宽 = 布局
 * padding-left(侧栏)之外的视口宽, 纯 CSS 无法在 top 里直接引用包含块宽度做除法
 * (容器查询单位 X5 内核不支持), 二值恒等。 */
/* 图片盒(2026-10-13 定稿, 宽度优先等比模型):
 *   盒宽 W = min(内容区宽, 1000px × 真实比例 r)
 *   盒高 H = W ÷ r, 下限 = 轮播盒高(min-height:100%, 更扁的图如 21/9 在窄窗
 *           会被托底、cover 轻裁 —— 用户允许的"高度已缩到盒底"分支)
 * 正常态 H=W/r 随宽度等比缩(零裁切); 高度封顶分支 H=1000、宽收窄到 1000×r
 * (此时 H ≥ 盒高+上提恒成立, 锚点公式自然落在 -toplift)。
 * 垂直锚点自适应(2026-10-13 用户反馈 375 横图被截): 等比高足以从页顶盖到盒底时
 * top:-toplift 正常铺满+下探; 缩到不够时下沉为底边贴盒底(top:0) —— 移动端横图
 * (盒也是 16/9)恰好 高度=盒高、完整覆盖模块区域、零裁切, 页头区域让给页面底色
 * (页头自带渐变玻璃)。竖图 r 已在 JS ÷0.6(裁上下各 20%), cover 居中正好实现。
 * ⚠️ 比例回退值必须写数字 1.7778 不能写 "16 / 9": top 公式里做除法,
 * var 替换后 X / 16 / 9 左结合 = X/144(错)。
 * width/height !important 压过 BaseImage 根内联 100%(不加则不生效, 踩过) */
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__image {
  position: absolute;
  top: max(
    calc(-1 * var(--jc-hero-toplift)),
    min(
      calc(100% - (100vw - var(--jc-side-pad, 0px)) / var(--jc-hero-ratio, 1.7778)),
      0%
    )
  );
  bottom: auto;
  left: 50%;
  width: min(100%, calc(1000px * var(--jc-hero-ratio, 1.7778))) !important;
  transform: translateX(-50%);
  height: auto !important;
  aspect-ratio: var(--jc-hero-ratio, 1.7778);
  min-height: 100%;
  pointer-events: none;
}
/* 图片等比平铺居中(2026-10-10 用户定稿): img 撑满 breakout 层, object-fit: cover
 * = 按 max(宽缩放, 高缩放) 等比缩放铺满(即"按最大宽度或最大高度平铺"),
 * object-position: center 保持图片中心对齐可视区域中心。!important 同上,
 * 压过 BaseImage 根内联 100% 与 :deep(img) 的宽高规则。 */
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__image :deep(img) {
  width: 100% !important;
  height: 100% !important;
  object-fit: cover;
  object-position: center;
}
/* 四层蒙版(已移入 slide, 与图片同 slide 同矩形, 见模板注释):
 * top 锚点公式/aspect-ratio/min-height 与图片盒完全同款(锚点自适应下沉、
 * 盒高托底), 宽度 = 图盒宽+16px —— 蒙版恒 ≥ 图盒(宽 +16 使等比高多 ~9px,
 * 下限兜底时同高), 渐隐永远贴图片真实边缘, 封顶留空时跟随图片收进来无硬边。
 * 水平比图片外扩 8px/侧(width +16px, 居中故两侧均分): 蒙版渐变带边缘是全强度
 * 页面底色, 外扩段落在图外与底色同色不可见 —— 图片与蒙版是两层独立合成, 亚像素
 * 取整可能让最边缘 1px 露出未加阴影的图(亮图右侧细线, 2026-10-11 用户反馈),
 * 外扩保证图片边缘始终处于蒙版全强度区之下 */
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__mask-top,
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__mask-right,
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__mask-bottom,
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__mask-left {
  display: block;
  inset: auto;
  top: max(
    calc(-1 * var(--jc-hero-toplift)),
    min(
      calc(100% - (100vw - var(--jc-side-pad, 0px)) / var(--jc-hero-ratio, 1.7778)),
      0%
    )
  );
  bottom: auto;
  left: 50%;
  width: calc(min(100%, 1000px * var(--jc-hero-ratio, 1.7778)) + 16px);
  transform: translateX(-50%);
  height: auto;
  aspect-ratio: var(--jc-hero-ratio, 1.7778);
  min-height: 100%;
}
/* 顶部渐隐(用户: 顶部阴影区比其他方向小): 120px 条带压暗垫页头下 */
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__mask-top {
  background-image: linear-gradient(180deg, rgba(11, 11, 15, 0.6), rgba(11, 11, 15, 0));
  background-size: 100% 120px;
  background-position: top;
  background-repeat: no-repeat;
}
/* 右侧渐隐: 图右缘淡入页面底色(2026-10-11 用户: 阴影带加宽一倍, 渐变同步拉宽 ——
 * 条带宽度 80/14vw/320 → 160/28vw/640) */
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__mask-right {
  background-image: linear-gradient(270deg, var(--jc-bg-base), rgba(11, 11, 15, 0));
  background-size: clamp(160px, 28vw, 640px) 100%;
  background-position: right;
  background-repeat: no-repeat;
}
/* 左侧渐隐 + 文字区压暗合一(阴影区域加宽一倍、渐变同步拉宽:
 * 边缘入底色 9%→18%, 文字区压暗 24%→48%, 放行 46%→92%) */
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__mask-left {
  background-image: linear-gradient(
    90deg,
    var(--jc-bg-base) 0%,
    rgba(11, 11, 15, 0) 18%,
    rgba(11, 11, 15, 0.55) 48%,
    rgba(11, 11, 15, 0) 92%
  );
}
/* 沉浸式渐变: 顶部轻压暗(垫在页头渐变下再补一层) → 中段透明 → 文字区加深 →
 * 渐隐尾放缓拉长, 让下探区的图"透"出来铺在继续观看区背后 */
html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__mask-bottom {
  /* 底部渐隐带区间砍半(2026-10-13 用户要求"阴影效果减小一半"): 起坡 38%→69%、
   * 半强度点 56%→78%, 阴影带约占层高 62%→31%; 100% 必须保持全强度 --jc-bg-base
   * (图底边与页面底色无缝融合), 故只能缩带长、不能降透明度, 降了会露硬缝 */
  background-image: linear-gradient(
    180deg,
    rgba(11, 11, 15, 0.25) 0%,
    rgba(11, 11, 15, 0) 14%,
    rgba(11, 11, 15, 0) 69%,
    rgba(11, 11, 15, 0.4) 78%,
    var(--jc-bg-base) 100%
  );
}
/* 移动端(<1024): 仅左右阴影范围砍半+减淡一半(2026-10-13 用户要求,
 * 顶/底维持原效果不减弱 —— 用户追认)。左右渐变贴图缘的端点(全强度
 * --jc-bg-base)不可减 —— 那是图缘与页面底色无缝融合的关键; 减淡落在:
 * 中间压暗档位 alpha 减半 + 渐变带长度减半(向各自锚点边收拢) */
@media (max-width: 1023.98px) {
  html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__mask-right {
    background-size: clamp(80px, 14vw, 320px) 100%;
  }
  html:not([data-mode='tv']) .jc-hero.jc-hero--immersive .jc-hero__mask-left {
    background-image: linear-gradient(
      90deg,
      var(--jc-bg-base) 0%,
      rgba(11, 11, 15, 0) 9%,
      rgba(11, 11, 15, 0.275) 24%,
      rgba(11, 11, 15, 0) 46%
    );
  }
}
</style>

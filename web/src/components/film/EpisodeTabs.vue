<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useAutoHideScrollbar } from '@/composables/useAutoHideScrollbar'
import type { PlaySource } from '@/types/film'

interface Props {
  sources: PlaySource[]
  currentSourceId?: string
  /** 当前集的 link */
  currentEpisode?: string
  /** 已观看集合（cookie 历史记录），按 link */
  watchedLinks?: string[]
  /** 单个分段大小, 超过则启用分段切换. 0 = 不分段 */
  pageSize?: number
  /** 各线路实测播放延时 ms (-1=失败, undefined=未测) */
  speeds?: Record<string, number>
  /** 测速进行中(PlayView testingLines): true 时测速点全部显示为脉冲 loading 态(方案 §C6 #1) */
  speedTesting?: boolean
  /** 正在播放的源 id(可选, 方案 §C6 #3): 该源不等于当前浏览(active)源时, tab 上出 ▶ 小标。
   *  播放页(PlayView)也传 currentSourceId, 但播放页 playing==active 恒成立 → 不出标("播放页=激活态");
   *  详情页浏览态切走后, ▶ 提示真正的播放所在源("FilmDetail 已知则传"= resumeRecord?.source)。 */
  playingSourceId?: string
  /** 影片名 — 用于去掉集名里冗余的剧名前缀(如"我是谁 第01集"→"第01集") */
  filmName?: string
}

const props = withDefaults(defineProps<Props>(), {
  currentSourceId: '',
  currentEpisode: '',
  watchedLinks: () => [],
  pageSize: 30,
  speeds: () => ({}),
  speedTesting: false,
  playingSourceId: '',
  filmName: ''
})

/** 去掉集名前缀里的剧名 + 分隔符. 例:
 *  "庆余年 第01集" → "第01集"; "庆余年-01" → "01"; "庆余年·HD" → "HD".
 *  仅当确实以剧名开头才剥离, 否则原样返回. */
function cleanEpisodeName(name: string): string {
  const n = (name ?? '').trim()
  const film = props.filmName.trim()
  if (!film || !n.startsWith(film)) return n
  const stripped = n.slice(film.length).replace(/^[\s·\-_:：|/]+/, '').trim()
  // 剥离后为空(集名 == 剧名)则保留原值, 免得 chip 空白
  return stripped || n
}

/** 最快线路 id (用于测速色点分级) */
const fastestId = computed(() => {
  let id = ''
  let best = Infinity
  for (const k in props.speeds) {
    const v = props.speeds[k]
    if (v === undefined || v < 0) continue
    if (v < best) {
      best = v
      id = k
    }
  }
  return id
})
/** 测速展示(方案 §C6 #1): ⚡数值文本降级为色点 — 绿(--fast)=最快线 / 黄(--mid)=已测一般 / 红(--fail)=失败(-1) /
 *  脉冲(--loading)=测速进行中; 数值走 title 悬浮不再占版面。
 *  渲染条件: 已测出值, 或测速进行中(全点转脉冲)。undefined=未测且未在测 → 不渲染。 */
function speedDot(id: string): boolean {
  return props.speedTesting || props.speeds[id] !== undefined
}
function speedDotClass(id: string): string {
  if (props.speedTesting) return 'jc-source-tab__speed--loading'
  const v = props.speeds[id]
  if (v === undefined) return ''
  if (v < 0) return 'jc-source-tab__speed--fail'
  return id === fastestId.value ? 'jc-source-tab__speed--fast' : 'jc-source-tab__speed--mid'
}
function speedTitle(id: string): string {
  // 测速中全点转脉冲(含上一轮旧值), title 同步统一为"测速中…"
  if (props.speedTesting) return '测速中…'
  const v = props.speeds[id]
  if (v === undefined) return '测速中…'
  if (v < 0) return '测速失败'
  return `${v}ms`
}

const emit = defineEmits<{
  (e: 'select', payload: { sourceId: string; episodeIndex: number; link: string }): void
  (e: 'change-source', sourceId: string): void
}>()

const activeSourceId = computed(() => {
  if (props.currentSourceId) return props.currentSourceId
  return props.sources[0]?.id ?? ''
})

const activeSource = computed(() =>
  props.sources.find((s) => s.id === activeSourceId.value) ??
  props.sources[0]
)

/** ===== 分段切换 (>30 集时显示) ===== */
const totalCount = computed(() => activeSource.value?.episodes.length ?? 0)
const needsSegments = computed(() => props.pageSize > 0 && totalCount.value > props.pageSize)
const segments = computed<Array<{ start: number; end: number; label: string }>>(() => {
  if (!needsSegments.value) return []
  const size = props.pageSize
  const out: Array<{ start: number; end: number; label: string }> = []
  for (let i = 0; i < totalCount.value; i += size) {
    const end = Math.min(i + size, totalCount.value) - 1
    out.push({ start: i, end, label: `${i + 1}-${end + 1}` })
  }
  return out
})
const segmentIndex = ref(0)

/** 切源时, 段索引重置, 但优先定位到包含当前播放集的段 */
watch([activeSourceId, () => props.currentEpisode], () => {
  if (!needsSegments.value) {
    segmentIndex.value = 0
    return
  }
  const src = activeSource.value
  if (!src) return
  const curIdx = src.episodes.findIndex((e) => e.link === props.currentEpisode)
  if (curIdx >= 0) {
    segmentIndex.value = Math.floor(curIdx / props.pageSize)
  } else {
    segmentIndex.value = 0
  }
}, { immediate: true })

/** 当前段范围内的 episodes (含原索引) */
const visibleEpisodes = computed(() => {
  const src = activeSource.value
  if (!src) return []
  if (!needsSegments.value) {
    return src.episodes.map((ep, idx) => ({ ep, idx }))
  }
  const seg = segments.value[segmentIndex.value]
  if (!seg) return []
  return src.episodes.slice(seg.start, seg.end + 1).map((ep, i) => ({
    ep,
    idx: seg.start + i
  }))
})

function selectSource(id: string): void {
  if (id === activeSourceId.value) return
  emit('change-source', id)
}

function selectEpisode(idx: number): void {
  const src = activeSource.value
  if (!src) return
  const ep = src.episodes[idx]
  if (!ep) return
  emit('select', { sourceId: src.id, episodeIndex: idx, link: ep.link })
}

function selectSegment(i: number): void {
  segmentIndex.value = i
}

/** 选集按钮: hover / 聚焦(遥控器"选中")时, 若集名宽度超出按钮就横向轮播完整名;
 *  不溢出则不动。轮播位移量按实际溢出像素算, 时长随溢出量线性增长(约 40px/s)。 */
function startMarquee(e: Event): void {
  const chip = e.currentTarget as HTMLElement | null
  if (!chip) return
  const label = chip.querySelector<HTMLElement>('.jc-episode-chip__label')
  if (!label) return
  // 默认态(省略号)下测量: scrollWidth=完整文字宽, clientWidth=被按钮裁掉后的可见宽
  const shift = label.scrollWidth - label.clientWidth
  if (shift <= 2) return
  chip.style.setProperty('--jc-ep-shift', `-${shift}px`)
  chip.style.setProperty('--jc-ep-dur', `${Math.max(2, shift / 40).toFixed(1)}s`)
  chip.classList.add('is-marquee')
}
function stopMarquee(e: Event): void {
  const chip = e.currentTarget as HTMLElement | null
  if (chip) chip.classList.remove('is-marquee')
}
/* ===== 源条横滚渐隐(2026-11-06)：左右阴影只在「该方向还能继续滑」时显示 =====
 * 原实现是常驻双侧 mask —— 滑到两端仍显示, 反而误导; 现按实际可滚方向加类。
 * 触发点：挂载 / 容器尺寸变化(ResizeObserver) / 滚动 / 源数量变化。 */
const tabsEl = ref<HTMLElement | null>(null)
const canScrollLeft = ref(false)
const canScrollRight = ref(false)

function syncScrollFade(): void {
  const el = tabsEl.value
  if (!el) {
    canScrollLeft.value = false
    canScrollRight.value = false
    return
  }
  const max = el.scrollWidth - el.clientWidth
  canScrollLeft.value = el.scrollLeft > 1
  canScrollRight.value = max > 1 && el.scrollLeft < max - 1
}

let fadeObserver: ResizeObserver | null = null

onMounted(() => {
  syncScrollFade()
  if (tabsEl.value && typeof ResizeObserver !== 'undefined') {
    fadeObserver = new ResizeObserver(() => syncScrollFade())
    fadeObserver.observe(tabsEl.value)
  }
})

onUnmounted(() => {
  fadeObserver?.disconnect()
  fadeObserver = null
})

// 源数量/名字变化 → 内容宽度变 → 重算（源条 v-if 显隐后 DOM 才存在, 故等一帧）
watch(
  () => props.sources,
  () => nextTick(syncScrollFade)
)
/** 滚动条自隐：选源横向条 / 集数网格滚动时显 6px 细条，停止 700ms 收起（用户 2026-11-06 要求） */
const { onScroll: onAutoHideScroll } = useAutoHideScrollbar()

/** 容器滚动 → 同时刷新「可滚方向」(渐隐 / 箭头禁用态) 与滚动条显隐 */
function onTabsScroll(e: Event): void {
  syncScrollFade()
  onAutoHideScroll(e)
}

/** 箭头翻页：点一次滑一排(≈可视宽 80%, 留一点上下文); 到两端时对应箭头 disabled */
function scrollTabs(dir: -1 | 1): void {
  const el = tabsEl.value
  if (!el) return
  el.scrollBy({ left: dir * el.clientWidth * 0.8, behavior: 'smooth' })
}

/**
 * 集名是否「全部偏短」(如「第13集」「03」) —— 短名时集格宽度折半（用户 2026-11-06 要求）。
 * 口径：当前源**全部集**清洗后片名长度 ≤ 4 才算（用全部集而非当前分段，避免翻分段时列数跳变）。
 */
const SHORT_EPISODE_NAME_MAX = 4
const allEpisodeNamesShort = computed(() => {
  const src = props.sources.find((s) => s.id === activeSourceId.value) ?? props.sources[0]
  const eps = src?.episodes ?? []
  if (!eps.length) return false
  return eps.every((e) => cleanEpisodeName(e.episode).length <= SHORT_EPISODE_NAME_MAX)
})
</script>

<template>
  <section class="jc-episodes flex flex-col gap-[var(--jc-space-4)]">
    <!-- 播放源 Tab(每个 tab 右上角已带该源集数徽标, 不再单独显示"共 N 集")
         源条保留 border-b 作层级分隔(方案 §C6 #5): 源=导航层(带底线), 集/分段=内容层(纯胶囊无线)。
         内层横滚容器不再重复 v-if(单源时整条不渲染, 方案 §C6 #9 合并重复判断)。 -->
    <div
      v-if="sources.length > 1"
      class="jc-source-bar flex items-center gap-[var(--jc-space-4)] border-b border-default"
    >
      <!-- 翻页箭头(2026-11-06): 鼠标点着翻一排, 同时作为「还能左右滑」的提示;
           到两端时对应箭头禁用; 无溢出时整对不渲染 -->
      <button
        v-if="canScrollLeft || canScrollRight"
        type="button"
        class="jc-source-arrow jc-hotarea"
        :disabled="!canScrollLeft"
        data-focusable="true"
        tabindex="0"
        aria-label="往前一排线路"
        @click="scrollTabs(-1)"
      >
        <BaseIcon name="chevron-left" size="1em" />
      </button>
      <div
        ref="tabsEl"
        class="jc-source-tabs jc-scrollauto flex items-center gap-[var(--jc-space-6)] overflow-x-auto"
        :class="{ 'is-fade-left': canScrollLeft, 'is-fade-right': canScrollRight }"
        @scroll="onTabsScroll"
      >
        <button
          v-for="s in sources"
          :key="s.id"
          class="jc-source-tab jc-hotarea"
          :class="s.id === activeSourceId ? 'jc-source-tab--active' : ''"
          data-focusable="true"
          tabindex="0"
          :aria-selected="s.id === activeSourceId"
          @click="selectSource(s.id)"
        >
          <!-- ▶ 正在播放的源(方案 §C6 #3): 仅当播放源 ≠ 当前浏览(active)源时出标;
               播放页 playing==active 恒成立 → 永不出标(播放态即激活态) -->
          <span
            v-if="s.id === playingSourceId && s.id !== activeSourceId"
            class="jc-source-tab__playing"
            aria-hidden="true"
          >▶</span>
          <span class="jc-source-tab__name">{{ s.name }}</span>
          <!-- 测速色点(方案 §C6 #1): 绿=最快/黄=一般/红=失败/脉冲=测速中; 数值进 title -->
          <span
            v-if="speedDot(s.id)"
            class="jc-source-tab__speed"
            :class="speedDotClass(s.id)"
            :title="speedTitle(s.id)"
          />
          <!-- 右上角集数徽标: 显示「该源」自己的集数, 区别于条尾「共 N 集」(当前源) -->
          <span
            v-if="s.episodes.length"
            class="jc-source-tab__count-badge"
            :aria-label="`该源 ${s.episodes.length} 集`"
          >
            {{ s.episodes.length }}
          </span>
        </button>
      </div>
      <button
        v-if="canScrollLeft || canScrollRight"
        type="button"
        class="jc-source-arrow jc-hotarea"
        :disabled="!canScrollRight"
        data-focusable="true"
        tabindex="0"
        aria-label="往后一排线路"
        @click="scrollTabs(1)"
      >
        <BaseIcon name="chevron-right" size="1em" />
      </button>
    </div>

    <!-- 分段切换 (集数 > pageSize 时显示) -->
    <div
      v-if="needsSegments"
      class="jc-episode-segments flex flex-wrap gap-[var(--jc-space-2)]"
      role="tablist"
      aria-label="集数分段"
    >
      <button
        v-for="(seg, i) in segments"
        :key="i"
        class="jc-episode-seg"
        :class="i === segmentIndex ? 'jc-episode-seg--active' : ''"
        data-focusable="true"
        tabindex="0"
        :aria-selected="i === segmentIndex"
        role="tab"
        @click="selectSegment(i)"
      >
        {{ seg.label }}
      </button>
    </div>

    <!-- 集数网格 -->
    <div
      v-if="visibleEpisodes.length"
      class="jc-episode-grid jc-scrollauto"
      :class="{ 'jc-episode-grid--compact': allEpisodeNamesShort }"
    >
      <button
        v-for="{ ep, idx } in visibleEpisodes"
        :key="ep.link + '-' + idx"
        class="jc-episode-chip jc-hotarea"
        :class="[
          ep.link === currentEpisode ? 'jc-episode-chip--active' : '',
          watchedLinks.includes(ep.link) ? 'jc-episode-chip--watched' : ''
        ]"
        data-focusable="true"
        tabindex="0"
        :aria-current="ep.link === currentEpisode ? 'true' : undefined"
        @click="selectEpisode(idx)"
        @mouseenter="startMarquee"
        @mouseleave="stopMarquee"
        @focus="startMarquee"
        @blur="stopMarquee"
      >
        <span class="jc-episode-chip__label">{{ cleanEpisodeName(ep.episode) }}</span>
        <span
          v-if="watchedLinks.includes(ep.link) && ep.link !== currentEpisode"
          class="jc-episode-chip__dot"
          aria-hidden="true"
        />
      </button>
    </div>
  </section>
</template>

<style scoped>
/* 源 tab 外层条: 左侧 tab 列表占据剩余宽并可横滚(旧"共 N 集"钉角布局已移除, 死样式 .jc-source-count 已删) */
.jc-source-tabs {
  flex: 1 1 auto;
  min-width: 0;
  /* 横向滚动容器。纵向必须是 hidden 而不是 visible —— CSS 规范会把「visible + auto」的组合
     强制成两轴 auto, 于是容器凭空多出纵向滚动范围(滚轮一滚"看着不动"), 手感像卡住,
     这正是 2026-11-06 用户报的「没法左右滑动」的成因。纵向 6px padding 足够容纳
     焦点环(outline 2px + offset 2px), 不会被裁。 */
  overflow-x: auto;
  overflow-y: hidden;
  padding-block: 6px;
  /* 隐藏滚动条(affordance 由下方"按可滚方向"显隐的渐隐承担): 标准属性 + 下方 ::-webkit-scrollbar */
  scrollbar-width: none;
}
.jc-source-tabs::-webkit-scrollbar {
  display: none;
}

/* 横滚渐隐(2026-11-06): 由 JS 按实际可滚方向加类 —— 滑到头就不再显示该侧阴影。
   chrome61 双声明回退照旧(先老语法后标准语法, 老内核不识别标准值时落到上一条)。 */
.jc-source-tabs.is-fade-left {
  -webkit-mask-image: linear-gradient(90deg, transparent, #000 16px);
  mask-image: linear-gradient(90deg, transparent, #000 16px);
}
.jc-source-tabs.is-fade-right {
  -webkit-mask-image: linear-gradient(90deg, #000 calc(100% - 16px), transparent);
  mask-image: linear-gradient(90deg, #000 calc(100% - 16px), transparent);
}
.jc-source-tabs.is-fade-left.is-fade-right {
  -webkit-mask-image: linear-gradient(90deg, transparent, #000 16px, #000 calc(100% - 16px), transparent);
  mask-image: linear-gradient(90deg, transparent, #000 16px, #000 calc(100% - 16px), transparent);
}

/* 源条翻页箭头(2026-11-06): 与源 tab 同语言的次级按钮; 到头禁用(降透明度 + not-allowed) */
.jc-source-arrow {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: var(--jc-ctrl-h);
  height: var(--jc-ctrl-h);
  min-width: var(--jc-ctrl-h);
  border: none;
  border-radius: var(--jc-ctrl-radius);
  background: transparent;
  color: var(--jc-text-secondary);
  font-size: var(--jc-ctrl-fs-sm);
  cursor: pointer;
  transition:
    color var(--jc-dur-fast) var(--jc-ease-standard),
    background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-source-arrow:hover:not(:disabled) {
  color: var(--jc-text-primary);
  background-color: rgba(255, 255, 255, 0.06);
}
.jc-source-arrow:disabled {
  opacity: 0.32;
  cursor: not-allowed;
}

/* 触达热区(方案 §C6 #2): 源 tab/集格视觉高度 <44px, 命中面积靠 .jc-hotarea(theme.css §三)外扩补偿;
   横向相邻(源 tab 行 / 集格 gap 8px 档)按 4px inset 收窄, 防命中区重叠 */
.jc-source-tab,
.jc-episode-chip {
  --jc-hotarea-inset: 4px;
}

.jc-source-tab {
  position: relative;
  background: transparent;
  border: none;
  /* 2026-11-05 三档统一: 绑 --jc-ctrl-h(随端 24/28/32); TV 由下方覆盖保持原尺寸 */
  height: var(--jc-ctrl-h);
  margin-block: 4px;
  padding: 0 var(--jc-ctrl-px);
  /* 2026-11-06 用户要求: 选源胶囊 → 圆角矩形(不再 9999px 全圆), 走控件圆角令牌 */
  border-radius: var(--jc-ctrl-radius);
  color: var(--jc-text-secondary);
  font-size: var(--jc-ctrl-fs);
  font-weight: var(--jc-fw-medium);
  cursor: pointer;
  white-space: nowrap;
  min-height: var(--jc-ctrl-h);
  transition:
    color var(--jc-dur-fast) var(--jc-ease-standard),
    background-color var(--jc-dur-fast) var(--jc-ease-standard);
}

.jc-source-tab:hover {
  color: var(--jc-text-primary);
  background-color: rgba(255, 255, 255, 0.06);
}

/* 选中态: 仅渐变背景(胶囊), 不再加被祖先 overflow 裁切的下划线/描边。
 * 光圈语义(方案 §C6 #7, 维持现状并写明): 源 tab=导航层 → 激活只上渐变胶囊,
 * 不加紫色光圈(内容层独占, 见 .jc-episode-chip--active 注释)。 */
.jc-source-tab--active {
  color: #fff;
  font-weight: var(--jc-fw-semibold);
  background-image: var(--jc-brand-gradient);
}
.jc-source-tab--active:hover {
  /* 覆盖 hover 的半透明白底, 保持渐变 */
  background-color: transparent;
}

/* Web 焦点环: 用跟随圆角的 outline(不被祖先 overflow 裁切), 替代默认方形 outline */
.jc-source-tab:focus-visible {
  outline: 2px solid var(--jc-brand-cyan);
  outline-offset: 2px;
}

/* 线路测速色点(方案 §C6 #1): ⚡数值文本降级为 7px 色点, 数值走 title 悬浮不再撑宽 tab。
 * 绿(--fast)=最快线 / 黄(--mid)=已测一般 / 红(--fail)=失败 / 脉冲(--loading)=测速进行中 */
.jc-source-tab__speed {
  display: inline-block;
  flex: 0 0 auto;
  margin-left: var(--jc-space-1);
  width: 7px;
  height: 7px;
  border-radius: 9999px;
  vertical-align: middle;
}
.jc-source-tab__speed--fast {
  background-color: var(--jc-success);
}
.jc-source-tab__speed--mid {
  background-color: var(--jc-warning);
}
.jc-source-tab__speed--fail {
  background-color: var(--jc-danger);
}
.jc-source-tab__speed--loading {
  background-color: var(--jc-text-muted);
  animation: jc-speed-pulse 1.1s var(--jc-ease-standard, ease) infinite;
}
@keyframes jc-speed-pulse {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.35;
    transform: scale(0.72);
  }
}
@media (prefers-reduced-motion: reduce) {
  .jc-source-tab__speed--loading {
    animation: none;
  }
}

/* ▶ 正在播放的源小标(方案 §C6 #3): 仅详情页浏览态切走播放源时渲染(见模板处注释) */
.jc-source-tab__playing {
  margin-right: var(--jc-space-1);
  font-size: 10px;
  line-height: 1;
  color: var(--jc-brand-primary);
}

/* 源 tab 右上角集数徽标 — 小巧, 绝对定位不挤压 tab 文字布局 */
.jc-source-tab__count-badge {
  position: absolute;
  top: 0;
  right: 0;
  transform: translate(35%, -35%);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 9999px;
  background-color: var(--jc-bg-elevated);
  color: var(--jc-text-secondary);
  font-size: 10px;
  font-weight: var(--jc-fw-bold);
  line-height: 1;
  box-shadow: 0 0 0 1.5px var(--jc-bg-base, #0b0b0f);
  pointer-events: none;
}
/* 选中态徽标: 在渐变胶囊上用反白底, 对比更清晰 */
.jc-source-tab--active .jc-source-tab__count-badge {
  background-color: rgba(255, 255, 255, 0.92);
  color: var(--jc-brand-primary, #6d28d9);
}

/* 分段 chip (1-30 / 31-60 ...) — 基态 ghost(透明底 + hover 白色洗地, 对齐 BaseButton ghost 语言)。
 * 高度绑 --jc-ctrl-h-sm、字号绑 --jc-ctrl-fs-sm(与源 tab 拉开一档; 2026-11-05 三档统一 §二.B)。
 * 光圈语义(方案 §C6 #7, 维持现状并写明): seg=导航层 → 不加紫色光圈
 * (历史: 原激活态 box-shadow 紫圈会被外层 scroll/overflow 容器按直角硬切, 已去;
 *  内容层独占光圈, 见 .jc-episode-chip--active 注释)。 */
.jc-episode-seg {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: var(--jc-ctrl-h-sm);
  padding: 0 var(--jc-ctrl-px);
  border: none;
  border-radius: var(--jc-chip-radius, 9999px);
  background-color: transparent;
  color: var(--jc-text-secondary);
  font-size: var(--jc-ctrl-fs-sm);
  font-weight: var(--jc-fw-medium);
  cursor: pointer;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-episode-seg:hover {
  background-color: rgba(255, 255, 255, 0.06);
  color: var(--jc-text-primary);
}
/* 激活态(2026-11-06 用户要求): 还原为紫蓝渐变底 + 白字 —— §C6 #6 的「白底 + 主色字」作废。
   仍置于 :hover 之后, 并补 --active:hover 清掉 hover 的半透明白底(与源 tab 同款写法)。 */
.jc-episode-seg--active {
  background-color: transparent;
  background-image: var(--jc-brand-gradient);
  color: #fff;
}
.jc-episode-seg--active:hover {
  background-color: transparent;
  color: #fff;
}

/* 每行集数: 减少列数, 给每集文字更多宽度(集名可能较长, 如"第01集 高清").
 * 之前 PC 8-10 列把每个 chip 挤太窄 → 文字被裁. 现按"列最小宽"自适应,
 * 列宽不够时自动减少列数, 保证每集文字能显示. */
/* 每行最多 6 个(用户指定). 小屏窄, 用 auto-fill 但上限 6 列;
 * 用 min(已算列宽, 6 等分) 保证不超 6, 且每个 chip 文字够宽不裁. */
.jc-episode-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--jc-space-2);
}
@media (min-width: 768px) {
  .jc-episode-grid {
    grid-template-columns: repeat(6, minmax(0, 1fr));
    gap: var(--jc-space-3);
  }
}

/* 集名「全部偏短」(「第13集」「03」) → 集格宽折半：列数翻倍。
 * 口径注：用 1fr 分配时, 多出来的那批间隙已在分配里扣掉, 所以不会溢出、也不用手写 calc；
 * 净效果 = (W-7g)/8, 比"原文宽一半"(W-3g)/8 再小 g/2 ≈ 4px —— 这就是用户说的"记得扣去边距"。 */
.jc-episode-grid--compact {
  grid-template-columns: repeat(8, minmax(0, 1fr));
}
@media (min-width: 768px) {
  .jc-episode-grid--compact {
    grid-template-columns: repeat(12, minmax(0, 1fr));
  }
}

.jc-episode-chip {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  /* 2026-11-05 三档统一: 绑标准档(原 38/40/42→34/36/38 定值阶梯作废) */
  height: var(--jc-ctrl-h);
  padding: 0 var(--jc-ctrl-px-sm);
  border-radius: var(--jc-radius-md);
  background-color: var(--jc-bg-elevated);
  color: var(--jc-text-secondary);
  font-size: var(--jc-ctrl-fs-sm);
  font-weight: var(--jc-fw-semibold);
  border: none;
  cursor: pointer;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard),
    transform var(--jc-dur-fast) var(--jc-ease-standard);
  /* 基态不裁切(方案 §C6 #2): overflow:hidden 会把自身 .jc-hotarea::after 外扩热区一并裁掉(热区失效);
   * 长名截断由 __label ellipsis 承担, marquee 态自行恢复裁切(见 .is-marquee)。 */
}

/* 2026-11-05 B4 清尾: 原 768→36 / 1024→38 的定值阶梯已删除,
   选集 chip 全端统一走 --jc-ctrl-h(基础规则见文件上方) */

.jc-episode-chip:hover {
  background-color: rgba(255, 255, 255, 0.08);
  color: var(--jc-text-primary);
}

/* Web 焦点环: 跟随圆角 outline, 不被网格/容器 overflow 裁切 */
.jc-episode-chip:focus-visible {
  outline: 2px solid var(--jc-brand-cyan);
  outline-offset: 2px;
}

/* 激活态(内容层)保留紫光圈 — 光圈语义(方案 §C6 #7, 维持现状并写明):
 * 集 chip=内容层 → 激活上渐变+紫光圈; 源 tab/分段=导航层 → 无光圈(见各自规则注释)。 */
.jc-episode-chip--active {
  background-image: var(--jc-brand-gradient);
  color: #fff;
  box-shadow: var(--jc-shadow-purple-glow);
}

.jc-episode-chip--watched .jc-episode-chip__dot {
  position: absolute;
  top: 6px;
  left: 6px;
  width: 6px;
  height: 6px;
  border-radius: 9999px;
  background-color: var(--jc-success);
}

.jc-episode-chip__label {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 文字超出按钮时(hover/聚焦), 左对齐起始 + 横向轮播完整集名;
 * marquee 态恢复裁切(基态已放开 overflow 给热区让路, 见 .jc-episode-chip 注释),
 * 防轮播文字溢出胶囊 */
.jc-episode-chip.is-marquee {
  justify-content: flex-start;
  overflow: hidden;
}
.jc-episode-chip.is-marquee .jc-episode-chip__label {
  max-width: none;
  overflow: visible;
  text-overflow: clip;
  animation: jc-ep-marquee var(--jc-ep-dur, 3s) linear infinite alternate;
}
/* 两端各停顿一下便于阅读; 位移量 = 实测溢出像素(JS 注入 --jc-ep-shift) */
@keyframes jc-ep-marquee {
  0%,
  12% {
    transform: translateX(0);
  }
  88%,
  100% {
    transform: translateX(var(--jc-ep-shift, 0));
  }
}
@media (prefers-reduced-motion: reduce) {
  .jc-episode-chip.is-marquee .jc-episode-chip__label {
    animation: none;
  }
}
</style>

<style>
[data-mode='tv'] .jc-episode-grid {
  /* P0: 列数自适应 — 大屏更多列, 集名仍够宽不裁; 960 ~6 列, 1920 ~10 列
   * 老内核(Chrome<79)回退: clamp 不识别时整条 grid-template-columns 失效 → 单列堆叠 */
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  grid-template-columns: repeat(auto-fit, minmax(clamp(110px, 11vw, 160px), 1fr));
  gap: var(--jc-space-3);
}
[data-mode='tv'] .jc-episode-chip {
  /* ===== TV 40px 豁免(方案 §C6 #8, 注释强化) =====
   * 例外口径(三档方案 docs/方案-web控件三档标准统一-2026-11-05.md): TV 为独立档、全不动 —
   * §二.B "EpisodeTabs 全绑 --jc-ctrl-h/-fs/-px; TV 覆盖不动"、§五.5 "TV 不动(独立档)"、
   * §六.D "TV 档保持 48/40/56 —— 默认保持"、§7.3 验收豁免表 "所有 [data-mode='tv'] 域全不动"。
   * 即本块 40px 定值不随 web 24/28/32 ladder 收紧, 保留遥控器十米可点性; 下方 seg 28px / 源 tab 40px 同口径。 */
  height: 40px;
  font-size: var(--jc-fs-sm);
  padding: 0 8px;
}
/* TV: 分段胶囊保持 28px 定值(TV 独立档豁免, 口径见 .jc-episode-chip TV 块注释) */
[data-mode='tv'] .jc-episode-seg {
  height: 28px;
  padding: 0 12px;
  font-size: var(--jc-fs-sm);
}
[data-mode='tv'] .jc-episode-chip:focus,
[data-mode='tv'] .jc-episode-chip:focus-visible {
  /* outline 随圆角且不被祖先 overflow 裁切; 替代易被裁的 box-shadow 环 */
  outline: 3px solid var(--jc-brand-cyan);
  outline-offset: 2px;
  box-shadow: 0 0 14px rgba(74, 209, 229, 0.4);
  background-color: rgba(255, 255, 255, 0.12);
  color: var(--jc-text-primary);
}
[data-mode='tv'] .jc-source-tab {
  /* TV 40px 豁免(TV 独立档, 口径同 .jc-episode-chip TV 块注释) */
  height: 40px;
  font-size: var(--jc-fs-sm);
  padding: 0 var(--jc-space-3);
  margin-block: 4px;
}
[data-mode='tv'] .jc-source-tab__count-badge {
  min-width: 18px;
  height: 18px;
  font-size: 11px;
  padding: 0 5px;
}
[data-mode='tv'] .jc-source-tab:focus,
[data-mode='tv'] .jc-source-tab:focus-visible {
  outline: 3px solid var(--jc-brand-cyan);
  outline-offset: 2px;
  box-shadow: 0 0 14px rgba(74, 209, 229, 0.4);
  border-radius: var(--jc-radius-sm);
  color: var(--jc-text-primary);
}
/* 源 tab 横滚条 / 集数容器: 纵向留白 + 不裁纵向, 让焦点框完整显示 */
[data-mode='tv'] .jc-source-tabs {
  overflow-y: visible;
  padding-block: 4px;
}
[data-mode='tv'] .jc-episodes {
  padding-block: 4px;
}

/* 方案 §C6 #9: 移动端无 hover(marquee 不可达), active 集格两行常显完整名, 不再永远截断。
 * data-mode 口径与 theme.css 移动端兜底一致; 非 active chip 仍走 ellipsis + marquee(hover/focus)。
 * 极端超长名兜底: 两行 -webkit-line-clamp 后仍裁(chrome61 支持 -webkit-box 语法)。
 * 注: 换行后 label 无横向溢出, marquee 测量 shift<=2 自然不触发, 两态共存无冲突。 */
[data-mode='mobile'] .jc-episode-chip--active {
  height: auto;
  min-height: var(--jc-ctrl-h);
}
[data-mode='mobile'] .jc-episode-chip--active .jc-episode-chip__label {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
  white-space: normal;
}
</style>

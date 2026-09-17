<script setup lang="ts">
import { computed, ref, watch } from 'vue'
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
  /** 影片名 — 用于去掉集名里冗余的剧名前缀(如"我是谁 第01集"→"第01集") */
  filmName?: string
}

const props = withDefaults(defineProps<Props>(), {
  currentSourceId: '',
  currentEpisode: '',
  watchedLinks: () => [],
  pageSize: 30,
  speeds: () => ({}),
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

/** 最快线路 id (用于高亮 ⚡) */
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
function speedLabel(id: string): string {
  const v = props.speeds[id]
  if (v === undefined) return ''
  if (v < 0) return '✗'
  return id === fastestId.value ? `⚡${v}ms` : `${v}ms`
}
function speedClass(id: string): string {
  const v = props.speeds[id]
  if (v === undefined) return ''
  if (v < 0) return 'jc-source-tab__speed--fail'
  return id === fastestId.value ? 'jc-source-tab__speed--fast' : 'jc-source-tab__speed--ok'
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
</script>

<template>
  <section class="jc-episodes flex flex-col gap-[var(--jc-space-4)]">
    <!-- 播放源 Tab(每个 tab 右上角已带该源集数徽标, 不再单独显示"共 N 集") -->
    <div
      v-if="sources.length > 1"
      class="jc-source-bar flex items-center gap-[var(--jc-space-4)] border-b border-default"
    >
      <div
        v-if="sources.length > 1"
        class="jc-source-tabs flex items-center gap-[var(--jc-space-6)] overflow-x-auto"
      >
        <button
          v-for="s in sources"
          :key="s.id"
          class="jc-source-tab"
          :class="s.id === activeSourceId ? 'jc-source-tab--active' : ''"
          data-focusable="true"
          tabindex="0"
          :aria-selected="s.id === activeSourceId"
          @click="selectSource(s.id)"
        >
          <span class="jc-source-tab__name">{{ s.name }}</span>
          <span v-if="speedLabel(s.id)" class="jc-source-tab__speed" :class="speedClass(s.id)">
            {{ speedLabel(s.id) }}
          </span>
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
    <div v-if="visibleEpisodes.length" class="jc-episode-grid">
      <button
        v-for="{ ep, idx } in visibleEpisodes"
        :key="ep.link + '-' + idx"
        class="jc-episode-chip"
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
/* 源 tab 外层条: 左侧 tab 列表占据剩余宽并可横滚, 右侧"共 N 集"固定钉右上角不随滚动 */
.jc-source-tabs {
  flex: 1 1 auto;
  min-width: 0;
  /* 纵向留白 + 不裁纵向, 让焦点框/激活态完整显示(避免被祖先 overflow 截断) */
  overflow-y: visible;
  padding-block: 6px;
}
.jc-source-count {
  flex: 0 0 auto;
  margin-left: auto;
  white-space: nowrap;
  color: var(--jc-text-muted);
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-medium);
}

.jc-source-tab {
  position: relative;
  background: transparent;
  border: none;
  /* 内部留白收紧: 高 40→36, 左右 12→10 (各断点一致, TV 由下方覆盖保持原尺寸) */
  height: 36px;
  margin-block: 4px;
  padding: 0 10px;
  border-radius: var(--jc-chip-radius, 9999px);
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-md);
  font-weight: var(--jc-fw-medium);
  cursor: pointer;
  white-space: nowrap;
  min-height: 36px;
  transition:
    color var(--jc-dur-fast) var(--jc-ease-standard),
    background-color var(--jc-dur-fast) var(--jc-ease-standard);
}

.jc-source-tab:hover {
  color: var(--jc-text-primary);
  background-color: rgba(255, 255, 255, 0.06);
}

/* 选中态: 仅渐变背景(胶囊), 不再加被祖先 overflow 裁切的下划线/描边 */
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

/* 线路测速延时小标 */
.jc-source-tab__speed {
  margin-left: var(--jc-space-1);
  font-size: var(--jc-fs-xs);
  font-weight: var(--jc-fw-medium);
}
.jc-source-tab__speed--fast {
  color: var(--jc-success);
}
.jc-source-tab__speed--ok {
  color: var(--jc-text-muted);
}
.jc-source-tab__speed--fail {
  color: var(--jc-danger);
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

/* 分段 chip (1-30 / 31-60 ...)
 * 纯色胶囊: 不加 border / 光圈。原先激活态的 box-shadow 紫色光圈会被
 * 外层 scroll/overflow 容器按直角硬切(上/左被截断, 右/下正常渐隐),
 * 看起来像"被胶囊切掉的边框", 故整体去掉描边装饰, 激活态只留渐变胶囊。
 * 内部留白收紧: 高 32→28, 左右 14→10 (TV 由下方覆盖保持原尺寸)。 */
.jc-episode-seg {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  padding: 0 10px;
  border: none;
  border-radius: var(--jc-chip-radius, 9999px);
  background-color: var(--jc-bg-elevated);
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-medium);
  cursor: pointer;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-episode-seg:hover {
  background-color: rgba(255, 255, 255, 0.08);
  color: var(--jc-text-primary);
}
.jc-episode-seg--active {
  background-image: var(--jc-brand-gradient);
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

.jc-episode-chip {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  /* 内部留白收紧(各断点): 高 38/40/42→34/36/38, 左右 8→6 */
  height: 34px;
  padding: 0 6px;
  border-radius: var(--jc-radius-md);
  background-color: var(--jc-bg-elevated);
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-semibold);
  border: none;
  cursor: pointer;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard),
    transform var(--jc-dur-fast) var(--jc-ease-standard);
  overflow: hidden;
}

@media (min-width: 768px) {
  .jc-episode-chip {
    height: 36px;
  }
}
@media (min-width: 1024px) {
  .jc-episode-chip {
    height: 38px;
  }
}

.jc-episode-chip:hover {
  background-color: rgba(255, 255, 255, 0.08);
  color: var(--jc-text-primary);
}

/* Web 焦点环: 跟随圆角 outline, 不被网格/容器 overflow 裁切 */
.jc-episode-chip:focus-visible {
  outline: 2px solid var(--jc-brand-cyan);
  outline-offset: 2px;
}

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

/* 文字超出按钮时(hover/聚焦), 左对齐起始 + 横向轮播完整集名 */
.jc-episode-chip.is-marquee {
  justify-content: flex-start;
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
  /* P0: 列数自适应 — 大屏更多列, 集名仍够宽不裁; 960 ~6 列, 1920 ~10 列 */
  grid-template-columns: repeat(auto-fit, minmax(clamp(120px, 13vw, 180px), 1fr));
  gap: var(--jc-space-4);
}
[data-mode='tv'] .jc-episode-chip {
  height: 48px;
  font-size: var(--jc-fs-sm);
}
/* TV: 分段胶囊不跟随 Web 的留白收紧, 保持 10 尺 UI 原有尺寸 */
[data-mode='tv'] .jc-episode-seg {
  height: 32px;
  padding: 0 14px;
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
  height: 48px;
  font-size: var(--jc-fs-base);
  padding: 0 var(--jc-space-4);
  margin-block: 6px;
}
[data-mode='tv'] .jc-source-tab__count-badge {
  min-width: 22px;
  height: 22px;
  font-size: 13px;
  padding: 0 6px;
}
[data-mode='tv'] .jc-source-count {
  font-size: var(--jc-fs-base);
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
  padding-block: 6px;
}
[data-mode='tv'] .jc-episodes {
  padding-block: 4px;
}
</style>

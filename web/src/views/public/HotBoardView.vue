<script setup lang="ts">
import { computed, ref } from 'vue'
import * as filmApi from '@/api/film'
import type { Card, HotBoardFilters } from '@/types/film'
import type { PageMeta } from '@/types/api'
import FilmGrid from '@/components/film/FilmGrid.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import FilmFilterBar, {
  type FilterGroup
} from '@/components/film/FilmFilterBar.vue'
import { useQuerySync } from '@/composables/useQuerySync'
import { useAbortable } from '@/composables/useAbortable'
import { useViewMode } from '@/composables/useViewMode'
import { useNavStore } from '@/stores'

/**
 * /hotBoard?Board=&Pid=&Year=&Area=&Language=&Sort=&current=
 * 独立热榜页(方案文档 §1)。
 *
 * query 大写键沿用旧站约定(与 ClassifySearchView 一致):
 *  Board / Pid / Year / Area / Language / Sort / current
 * 发请求时再转成后端小写下划线 query(board/pid/year/area/language/sort)。
 */

type QueryShape = {
  Board: string
  Pid: string
  Year: string
  Area: string
  Language: string
  Sort: string
  current: number
} & Record<string, string | number>

const initial: QueryShape = {
  Board: '',
  Pid: '',
  Year: '',
  Area: '',
  Language: '',
  // 全榜并集(Board 为空)时 rank 无意义, 后端会回落 hot; 首屏即全榜 → 默认热度。
  Sort: 'hot',
  current: 1
}

const { params, push } = useQuerySync<QueryShape>(initial, {
  path: '/hotBoard',
  onChange: () => {
    void load()
  }
})

const { isTV } = useViewMode()
const navStore = useNavStore()
const films = ref<Card[]>([])
const page = ref<PageMeta>({ size: 49, current: 1, pageCount: 0, total: 0 })
const filters = ref<HotBoardFilters | null>(null)

const loading = ref<boolean>(true)
const loaded = ref<boolean>(false)
const errorMsg = ref<string>('')
const abortable = useAbortable()

/** 排序固定选项(无"全部"态): rank=榜位(仅单榜) / hot=热度 / score=评分 / latest=最新上线 */
const SORT_OPTIONS: { value: string; label: string }[] = [
  { value: 'rank', label: '榜位' },
  { value: 'hot', label: '热度' },
  { value: 'score', label: '评分' },
  { value: 'latest', label: '最新上线' }
]

async function load(): Promise<void> {
  loading.value = true
  errorMsg.value = ''
  const signal = abortable.refresh()
  void navStore.ensureLoaded()
  try {
    filters.value = await filmApi.getHotBoardFilters()
  } catch { /* 筛选维度失败不阻断列表 */ }
  try {
    const r = await filmApi.getHotBoard(
      {
        board: params.value.Board || '',
        pid: Number(params.value.Pid) || 0,
        year: Number(params.value.Year) || 0,
        area: params.value.Area || '',
        language: params.value.Language || '',
        sort: params.value.Sort || 'hot',
        page: params.value.current || 1,
        size: 49
      },
      { signal }
    )
    films.value = r.list ?? []
    page.value = r.page ?? page.value
    errorMsg.value = ''
    loaded.value = true
  } catch (e) {
    if ((e as { name?: string })?.name === 'CanceledError') return
    errorMsg.value = '加载失败，请稍后重试'
    loaded.value = true
  } finally {
    loading.value = false
  }
}
void load()

/** 把固定维度拼成 FilterGroup[]; 榜单/分类/年份/地区/语言/排序六组 */
const filterGroups = computed<FilterGroup[]>(() => {
  const groups: FilterGroup[] = []
  const cur = (k: string): string => String(params.value[k] ?? '')

  // 1. 榜单(全部 + 8 榜)
  groups.push({
    key: 'Board',
    title: '榜单',
    options: [
      { value: '', label: '全部榜单' },
      ...(filters.value?.boards ?? []).map((b) => ({ value: b.value, label: b.label }))
    ],
    current: cur('Board')
  })

  // 2. 分类(全部 + nav 一级分类; 隐藏分类天然不在 nav 里)
  groups.push({
    key: 'Pid',
    title: '分类',
    options: [
      { value: '', label: '全部' },
      ...navStore.list.map((c) => ({ value: String(c.id), label: c.name }))
    ],
    current: cur('Pid')
  })

  // 3. 年份 / 4. 地区 / 5. 语言(全部 + filters 聚合选项)
  const simpleGroup = (
    key: string,
    title: string,
    values: (string | number)[]
  ): FilterGroup => ({
    key,
    title,
    options: [
      { value: '', label: '全部' },
      ...values.map((v) => ({ value: String(v), label: String(v) }))
    ],
    current: cur(key)
  })
  groups.push(simpleGroup('Year', '年份', filters.value?.years ?? []))
  groups.push(simpleGroup('Area', '地区', filters.value?.areas ?? []))
  groups.push(simpleGroup('Language', '语言', filters.value?.languages ?? []))

  // 6. 排序(无"全部")
  groups.push({ key: 'Sort', title: '排序', options: SORT_OPTIONS, current: cur('Sort') })

  return groups
})

function onFilterChange(payload: { key: string; value: string | number }): void {
  const allowed = ['Board', 'Pid', 'Year', 'Area', 'Language', 'Sort']
  if (!allowed.includes(payload.key)) return
  const isSame = String(params.value[payload.key] ?? '') === String(payload.value)
  const next: Partial<QueryShape> = {}
  // 排序维度无"全部": 再点同项保持选中, 不取消
  if (payload.key !== 'Sort' && isSame) {
    next[payload.key] = ''
  } else {
    next[payload.key] = String(payload.value)
  }
  // 榜单切换联动排序: 选具体榜 → 按榜位; 回到全部榜单 → 按热度(rank 对并集无效)。
  if (payload.key === 'Board') {
    next.Sort = String(payload.value) ? 'rank' : 'hot'
  }
  // 切换任意筛选 → current 重置 1
  next.current = 1
  void push(next)
}

function onPaginate(p: number): void {
  void push({ current: p })
  if (typeof window !== 'undefined') {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
}

/** TV 翻页 */
const tvPageCount = computed(() => Math.max(1, page.value.pageCount || 1))
const tvCurrent = computed(() => page.value.current || 1)
function tvGoPage(delta: number): void {
  const next = tvCurrent.value + delta
  if (next < 1 || next > tvPageCount.value) return
  onPaginate(next)
}

/** 当前榜单名(TV/桌面标题用) */
const currentBoardName = computed(() => {
  const b = params.value.Board
  if (!b) return '全部榜单'
  return filters.value?.boards.find((x) => x.value === b)?.label ?? '热榜'
})
</script>

<template>
  <!-- ===================== TV 分支 (复用 jc-cs-tv 左右分屏版式) ===================== -->
  <section v-if="isTV" class="jc-cs-tv jc-hb-tv container-page py-[var(--jc-space-6)]">
    <div class="jc-cs-tv__head jc-tv-sec">
      <span class="jc-hb-tv__title">热榜 · {{ currentBoardName }}</span>
      <span v-if="page.total > 0" class="jc-cs-tv__count">
        共 <strong>{{ page.total }}</strong> 部影片 · 第 {{ tvCurrent }} / {{ tvPageCount }} 页
      </span>
    </div>

    <BaseEmpty
      v-if="!loading && errorMsg"
      :title="errorMsg"
      description="请稍后重试"
    />

    <div v-else class="jc-cs-tv__split">
      <!-- 左: 竖向筛选栏 -->
      <aside class="jc-cs-tv__filter">
        <div
          v-for="group in filterGroups"
          :key="group.key"
          class="jc-cs-tv__grp"
        >
          <div class="jc-cs-tv__grp-title">{{ group.title }}</div>
          <div class="jc-cs-tv__grp-chips">
            <button
              v-for="opt in group.options"
              :key="String(opt.value)"
              type="button"
              class="jc-tv-chip jc-cs-tv__chip"
              :class="String(opt.value) === String(group.current) ? 'sel' : ''"
              data-focusable="true"
              tabindex="0"
              :aria-pressed="String(opt.value) === String(group.current)"
              @click="onFilterChange({ key: group.key, value: opt.value })"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>
      </aside>

      <!-- 右: 结果网格 -->
      <main class="jc-cs-tv__main">
        <div v-if="loading && films.length === 0" class="jc-tv-grid">
          <BaseSkeleton
            v-for="i in 12"
            :key="i"
            height="auto"
            ratio="3/4"
          />
        </div>

        <div v-else-if="films.length > 0" class="jc-tv-grid">
          <FilmCard
            v-for="item in films"
            :key="item.mid"
            :item="item"
            :show-title-below="true"
          />
        </div>

        <BaseEmpty
          v-else-if="!loading && loaded"
          title="该榜单暂无影片"
          description="试试切换其他筛选条件"
        />

        <div
          v-if="!loading && page.total > 0 && tvPageCount > 1"
          class="jc-cs-tv__pager"
        >
          <button
            type="button"
            class="jc-tv-btn"
            :disabled="tvCurrent <= 1"
            data-focusable="true"
            tabindex="0"
            @click="tvGoPage(-1)"
          >
            上一页
          </button>
          <span class="jc-cs-tv__pager-info">{{ tvCurrent }} / {{ tvPageCount }}</span>
          <button
            type="button"
            class="jc-tv-btn cyan"
            :disabled="tvCurrent >= tvPageCount"
            data-focusable="true"
            tabindex="0"
            @click="tvGoPage(1)"
          >
            下一页
          </button>
        </div>
      </main>
    </div>
  </section>

  <!-- ===================== 桌面 / 移动分支 ===================== -->
  <div v-else class="jc-classify-search jc-hot-board container-page py-[var(--jc-space-6)]">
    <header
      class="flex items-center gap-[var(--jc-space-3)] mb-[var(--jc-space-6)]"
    >
      <h1 class="jc-hot-board__title">热榜</h1>
      <span class="jc-hot-board__sub">{{ currentBoardName }}</span>
    </header>

    <BaseEmpty
      v-if="!loading && errorMsg"
      :title="errorMsg"
      description="请稍后重试"
    />

    <template v-else>
      <div class="jc-classify-search__filter-wrap">
        <FilmFilterBar
          :groups="filterGroups"
          @change="onFilterChange"
        />
      </div>

      <div
        v-if="page.total > 0"
        class="jc-classify-search__count flex items-baseline justify-between mb-[var(--jc-space-4)] mt-[var(--jc-space-5)]"
      >
        <p class="text-secondary text-[length:var(--jc-fs-sm)]">
          共 <strong class="text-primary">{{ page.total }}</strong> 部影片
        </p>
        <span class="text-muted text-[length:var(--jc-fs-xs)]">
          第 {{ page.current || 1 }} / {{ Math.max(1, page.pageCount || 1) }} 页
        </span>
      </div>

      <div
        v-if="loading && films.length === 0"
        class="jc-card-grid"
      >
        <BaseSkeleton
          v-for="i in 12"
          :key="i"
          height="auto"
          ratio="3/4"
        />
      </div>

      <FilmGrid v-else-if="films.length > 0" :items="films" />

      <div
        v-if="!loading && page.total > 0"
        class="mt-[var(--jc-space-8)] flex justify-center"
      >
        <BasePagination
          :current="page.current || 1"
          :page-size="page.size || 49"
          :total="page.total"
          @change="onPaginate"
        />
      </div>

      <BaseEmpty
        v-if="!loading && loaded && films.length === 0 && !errorMsg"
        title="该榜单暂无影片"
        description="试试切换其他筛选条件"
      />
    </template>
  </div>
</template>

<style scoped>
.jc-hot-board__title {
  font-size: var(--jc-fs-2xl);
  font-weight: var(--jc-fw-bold);
  margin: 0;
}
.jc-hot-board__sub {
  color: var(--jc-text-muted);
  font-size: var(--jc-fs-base);
}
@media (max-width: 767px) {
  .jc-hot-board__title {
    font-size: var(--jc-fs-xl);
  }
}

/* TV 标题: 与分类筛选页 jc-cs-tv 标题同档 */
.jc-hb-tv__title {
  font-size: var(--jc-fs-base);
  font-weight: var(--jc-fw-bold);
}

/* 复用 ClassifySearchView 的 TV 左右分屏 / 玻璃筛选栏 / 翻页样式 */
.jc-hb-tv .jc-cs-tv__head {
  margin-bottom: var(--jc-space-3);
}
.jc-hb-tv .jc-cs-tv__count {
  margin-left: var(--jc-space-4);
  color: var(--jc-text-muted);
  font-size: var(--jc-fs-xs);
}
.jc-hb-tv .jc-cs-tv__count strong {
  color: var(--jc-brand-cyan);
}
.jc-hb-tv .jc-cs-tv__split {
  display: grid;
  grid-template-columns: 2fr 4fr;
  gap: var(--jc-space-6);
  align-items: start;
}
.jc-hb-tv .jc-cs-tv__filter {
  border-radius: var(--jc-radius-xl, 20px);
  border: 1px solid var(--jc-tv-stroke, rgba(255, 255, 255, 0.13));
  background: var(--jc-tv-glass, rgba(21, 21, 28, 0.88));
  padding: var(--jc-space-4) var(--jc-space-4) var(--jc-space-5);
}
.jc-hb-tv .jc-cs-tv__grp {
  padding: var(--jc-space-3) 0;
}
.jc-hb-tv .jc-cs-tv__grp + .jc-cs-tv__grp {
  border-top: 1px solid var(--jc-tv-stroke, rgba(255, 255, 255, 0.13));
}
.jc-hb-tv .jc-cs-tv__grp-title {
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-semibold);
  color: var(--jc-text-muted);
  margin-bottom: var(--jc-space-2);
}
.jc-hb-tv .jc-cs-tv__grp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--jc-space-2);
}
.jc-hb-tv .jc-cs-tv__chip {
  height: 34px;
  padding: 0 10px;
  font-size: var(--jc-fs-xs);
}
.jc-hb-tv .jc-cs-tv__main {
  min-width: 0;
}
.jc-hb-tv .jc-cs-tv__main .jc-tv-grid {
  grid-template-columns: repeat(4, 1fr);
}
.jc-hb-tv .jc-cs-tv__pager {
  margin-top: var(--jc-space-8);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--jc-space-4);
}
.jc-hb-tv .jc-cs-tv__pager-info {
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-base);
  font-weight: var(--jc-fw-semibold);
  min-width: 72px;
  text-align: center;
}
.jc-hb-tv .jc-cs-tv__pager .jc-tv-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* 桌面/移动筛选栏: 与分类筛选页同款吸边处理(随页面滚动, 不 sticky) */
.jc-hot-board .jc-classify-search__filter-wrap {
  background-color: rgba(11, 11, 15, 0.92);
  margin-inline: calc(-1 * var(--jc-gutter-mobile));
  padding: var(--jc-space-2) var(--jc-gutter-mobile);
  border-bottom: 1px solid var(--jc-border-subtle);
}
@media (min-width: 768px) {
  .jc-hot-board .jc-classify-search__filter-wrap {
    padding: var(--jc-space-3) var(--jc-gutter-tablet);
    margin-inline: calc(-1 * var(--jc-gutter-tablet));
  }
}
@media (min-width: 1024px) {
  .jc-hot-board .jc-classify-search__filter-wrap {
    margin-inline: calc(-1 * var(--jc-gutter-desktop));
    padding-inline: var(--jc-gutter-desktop);
  }
}
</style>

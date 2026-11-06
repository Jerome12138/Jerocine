<script setup lang="ts">
import { computed, ref } from 'vue'
import * as filmApi from '@/api/film'
import type { Card, HotBoardFilters } from '@/types/film'
import type { PageMeta } from '@/types/api'
import type { FilterGroup } from '@/components/film/FilmFilterBar.vue'
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

/** TV 页头"第 X / Y 页"计数; 翻页边界保护已随 W2a 收编进 TvResultBoard(越界不上抛) */
const tvPageCount = computed(() => Math.max(1, page.value.pageCount || 1))
const tvCurrent = computed(() => page.value.current || 1)

/** 当前榜单名(TV/桌面标题用) */
const currentBoardName = computed(() => {
  const b = params.value.Board
  if (!b) return '全部榜单'
  return filters.value?.boards.find((x) => x.value === b)?.label ?? '热榜'
})
</script>

<template>
  <!-- ===================== TV 分支 (复用 jc-cs-tv 左右分屏版式, W2a 收编进 TvResultBoard) ===================== -->
  <TvResultBoard
    v-if="isTV"
    class="jc-cs-tv jc-hb-tv container-page py-[var(--jc-space-6)]"
    :loading="loading"
    :loaded="loaded"
    :error-msg="errorMsg"
    error-description="请稍后重试"
    :films="films"
    :page="page"
    :filter-groups="filterGroups"
    empty-title="该榜单暂无影片"
    @filter-change="onFilterChange"
    @paginate="onPaginate"
  >
    <!-- 页头(§C8#5 收编 TvSectionHeader, 异形头走默认槽自绘) -->
    <template #head>
      <TvSectionHeader class="jc-cs-tv__head">
        <span class="jc-hb-tv__title">热榜 · {{ currentBoardName }}</span>
        <span v-if="page.total > 0" class="jc-cs-tv__count">
          共 <strong>{{ page.total }}</strong> 部影片 · 第 {{ tvCurrent }} / {{ tvPageCount }} 页
        </span>
      </TvSectionHeader>
    </template>
  </TvResultBoard>

  <!-- ===================== 桌面 / 移动分支 (W2b 收编进 FilmResultSection) ===================== -->
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
      <FilmResultSection
        :loading="loading"
        :loaded="loaded"
        :error-msg="errorMsg"
        :films="films"
        :page="page"
        :filter-groups="filterGroups"
        empty-title="该榜单暂无影片"
        @filter-change="onFilterChange"
        @paginate="onPaginate"
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

/* 页头(榜单名+计数)仍由本视图经 #head 插槽提供, scoped 命中插槽内容 —— 留在本文件;
   分屏/玻璃筛选栏/翻页样式已随 W2a 收编进 TvResultBoard(原 .jc-hb-tv 前缀抄写副本删除) */
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

/* 桌面/移动筛选栏吸边样式已随 W2b 收编进 FilmResultSection */
</style>

<script setup lang="ts">
import { computed, ref } from 'vue'
import * as filmApi from '@/api/film'
import type { Card, FilterTag, Filters } from '@/types/film'
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
 * /filmClassifySearch?Pid=&Category=&Plot=&Area=&Language=&Year=&Sort=&current=
 * STORY-012 分类筛选页
 *
 * 注意：query 字段大小写严格保留旧站约定
 *  Pid / Category / Plot / Area / Language / Year / Sort / current
 */

type QueryShape = {
  Pid: string
  Category: string
  Plot: string
  Area: string
  Language: string
  Year: string
  Sort: string
  current: number
} & Record<string, string | number>

const initial: QueryShape = {
  Pid: '',
  Category: '',
  Plot: '',
  Area: '',
  Language: '',
  Year: '',
  Sort: '',
  current: 1
}

const { params, push } = useQuerySync<QueryShape>(initial, {
  path: '/filmClassifySearch',
  onChange: () => {
    void load()
  }
})

const { isTV } = useViewMode()
const navStore = useNavStore()
const films = ref<Card[]>([])
const page = ref<PageMeta>({ size: 49, current: 1, pageCount: 0, total: 0 })
const filters = ref<Filters | null>(null)
const titleName = computed(() => navStore.findById(Number(params.value.Pid) || 0)?.name ?? '')
const pidStr = computed(() => params.value.Pid)

const loading = ref<boolean>(true)
const loaded = ref<boolean>(false)
const errorMsg = ref<string>('')
const abortable = useAbortable()

async function load(): Promise<void> {
  const pid = (params.value.Pid || '').trim()
  if (!pid) {
    errorMsg.value = '缺少分类参数 Pid'
    loading.value = false
    loaded.value = true
    return
  }
  loading.value = true
  errorMsg.value = ''
  const signal = abortable.refresh()
  void navStore.ensureLoaded()
  try {
    filters.value = await filmApi.getFilters(pid)
  } catch { /* 筛选维度失败不阻断列表 */ }
  try {
    const r = await filmApi.getFilms(
      {
        pid: Number(pid) || 0,
        category: Number(params.value.Category) || 0,
        plot: params.value.Plot || '',
        area: params.value.Area || '',
        language: params.value.Language || '',
        year: Number(params.value.Year) || 0,
        sort: params.value.Sort || '',
        page: params.value.current || 1,
        size: 49
      },
      { signal }
    )
    films.value = r.list ?? []
    page.value = r.page ?? page.value
    loaded.value = true
  } catch (e) {
    if ((e as { name?: string })?.name === 'CanceledError') return
    errorMsg.value = '加载失败，请稍后重试'
    loaded.value = true
  } finally {
    loading.value = false
  }
}
// useQuerySync 的 onChange 会在路由 query 变化时触发 load；
// 进入页面（即使无 query 变化）也立即 load 一次保证首屏渲染。
void load()

/**
 * 把 /categories/:pid/filters 的 sortList / titles / tags 拼成 FilterGroup[]
 * 用 params 中当前值作为 current
 */
const filterGroups = computed<FilterGroup[]>(() => {
  const f = filters.value
  if (!f?.sortList?.length) return []
  return f.sortList
    .map<FilterGroup | null>((key) => {
      const tagList: FilterTag[] = f.tags?.[key] || []
      if (tagList.length === 0) return null
      return {
        key,
        title: f.titles?.[key] || key,
        options: tagList.map((t) => ({
          value: String(t.value ?? ''),
          label: String(t.name ?? '')
        })),
        current: String(params.value[key] ?? '')
      }
    })
    .filter((g): g is FilterGroup => g !== null)
})

function onFilterChange(payload: { key: string; value: string | number }): void {
  const allowed = ['Category', 'Plot', 'Area', 'Language', 'Year', 'Sort']
  if (!allowed.includes(payload.key)) return
  const next: Partial<QueryShape> = {
    [payload.key]: String(payload.value)
  } as Partial<QueryShape>
  // 切换筛选项 → current 重置 1
  next.current = 1
  void push(next)
}

function onPaginate(p: number): void {
  void push({ current: p })
  if (typeof window !== 'undefined') {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
}

/** TV 分支翻页: 边界保护后复用 onPaginate */
const tvPageCount = computed(() => Math.max(1, page.value.pageCount || 1))
const tvCurrent = computed(() => page.value.current || 1)
function tvGoPage(delta: number): void {
  const next = tvCurrent.value + delta
  if (next < 1 || next > tvPageCount.value) return
  onPaginate(next)
}
</script>

<template>
  <!-- ===================== TV 分支 (雷鸟卡片式: 左右分屏) ===================== -->
  <section
    v-if="isTV"
    class="gf-cs-tv container-page py-[var(--gf-space-6)]"
  >
    <!-- 标题切换: 电视剧(分类页) | 电视剧库(当前筛选页, 渐变高亮) + 结果统计 -->
    <div v-if="titleName" class="gf-cs-tv__head gf-tv-sec">
      <RouterLink
        :to="{ path: '/filmClassify', query: { Pid: pidStr } }"
        class="gf-cs-tv__title-link"
        data-focusable="true"
        tabindex="0"
      >
        {{ titleName }}
      </RouterLink>
      <span class="gf-cs-tv__title-divider" aria-hidden="true">|</span>
      <RouterLink
        :to="{ path: '/filmClassifySearch', query: { Pid: pidStr } }"
        class="gf-cs-tv__title-active"
        data-focusable="true"
        tabindex="0"
      >
        {{ titleName }}库
      </RouterLink>
      <span v-if="page.total > 0" class="gf-cs-tv__count">
        共 <strong>{{ page.total }}</strong> 部影片 · 第 {{ tvCurrent }} / {{ tvPageCount }} 页
      </span>
    </div>

    <!-- 错误态 -->
    <BaseEmpty
      v-if="!loading && errorMsg"
      :title="errorMsg"
      description="请检查链接中的 Pid 参数"
    />

    <div v-else class="gf-cs-tv__split">
      <!-- ===== 左: 竖向筛选分区栏 ===== -->
      <aside
        v-if="filterGroups.length > 0"
        class="gf-cs-tv__filter"
      >
        <div
          v-for="group in filterGroups"
          :key="group.key"
          class="gf-cs-tv__grp"
        >
          <div class="gf-cs-tv__grp-title">{{ group.title }}</div>
          <div class="gf-cs-tv__grp-chips">
            <button
              v-for="opt in group.options"
              :key="String(opt.value)"
              type="button"
              class="gf-tv-chip gf-cs-tv__chip"
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

      <!-- ===== 右: 结果网格 ===== -->
      <main class="gf-cs-tv__main">
        <!-- 骨架 -->
        <div v-if="loading && films.length === 0" class="gf-tv-grid">
          <BaseSkeleton
            v-for="i in 12"
            :key="i"
            height="auto"
            ratio="3/4"
          />
        </div>

        <!-- 网格 -->
        <div v-else-if="films.length > 0" class="gf-tv-grid">
          <FilmCard
            v-for="item in films"
            :key="item.mid"
            :item="item"
            :show-title-below="true"
          />
        </div>

        <!-- 空态 -->
        <BaseEmpty
          v-else-if="!loading && loaded"
          title="无符合条件的影片"
          description="试试切换其他筛选条件"
        />

        <!-- 翻页 -->
        <div
          v-if="!loading && page.total > 0 && tvPageCount > 1"
          class="gf-cs-tv__pager"
        >
          <button
            type="button"
            class="gf-tv-btn"
            :disabled="tvCurrent <= 1"
            data-focusable="true"
            tabindex="0"
            @click="tvGoPage(-1)"
          >
            上一页
          </button>
          <span class="gf-cs-tv__pager-info">{{ tvCurrent }} / {{ tvPageCount }}</span>
          <button
            type="button"
            class="gf-tv-btn cyan"
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

  <!-- ===================== 桌面 / 移动分支 (原样保留) ===================== -->
  <div v-else class="gf-classify-search container-page py-[var(--gf-space-6)]">
    <!-- 顶部 title 切换：当前页选中"XX 库" -->
    <header
      v-if="titleName"
      class="gf-classify__title flex items-center gap-[var(--gf-space-3)] mb-[var(--gf-space-6)]"
    >
      <RouterLink
        :to="{ path: '/filmClassify', query: { Pid: pidStr } }"
        class="gf-classify__title-link"
        data-focusable="true"
        tabindex="0"
      >
        {{ titleName }}
      </RouterLink>
      <span class="gf-classify__title-divider" aria-hidden="true">|</span>
      <RouterLink
        :to="{ path: '/filmClassifySearch', query: { Pid: pidStr } }"
        class="gf-classify__title-active"
        data-focusable="true"
        tabindex="0"
      >
        {{ titleName }}库
      </RouterLink>
    </header>

    <!-- 错误态 -->
    <BaseEmpty
      v-if="!loading && errorMsg"
      :title="errorMsg"
      description="请检查链接中的 Pid 参数"
    />

    <template v-else>
      <!-- 筛选栏 (sticky 吸顶, bilibili 风格保持滚动时可见) -->
      <div
        v-if="filterGroups.length > 0"
        class="gf-classify-search__filter-wrap"
      >
        <FilmFilterBar
          :groups="filterGroups"
          @change="onFilterChange"
        />
      </div>

      <!-- 结果统计 -->
      <div
        v-if="page.total > 0"
        class="gf-classify-search__count flex items-baseline justify-between mb-[var(--gf-space-4)] mt-[var(--gf-space-5)]"
      >
        <p class="text-secondary text-[var(--gf-fs-sm)]">
          共 <strong class="text-primary">{{ page.total }}</strong> 部影片
        </p>
        <span class="text-muted text-[var(--gf-fs-xs)]">
          第 {{ page.current || 1 }} / {{ Math.max(1, page.pageCount || 1) }} 页
        </span>
      </div>

      <!-- 骨架 -->
      <div
        v-if="loading && films.length === 0"
        class="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-[var(--gf-space-4)]"
      >
        <BaseSkeleton
          v-for="i in 12"
          :key="i"
          height="auto"
          ratio="3/4"
        />
      </div>

      <!-- 网格列表 -->
      <FilmGrid v-else-if="films.length > 0" :items="films" />

      <!-- 分页 -->
      <div
        v-if="!loading && page.total > 0"
        class="mt-[var(--gf-space-8)] flex justify-center"
      >
        <BasePagination
          :current="page.current || 1"
          :page-size="page.size || 49"
          :total="page.total"
          @change="onPaginate"
        />
      </div>

      <!-- 空状态 -->
      <BaseEmpty
        v-if="!loading && loaded && films.length === 0 && !errorMsg"
        title="无符合条件的影片"
        description="试试切换其他筛选条件"
      />
    </template>
  </div>
</template>

<style scoped>
.gf-classify__title-active,
.gf-classify__title-link {
  text-decoration: none;
  font-size: var(--gf-fs-2xl);
  font-weight: var(--gf-fw-bold);
  outline: none;
  border-radius: var(--gf-radius-sm);
  transition: opacity var(--gf-dur-fast) var(--gf-ease-standard);
}

.gf-classify__title-active {
  background-image: var(--gf-brand-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
}

.gf-classify__title-link {
  color: var(--gf-text-secondary);
}
.gf-classify__title-link:hover,
.gf-classify__title-link:focus-visible {
  color: var(--gf-text-primary);
}
.gf-classify__title-active:focus-visible,
.gf-classify__title-link:focus-visible {
  box-shadow: var(--gf-shadow-focus-ring);
}

.gf-classify__title-divider {
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-lg);
}

@media (max-width: 767px) {
  .gf-classify__title-active,
  .gf-classify__title-link {
    font-size: var(--gf-fs-xl);
  }
}

/* 筛选栏: mobile 不 sticky (避免占满半屏), md+ 才吸顶 */
.gf-classify-search__filter-wrap {
  background-color: rgba(11, 11, 15, 0.92);
  margin-inline: calc(-1 * var(--gf-gutter-mobile));
  padding: var(--gf-space-2) var(--gf-gutter-mobile);
  border-bottom: 1px solid var(--gf-border-subtle);
}
@media (min-width: 768px) {
  .gf-classify-search__filter-wrap {
    /* 取消 sticky 吸顶: 滚动后固定在顶部太占位置, 改为随页面滚走 */
    padding: var(--gf-space-3) var(--gf-gutter-tablet);
    margin-inline: calc(-1 * var(--gf-gutter-tablet));
  }
}

@media (min-width: 768px) {
  .gf-classify-search__filter-wrap {
    margin-inline: calc(-1 * var(--gf-gutter-tablet));
    padding-inline: var(--gf-gutter-tablet);
  }
}

@media (min-width: 1024px) {
  .gf-classify-search__filter-wrap {
    margin-inline: calc(-1 * var(--gf-gutter-desktop));
    padding-inline: var(--gf-gutter-desktop);
  }
}

/* ===================== TV 分支专属样式 ===================== */
.gf-cs-tv__head {
  margin-bottom: var(--gf-space-3);
}
.gf-cs-tv__title-link,
.gf-cs-tv__title-active {
  text-decoration: none;
  /* 对齐分类页(ClassifyView)标题: 2xl + bold(原 black 偏重显大) */
  font-size: var(--gf-fs-2xl);
  font-weight: var(--gf-fw-bold);
  outline: none;
  border-radius: var(--gf-radius-sm);
  padding: 2px 4px;
}
.gf-cs-tv__title-link {
  color: var(--gf-text-secondary);
}
.gf-cs-tv__title-active {
  background-image: var(--gf-brand-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
}
.gf-cs-tv__title-divider {
  margin: 0 var(--gf-space-3);
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-lg);
}
.gf-cs-tv__count {
  margin-left: var(--gf-space-4);
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-sm);
}
.gf-cs-tv__count strong {
  color: var(--gf-brand-cyan);
}

/* 左右分屏: 左固定宽筛选栏 + 右弹性结果网格 */
.gf-cs-tv__split {
  display: grid;
  /* 左筛选占 2 列宽 / 右影片占 4 列(用户指定 6 等分: 2 + 4) */
  grid-template-columns: 2fr 4fr;
  gap: var(--gf-space-6);
  align-items: start;
}

/* 左: 玻璃面板 + 分区 */
.gf-cs-tv__filter {
  border-radius: var(--gf-radius-xl, 20px);
  border: 1px solid var(--gf-tv-stroke, rgba(255, 255, 255, 0.13));
  background: var(--gf-tv-glass, rgba(21, 21, 28, 0.88));
  padding: var(--gf-space-4) var(--gf-space-4) var(--gf-space-5);
}
.gf-cs-tv__grp {
  padding: var(--gf-space-3) 0;
}
.gf-cs-tv__grp + .gf-cs-tv__grp {
  border-top: 1px solid var(--gf-tv-stroke, rgba(255, 255, 255, 0.13));
}
.gf-cs-tv__grp-title {
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-semibold);
  color: var(--gf-text-muted);
  margin-bottom: var(--gf-space-2);
}
.gf-cs-tv__grp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--gf-space-2);
}
/* 竖栏空间窄, chip 紧凑些 */
.gf-cs-tv__chip {
  height: 40px;
  padding: 0 16px;
  font-size: var(--gf-fs-sm);
}

/* 右: 结果主区 — 影片固定 4 列(用户指定) */
.gf-cs-tv__main {
  min-width: 0;
}
.gf-cs-tv__main .gf-tv-grid {
  grid-template-columns: repeat(4, 1fr);
}

/* 翻页 */
.gf-cs-tv__pager {
  margin-top: var(--gf-space-8);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--gf-space-4);
}
.gf-cs-tv__pager-info {
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-base);
  font-weight: var(--gf-fw-semibold);
  min-width: 72px;
  text-align: center;
}
.gf-cs-tv__pager .gf-tv-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>

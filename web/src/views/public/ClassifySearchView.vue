<script setup lang="ts">
import { computed, ref } from 'vue'
import * as filmApi from '@/api/film'
import type { Card, FilterTag, Filters } from '@/types/film'
import type { PageMeta } from '@/types/api'
import type { FilterGroup } from '@/components/film/FilmFilterBar.vue'
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
  // 排序维度无"全部"态: 默认"热度优先"(hot)。老链接/API 直调无 Sort 时由后端 defaultSort 兜底(同为 hot)。
  Sort: 'hot',
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
    // 成功即清错误态: 否则上一次失败(或被取代请求误判)留下的错误提示会一直挂着,
    // 表现为"点了筛选一直显示加载失败, 只能刷新恢复"(v-if="!loading && errorMsg" 优先于列表)。
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
        // 首位放"全部"(value='') — 点击即取消该维度筛选; 再点已选中项同样取消。
        // 排序维度例外: 无"全部"态(默认热度, 见 initial), 只渲染后端 Sort 选项。
        options: [
          ...(key === 'Sort' ? [] : [{ value: '', label: '全部' }]),
          ...tagList.map((t) => ({
            value: String(t.value ?? ''),
            label: String(t.name ?? '')
          }))
        ],
        current: String(params.value[key] ?? '')
      }
    })
    .filter((g): g is FilterGroup => g !== null)
})

function onFilterChange(payload: { key: string; value: string | number }): void {
  const allowed = ['Category', 'Plot', 'Area', 'Language', 'Year', 'Sort']
  if (!allowed.includes(payload.key)) return
  // 再点已选中的选项 = 取消该筛选(回"全部")。排序维度无"全部"态: 再点同项保持选中(不取消)
  const isSame = String(params.value[payload.key] ?? '') === String(payload.value)
  const next: Partial<QueryShape> = {
    [payload.key]: isSame && payload.key !== 'Sort' ? '' : String(payload.value)
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

/** TV 页头"第 X / Y 页"计数; 翻页边界保护已随 W2a 收编进 TvResultBoard(越界不上抛) */
const tvPageCount = computed(() => Math.max(1, page.value.pageCount || 1))
const tvCurrent = computed(() => page.value.current || 1)
</script>

<template>
  <!-- ===================== TV 分支 (雷鸟卡片式: 左右分屏, W2a 收编进 TvResultBoard) ===================== -->
  <TvResultBoard
    v-if="isTV"
    class="jc-cs-tv container-page py-[var(--jc-space-6)]"
    :loading="loading"
    :loaded="loaded"
    :error-msg="errorMsg"
    error-description="请检查链接中的 Pid 参数"
    :films="films"
    :page="page"
    :filter-groups="filterGroups"
    empty-title="无符合条件的影片"
    @filter-change="onFilterChange"
    @paginate="onPaginate"
  >
    <!-- 标题切换: 电视剧(分类页) | 电视剧库(当前筛选页, 渐变高亮) + 结果统计
         (§C8#5 收编 TvSectionHeader, 异形头走默认槽自绘; 页头样式留在本文件的理由不变 —— scoped 命中插槽内容) -->
    <template #head>
      <TvSectionHeader v-if="titleName" class="jc-cs-tv__head">
        <RouterLink
          :to="{ path: '/filmClassify', query: { Pid: pidStr } }"
          class="jc-cs-tv__title-link"
          data-focusable="true"
          tabindex="0"
        >
          {{ titleName }}
        </RouterLink>
        <span class="jc-cs-tv__title-divider" aria-hidden="true">|</span>
        <RouterLink
          :to="{ path: '/filmClassifySearch', query: { Pid: pidStr } }"
          class="jc-cs-tv__title-active"
          data-focusable="true"
          tabindex="0"
        >
          {{ titleName }}库
        </RouterLink>
        <span v-if="page.total > 0" class="jc-cs-tv__count">
          共 <strong>{{ page.total }}</strong> 部影片 · 第 {{ tvCurrent }} / {{ tvPageCount }} 页
        </span>
      </TvSectionHeader>
    </template>
  </TvResultBoard>

  <!-- ===================== 桌面 / 移动分支 (W2b 收编进 FilmResultSection) ===================== -->
  <div v-else class="jc-classify-search container-page py-[var(--jc-space-6)]">
    <!-- 顶部 title 切换：当前页选中"XX 库" -->
    <header
      v-if="titleName"
      class="jc-classify__title flex items-center gap-[var(--jc-space-3)] mb-[var(--jc-space-6)]"
    >
      <RouterLink
        :to="{ path: '/filmClassify', query: { Pid: pidStr } }"
        class="jc-classify__title-link"
        data-focusable="true"
        tabindex="0"
      >
        {{ titleName }}
      </RouterLink>
      <span class="jc-classify__title-divider" aria-hidden="true">|</span>
      <RouterLink
        :to="{ path: '/filmClassifySearch', query: { Pid: pidStr } }"
        class="jc-classify__title-active"
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
      <FilmResultSection
        :loading="loading"
        :loaded="loaded"
        :error-msg="errorMsg"
        :films="films"
        :page="page"
        :filter-groups="filterGroups"
        empty-title="无符合条件的影片"
        @filter-change="onFilterChange"
        @paginate="onPaginate"
      />
    </template>
  </div>
</template>

<style scoped>
.jc-classify__title-active,
.jc-classify__title-link {
  text-decoration: none;
  font-size: var(--jc-fs-2xl);
  font-weight: var(--jc-fw-bold);
  outline: none;
  border-radius: var(--jc-radius-sm);
  transition: opacity var(--jc-dur-fast) var(--jc-ease-standard);
}

.jc-classify__title-active {
  background-image: var(--jc-brand-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
}

.jc-classify__title-link {
  color: var(--jc-text-secondary);
}
.jc-classify__title-link:hover,
.jc-classify__title-link:focus-visible {
  color: var(--jc-text-primary);
}
.jc-classify__title-active:focus-visible,
.jc-classify__title-link:focus-visible {
  box-shadow: var(--jc-shadow-focus-ring);
}

.jc-classify__title-divider {
  color: var(--jc-text-muted);
  font-size: var(--jc-fs-lg);
}

@media (max-width: 767px) {
  .jc-classify__title-active,
  .jc-classify__title-link {
    font-size: var(--jc-fs-xl);
  }
}

/* ===================== TV 分支专属样式 =====================
   (筛选栏吸边/分屏/竖栏 chip/翻页样式已随 W2a/W2b 分别收编进
   TvResultBoard / FilmResultSection; 这里只剩页头 —— 头部 DOM 由本视图
   经 #head 插槽提供, scoped 命中插槽内容, 故页头样式必须留在本文件) */
.jc-cs-tv__head {
  margin-bottom: var(--jc-space-3);
}
.jc-cs-tv__title-link,
.jc-cs-tv__title-active {
  text-decoration: none;
  /* 与分类页(ClassifyView)页头同级 —— 那边标题走 TvSectionHeader 的 .t = var(--jc-fs-base),
     这里必须显式给同一个 token, 否则会回到 2xl(36) 比分类页大一大截(用户反馈过不一致) */
  font-size: var(--jc-fs-base);
  font-weight: var(--jc-fw-bold);
  outline: none;
  border-radius: var(--jc-radius-sm);
  padding: 2px 4px;
}
.jc-cs-tv__title-link {
  color: var(--jc-text-secondary);
}
.jc-cs-tv__title-active {
  background-image: var(--jc-brand-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
}
.jc-cs-tv__title-divider {
  margin: 0 var(--jc-space-3);
  color: var(--jc-text-muted);
  font-size: var(--jc-fs-base);
}
.jc-cs-tv__count {
  margin-left: var(--jc-space-4);
  color: var(--jc-text-muted);
  font-size: var(--jc-fs-xs);
}
.jc-cs-tv__count strong {
  color: var(--jc-brand-cyan);
}
</style>

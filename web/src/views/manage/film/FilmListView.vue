<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { manageApi } from '@/api'
import type { FilmClass, ManageFilmRow, ManageFilmStatus } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import type { Column } from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageTabs from '@/components/manage/ManageTabs.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import ManageCascader from '@/components/manage/ManageCascader.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import { toast } from '@/api/http'
import { usePagedList } from '@/composables/usePagedList'
import { useAsyncAction } from '@/composables/useAsyncAction'
import { formatDateTime } from '@/utils/format'
import { FILM_STATUS_TABS, FILM_STATUS_TAG_MAP } from '@/views/manage/constants'

const router = useRouter()
// ---- 列表骨架: usePagedList 收敛 load/search/翻页/末页回夹/竞态守卫/loading/loadError ----
const params = reactive({
  name: '',
  pid: 0,
  cid: 0
})
const status = ref<ManageFilmStatus>('active')
// 排序: 与 server allowedSort 白名单对齐(hot/latest/update_stamp/score), 默认热度
interface SortOption { value: string; label: string; icon: string }
const SORT_OPTIONS: SortOption[] = [
  { value: 'hot', label: '热度', icon: 'fire' },
  { value: 'latest', label: '最新上映', icon: 'film' },
  { value: 'update_stamp', label: '最近更新', icon: 'refresh' },
  { value: 'score', label: '评分', icon: 'star' }
]
const sort = ref<string>('hot')
const sortOpen = ref(false)
const sortRef = ref<HTMLElement | null>(null)
const currentSort = computed<SortOption>(
  () => SORT_OPTIONS.find((o) => o.value === sort.value) ?? SORT_OPTIONS[0]!
)

// 分类多选: id → checked; 点击确定后才应用筛选, pending 为中间态
const selectedCatIds = ref<Set<number>>(new Set())
const appliedCatIds = ref<Set<number>>(new Set())

const paged = usePagedList<ManageFilmRow>({
  fetch: (page, size) =>
    manageApi.film
      .searchList({
        keyword: params.name,
        pid: params.pid,
        cid: params.cid,
        // 分类级联多选: appliedCatIds 才是生效态(pending 勾选不算), 空集不传(拦截器会清理 undefined)
        catIds: appliedCatIds.value.size ? [...appliedCatIds.value].join(',') : undefined,
        status: status.value,
        sort: sort.value,
        page,
        size
      })
      .then((r) => ({ list: r.list ?? [], total: r.page?.total ?? 0 }))
})

// ---- 写操作: useAsyncAction 收敛 busy+confirm+toast+reload(请求 silent, 文案 run 单点) ----
const { busy, run } = useAsyncAction()

const columns: Column<ManageFilmRow>[] = [
  { key: 'cover', label: '海报', width: '90px' },
  { key: 'name', label: '名称' },
  { key: 'cName', label: '分类', width: '120px' },
  { key: 'year', label: '年份', width: '80px' },
  { key: 'area', label: '地区', width: '100px' },
  { key: 'dbScore', label: '评分', width: '70px' },
  { key: 'remarks', label: '备注', width: '140px' },
  { key: 'deletedAt', label: '状态', width: '120px' }
]

// ---- 分类树(筛选弹层) ----
const categoryTree = ref<FilmClass[]>([])
const treeLoading = ref(false)
const treeError = ref('')
const treePopperOpen = ref(false)
const catFilterRef = ref<HTMLElement | null>(null)

async function loadTree(force = false): Promise<void> {
  if (!force && (categoryTree.value.length || treeLoading.value)) return
  treeLoading.value = true
  treeError.value = ''
  try {
    categoryTree.value = await manageApi.film.classTree()
  } catch (e) {
    // 失败保留错误态: 弹层里显示错误文案+重试, 不冒充"暂无分类"
    treeError.value = (e as Error).message ?? '加载失败'
  } finally {
    treeLoading.value = false
  }
}

function retryTree(): void {
  void loadTree(true)
}

function applyCatFilter(): void {
  // 关键字与分类筛选互斥(server 端有关键字即走 FULLTEXT 检索, 分类条件会被丢弃):
  // 应用分类筛选时清空关键词, 两边都不会"看起来生效实则被忽略"。
  if (params.name.trim()) {
    params.name = ''
    toast('info', '分类筛选与关键词搜索互斥，已清空关键词')
  }
  // 用 pending 态把"当前勾选"同步为"已应用", 并重新加载
  appliedCatIds.value = new Set(selectedCatIds.value)
  paged.search()
}

function clearCatFilter(): void {
  selectedCatIds.value = new Set()
  appliedCatIds.value = new Set()
  paged.search()
}

function search(): void {
  // 关键字与分类筛选互斥(同 applyCatFilter): 关键词搜索时清掉分类筛选,
  // 否则工具栏徽标仍显示"筛选生效中", 实际结果已混入其他分类。
  if (params.name.trim() && appliedCatIds.value.size) {
    selectedCatIds.value = new Set()
    appliedCatIds.value = new Set()
    toast('info', '关键词搜索不区分分类，已清除分类筛选')
  }
  paged.search()
}

function switchStatus(next: ManageFilmStatus): void {
  if (status.value === next) return
  status.value = next
  paged.search()
}

function setSort(next: string | number): void {
  const v = String(next)
  if (sort.value === v) return
  sort.value = v
  paged.search()
}

/** 热度下拉菜单选择排序项(与分类筛选同款按钮 + 菜单) */
function pickSort(v: string): void {
  sortOpen.value = false
  setSort(v)
}

function viewDetail(row: ManageFilmRow): void {
  const idStr = String(row.mid ?? '').trim()
  if (!idStr) return
  router.push({ path: '/manage/film/detail', query: { id: idStr } })
}

function isDeleted(row: ManageFilmRow): boolean {
  return (row.deletedAt ?? 0) > 0
}

function removeFilm(row: ManageFilmRow): void {
  void run({
    confirm: {
      title: `删除《${row.name}》？`,
      desc: '影片会从站内所有列表与详情页隐藏，但不会真正删除，可在「回收站」页签恢复。',
      okText: '删除',
      danger: true
    },
    action: () => manageApi.film.softDelete(row.mid, { silent: true }),
    successMsg: '已移入回收站',
    after: () => paged.load()
  })
}

function restoreFilm(row: ManageFilmRow): void {
  void run({
    confirm: {
      title: `恢复《${row.name}》？`,
      desc: '恢复后影片会重新出现在公开列表与详情页。',
      okText: '恢复'
    },
    action: () => manageApi.film.restore(row.mid, { silent: true }),
    successMsg: '已恢复',
    after: () => paged.load()
  })
}

// 分类筛选弹层 / 热度菜单: 点击外部 / ESC 关闭(此前只能靠再点按钮或「确定」, 弹层会驻留遮挡)
function closeCatPopper(): void {
  treePopperOpen.value = false
}
function closeSortMenu(): void {
  sortOpen.value = false
}
onClickOutside(catFilterRef, closeCatPopper)
onClickOutside(sortRef, closeSortMenu)
useEventListener(window, 'keydown', (e: KeyboardEvent) => {
  if (e.key === 'Escape') {
    if (treePopperOpen.value) closeCatPopper()
    if (sortOpen.value) closeSortMenu()
  }
})

onMounted(() => {
  void loadTree()
})
</script>

<template>
  <!-- 页面根只提供纵向 flex + 间距; 表格高度链由 ManageLayout .jc-manage-main(表格线 T0)贯通 -->
  <div class="flex min-h-0 flex-col gap-[var(--jc-space-4)]">
    <!-- 加载失败且无旧数据可显示: 明确错误态+重试, 不冒充"未找到影片" -->
    <section
      v-if="paged.loadError.value && !paged.rows.value.length"
      class="bg-surface rounded-card shadow-card"
    >
      <div class="flex flex-col items-center gap-[var(--jc-space-3)] py-[var(--jc-space-10)]">
        <BaseEmpty title="加载失败" :description="paged.loadError.value" hide-icon />
        <BaseButton variant="outline" size="sm" @click="paged.load()">
          <BaseIcon name="refresh" size="16px" /> 重试
        </BaseButton>
      </div>
    </section>

    <ManageTable
      v-else
      :columns="columns"
      :rows="paged.rows.value"
      row-key="mid"
      :loading="paged.loading.value"
      :total="paged.total.value"
      v-model:current="paged.current.value"
      :page-size="paged.pageSize.value"
      empty="未找到影片"
      actions-width="160px"
    >
      <template #toolbar>
        <div class="flex items-center gap-[var(--jc-space-3)] flex-wrap">
          <h2 class="text-lg font-[var(--jc-fw-semibold)]">影片管理</h2>
          <ManageTabs :model-value="status" :tabs="FILM_STATUS_TABS" @update:model-value="switchStatus" />
          <!-- 分类筛选(级联选择器): 紧随页签, 放左侧 -->
          <div ref="catFilterRef" class="relative">
            <BaseButton
              variant="ghost"
              size="sm"
              :class="{ 'border-brand-purple/50': appliedCatIds.size > 0 }"
              @click="void loadTree().then(() => (treePopperOpen = !treePopperOpen))"
            >
              <BaseIcon name="menu" size="16px" class="mr-1" />
              分类筛选
              <BaseTag v-if="appliedCatIds.size > 0" variant="purple" size="xs" class="ml-1">
                {{ appliedCatIds.size }}
              </BaseTag>
            </BaseButton>
            <!-- 下拉级联面板(多列逐级展开) -->
            <div
              v-if="treePopperOpen"
              class="absolute z-50 top-full left-0 mt-1 bg-surface border border-subtle rounded-lg shadow-lg p-3 min-w-[260px] max-h-[420px] overflow-auto"
              @click.stop
            >
              <div v-if="treeLoading" class="text-sm text-muted py-2 text-center">加载中…</div>
              <div v-else-if="treeError" class="flex flex-col items-center gap-2 py-2">
                <span class="text-sm text-[var(--jc-danger)]">分类树加载失败：{{ treeError }}</span>
                <BaseButton variant="outline" size="sm" @click="retryTree">重试</BaseButton>
              </div>
              <template v-else-if="categoryTree.length">
                <ManageCascader
                  :nodes="categoryTree"
                  :selected="selectedCatIds"
                  @change="selectedCatIds = $event"
                />
              </template>
              <div v-else class="text-sm text-muted py-2 text-center">暂无分类</div>
              <div class="border-t border-subtle mt-2 pt-2 flex justify-end gap-2">
                <BaseButton variant="ghost" size="sm" @click="clearCatFilter">清除筛选</BaseButton>
                <BaseButton
                  variant="gradient"
                  size="sm"
                  :disabled="!selectedCatIds.size"
                  @click="applyCatFilter(); treePopperOpen = false"
                >确定</BaseButton>
              </div>
            </div>
          </div>
          <!-- 热度(排序): 与分类筛选同款按钮 + 下拉菜单; server allowedSort 白名单 -->
          <div ref="sortRef" class="relative">
            <BaseButton variant="ghost" size="sm" aria-label="排序方式" @click="sortOpen = !sortOpen">
              <BaseIcon :name="currentSort.icon" size="16px" class="mr-1" />
              {{ currentSort.label }}
              <BaseIcon name="chevron-down" size="14px" class="ml-1" />
            </BaseButton>
            <div
              v-if="sortOpen"
              class="absolute z-50 top-full left-0 mt-1 bg-surface border border-subtle rounded-lg shadow-lg py-1 min-w-[140px]"
              @click.stop
            >
              <button
                v-for="opt in SORT_OPTIONS"
                :key="opt.value"
                type="button"
                class="w-full text-left px-3 py-1.5 text-sm flex items-center gap-2 hover:bg-[rgba(255,255,255,0.06)]"
                :class="{ 'text-brand-purple': sort === opt.value }"
                @click="pickSort(opt.value)"
              >
                <BaseIcon :name="opt.icon" size="14px" /> {{ opt.label }}
              </button>
            </div>
          </div>
        </div>
        <!-- 右侧: 搜索 + 新增 -->
        <div class="flex gap-[var(--jc-space-2)] flex-wrap items-center">
          <div class="flex gap-[var(--jc-space-2)] flex-1 min-w-0">
            <ManageInput v-model="params.name" placeholder="影片名关键字" @keydown.enter="search" />
            <BaseButton variant="gradient" size="sm" @click="search">
              <BaseIcon name="search" size="16px" /> 搜索
            </BaseButton>
            <BaseButton variant="ghost" size="sm" @click="router.push('/manage/film/add')">
              <BaseIcon name="plus" size="16px" /> 新增
            </BaseButton>
          </div>
        </div>
      </template>

      <template #mobile-card="{ row }">
        <div
          class="flex items-center gap-[var(--jc-space-3)] cursor-pointer select-none"
          data-focusable="true"
          tabindex="0"
          role="button"
          :aria-label="`查看 ${row.name}`"
          @click="viewDetail(row)"
          @keydown.enter="viewDetail(row)"
        >
          <div class="w-[48px] h-[64px] shrink-0 rounded-[var(--jc-radius-md)] overflow-hidden">
            <BaseImage :src="row.cover" :alt="row.name" ratio="3/4" />
          </div>
          <div class="min-w-0 flex-1 flex flex-col gap-[var(--jc-space-1)]">
            <span class="text-primary font-[var(--jc-fw-medium)] truncate">
              {{ row.name }}
            </span>
            <span class="text-muted text-sm truncate">
              {{ [row.year, row.cName, row.remarks].filter(Boolean).join(' · ') || '—' }}
            </span>
          </div>
          <BaseTag v-if="isDeleted(row)" variant="danger" size="xs">已删</BaseTag>
          <BaseIcon name="chevron-right" size="20px" class="text-secondary shrink-0" />
        </div>
        <!-- 移动端操作行: 此前页面覆盖 mobile-card 插槽后删除/恢复入口完全不可达 -->
        <div
          class="mt-[var(--jc-space-3)] pt-[var(--jc-space-3)] border-t border-subtle flex gap-[var(--jc-space-2)] justify-end"
          @click.stop
        >
          <BaseButton variant="ghost" size="sm" @click="viewDetail(row)">查看</BaseButton>
          <BaseButton
            v-if="isDeleted(row)"
            variant="ghost"
            size="sm"
            :disabled="busy"
            @click="restoreFilm(row)"
          >恢复</BaseButton>
          <BaseButton
            v-else
            variant="ghost"
            size="sm"
            :disabled="busy"
            @click="removeFilm(row)"
          >删除</BaseButton>
        </div>
      </template>

      <template #cell="{ row, col }">
        <div v-if="col.key === 'cover'" class="w-[60px] h-[80px] rounded-[var(--jc-radius-md)] overflow-hidden">
          <BaseImage :src="row.cover" :alt="row.name" ratio="3/4" />
        </div>
        <span v-else-if="col.key === 'name'" class="font-[var(--jc-fw-medium)]">
          {{ row.name }}
        </span>
        <span
          v-else-if="col.key === 'dbScore'"
          class="tabular-nums"
          :class="row.dbScore > 0 ? 'text-[var(--jc-warning)]' : 'text-muted'"
        >
          {{ row.dbScore > 0 ? row.dbScore.toFixed(1) : '—' }}
        </span>
        <span v-else-if="col.key === 'remarks'" class="text-secondary truncate block max-w-[130px]" :title="row.remarks || ''">
          {{ row.remarks || '—' }}
        </span>
        <!-- 状态列: StatusTag 统一「状态值→文案/variant」映射 -->
        <StatusTag
          v-else-if="col.key === 'deletedAt'"
          :value="isDeleted(row) ? 'deleted' : 'active'"
          :map="FILM_STATUS_TAG_MAP"
          size="sm"
          :title="isDeleted(row) ? `删除于 ${formatDateTime(row.deletedAt ?? 0)}` : ''"
        />
        <!-- || 兜底: 0(年份)与空串(分类/地区)同样显示 —, 与 remarks/dbScore 口径一致 -->
        <span v-else>{{ row[col.key] || '—' }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--jc-space-1)] justify-end">
          <BaseButton variant="ghost" size="sm" @click="viewDetail(row)">查看</BaseButton>
          <BaseButton
            v-if="isDeleted(row)"
            variant="ghost"
            size="sm"
            :disabled="busy"
            @click="restoreFilm(row)"
          >恢复</BaseButton>
          <BaseButton
            v-else
            variant="ghost"
            size="sm"
            :disabled="busy"
            @click="removeFilm(row)"
          >删除</BaseButton>
        </div>
      </template>
    </ManageTable>
  </div>
</template>

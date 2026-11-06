<script setup lang="ts" generic="T extends Record<string, any>">
import { computed, ref, type VNodeChild } from 'vue'
import { useEventListener } from '@vueuse/core'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import ManageCheckbox from '@/components/manage/ManageCheckbox.vue'
import { useViewMode } from '@/composables/useViewMode'

export interface ColumnFilterOption {
  label: string
  value: string | number
}

export interface ColumnFilter {
  options: ColumnFilterOption[]
  /** 受控值; undefined = 未筛选; multiple 时为数组 */
  value?: string | number | Array<string | number>
  /** 多选, 默认 false (单选) */
  multiple?: boolean
}

export interface Column<U> {
  /** 列标识 (v-for key 与事件 key); 允许派生列名, 不再限定 keyof U */
  key: string
  /** 取值字段; 省略时回落 key (旧用法 `key: 'name' as const` 继续成立) */
  field?: keyof U & string
  label: string
  width?: string
  align?: 'left' | 'center' | 'right'
  /** 受控排序开关 (无 sorter 时仅 emit) */
  sortable?: boolean
  /** 客户端排序回调; 提供后点击表头自动排序 (非受控) */
  sorter?: (a: U, b: U) => number
  /** 表头筛选配置 (漏斗按钮 + 内联面板, 全受控) */
  filter?: ColumnFilter
  /** 列内联渲染; 优先级 render > #cell 插槽 > 默认文本 */
  render?: (row: U) => VNodeChild
}

/**
 * #cell 插槽收到的列类型: key 仍限定 keyof U & string (签名不变的旧契约),
 * 页面 `row[col.key]` 的既有写法继续类型安全; 派生列放宽只发生在 props 输入侧。
 */
export type TableCellCol<U> = Column<U> & { key: keyof U & string }

export interface ManageTableProps<U extends Record<string, any>> {
  columns: Column<U>[]
  rows: U[]
  rowKey: keyof U & string
  loading?: boolean
  empty?: string
  mobileVariant?: 'card' | 'scroll'
  /** 操作列宽度 (桌面端 th 列宽), 默认 180px */
  actionsWidth?: string
  /** 服务端总数; 传了且 >0 才渲染内置分页条 */
  total?: number
  /** 受控页码 (配 v-model:current 使用) */
  current?: number
  /** 页大小, 默认 20 */
  pageSize?: number
  /** 透传 BasePagination 的桌面页码数, 默认 7 */
  pagerCount?: number
  /** 桌面高度自适应, 默认 true; mobile card 模式忽略 */
  fit?: boolean
  /** fit 模式滚动区最低高度, 默认 '320px' */
  minHeight?: string
  /** 受控排序箭头显示 (无 sorter 的 sortable 列用) */
  sortState?: { key: string; order: 'asc' | 'desc' }
}

const props = withDefaults(defineProps<ManageTableProps<T>>(), {
  mobileVariant: 'card',
  actionsWidth: '180px',
  pageSize: 20,
  pagerCount: 7,
  fit: true,
  minHeight: '320px'
})

const emit = defineEmits<{
  (e: 'update:current', page: number): void
  (e: 'page-change', page: number): void
  (e: 'sort-change', s: { key: string; order: 'asc' | 'desc' | null }): void
  (e: 'filter-change', f: { key: string; value: string | number | Array<string | number> | undefined }): void
}>()

/** 内置分页条渲染条件: total 显式传入且 > 0 */
const showPagination = computed(() => props.total !== undefined && props.total > 0)

function onPagerChange(page: number): void {
  // page-change 语义同 BasePagination.change (用户点击); 越界钳制只走 update:current
  emit('page-change', page)
}

function onPagerCurrentUpdate(page: number): void {
  emit('update:current', page)
}

defineSlots<{
  cell(props: { row: T; col: TableCellCol<T>; value: unknown }): unknown
  actions(props: { row: T }): unknown
  toolbar(): unknown
  'mobile-card'(props: { row: T; index: number }): unknown
}>()

const { isMobile } = useViewMode()
const useCardMode = computed(() => isMobile.value && props.mobileVariant === 'card')

/** fit 只作用于桌面表格形态; mobile card 模式忽略 (卡片流天然不限高) */
const isFit = computed(() => props.fit && !useCardMode.value)

/** 列取值 helper: field 优先, 回落 key; 桌面与卡片分支共享 */
function cellValue(row: T, col: Column<T>): unknown {
  return row[(col.field ?? col.key) as keyof T]
}

/**
 * render 列的渲染载体: 函数式组件, 直接返回 col.render(row) 的 vnode。
 * (模板插值无法渲染 vnode, 经此转发; 桌面 td 与卡片分支共用。)
 */
function CellRender(p: { row: T; col: Column<T> }): VNodeChild {
  return p.col.render!(p.row)
}

// ---- 表头排序 ----
// sorter 列: 非受控, 内部状态驱动箭头 + computed 排序输出 (不改 props.rows);
// sortable 无 sorter 列: 点击仅 emit, 箭头由 props.sortState 受控显示。
const sortKey = ref<string | null>(null)
const sortOrder = ref<'asc' | 'desc' | null>(null)

function canSort(col: Column<T>): boolean {
  return typeof col.sorter === 'function' || col.sortable === true
}

function sortArrow(col: Column<T>): 'asc' | 'desc' | null {
  if (typeof col.sorter === 'function') {
    return sortKey.value === col.key ? sortOrder.value : null
  }
  if (props.sortState && props.sortState.key === col.key) {
    return props.sortState.order
  }
  return null
}

function onSortClick(col: Column<T>): void {
  let next: 'asc' | 'desc' | null
  if (sortKey.value !== col.key || !sortOrder.value) {
    next = 'asc'
  } else if (sortOrder.value === 'asc') {
    next = 'desc'
  } else {
    next = null
  }
  sortKey.value = next ? col.key : null
  sortOrder.value = next
  emit('sort-change', { key: col.key, order: next })
}

/** 桌面表格实际渲染行: 内部排序命中时输出副本, 否则原样返回 */
const displayRows = computed<T[]>(() => {
  const key = sortKey.value
  const order = sortOrder.value
  if (!key || !order) return props.rows
  const col = props.columns.find(c => c.key === key)
  if (!col || typeof col.sorter !== 'function') return props.rows
  const sorted = [...props.rows].sort(col.sorter)
  return order === 'desc' ? sorted.reverse() : sorted
})

// ---- 表头筛选 (全受控: 高亮/勾选态来自 col.filter.value, 组件只 emit) ----
const openFilterKey = ref<string | null>(null)
const filterWraps = new Map<string, HTMLElement>()

function setFilterWrap(key: string, el: unknown): void {
  const node = el as HTMLElement | null
  if (node) {
    filterWraps.set(key, node)
  } else {
    filterWraps.delete(key)
  }
}

function toggleFilter(col: Column<T>): void {
  openFilterKey.value = openFilterKey.value === col.key ? null : col.key
}

function closeFilter(): void {
  openFilterKey.value = null
}

// 面板关闭三通道: ESC / 点面板外 / 再点漏斗。漏斗在面板包裹层内, 不被外点误判。
useEventListener(document, 'keydown', (e: KeyboardEvent) => {
  if (e.key === 'Escape') closeFilter()
})

function onDocPointerDown(e: Event): void {
  const key = openFilterKey.value
  if (!key) return
  const wrap = filterWraps.get(key)
  const target = e.target as Node | null
  if (wrap && target && !wrap.contains(target)) closeFilter()
}
useEventListener(document, 'pointerdown', onDocPointerDown)
useEventListener(document, 'mousedown', onDocPointerDown)

function filterActiveValues(f: ColumnFilter): Array<string | number> {
  if (f.value === undefined) return []
  return Array.isArray(f.value) ? f.value : [f.value]
}

function isFilterActive(col: Column<T>): boolean {
  const f = col.filter
  if (!f || f.value === undefined) return false
  return Array.isArray(f.value) ? f.value.length > 0 : true
}

function isOptionHit(f: ColumnFilter, option: ColumnFilterOption): boolean {
  if (f.multiple) return filterActiveValues(f).some(v => v === option.value)
  return f.value === option.value
}

function onFilterOptionClick(col: Column<T>, option: ColumnFilterOption): void {
  const f = col.filter
  if (!f) return
  if (f.multiple) {
    const cur = filterActiveValues(f)
    const has = cur.some(v => v === option.value)
    const next = has ? cur.filter(v => v !== option.value) : [...cur, option.value]
    emit('filter-change', { key: col.key, value: next.length > 0 ? next : undefined })
  } else {
    const cleared = f.value === option.value
    emit('filter-change', { key: col.key, value: cleared ? undefined : option.value })
  }
}
</script>

<template>
  <section class="bg-surface rounded-card shadow-card overflow-hidden" :class="{ 'jc-table--fit': isFit }">
    <header
      v-if="$slots.toolbar"
      class="px-[var(--jc-space-5)] py-[var(--jc-space-4)] border-b border-subtle flex flex-wrap gap-[var(--jc-space-3)] items-center justify-between"
    >
      <slot name="toolbar" />
    </header>

    <div v-if="props.loading" class="p-[var(--jc-space-5)] flex flex-col gap-[var(--jc-space-3)]">
      <BaseSkeleton v-for="i in 5" :key="i" shape="rect" :height="'40px'" />
    </div>

    <div v-else-if="props.rows.length === 0" class="py-[var(--jc-space-10)]">
      <BaseEmpty :description="props.empty || '暂无数据'" />
    </div>

    <div
      v-else-if="useCardMode"
      class="jc-card-list flex flex-col gap-[var(--jc-space-3)] p-[var(--jc-space-3)]"
    >
      <article
        v-for="(row, idx) in props.rows"
        :key="String(row[props.rowKey])"
        class="bg-elevated rounded-[var(--jc-radius-md)] border border-default p-[var(--jc-space-4)] min-h-[44px]"
      >
        <slot name="mobile-card" :row="row" :index="idx">
          <header class="font-[var(--jc-fw-semibold)] text-primary mb-[var(--jc-space-2)]">
            <CellRender v-if="props.columns[0]?.render" :row="row" :col="props.columns[0]!" />
            <slot
              v-else
              name="cell"
              :row="row"
              :col="props.columns[0] as TableCellCol<T>"
              :value="cellValue(row, props.columns[0]!)"
            >
              {{ cellValue(row, props.columns[0]!) ?? '—' }}
            </slot>
          </header>
          <dl class="text-sm text-secondary space-y-[var(--jc-space-1)]">
            <div
              v-for="col in props.columns.slice(1)"
              :key="col.key"
              class="flex gap-[var(--jc-space-2)]"
            >
              <dt class="text-muted shrink-0">{{ col.label }}:</dt>
              <dd class="min-w-0 break-words">
                <CellRender v-if="col.render" :row="row" :col="col" />
                <slot v-else name="cell" :row="row" :col="col as TableCellCol<T>" :value="cellValue(row, col)">
                  {{ cellValue(row, col) ?? '—' }}
                </slot>
              </dd>
            </div>
          </dl>
          <div
            v-if="$slots.actions"
            class="mt-[var(--jc-space-3)] pt-[var(--jc-space-3)] border-t border-subtle flex gap-[var(--jc-space-2)] justify-end flex-wrap"
          >
            <slot name="actions" :row="row" />
          </div>
        </slot>
      </article>
    </div>

    <div
      v-else
      :class="isFit ? 'jc-table__scroll' : 'overflow-x-auto'"
      :style="isFit ? { minHeight: props.minHeight } : undefined"
    >
      <table class="min-w-full text-sm">
        <thead>
          <tr class="border-b border-subtle text-secondary">
            <th
              v-for="col in props.columns"
              :key="col.key"
              :style="col.width ? { width: col.width } : undefined"
              class="jc-table__th text-left font-[var(--jc-fw-semibold)] px-[var(--jc-space-4)]"
              :class="{
                'text-right': col.align === 'right',
                'text-center': col.align === 'center'
              }"
            >
              <div
                class="flex items-center gap-[var(--jc-space-1)]"
                :class="col.align === 'right' ? 'justify-end' : col.align === 'center' ? 'justify-center' : ''"
              >
                <button
                  v-if="canSort(col)"
                  type="button"
                  class="jc-hotarea jc-th-sort inline-flex items-center gap-[2px] hover:text-primary transition-colors"
                  style="--jc-hotarea-inset: 4px"
                  data-focusable="true"
                  @click="onSortClick(col)"
                >
                  <span>{{ col.label }}</span>
                  <span
                    v-if="sortArrow(col)"
                    class="jc-th-sort__arrow text-[10px] leading-none"
                    aria-hidden="true"
                    >{{ sortArrow(col) === 'asc' ? '▲' : '▼' }}</span
                  >
                </button>
                <span v-else>{{ col.label }}</span>
                <div
                  v-if="col.filter"
                  class="relative inline-flex"
                  :ref="el => setFilterWrap(col.key, el)"
                >
                  <button
                    type="button"
                    class="jc-hotarea jc-th-filter inline-flex items-center justify-center w-[var(--jc-ctrl-h)] h-[var(--jc-ctrl-h)] rounded-[var(--jc-radius-sm)] transition-colors"
                    style="--jc-hotarea-inset: 4px"
                    :class="
                      isFilterActive(col)
                        ? 'jc-th-filter--active text-[var(--jc-brand-primary)]'
                        : 'text-muted hover:text-secondary'
                    "
                    data-focusable="true"
                    :aria-label="`筛选 ${col.label}`"
                    :aria-expanded="openFilterKey === col.key"
                    @click="toggleFilter(col)"
                  >
                    <svg
                      viewBox="0 0 24 24"
                      width="14"
                      height="14"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      aria-hidden="true"
                    >
                      <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />
                    </svg>
                  </button>
                  <div
                    v-if="openFilterKey === col.key"
                    class="jc-th-filter__panel absolute left-0 top-full z-20 mt-[var(--jc-space-1)] min-w-[140px] rounded-[var(--jc-radius-md)] border border-subtle bg-surface p-[var(--jc-space-2)] shadow-card"
                  >
                    <template v-if="col.filter.multiple">
                      <!-- ManageCheckbox 统一勾选框: 原裸 input 的悬空 token(--jc-brand)随之消灭;
                           选项行样式经 attrs 透传到组件根 label -->
                      <ManageCheckbox
                        v-for="opt in col.filter.options"
                        :key="String(opt.value)"
                        class="w-full rounded-[var(--jc-radius-sm)] px-[var(--jc-space-2)] py-[var(--jc-space-1)] hover:bg-[rgba(255,255,255,0.06)]"
                        :model-value="isOptionHit(col.filter, opt)"
                        @update:model-value="onFilterOptionClick(col, opt)"
                      >
                        <span class="text-sm">{{ opt.label }}</span>
                      </ManageCheckbox>
                    </template>
                    <template v-else>
                      <button
                        v-for="opt in col.filter.options"
                        :key="String(opt.value)"
                        type="button"
                        class="block w-full text-left rounded-[var(--jc-radius-sm)] px-[var(--jc-space-2)] py-[var(--jc-space-1)] text-sm transition-colors"
                        :class="
                          isOptionHit(col.filter, opt)
                            ? 'jc-th-filter__option-btn--active bg-[rgba(255,255,255,0.08)] text-primary'
                            : 'text-secondary hover:bg-[rgba(255,255,255,0.06)]'
                        "
                        data-focusable="true"
                        @click="onFilterOptionClick(col, opt)"
                      >
                        {{ opt.label }}
                      </button>
                    </template>
                  </div>
                </div>
              </div>
            </th>
            <th
              v-if="$slots.actions"
              :style="{ width: props.actionsWidth }"
              class="jc-table__th text-right font-[var(--jc-fw-semibold)] px-[var(--jc-space-4)]"
            >
              操作
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in displayRows"
            :key="String(row[props.rowKey])"
            class="border-b border-subtle hover:bg-elevated transition-colors"
          >
            <td
              v-for="col in props.columns"
              :key="col.key"
              class="jc-table__td px-[var(--jc-space-4)] text-primary"
              :class="{
                'text-right': col.align === 'right',
                'text-center': col.align === 'center'
              }"
            >
              <CellRender v-if="col.render" :row="row" :col="col" />
              <slot v-else name="cell" :row="row" :col="col as TableCellCol<T>" :value="cellValue(row, col)">
                {{ cellValue(row, col) ?? '—' }}
              </slot>
            </td>
            <td v-if="$slots.actions" class="jc-table__td px-[var(--jc-space-4)] text-right">
              <slot name="actions" :row="row" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 内置分页条: total 显式传入且 > 0 才渲染; 组件不持有页码状态 (受控) -->
    <footer
      v-if="showPagination"
      class="jc-table__pagination border-t border-subtle px-[var(--jc-space-4)] py-[var(--jc-space-3)]"
    >
      <BasePagination
        :current="props.current ?? 1"
        :page-size="props.pageSize"
        :total="props.total ?? 0"
        :pager-count="props.pagerCount"
        @change="onPagerChange"
        @update:current="onPagerCurrentUpdate"
      />
    </footer>
  </section>
</template>

<style scoped>
/* fit 高度链: 根纵排 + 滚动区吃满剩余空间 (ManageLayout 的 .jc-manage-main 提供外层高度) */
.jc-table--fit {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.jc-table__scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

/* 桌面/TV 紧凑密度: 单元格纵向 padding 走令牌 (theme.css 定义 --jc-table-cell-py, 缺省 10px) */
.jc-table__th,
.jc-table__td {
  padding-top: var(--jc-table-cell-py, 10px);
  padding-bottom: var(--jc-table-cell-py, 10px);
}
</style>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { manageApi } from '@/api'
import type { ManageFilmRow, ManageFilmStatus } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import { confirm } from '@/composables/useConfirm'
import { toast } from '@/api/http'

/**
 * 影片管理。
 * 删除走软删：影片打上删除时间戳后从公开页面消失，但仍在库中，可在「回收站」页签恢复。
 * 不做物理删除是因为采集源随时会把同一部片再推一遍，删了早晚会被长回来。
 */

const router = useRouter()
const rows = ref<ManageFilmRow[]>([])
const loading = ref(true)
const busy = ref(false)
const total = ref(0)
const pageSize = ref(20)
const status = ref<ManageFilmStatus>('active')

const STATUS_TABS: { value: ManageFilmStatus; label: string }[] = [
  { value: 'active', label: '在架' },
  { value: 'deleted', label: '回收站' },
  { value: 'all', label: '全部' }
]

/** 后端 query 字段：keyword / pid / cid / status / page / size */
const params = reactive({
  name: '',
  pid: 0,
  cid: 0,
  current: 1,
  pageSize: 20
})

const columns = [
  { key: 'cover' as const, label: '海报', width: '90px' },
  { key: 'name' as const, label: '名称' },
  { key: 'cName' as const, label: '分类', width: '120px' },
  { key: 'year' as const, label: '年份', width: '80px' },
  { key: 'area' as const, label: '地区', width: '100px' },
  { key: 'deletedAt' as const, label: '状态', width: '120px' }
]

async function load(): Promise<void> {
  loading.value = true
  try {
    const resp = await manageApi.film.searchList({
      keyword: params.name,
      pid: params.pid,
      cid: params.cid,
      status: status.value,
      page: params.current,
      size: params.pageSize
    })
    rows.value = resp.list ?? []
    total.value = resp.page?.total ?? 0
    pageSize.value = resp.page?.size ?? params.pageSize
  } finally {
    loading.value = false
  }
}

function search(): void {
  params.current = 1
  load()
}

function switchStatus(next: ManageFilmStatus): void {
  if (status.value === next) return
  status.value = next
  params.current = 1
  load()
}

function changePage(p: number): void {
  params.current = p
  load()
}

function viewDetail(row: ManageFilmRow): void {
  const idStr = String(row.mid ?? '').trim()
  if (!idStr) {
    return
  }
  router.push({ path: '/manage/film/detail', query: { id: idStr } })
}

function isDeleted(row: ManageFilmRow): boolean {
  return (row.deletedAt ?? 0) > 0
}

function fmtTime(ms?: number): string {
  if (!ms) return '—'
  const d = new Date(ms)
  const p = (n: number): string => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

async function removeFilm(row: ManageFilmRow): Promise<void> {
  const ok = await confirm({
    title: `删除《${row.name}》？`,
    desc: '影片会从站内所有列表与详情页隐藏，但不会真正删除，可在「回收站」页签恢复。',
    okText: '删除',
    danger: true
  })
  if (!ok) return
  busy.value = true
  try {
    await manageApi.film.softDelete(row.mid)
    toast('success', '已移入回收站')
    await load()
  } catch (e) {
    toast('error', (e as Error).message ?? '删除失败')
  } finally {
    busy.value = false
  }
}

async function restoreFilm(row: ManageFilmRow): Promise<void> {
  const ok = await confirm({
    title: `恢复《${row.name}》？`,
    desc: '恢复后影片会重新出现在公开列表与详情页。',
    okText: '恢复'
  })
  if (!ok) return
  busy.value = true
  try {
    await manageApi.film.restore(row.mid)
    toast('success', '已恢复')
    await load()
  } catch (e) {
    toast('error', (e as Error).message ?? '恢复失败')
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="flex flex-col gap-[var(--gf-space-4)]">
    <ManageTable
      :columns="columns"
      :rows="rows"
      row-key="mid"
      :loading="loading"
      empty="未找到影片"
      actions-width="160px"
    >
      <template #toolbar>
        <div class="flex items-center gap-[var(--gf-space-3)] flex-wrap">
          <h2 class="text-lg font-[var(--gf-fw-semibold)]">影片管理</h2>
          <div class="flex gap-[var(--gf-space-1)]">
            <BaseButton
              v-for="t in STATUS_TABS"
              :key="t.value"
              :variant="status === t.value ? 'gradient' : 'ghost'"
              size="sm"
              @click="switchStatus(t.value)"
            >
              {{ t.label }}
            </BaseButton>
          </div>
        </div>
        <div class="flex gap-[var(--gf-space-2)] flex-wrap">
          <ManageInput v-model="params.name" placeholder="影片名关键字" @keydown.enter="search" />
          <BaseButton variant="gradient" size="sm" @click="search">
            <BaseIcon name="search" size="16px" /> 搜索
          </BaseButton>
          <BaseButton variant="ghost" size="sm" @click="router.push('/manage/film/add')">
            <BaseIcon name="plus" size="16px" /> 新增
          </BaseButton>
        </div>
      </template>

      <template #mobile-card="{ row }">
        <div
          class="flex items-center gap-[var(--gf-space-3)] cursor-pointer select-none"
          data-focusable="true"
          tabindex="0"
          role="button"
          :aria-label="`查看 ${row.name}`"
          @click="viewDetail(row)"
          @keydown.enter="viewDetail(row)"
        >
          <div class="w-[48px] h-[64px] shrink-0 rounded-[var(--gf-radius-md)] overflow-hidden">
            <BaseImage :src="row.cover" :alt="row.name" ratio="3/4" />
          </div>
          <div class="min-w-0 flex-1 flex flex-col gap-[var(--gf-space-1)]">
            <span class="text-primary font-[var(--gf-fw-medium)] truncate">
              {{ row.name }}
            </span>
            <span class="text-muted text-sm truncate">
              {{ [row.year, row.cName, row.remarks].filter(Boolean).join(' · ') || '—' }}
            </span>
          </div>
          <BaseTag v-if="isDeleted(row)" variant="danger" size="xs">已删</BaseTag>
          <BaseIcon
            name="chevron-right"
            size="20px"
            class="text-secondary shrink-0"
          />
        </div>
      </template>

      <template #cell="{ row, col }">
        <div v-if="col.key === 'cover'" class="w-[60px] h-[80px] rounded-[var(--gf-radius-md)] overflow-hidden">
          <BaseImage :src="row.cover" :alt="row.name" ratio="3/4" />
        </div>
        <span v-else-if="col.key === 'name'" class="font-[var(--gf-fw-medium)]">
          {{ row.name }}
        </span>
        <BaseTag
          v-else-if="col.key === 'deletedAt'"
          :variant="isDeleted(row) ? 'danger' : 'success'"
          size="sm"
          :title="isDeleted(row) ? `删除于 ${fmtTime(row.deletedAt)}` : ''"
        >
          {{ isDeleted(row) ? '回收站' : '在架' }}
        </BaseTag>
        <span v-else>{{ row[col.key] ?? '—' }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--gf-space-1)] justify-end">
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

    <BasePagination
      :current="params.current"
      :page-size="pageSize"
      :total="total"
      @change="changePage"
    />
  </div>
</template>

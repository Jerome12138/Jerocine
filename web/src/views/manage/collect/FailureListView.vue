<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import * as collectApi from '@/api/manage/collect'
import type { CollectFailure } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import { toast } from '@/api/http'
import { confirm } from '@/composables/useConfirm'

/**
 * 采集失败台账（页级失败补采）
 *  - 采集时某一页失败会落一条台账；失败页里的影片否则就永久丢了，事后也查不到丢的是哪页。
 *  - 增量失败（hours>0）→ 补采时把时间窗扩大后整段重扫；全量/超长范围 → 按页码精确重放。
 *  - 补采是后台异步的（逐源逐页可能要很久），点完按钮靠"刷新"看结果。
 */

const STATUS_TABS = [
  { value: 0, label: '待补采' },
  { value: 1, label: '已处理' },
  { value: -1, label: '全部' }
] as const

const rows = ref<CollectFailure[]>([])
const sourceNames = ref<Record<string, string>>({})
const loading = ref(true)
const busy = ref(false)
const status = ref<number>(0)
const page = ref(1)
const size = ref(20)
const total = ref(0)

const columns = [
  { key: 'sourceId' as const, label: '采集源', width: '160px' },
  { key: 'pageNo' as const, label: '页码', width: '80px', align: 'center' as const },
  { key: 'hours' as const, label: '采集范围', width: '120px' },
  { key: 'cause' as const, label: '失败原因' },
  { key: 'attempts' as const, label: '重复', width: '70px', align: 'center' as const },
  { key: 'status' as const, label: '状态', width: '90px', align: 'center' as const },
  { key: 'createdAt' as const, label: '失败时间', width: '170px' }
]

async function load(): Promise<void> {
  loading.value = true
  try {
    const resp = await collectApi.failures({ status: status.value, page: page.value, size: size.value })
    rows.value = Array.isArray(resp?.list) ? resp.list : []
    total.value = resp?.page?.total ?? rows.value.length
  } finally {
    loading.value = false
  }
}

async function loadSources(): Promise<void> {
  try {
    const list = await collectApi.list()
    const map: Record<string, string> = {}
    for (const s of list ?? []) map[s.id] = s.name || s.id
    sourceNames.value = map
  } catch {
    sourceNames.value = {}
  }
}

function switchStatus(v: number): void {
  if (status.value === v) return
  status.value = v
  page.value = 1
  void load()
}

function changePage(p: number): void {
  page.value = p
  void load()
}

/** 补采：ids 为空 = 全部待补采；否则只补这些。接口异步受理，这里只提示。 */
async function recover(ids?: number[]): Promise<void> {
  busy.value = true
  try {
    const r = await collectApi.recoverFailures(ids)
    toast('success', `已提交补采，当前待补采 ${r?.pending ?? '?'} 条，稍后刷新查看`)
    // 后台异步执行，给一点时间让服务端开始跑；不需要立刻刷新
    if (status.value === 1) await load()
  } catch (e) {
    toast('error', (e as Error).message ?? '补采提交失败')
  } finally {
    busy.value = false
  }
}

async function clearHandled(): Promise<void> {
  const ok = await confirm({
    title: '清理已处理记录？',
    desc: '只清理状态为「已处理」的台账（待补采的会保留），清理后不可恢复。',
    okText: '清理',
    danger: true
  })
  if (!ok) return
  busy.value = true
  try {
    const r = await collectApi.clearHandledFailures()
    toast('success', `已清理 ${r?.deleted ?? 0} 条`)
    await load()
  } catch (e) {
    toast('error', (e as Error).message ?? '清理失败')
  } finally {
    busy.value = false
  }
}

const pendingCount = ref(0)
async function loadPending(): Promise<void> {
  try {
    const resp = await collectApi.failures({ status: 0, page: 1, size: 1 })
    pendingCount.value = resp?.page?.total ?? 0
  } catch {
    pendingCount.value = 0
  }
}

const canRecoverAll = computed(() => pendingCount.value > 0)

function srcLabel(id: string): string {
  return sourceNames.value[id] ?? id
}

function rangeLabel(hours: number): string {
  if (hours <= 0) return '全量'
  return `近 ${hours} 小时`
}

function fmtTime(ms: number): string {
  if (!ms) return '—'
  const d = new Date(ms)
  const p = (n: number): string => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

onMounted(async () => {
  await Promise.all([loadSources(), loadPending()])
  await load()
})
</script>

<template>
  <div class="flex flex-col gap-[var(--gf-space-4)]">
    <ManageTable
      :columns="columns"
      :rows="rows"
      row-key="id"
      :loading="loading"
      empty="暂无失败记录 —— 采集页全部成功"
      actions-width="120px"
    >
      <template #toolbar>
        <div class="flex items-center gap-[var(--gf-space-3)] flex-wrap">
          <h2 class="text-lg font-[var(--gf-fw-semibold)]">采集失败台账</h2>
          <div class="flex gap-[var(--gf-space-1)]">
            <BaseButton
              v-for="t in STATUS_TABS"
              :key="t.value"
              :variant="status === t.value ? 'gradient' : 'ghost'"
              size="sm"
              @click="switchStatus(t.value)"
            >
              {{ t.label }}
              <template v-if="t.value === 0 && pendingCount > 0">（{{ pendingCount }}）</template>
            </BaseButton>
          </div>
        </div>
        <div class="flex gap-[var(--gf-space-2)] flex-wrap">
          <BaseButton variant="ghost" size="sm" @click="load">
            <BaseIcon name="refresh" size="16px" /> 刷新
          </BaseButton>
          <BaseButton
            variant="gradient"
            size="sm"
            :disabled="busy || !canRecoverAll"
            @click="recover()"
          >
            补采全部待处理
          </BaseButton>
          <BaseButton variant="ghost" size="sm" :disabled="busy" @click="clearHandled">
            清理已处理
          </BaseButton>
        </div>
      </template>

      <template #cell="{ row, col }">
        <span v-if="col.key === 'sourceId'" :title="row.sourceId">{{ srcLabel(row.sourceId) }}</span>
        <BaseTag v-else-if="col.key === 'status'" :variant="row.status === 1 ? 'success' : 'warning'" size="sm">
          {{ row.status === 1 ? '已处理' : '待补采' }}
        </BaseTag>
        <span v-else-if="col.key === 'hours'">{{ rangeLabel(row.hours) }}</span>
        <span v-else-if="col.key === 'cause'" class="text-muted text-xs break-all">
          {{ row.cause || '—' }}
        </span>
        <span v-else-if="col.key === 'createdAt'">{{ fmtTime(row.createdAt) }}</span>
        <span v-else>{{ row[col.key] ?? '—' }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--gf-space-1)] justify-end">
          <BaseButton
            v-if="row.status === 0"
            variant="ghost"
            size="sm"
            :disabled="busy"
            @click="recover([row.id])"
          >补采</BaseButton>
          <span v-else class="text-muted text-xs">—</span>
        </div>
      </template>
    </ManageTable>

    <BasePagination
      v-if="total > 0"
      :current="page"
      :page-size="size"
      :total="total"
      @change="changePage"
    />

    <p class="text-muted text-xs leading-relaxed">
      补采策略：增量采集（近 N 小时）失败的，补采时把时间窗扩到「N + 距失败已过小时数」整段重扫；
      全量采集失败的，按「源 + 时长 + 页码」精确重放那一页。默认每周日凌晨 4 点自动补采一次
      （见「定时任务」里的补采任务，可改时间或停用）。
    </p>
  </div>
</template>

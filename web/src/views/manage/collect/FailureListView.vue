<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import * as collectApi from '@/api/manage/collect'
import type { CollectFailure } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageTabs from '@/components/manage/ManageTabs.vue'
import type { TabItem } from '@/components/manage/ManageTabs.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import type { StatusMap } from '@/components/manage/StatusTag.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { toast } from '@/api/http'
import { usePagedList } from '@/composables/usePagedList'
import { useAsyncAction } from '@/composables/useAsyncAction'
import { formatDateTime } from '@/utils/format'

/**
 * 采集失败台账（页级失败补采）
 *  - 采集时某一页失败会落一条台账；失败页里的影片否则就永久丢了，事后也查不到丢的是哪页。
 *  - 增量失败（hours>0）→ 补采时把时间窗扩大后整段重扫；全量/超长范围 → 按页码精确重放。
 *  - 补采是后台异步的（逐源逐页可能要很久），点完按钮靠"刷新"看结果。
 *  - 列表状态经 usePagedList 收敛(loadSeq 守卫/末页回夹), 翻页走表格内置分页。
 */

// 台账状态 → StatusTag 词汇表
const FAIL_STATUS_MAP: StatusMap = {
  0: { label: '待补采', variant: 'warning' },
  1: { label: '已处理', variant: 'success' }
}

const status = ref<number>(0)
const sourceNames = ref<Record<string, string>>({})
const { busy, run } = useAsyncAction()

// 分页列表: 筛选参数(status)由闭包捕获; immediate=false → 挂载时先探测徽标再初载(保持探测先行)
const paged = usePagedList<CollectFailure>({
  fetch: (page, size) =>
    collectApi.failures({ status: status.value, page, size }).then((resp) => ({
      list: Array.isArray(resp?.list) ? resp.list : [],
      total: resp?.page?.total ?? (resp?.list?.length ?? 0)
    })),
  immediate: false
})
const { rows, total, current, pageSize, loading, loadError, search, load } = paged

const columns = [
  { key: 'sourceId' as const, label: '采集源', width: '160px' },
  { key: 'pageNo' as const, label: '页码', width: '80px', align: 'center' as const },
  { key: 'hours' as const, label: '采集范围', width: '120px' },
  { key: 'cause' as const, label: '失败原因' },
  { key: 'attempts' as const, label: '重复', width: '70px', align: 'center' as const },
  { key: 'status' as const, label: '状态', width: '90px', align: 'center' as const },
  { key: 'createdAt' as const, label: '失败时间', width: '170px' }
]

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

// 页签切状态: 回第 1 页重查(search 在第 1 页时直接 load, 否则由 current watch 触发)
function switchStatus(v: number): void {
  if (status.value === v) return
  status.value = v
  search()
}

/** 补采：ids 为空 = 全部待补采；否则只补这些。接口异步受理，这里只提示。 */
async function recover(ids?: number[]): Promise<void> {
  await run({
    action: async () => {
      // silent: 拦截器的通用"操作成功"与下面具体文案会叠两条
      const r = await collectApi.recoverFailures(ids, { silent: true })
      // 动态成功文案(带服务端回填的 pending 数) → action 内自管, run 不再传静态 successMsg
      toast('success', `已提交补采，当前待补采 ${r?.pending ?? '?'} 条，稍后刷新查看`)
      // 补采提交后徽标/按钮可用性要跟上: 立即重查 pending 数
      void loadPending()
      // 后台异步执行，给一点时间让服务端开始跑；不需要立刻刷新
      if (status.value === 1) await load()
    }
  })
}

async function clearHandled(): Promise<void> {
  await run({
    confirm: {
      title: '清理已处理记录？',
      desc: '只清理状态为「已处理」的台账（待补采的会保留），清理后不可恢复。',
      okText: '清理',
      danger: true
    },
    action: async () => {
      const r = await collectApi.clearHandledFailures({ silent: true })
      toast('success', `已清理 ${r?.deleted ?? 0} 条`)
    },
    after: async () => {
      await load()
      void loadPending()
    }
  })
}

// 待补采徽标: null = 尚未成功拿到(不冒充 0), 0 = 确认无待补采。
// 此前只在挂载时取一次且失败归 0 → 探测请求一挂, 「补采全部」被永久误禁用且无重试入口。
const pendingCount = ref<number | null>(null)
async function loadPending(): Promise<void> {
  try {
    const resp = await collectApi.failures({ status: 0, page: 1, size: 1 })
    pendingCount.value = resp?.page?.total ?? 0
  } catch {
    /* 失败保持原值(首次为 null), 不归 0 —— 宁可多给一次补采入口, 不误禁用 */
  }
}

// pendingCount=null(未知)时不因徽标缺失而禁用「补采全部」; 确认为 0 才禁
const canRecoverAll = computed(() => pendingCount.value === null || pendingCount.value > 0)

// 状态页签(segment, D① 后 pill 分支已删): 待补采带 count 徽标(未知 null 不冒充 0 → 不渲染徽标)
const statusTabs = computed<TabItem<number>[]>(() => [
  { value: 0, label: '待补采', count: pendingCount.value ?? undefined },
  { value: 1, label: '已处理' },
  { value: -1, label: '全部' }
])

function srcLabel(id: string): string {
  return sourceNames.value[id] ?? id
}

function rangeLabel(hours: number): string {
  if (hours <= 0) return '全量'
  return `近 ${hours} 小时`
}

/** 工具栏「刷新」: 列表与待补采徽标一起重查(此前只调 load, 徽标不更新) */
async function refresh(): Promise<void> {
  await Promise.all([load(), loadPending()])
}

onMounted(async () => {
  await Promise.all([loadSources(), loadPending()])
  await load()
})</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col gap-[var(--jc-space-4)]">
    <!-- 加载失败: 明确报错 + 重试, 不落"暂无失败记录"空态 -->
    <div
      v-if="loadError"
      class="flex items-center gap-[var(--jc-space-2)] bg-elevated border border-[var(--jc-danger)] rounded-[var(--jc-radius-md)] px-[var(--jc-space-4)] py-[var(--jc-space-3)]"
    >
      <BaseIcon name="info" size="16px" class="text-[var(--jc-danger)]" />
      <span class="text-sm text-[var(--jc-danger)]">加载失败: {{ loadError }}</span>
      <ManageButton variant="outline" size="sm" class="ml-auto" @click="refresh">点击重试</ManageButton>
    </div>

    <ManageTable
      :columns="columns"
      :rows="rows"
      row-key="id"
      :loading="loading"
      :total="total"
      v-model:current="current"
      :page-size="pageSize"
      empty="暂无失败记录 —— 采集页全部成功"
      actions-width="120px"
    >
      <template #toolbar>
        <div class="flex items-center gap-[var(--jc-space-3)] flex-wrap">
          <h2 class="text-lg font-[var(--jc-fw-semibold)]">采集失败台账</h2>
          <ManageTabs variant="segment" :model-value="status" :tabs="statusTabs" @update:model-value="switchStatus" />
        </div>
        <div class="flex gap-[var(--jc-space-2)] flex-wrap">
          <ManageButton variant="ghost" size="sm" @click="refresh">
            <BaseIcon name="refresh" size="16px" /> 刷新
          </ManageButton>
          <ManageButton
            variant="gradient"
            size="sm"
            :disabled="busy || !canRecoverAll"
            @click="recover()"
          >
            补采全部待处理
          </ManageButton>
          <ManageButton variant="ghost" size="sm" :disabled="busy" @click="clearHandled">
            清理已处理
          </ManageButton>
        </div>
      </template>

      <template #cell="{ row, col }">
        <span v-if="col.key === 'sourceId'" :title="row.sourceId">{{ srcLabel(row.sourceId) }}</span>
        <StatusTag v-else-if="col.key === 'status'" :value="row.status" :map="FAIL_STATUS_MAP" />
        <span v-else-if="col.key === 'hours'">{{ rangeLabel(row.hours) }}</span>
        <span v-else-if="col.key === 'cause'" class="text-muted text-xs break-all">
          {{ row.cause || '—' }}
        </span>
        <span v-else-if="col.key === 'createdAt'">{{ row.createdAt ? formatDateTime(row.createdAt) : '—' }}</span>
        <span v-else>{{ row[col.key] ?? '—' }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--jc-space-1)] justify-end">
          <ManageButton
            v-if="row.status === 0"
            variant="ghost"
            size="sm"
            :disabled="busy"
            @click="recover([row.id])"
          >补采</ManageButton>
          <span v-else class="text-muted text-xs">—</span>
        </div>
      </template>
    </ManageTable>

    <p class="text-muted text-xs leading-relaxed">
      补采策略：增量采集（近 N 小时）失败的，补采时把时间窗扩到「N + 距失败已过小时数」整段重扫；
      全量采集失败的，按「源 + 时长 + 页码」精确重放那一页。默认每周日凌晨 4 点自动补采一次
      （见「定时任务」里的补采任务，可改时间或停用）。
    </p>
  </div>
</template>

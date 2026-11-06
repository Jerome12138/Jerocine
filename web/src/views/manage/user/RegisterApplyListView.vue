<script setup lang="ts">
import { reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { RegisterApplyRow, RegisterApplyStatus } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import type { Column } from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import type { StatusMap } from '@/components/manage/StatusTag.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import ManageErrorState from '@/components/manage/ManageErrorState.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { toast } from '@/api/http'
import { confirm } from '@/composables/useConfirm'
import { openManageDialog } from '@/composables/useManageDialog'
import { useAsyncAction } from '@/composables/useAsyncAction'
import { usePagedList } from '@/composables/usePagedList'
import { formatDateTime } from '@/utils/format'
import RegisterRejectDialog from './RegisterRejectDialog.vue'
import type { RegisterRejectPayload } from './RegisterRejectDialog.vue'

/**
 * 注册审核(apply 模式): pending 列表 → 通过(自动发激活邮件) / 驳回(理由必填)。
 * 审批决策先落库再发信: 邮件失败不阻断, 响应 warning 透出 SMTP 原文, 可补发。
 * 通过后激活邮件 7 天有效, 用户侧丢了/过期可在此补发(作废旧 token 发新链接)。
 *
 * 列表骨架: usePagedList + ManageTable 内置分页; 弹窗: 驳回经 openManageDialog 函数化;
 * 写操作: useAsyncAction(warning 是 HTTP 200 业务字段, 分支 toast 留 action 内, 异常由 run 单点)。
 */

/** -1 不限 / 0 待审 / 1 已通过 / 2 已驳回 */
const params = reactive({ keyword: '', status: -1 })

const paged = usePagedList<RegisterApplyRow>({
  fetch: (page, size) =>
    manageApi.register
      .list({ keyword: params.keyword, status: params.status, page, size })
      .then((r) => ({ list: r.list ?? [], total: r.page?.total ?? 0 }))
})

const { busy, run } = useAsyncAction()

const columns: Column<RegisterApplyRow>[] = [
  { key: 'id', label: 'ID', width: '70px' },
  { key: 'userName', label: '用户名' },
  { key: 'email', label: '邮箱' },
  { key: 'reason', label: '申请理由' },
  { key: 'sourceIp', label: '来源 IP', width: '130px' },
  { key: 'status', label: '状态', width: '100px' },
  { key: 'createdAt', label: '申请时间', width: '170px' }
]

/** 状态值 → StatusTag 映射(就近页面顶部; 仅本页消费) */
const APPLY_STATUS_TAG_MAP: StatusMap = {
  0: { label: '待审', variant: 'warning' },
  1: { label: '已通过', variant: 'success' },
  2: { label: '已驳回', variant: 'danger' }
}

const STATUS_OPTIONS = [
  { label: '全部状态', value: -1 },
  { label: '待审', value: 0 },
  { label: '已通过', value: 1 },
  { label: '已驳回', value: 2 }
]

function onStatusChange(s: string | number): void {
  params.status = typeof s === 'number' ? s : Number(s)
  paged.search()
}

/** 后端可空时间列: 未激活 activatedAt 为 null(2026-10-23 起不再是 Go 零值串) */
function isActivated(row: RegisterApplyRow): boolean {
  return !!row.activatedAt
}

// ---- 通过(warning=HTTP 200 业务字段, 分支 toast 留 action 内; 异常由 run 单点) ----
async function approve(row: RegisterApplyRow): Promise<void> {
  await run({
    confirm: {
      title: `通过「${row.userName}」的注册申请？`,
      desc: '将通过后自动发送激活邮件（一次性链接，7 天有效）；邮件发送失败会在结果中提示，可稍后补发。',
      okText: '通过'
    },
    action: async () => {
      const r = await manageApi.register.approve(row.id)
      if (r.warning) toast('warning', `已通过，但激活邮件发送失败：${r.warning}（可在列表中补发）`)
      else toast('success', `已通过「${row.userName}」，激活邮件已发送`)
    },
    after: () => paged.load()
  })
}

// ---- 驳回(理由必填), 函数化弹窗 ----
async function openReject(row: RegisterApplyRow): Promise<void> {
  const payload = await openManageDialog<RegisterRejectPayload>({
    title: `驳回申请 — ${row.userName}`,
    component: RegisterRejectDialog,
    props: { target: row },
    mobileMode: 'sheet'
  })
  if (!payload) return
  paged.load()
}

// ---- 补发激活邮件(不改申请状态, 不刷新列表) ----
async function resend(row: RegisterApplyRow): Promise<void> {
  await run({
    action: async () => {
      const r = await manageApi.register.resendActivation(row.id)
      if (r.warning) toast('warning', `补发失败：${r.warning}`)
      else toast('success', `已向 ${row.email} 补发激活邮件`)
    }
  })
}
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col gap-[var(--jc-space-4)]">
    <!-- 加载失败且无旧数据可显示: 明确错误态+重试, 不冒充"暂无注册申请"(§C8#1 收口 ManageErrorState card) -->
    <ManageErrorState
      v-if="paged.loadError.value && !paged.rows.value.length"
      title="加载失败"
      :description="paged.loadError.value"
      @retry="paged.load()"
    />

    <ManageTable
      v-else
      :columns="columns"
      :rows="paged.rows.value"
      row-key="id"
      :loading="paged.loading.value"
      empty="暂无注册申请"
      actions-width="220px"
      :total="paged.total.value"
      v-model:current="paged.current.value"
      :page-size="paged.pageSize.value"
    >
      <template #toolbar>
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">注册审核</h2>
        <!-- 输入框单独给宽度: ManageInput 自身是 w-full, 直接放进 flex 会独占一行把按钮挤到下一行 -->
        <div class="flex flex-wrap items-center gap-[var(--jc-space-2)] sm:flex-nowrap">
          <div class="w-full sm:w-[220px]">
            <ManageInput v-model="params.keyword" placeholder="用户名 / 邮箱" @keydown.enter="paged.search()" />
          </div>
          <ManageButton variant="gradient" @click="paged.search()">
            <BaseIcon name="search" size="16px" /> 搜索
          </ManageButton>
          <div class="w-[140px]">
            <ManageSelect
              :model-value="params.status"
              :options="STATUS_OPTIONS"
              aria-label="状态筛选"
              @update:model-value="onStatusChange"
            />
          </div>
        </div>
        <span class="text-xs text-muted">申请制站点：通过后自动发激活邮件，用户到站设密码即注册成功</span>
      </template>

      <template #cell="{ row, col }">
        <span v-if="col.key === 'userName'" class="font-[var(--jc-fw-medium)]">{{ row.userName }}</span>
        <!-- 已驳回行附当初填写的驳回理由(用户登录可见的那份), 不用翻审核记录 -->
        <div v-else-if="col.key === 'status'" class="flex flex-col gap-[var(--jc-space-1)]">
          <StatusTag :value="row.status" :map="APPLY_STATUS_TAG_MAP" size="sm" />
          <span
            v-if="row.status === 2 && row.rejectReason"
            class="text-xs text-secondary break-all"
            :title="row.rejectReason"
          >
            理由：{{ row.rejectReason }}
          </span>
        </div>
        <span v-else-if="col.key === 'reason'" class="text-xs text-secondary">{{ row.reason || '—' }}</span>
        <span v-else-if="col.key === 'createdAt'" class="flex flex-col gap-[var(--jc-space-1)]">
          {{ formatDateTime(row.createdAt) }}
          <span v-if="row.reviewedAt" class="text-xs text-muted">审核：{{ formatDateTime(row.reviewedAt) }}</span>
        </span>
        <span v-else class="text-xs">{{ row[col.key] ?? '—' }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--jc-space-1)] justify-end">
          <template v-if="row.status === 0">
            <ManageButton variant="ghost" size="sm" :disabled="busy" @click="approve(row)">通过</ManageButton>
            <ManageButton variant="ghost" size="sm" :disabled="busy" @click="openReject(row)">驳回</ManageButton>
          </template>
          <ManageButton
            v-else-if="row.status === 1 && !isActivated(row)"
            variant="ghost"
            size="sm"
            :disabled="busy"
            @click="resend(row)"
          >
            补发激活邮件
          </ManageButton>
          <span v-else class="text-xs text-muted">—</span>
        </div>
      </template>
    </ManageTable>
  </div>
</template>

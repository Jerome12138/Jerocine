<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { RegisterApplyRow, RegisterApplyStatus } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import { confirm } from '@/composables/useConfirm'
import { toast } from '@/api/http'

/**
 * 注册审核(apply 模式): pending 列表 → 通过(自动发激活邮件) / 驳回(理由必填)。
 * 审批决策先落库再发信: 邮件失败不阻断, 响应 warning 透出 SMTP 原文, 可补发。
 * 通过后激活邮件 7 天有效, 用户侧丢了/过期可在此补发(作废旧 token 发新链接)。
 */

const rows = ref<RegisterApplyRow[]>([])
const loading = ref(true)
const busy = ref(false)
const total = ref(0)
const pageSize = ref(20)
/** -1 不限 / 0 待审 / 1 已通过 / 2 已驳回 */
const params = reactive({ status: -1, current: 1, pageSize: 20 })

const columns = [
  { key: 'id' as const, label: 'ID', width: '70px' },
  { key: 'userName' as const, label: '用户名' },
  { key: 'email' as const, label: '邮箱' },
  { key: 'reason' as const, label: '申请理由' },
  { key: 'sourceIp' as const, label: '来源 IP', width: '130px' },
  { key: 'status' as const, label: '状态', width: '100px' },
  { key: 'createdAt' as const, label: '申请时间', width: '170px' }
]

async function load(): Promise<void> {
  loading.value = true
  try {
    const resp = await manageApi.register.list({
      status: params.status,
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

function changeStatus(s: number): void {
  params.status = s
  params.current = 1
  load()
}

function changePage(p: number): void {
  params.current = p
  load()
}

function statusName(s: RegisterApplyStatus): string {
  return s === 0 ? '待审' : s === 1 ? '已通过' : '已驳回'
}

function statusVariant(s: RegisterApplyStatus): 'warning' | 'success' | 'danger' {
  return s === 0 ? 'warning' : s === 1 ? 'success' : 'danger'
}

/** Go time.Time 零值序列化为 0001-01-01T00:00:00Z: 据此判"未激活" */
function isActivated(row: RegisterApplyRow): boolean {
  return !!row.activatedAt && !row.activatedAt.startsWith('0001')
}

function fmtTime(s?: string): string {
  if (!s || s.startsWith('0001')) return '—'
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  const p = (n: number): string => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

// ---- 通过 ----
async function approve(row: RegisterApplyRow): Promise<void> {
  const ok = await confirm({
    title: `通过「${row.userName}」的注册申请？`,
    desc: '将通过后自动发送激活邮件（一次性链接，7 天有效）；邮件发送失败会在结果中提示，可稍后补发。',
    okText: '通过'
  })
  if (!ok) return
  busy.value = true
  try {
    const r = await manageApi.register.approve(row.id)
    if (r.warning) toast('warning', `已通过，但激活邮件发送失败：${r.warning}（可在列表中补发）`)
    else toast('success', `已通过「${row.userName}」，激活邮件已发送`)
    await load()
  } catch (e) {
    toast('error', (e as Error).message ?? '操作失败')
  } finally {
    busy.value = false
  }
}

// ---- 驳回(理由必填) ----
const rejectOpen = ref(false)
const rejectTarget = ref<RegisterApplyRow | null>(null)
const rejectReason = ref('')

function openReject(row: RegisterApplyRow): void {
  rejectTarget.value = row
  rejectReason.value = ''
  rejectOpen.value = true
}

async function submitReject(): Promise<void> {
  const target = rejectTarget.value
  if (!target) return
  const reason = rejectReason.value.trim()
  if (!reason) {
    toast('error', '请填写驳回理由（用户可见）')
    return
  }
  if (reason.length > 255) {
    toast('error', '驳回理由不能超过 255 字')
    return
  }
  busy.value = true
  try {
    const r = await manageApi.register.reject(target.id, reason)
    if (r.warning) toast('warning', `已驳回，但通知邮件发送失败：${r.warning}`)
    else toast('success', `已驳回「${target.userName}」`)
    rejectOpen.value = false
    await load()
  } catch (e) {
    toast('error', (e as Error).message ?? '操作失败')
  } finally {
    busy.value = false
  }
}

// ---- 补发激活邮件 ----
async function resend(row: RegisterApplyRow): Promise<void> {
  busy.value = true
  try {
    const r = await manageApi.register.resendActivation(row.id)
    if (r.warning) toast('warning', `补发失败：${r.warning}`)
    else toast('success', `已向 ${row.email} 补发激活邮件`)
  } catch (e) {
    toast('error', (e as Error).message ?? '补发失败')
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-4)]">
    <ManageTable
      :columns="columns"
      :rows="rows"
      row-key="id"
      :loading="loading"
      empty="暂无注册申请"
      actions-width="220px"
    >
      <template #toolbar>
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">注册审核</h2>
        <select
          v-model.number="params.status"
          class="bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-2)] text-sm"
          data-focusable="true"
          @change="changeStatus(params.status)"
        >
          <option :value="-1">全部状态</option>
          <option :value="0">待审</option>
          <option :value="1">已通过</option>
          <option :value="2">已驳回</option>
        </select>
        <span class="text-xs text-muted">申请制站点：通过后自动发激活邮件，用户到站设密码即注册成功</span>
      </template>

      <template #cell="{ row, col }">
        <span v-if="col.key === 'userName'" class="font-[var(--jc-fw-medium)]">{{ row.userName }}</span>
        <BaseTag v-else-if="col.key === 'status'" :variant="statusVariant(row.status)" size="sm">
          {{ statusName(row.status) }}
        </BaseTag>
        <span v-else-if="col.key === 'reason'" class="text-xs text-secondary">{{ row.reason || '—' }}</span>
        <span v-else-if="col.key === 'createdAt'">{{ fmtTime(row.createdAt) }}</span>
        <span v-else class="text-xs">{{ row[col.key] ?? '—' }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--jc-space-1)] justify-end">
          <template v-if="row.status === 0">
            <BaseButton variant="ghost" size="sm" :disabled="busy" @click="approve(row)">通过</BaseButton>
            <BaseButton variant="ghost" size="sm" :disabled="busy" @click="openReject(row)">驳回</BaseButton>
          </template>
          <BaseButton
            v-else-if="row.status === 1 && !isActivated(row)"
            variant="ghost"
            size="sm"
            :disabled="busy"
            @click="resend(row)"
          >
            补发激活邮件
          </BaseButton>
          <span v-else class="text-xs text-muted">—</span>
        </div>
      </template>
    </ManageTable>

    <BasePagination
      :current="params.current"
      :page-size="pageSize"
      :total="total"
      @change="changePage"
    />

    <ManageSheet v-model="rejectOpen" :title="`驳回申请 — ${rejectTarget?.userName ?? ''}`">
      <form class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="submitReject">
        <label class="flex flex-col gap-[var(--jc-space-2)]">
          <span class="text-sm text-secondary">驳回理由（必填，用户登录与进度查询可见，≤255 字）</span>
          <textarea
            v-model="rejectReason"
            rows="3"
            maxlength="255"
            class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-2)] text-sm resize-y"
            placeholder="例如：邀请人信息无法核实 / 请补充联系方式"
            data-focusable="true"
          />
        </label>
        <p class="text-xs text-muted">驳回后该邮箱仍可重新提交申请；通知邮件发送失败不影响驳回结果。</p>
        <div class="flex justify-end gap-[var(--jc-space-2)]">
          <BaseButton variant="ghost" size="sm" @click="rejectOpen = false">取消</BaseButton>
          <BaseButton variant="gradient" size="sm" type="submit" :disabled="busy">确认驳回</BaseButton>
        </div>
      </form>
    </ManageSheet>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { RegisterApplyRow, RegisterApplyStatus } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
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
/** 加载失败态: 与"确实没有申请"的空态区分开, 提供重试入口 */
const loadError = ref('')
// 请求序号: 仅采纳最后一次请求的响应, 防快速切筛选/翻页时旧响应后到覆盖新列表
let loadSeq = 0
/** -1 不限 / 0 待审 / 1 已通过 / 2 已驳回 */
const params = reactive({ keyword: '', status: -1, current: 1, pageSize: 20 })

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
  const seq = ++loadSeq
  loading.value = true
  loadError.value = ''
  try {
    const resp = await manageApi.register.list({
      keyword: params.keyword,
      status: params.status,
      page: params.current,
      size: params.pageSize
    })
    if (seq !== loadSeq) return // 已有更新的请求在途/完成, 丢弃旧响应
    rows.value = resp.list ?? []
    total.value = resp.page?.total ?? 0
    pageSize.value = resp.page?.size ?? params.pageSize
    // 末页删空回夹: 通过/驳回后当前页可能整体消失, 页码越界会落进空页, 回夹到最后一页重拉一次
    const size = resp.page?.size ?? params.pageSize
    const pageCount = Math.max(1, Math.ceil(total.value / size))
    if (!rows.value.length && params.current > 1 && total.value > 0 && pageCount < params.current) {
      params.current = pageCount
      return load()
    }
  } catch (e) {
    if (seq !== loadSeq) return
    // 失败保留错误态: 不冒充"暂无注册申请"; http 拦截器已全局 toast, 这里落页面内文案
    loadError.value = (e as Error).message ?? '加载失败'
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

function retryLoad(): void {
  load()
}

function search(): void {
  params.current = 1
  load()
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

/** 后端可空时间列: 未激活 activatedAt 为 null(2026-10-23 起不再是 Go 零值串) */
function isActivated(row: RegisterApplyRow): boolean {
  return !!row.activatedAt
}

function fmtTime(s?: string | null): string {
  if (!s) return '—'
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
    <!-- 加载失败且无旧数据可显示: 明确错误态+重试, 不冒充"暂无注册申请" -->
    <section v-if="loadError && !rows.length" class="bg-surface rounded-card shadow-card">
      <div class="flex flex-col items-center gap-[var(--jc-space-3)] py-[var(--jc-space-10)]">
        <BaseEmpty title="加载失败" :description="loadError" hide-icon />
        <BaseButton variant="outline" size="sm" @click="retryLoad">
          <BaseIcon name="refresh" size="16px" /> 重试
        </BaseButton>
      </div>
    </section>

    <ManageTable
      v-else
      :columns="columns"
      :rows="rows"
      row-key="id"
      :loading="loading"
      empty="暂无注册申请"
      actions-width="220px"
    >
      <template #toolbar>
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">注册审核</h2>
        <!-- 输入框单独给宽度: ManageInput 自身是 w-full, 直接放进 flex 会独占一行把按钮挤到下一行 -->
        <div class="flex flex-wrap items-center gap-[var(--jc-space-2)] sm:flex-nowrap">
          <div class="w-full sm:w-[220px]">
            <ManageInput v-model="params.keyword" placeholder="用户名 / 邮箱" @keydown.enter="search" />
          </div>
          <BaseButton variant="gradient" size="sm" @click="search">
            <BaseIcon name="search" size="16px" /> 搜索
          </BaseButton>
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
        </div>
        <span class="text-xs text-muted">申请制站点：通过后自动发激活邮件，用户到站设密码即注册成功</span>
      </template>

      <template #cell="{ row, col }">
        <span v-if="col.key === 'userName'" class="font-[var(--jc-fw-medium)]">{{ row.userName }}</span>
        <!-- 已驳回行附当初填写的驳回理由(用户登录可见的那份), 不用翻审核记录 -->
        <div v-else-if="col.key === 'status'" class="flex flex-col gap-[var(--jc-space-1)]">
          <BaseTag :variant="statusVariant(row.status)" size="sm">{{ statusName(row.status) }}</BaseTag>
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
          {{ fmtTime(row.createdAt) }}
          <span v-if="row.reviewedAt" class="text-xs text-muted">审核：{{ fmtTime(row.reviewedAt) }}</span>
        </span>
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

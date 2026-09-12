<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { ManageUserRow } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import { confirm } from '@/composables/useConfirm'
import { toast } from '@/api/http'
import { useUserStore } from '@/stores/user'

/**
 * 用户管理: 列表(用户名搜索) / 禁用启用 / 重置密码。
 * 禁用与重置密码都会让目标用户全部设备下线; 不能操作自己(后端同样校验, 防唯一管理员自锁)。
 */

const userStore = useUserStore()
const rows = ref<ManageUserRow[]>([])
const loading = ref(true)
const busy = ref(false)
const total = ref(0)
const pageSize = ref(20)

const params = reactive({ keyword: '', current: 1, pageSize: 20 })

const columns = [
  { key: 'ID' as const, label: 'ID', width: '80px' },
  { key: 'userName' as const, label: '用户名' },
  { key: 'role' as const, label: '角色', width: '100px' },
  { key: 'disabled' as const, label: '状态', width: '100px' },
  { key: 'CreatedAt' as const, label: '注册时间', width: '170px' }
]

async function load(): Promise<void> {
  loading.value = true
  try {
    const resp = await manageApi.user.list({
      keyword: params.keyword,
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

function changePage(p: number): void {
  params.current = p
  load()
}

function isSelf(row: ManageUserRow): boolean {
  return row.ID === Number(userStore.info?.id ?? 0)
}

function roleLabel(row: ManageUserRow): string {
  return row.role === 1 ? '管理员' : '普通用户'
}

function fmtTime(s?: string): string {
  if (!s) return '—'
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  const p = (n: number): string => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

async function toggleDisabled(row: ManageUserRow): Promise<void> {
  const disabling = row.disabled === 0
  const ok = await confirm({
    title: disabling ? `禁用「${row.userName}」？` : `启用「${row.userName}」？`,
    desc: disabling
      ? '禁用后该账号无法登录, 其全部已登录设备会立即下线。'
      : '启用后该账号可正常登录。',
    okText: disabling ? '禁用' : '启用',
    danger: disabling
  })
  if (!ok) return
  busy.value = true
  try {
    await manageApi.user.setDisabled(row.ID, disabling)
    toast('success', disabling ? '已禁用' : '已启用')
    await load()
  } catch (e) {
    toast('error', (e as Error).message ?? '操作失败')
  } finally {
    busy.value = false
  }
}

// ---- 重置密码弹窗 ----
const resetOpen = ref(false)
const resetTarget = ref<ManageUserRow | null>(null)
const resetPassword = ref('')

function openReset(row: ManageUserRow): void {
  resetTarget.value = row
  resetPassword.value = ''
  resetOpen.value = true
}

async function submitReset(): Promise<void> {
  const target = resetTarget.value
  if (!target) return
  if (resetPassword.value.length < 6 || resetPassword.value.length > 64) {
    toast('error', '密码长度需在 6-64 位之间')
    return
  }
  busy.value = true
  try {
    await manageApi.user.resetPassword(target.ID, resetPassword.value)
    toast('success', `已重置「${target.userName}」的密码, 该用户已全部下线`)
    resetOpen.value = false
  } catch (e) {
    toast('error', (e as Error).message ?? '重置失败')
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
      row-key="ID"
      :loading="loading"
      empty="未找到用户"
      actions-width="220px"
    >
      <template #toolbar>
        <h2 class="text-lg font-[var(--gf-fw-semibold)]">用户管理</h2>
        <div class="flex gap-[var(--gf-space-2)] flex-wrap">
          <ManageInput v-model="params.keyword" placeholder="用户名关键字" @keydown.enter="search" />
          <BaseButton variant="gradient" size="sm" @click="search">
            <BaseIcon name="search" size="16px" /> 搜索
          </BaseButton>
        </div>
      </template>

      <template #cell="{ row, col }">
        <span v-if="col.key === 'userName'" class="font-[var(--gf-fw-medium)]">
          {{ row.userName }}
          <BaseTag v-if="isSelf(row)" variant="info" size="xs" class="ml-[var(--gf-space-1)]">当前登录</BaseTag>
        </span>
        <BaseTag v-else-if="col.key === 'role'" :variant="row.role === 1 ? 'warning' : 'default'" size="sm">
          {{ roleLabel(row) }}
        </BaseTag>
        <BaseTag v-else-if="col.key === 'disabled'" :variant="row.disabled === 1 ? 'danger' : 'success'" size="sm">
          {{ row.disabled === 1 ? '已禁用' : '正常' }}
        </BaseTag>
        <span v-else-if="col.key === 'CreatedAt'">{{ fmtTime(row.CreatedAt) }}</span>
        <span v-else>{{ row[col.key] ?? '—' }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--gf-space-1)] justify-end">
          <BaseButton variant="ghost" size="sm" :disabled="busy || isSelf(row)" @click="toggleDisabled(row)">
            {{ row.disabled === 1 ? '启用' : '禁用' }}
          </BaseButton>
          <BaseButton variant="ghost" size="sm" :disabled="busy" @click="openReset(row)">重置密码</BaseButton>
        </div>
      </template>
    </ManageTable>

    <BasePagination
      :current="params.current"
      :page-size="pageSize"
      :total="total"
      @change="changePage"
    />

    <ManageSheet v-model="resetOpen" :title="`重置密码 — ${resetTarget?.userName ?? ''}`">
      <form class="flex flex-col gap-[var(--gf-space-4)]" @submit.prevent="submitReset">
        <label class="flex flex-col gap-[var(--gf-space-2)]">
          <span class="text-sm text-secondary">新密码(6-64 位, 重置后该用户全部设备下线)</span>
          <ManageInput v-model="resetPassword" type="password" placeholder="输入新密码" />
        </label>
        <div class="flex justify-end gap-[var(--gf-space-2)]">
          <BaseButton variant="ghost" size="sm" @click="resetOpen = false">取消</BaseButton>
          <BaseButton variant="gradient" size="sm" type="submit" :disabled="busy">确认重置</BaseButton>
        </div>
      </form>
    </ManageSheet>
  </div>
</template>

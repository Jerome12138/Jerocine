<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import { confirm } from '@/composables/useConfirm'
import type { LoginSessionRow } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import { formatDateTime } from '@/utils/format'

/**
 * 在线会话：当前生效的登录凭据（每个 token 一行）。
 * 数据来自登录时记录的会话元数据（用户/IP/UA/登录时间）+ 请求节流更新的最近活跃；
 * 「踢下线」只摘掉该 token，其它设备不受影响。改密/禁用/删用户会连带清掉该用户全部会话。
 */
const rows = ref<LoginSessionRow[]>([])
const loading = ref(false)
const keyword = ref('')
const revoking = ref('')

const columns = [
  { key: 'userName' as const, label: '用户' },
  { key: 'ip' as const, label: '登录 IP' },
  { key: 'ua' as const, label: '设备 / UA' },
  { key: 'loginAt' as const, label: '登录时间' },
  { key: 'lastSeen' as const, label: '最近活跃' }
]

async function load(): Promise<void> {
  loading.value = true
  try {
    const res = await manageApi.system.listSessions({
      keyword: keyword.value.trim() || undefined,
      limit: 200
    })
    rows.value = res.list
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function kick(row: LoginSessionRow): Promise<void> {
  const ok = await confirm({
    title: '踢下线',
    desc: `确定让「${row.userName}」在 ${row.ip || '未知 IP'} 的这次登录失效吗？其它设备不受影响。`,
    okText: '踢下线',
    danger: true
  })
  if (!ok) return
  revoking.value = row.id
  try {
    await manageApi.system.revokeSession(row.id)
    toast('success', '已踢下线')
    await load()
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '操作失败')
  } finally {
    revoking.value = ''
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
      empty="暂无在线会话（登录后才会出现在这里）"
      actions-width="120px"
    >
      <template #toolbar>
        <div class="flex items-baseline gap-[var(--jc-space-3)]">
          <h2 class="text-lg font-[var(--jc-fw-semibold)]">在线会话</h2>
          <span class="text-xs text-muted">共 {{ rows.length }} 条 · 最近活跃倒序</span>
        </div>
        <div class="flex items-center gap-[var(--jc-space-2)]">
          <div class="w-[220px]">
            <ManageInput v-model="keyword" placeholder="用户名 / 用户 ID / IP" />
          </div>
          <BaseButton variant="outline" size="sm" @click="load">查询</BaseButton>
          <BaseButton variant="ghost" size="sm" @click="load">刷新</BaseButton>
        </div>
      </template>

      <template #cell="{ row, col }">
        <span v-if="col.key === 'userName'" class="text-sm">
          {{ row.userName }}<span class="text-muted"> #{{ row.userId }}</span>
        </span>
        <span v-else-if="col.key === 'ip'" class="text-sm">{{ row.ip || '—' }}</span>
        <span v-else-if="col.key === 'ua'" class="block max-w-[320px] truncate text-xs text-secondary" :title="row.ua">
          {{ row.ua || '—' }}
        </span>
        <span v-else-if="col.key === 'loginAt'" class="text-xs text-secondary">
          {{ formatDateTime(row.loginAt * 1000) }}
        </span>
        <span v-else-if="col.key === 'lastSeen'" class="text-xs text-secondary">
          {{ formatDateTime(row.lastSeen * 1000) }}
        </span>
        <span v-else>{{ row[col.key] }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex justify-end">
          <BaseButton
            variant="ghost"
            size="sm"
            :disabled="revoking === row.id"
            @click="kick(row)"
          >
            踢下线
          </BaseButton>
        </div>
      </template>
    </ManageTable>
  </div>
</template>

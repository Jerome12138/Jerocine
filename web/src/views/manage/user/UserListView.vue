<script setup lang="ts">
import { reactive, computed } from 'vue'
import { manageApi } from '@/api'
import type { ManageUserRow } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import type { Column } from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import type { StatusMap } from '@/components/manage/StatusTag.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import { openManageDialog } from '@/composables/useManageDialog'
import { useAsyncAction } from '@/composables/useAsyncAction'
import { usePagedList } from '@/composables/usePagedList'
import { formatDateTime } from '@/utils/format'
import { useUserStore } from '@/stores/user'
import UserFormDialog from './UserFormDialog.vue'
import type { UserFormPayload } from './UserFormDialog.vue'
import UserResetPasswordDialog from './UserResetPasswordDialog.vue'

/**
 * 用户管理: 列表(用户名搜索) / 新增 / 编辑(用户名+角色) / 删除 / 禁用启用 / 重置密码。
 * 禁用与重置密码都会让目标用户全部设备下线; 不能操作自己(后端同样校验, 防唯一管理员自锁)。
 * 删除为硬删, 连同其观看历史/收藏/跳过设置; 改自己的角色被前后端双重禁止(防降权自锁)。
 *
 * 列表骨架: usePagedList 收敛 load/search/翻页/末页回夹/竞态守卫/loading/loadError;
 * 表格分页由 ManageTable 内置(:total + v-model:current), 页面不再拼 BasePagination。
 * 弹窗: 新增/编辑与重置密码经 openManageDialog 函数化发起(内容组件自治校验+提交)。
 */

const userStore = useUserStore()
const selfId = computed(() => Number(userStore.info?.id ?? 0))

const params = reactive({ keyword: '' })

// ---- 列表骨架: usePagedList ----
const paged = usePagedList<ManageUserRow>({
  fetch: (page, size) =>
    manageApi.user
      .list({ keyword: params.keyword, page, size })
      .then((r) => ({ list: r.list ?? [], total: r.page?.total ?? 0 }))
})

// ---- 写操作: useAsyncAction 收敛 busy+confirm+toast+reload(请求 silent, 文案 run 单点) ----
const { busy, run } = useAsyncAction()

const columns: Column<ManageUserRow>[] = [
  { key: 'ID', label: 'ID', width: '80px' },
  { key: 'userName', label: '用户名' },
  { key: 'email', label: '邮箱', width: '220px' },
  { key: 'role', label: '角色', width: '100px' },
  { key: 'disabled', label: '状态', width: '100px' },
  { key: 'CreatedAt', label: '注册时间', width: '170px' }
]

/** 状态值 → StatusTag 映射(就近页面顶部; 仅本页消费, 不提升 constants) */
const ROLE_TAG_MAP: StatusMap = {
  0: { label: '普通用户', variant: 'default' },
  1: { label: '管理员', variant: 'warning' }
}
const STATE_TAG_MAP: StatusMap = {
  0: { label: '正常', variant: 'success' },
  1: { label: '已禁用', variant: 'danger' }
}

function isSelf(row: ManageUserRow): boolean {
  return row.ID === selfId.value
}

async function toggleDisabled(row: ManageUserRow): Promise<void> {
  const disabling = row.disabled === 0
  await run({
    confirm: {
      title: disabling ? `禁用「${row.userName}」？` : `启用「${row.userName}」？`,
      desc: disabling
        ? '禁用后该账号无法登录, 其全部已登录设备会立即下线。'
        : '启用后该账号可正常登录。',
      okText: disabling ? '禁用' : '启用',
      danger: disabling
    },
    action: () => manageApi.user.setDisabled(row.ID, disabling, { silent: true }),
    successMsg: disabling ? '已禁用' : '已启用',
    after: () => paged.load()
  })
}

async function removeUser(row: ManageUserRow): Promise<void> {
  await run({
    confirm: {
      title: `删除「${row.userName}」？`,
      desc: '将永久删除该账号及其观看历史、收藏、跳过设置, 该账号全部设备立即下线, 此操作不可恢复。',
      okText: '删除',
      danger: true
    },
    action: () => manageApi.user.remove(row.ID, { silent: true }),
    successMsg: '已删除',
    after: () => paged.load()
  })
}

// ---- 新增/编辑用户弹窗(editing 为 null 表示新增), 函数化发起 ----
async function openCreate(): Promise<void> {
  const payload = await openManageDialog<UserFormPayload>({
    title: '新增用户',
    component: UserFormDialog,
    props: { selfId: selfId.value },
    mobileMode: 'sheet'
  })
  if (!payload) return
  paged.load()
}

async function openEdit(row: ManageUserRow): Promise<void> {
  const payload = await openManageDialog<UserFormPayload>({
    title: `编辑用户 — ${row.userName}`,
    component: UserFormDialog,
    props: { initial: row, selfId: selfId.value },
    mobileMode: 'sheet'
  })
  if (!payload) return
  paged.load()
}

// ---- 重置密码弹窗(函数化发起) ----
async function openReset(row: ManageUserRow): Promise<void> {
  await openManageDialog({
    title: `重置密码 — ${row.userName}`,
    component: UserResetPasswordDialog,
    props: { target: row },
    mobileMode: 'sheet'
  })
}
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col gap-[var(--jc-space-4)]">
    <!-- 加载失败且无旧数据可显示: 明确错误态+重试, 不冒充"未找到用户" -->
    <section v-if="paged.loadError.value && !paged.rows.value.length" class="bg-surface rounded-card shadow-card">
      <div class="flex flex-col items-center gap-[var(--jc-space-3)] py-[var(--jc-space-10)]">
        <BaseEmpty title="加载失败" :description="paged.loadError.value" hide-icon />
        <ManageButton variant="outline" @click="paged.load()">
          <BaseIcon name="refresh" size="16px" /> 重试
        </ManageButton>
      </div>
    </section>

    <ManageTable
      v-else
      :columns="columns"
      :rows="paged.rows.value"
      row-key="ID"
      :loading="paged.loading.value"
      empty="未找到用户"
      actions-width="300px"
      :total="paged.total.value"
      v-model:current="paged.current.value"
      :page-size="paged.pageSize.value"
    >
      <template #toolbar>
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">用户管理</h2>
        <!-- 输入框单独给宽度: ManageInput 自身是 w-full, 直接放进 flex 会独占一行把按钮挤到下一行 -->
        <div class="flex flex-wrap items-center gap-[var(--jc-space-2)] sm:flex-nowrap">
          <div class="w-full sm:w-[220px]">
            <ManageInput v-model="params.keyword" placeholder="用户名关键字" @keydown.enter="paged.search()" />
          </div>
          <ManageButton variant="gradient" @click="paged.search()">
            <BaseIcon name="search" size="16px" /> 搜索
          </ManageButton>
          <ManageButton variant="gradient" @click="openCreate">
            <BaseIcon name="plus" size="16px" /> 新增用户
          </ManageButton>
        </div>
      </template>

      <template #cell="{ row, col }">
        <span v-if="col.key === 'userName'" class="font-[var(--jc-fw-medium)]">
          {{ row.userName }}
          <BaseTag v-if="isSelf(row)" variant="info" size="xs" class="ml-[var(--jc-space-1)]">当前登录</BaseTag>
        </span>
        <span v-else-if="col.key === 'email'" class="text-secondary truncate block max-w-[210px]" :title="row.email || ''">
          {{ row.email || '—' }}
        </span>
        <StatusTag v-else-if="col.key === 'role'" :value="row.role" :map="ROLE_TAG_MAP" size="sm" />
        <StatusTag v-else-if="col.key === 'disabled'" :value="row.disabled" :map="STATE_TAG_MAP" size="sm" />
        <span v-else-if="col.key === 'CreatedAt'">{{ formatDateTime(row.CreatedAt) }}</span>
        <span v-else>{{ row[col.key] ?? '—' }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--jc-space-1)] justify-end">
          <ManageButton variant="ghost" size="sm" :disabled="busy" @click="openEdit(row)">编辑</ManageButton>
          <ManageButton variant="ghost" size="sm" :disabled="busy || isSelf(row)" @click="toggleDisabled(row)">
            {{ row.disabled === 1 ? '启用' : '禁用' }}
          </ManageButton>
          <!-- 重置密码对自己的行禁用: 重置即清目标用户全部 token, 会把自己当前会话也踢下线(服务端同样校验) -->
          <ManageButton variant="ghost" size="sm" :disabled="busy || isSelf(row)" @click="openReset(row)">重置密码</ManageButton>
          <ManageButton variant="ghost" size="sm" :disabled="busy || isSelf(row)" @click="removeUser(row)">删除</ManageButton>
        </div>
      </template>
    </ManageTable>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import { ApiError } from '@/types/api'
import type { LoginSessionRow } from '@/types/manage'
import { useUserStore } from '@/stores/user'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import { formatDateTime } from '@/utils/format'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 在线会话：当前生效的登录凭据（每个 token 一行）。
 * 数据来自登录时记录的会话元数据（用户/IP/UA/登录时间）+ 请求节流更新的最近活跃；
 * 「踢下线」只摘掉该 token，其它设备不受影响。改密/禁用/删用户会连带清掉该用户全部会话。
 *
 * 非分页列表(服务端按 limit 截断, 无 page/total), 不套 usePagedList。
 * 写操作口径: kick 走 useAsyncAction(请求已内置 silent; 404=会话已下线是业务分支,
 * action 内吞掉 + info toast, 其余异常 rethrow 由 run 单点 error toast, after 统一重载)。
 */
const userStore = useUserStore()
const rows = ref<LoginSessionRow[]>([])
const loading = ref(false)
/** 加载失败: 展示错误态 + 重试, 不冒充"暂无会话"空态 */
const loadError = ref(false)
const keyword = ref('')

const { busy, run } = useAsyncAction()

/** 常规拉取条数; 服务端对无关键字请求按此截断(只回最近 limit 条) */
const LIST_LIMIT = 200
/** 未截断时隐藏提示 */
const truncated = computed(() => rows.value.length >= LIST_LIMIT && !keyword.value.trim())

/** 是否管理员自己的当前会话(误点会把自己登出, 行内加标识 + 确认弹窗说明) */
function isSelf(row: LoginSessionRow): boolean {
  return Number(row.userId) === Number(userStore.info?.id ?? 0)
}

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
      limit: LIST_LIMIT
    })
    rows.value = res.list
    loadError.value = false
  } catch (e) {
    loadError.value = true
    toast('error', e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function kick(row: LoginSessionRow): Promise<void> {
  await run({
    confirm: {
      title: '踢下线',
      desc: isSelf(row)
        ? `这是你当前登录的会话（${row.userName}，${row.ip || '未知 IP'}），踢下线会立即登出自己。确定吗？`
        : `确定让「${row.userName}」在 ${row.ip || '未知 IP'} 的这次登录失效吗？其它设备不受影响。`,
      okText: '踢下线',
      danger: true
    },
    action: async () => {
      try {
        await manageApi.system.revokeSession(row.id)
        toast('success', '已踢下线')
      } catch (e) {
        // 会话已失效(对方登出/被别人先踢): 服务端 404 —— after 会刷新列表清掉死行,
        // 别让管理员反复点反复报错; 其余异常原样抛出交 run 单点 toast
        if (e instanceof ApiError && e.status === 404) {
          toast('info', '该会话已下线，列表已刷新')
        } else {
          throw e
        }
      }
    },
    after: () => load()
  })
}

onMounted(load)
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col gap-[var(--jc-space-4)]">
    <!-- 加载失败且无旧数据可显示: 明确错误态+重试, 不冒充"暂无会话"空态 -->
    <section v-if="loadError && !rows.length" class="bg-surface rounded-card shadow-card">
      <div class="flex flex-col items-center gap-[var(--jc-space-3)] py-[var(--jc-space-10)]">
        <BaseEmpty title="加载失败" description="在线会话读取失败，请重试" hide-icon />
        <ManageButton variant="outline" size="sm" @click="load">重试</ManageButton>
      </div>
    </section>

    <ManageTable
      v-else
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
            <ManageInput
              v-model="keyword"
              placeholder="用户名 / 用户 ID / IP"
              @keydown.enter="load"
            />
          </div>
          <ManageButton variant="outline" size="sm" @click="load">查询</ManageButton>
          <ManageButton variant="ghost" size="sm" @click="load">刷新</ManageButton>
        </div>
      </template>

      <template #cell="{ row, col }">
        <span v-if="col.key === 'userName'" class="text-sm">
          {{ row.userName }}<span class="text-muted"> #{{ row.userId }}</span>
          <BaseTag v-if="isSelf(row)" variant="info" size="xs" class="ml-[var(--jc-space-1)]">
            当前登录
          </BaseTag>
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
          <ManageButton
            variant="ghost"
            size="sm"
            :disabled="busy"
            @click="kick(row)"
          >
            踢下线
          </ManageButton>
        </div>
      </template>
    </ManageTable>

    <!-- 未输关键字时服务端只回最近 limit 条: 达到上限就明示, 别让"共 N 条"被当成全量 -->
    <p v-if="truncated" class="text-xs text-[var(--jc-warning)]">
      会话较多：未输入关键字时仅显示最近活跃的 {{ LIST_LIMIT }} 条；输入用户名 / IP
      可搜索更早的会话（服务端最多追溯 500 条）。
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { CronTask, CollectSource } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import SchedulePicker from '@/components/manage/SchedulePicker.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import ManageErrorState from '@/components/manage/ManageErrorState.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import type { StatusMap } from '@/components/manage/StatusTag.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { confirm } from '@/composables/useConfirm'
import { formatShortDateTime } from '@/utils/format'
import { scheduleError, SCHEDULE_HINT } from '@/utils/schedule'

// 状态/类型列 → StatusTag 词汇表(替代 lastRunVariant/lastRunText 等自写映射)
const CRON_STATE_MAP: StatusMap = {
  true: { label: '运行中', variant: 'success' },
  false: { label: '停止', variant: 'default' }
}
const CRON_MODEL_MAP: StatusMap = {
  0: { label: '自动', variant: 'purple' },
  1: { label: '指定', variant: 'purple' },
  2: { label: '补采', variant: 'purple' }
}
const RUN_STATUS_MAP: StatusMap = {
  success: { label: '成功', variant: 'success' },
  failed: { label: '失败', variant: 'danger' },
  running: { label: '运行中', variant: 'brand' },
  canceled: { label: '已取消', variant: 'warning' }
}
const RUN_STATUS_FALLBACK = { label: '无记录', variant: 'default' } as const

const rows = ref<CronTask[]>([])
const sources = ref<CollectSource[]>([])
const loading = ref(true)
const loadError = ref('') // 列表加载失败: 明确报错+重试, 不伪装成"暂无定时任务"
const keyword = ref('') // 客户端关键字过滤(ID/表达式/备注)
const dialogOpen = ref(false)
const submitting = ref(false)
const editing = ref<CronTask | null>(null)
const busyId = ref('') // 正在执行行内操作(立即执行/启停)的任务 id, 防连点并发
const formError = ref('') // 弹窗表单校验错误, 内联展示

const form = reactive<CronTask>({
  id: '',
  ids: [],
  time: 24,
  schedule: '',
  model: 0,
  state: true,
  remark: ''
})

const columns = [
  { key: 'id' as const, label: '任务 ID', width: '160px' },
  { key: 'schedule' as const, label: '执行周期', width: '200px' },
  { key: 'model' as const, label: '类型', width: '110px' },
  { key: 'time' as const, label: '采集时长', width: '110px' },
  { key: 'state' as const, label: '状态', width: '80px', align: 'center' as const },
  { key: 'lastRunAt' as const, label: '上次运行', width: '170px' },
  { key: 'remark' as const, label: '备注' }
]

// 任务类型下拉(value 保数字类型)
const MODEL_OPTIONS = [
  { value: 0, label: '自动更新已启用站点' },
  { value: 1, label: '仅更新所选资源站' },
  { value: 2, label: '补采失败页' }
]

/** 关键字过滤: 命中 ID / Cron 表达式 / 备注(服务端暂无分页, 前端过滤即可) */
const filteredRows = computed<CronTask[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return rows.value
  return rows.value.filter(
    (r) =>
      r.id.toLowerCase().includes(kw) ||
      r.schedule.toLowerCase().includes(kw) ||
      (r.remark ?? '').toLowerCase().includes(kw)
  )
})

/** 立即执行一次(不改变调度表, 在任务管理-历史中登记为手动任务) */
async function runNow(row: CronTask): Promise<void> {
  // 服务端 TriggerCron 无锁且拒绝已停用任务(400), 前端先拦: 停用禁点 + 行内防重
  if (busyId.value || !row.state) return
  const ok = await confirm({
    title: '立即执行',
    desc: `手动触发任务「${row.id}」执行一次? 完成后可在 任务管理-历史记录 查看结果.`,
    okText: '执行'
  })
  if (!ok) return
  busyId.value = row.id
  try {
    await manageApi.cron.runNow(row.id)
    await load()
  } catch {
    /* 全局拦截器已 toast, 这里只兜住 unhandled rejection */
  } finally {
    busyId.value = ''
  }
}

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const resp = await manageApi.cron.list()
    rows.value = Array.isArray(resp) ? resp : []
  } catch (e) {
    // 失败明确报错并可重试: 此前无 catch, 加载失败会复位居 rows 为空,
    // 表格落到"暂无定时任务"空态, 与错误事实矛盾
    loadError.value = e instanceof Error ? e.message : '加载定时任务失败'
  } finally {
    loading.value = false
  }
  try {
    sources.value = await manageApi.collect.list()
  } catch {
    sources.value = []
  }
}

function openAdd(): void {
  editing.value = null
  formError.value = ''
  Object.assign(form, { id: '', ids: [], time: 24, schedule: '', model: 0, state: true, remark: '' })
  dialogOpen.value = true
}

function openEdit(row: CronTask): void {
  editing.value = row
  formError.value = ''
  // 深拷贝 ids 数组，否则 toggleId 会污染列表 row
  Object.assign(form, { ...row, ids: [...(row.ids ?? [])] })
  dialogOpen.value = true
}

/**
 * 周期校验统一走 utils/schedule（与后端 taskcenter.ValidateSchedule 同规则）。
 * ⚠️ 不要在本文件写"冒号紧跟左括号"的正则：UnoCSS transformerVariantGroup 会把
 * 它当变体组改写掉（括号被吃、无报错），详见 utils/schedule.ts 头部说明。
 */

async function submit(): Promise<void> {
  const sched = form.schedule.trim()
  if (!sched) {
    formError.value = `请填写执行周期（如 ${SCHEDULE_HINT}）`
    return
  }
  const schedErr = scheduleError(sched)
  if (schedErr) {
    formError.value = schedErr
    return
  }
  // time 落库后 0/负数会被服务端引擎反转为全量采集, 必须为 ≥1 整数
  const time = Number(form.time)
  if (!Number.isInteger(time) || time < 1) {
    formError.value = '采集时长需为不小于 1 的整数（小时），清空或填 0 会触发全量采集'
    return
  }
  if (form.model === 1 && form.ids.length === 0) {
    formError.value = '任务类型为「仅更新所选资源站」时，请至少勾选一个资源站'
    return
  }
  formError.value = ''
  submitting.value = true
  try {
    form.schedule = sched
    form.time = time
    if (editing.value) await manageApi.cron.update({ ...form })
    else await manageApi.cron.add({ ...form })
    dialogOpen.value = false
    await load()
  } catch (e) {
    // 服务端错误全局拦截器已 toast; 保存失败保持弹窗打开, 错误不丢
    formError.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    submitting.value = false
  }
}

async function toggleState(row: CronTask): Promise<void> {
  if (busyId.value) return
  busyId.value = row.id
  try {
    await manageApi.cron.change({ ...row, state: !row.state })
    await load()
  } catch {
    /* 全局拦截器已 toast, 这里只兜住 unhandled rejection */
  } finally {
    busyId.value = ''
  }
}

async function remove(row: CronTask): Promise<void> {
  if (busyId.value) return
  const ok = await confirm({
    title: '确认删除任务？',
    desc: `任务「${row.id}」删除后不可恢复`,
    okText: '删除',
    danger: true
  })
  if (!ok) return
  busyId.value = row.id
  try {
    await manageApi.cron.remove(row.id)
    await load()
  } catch {
    /* 全局拦截器已 toast, 这里只兜住 unhandled rejection */
  } finally {
    busyId.value = ''
  }
}

function toggleId(id: string): void {
  const idx = form.ids.indexOf(id)
  if (idx >= 0) form.ids.splice(idx, 1)
  else form.ids.push(id)
}

onMounted(load)
</script>

<template>
  <!-- 页面根: flex 高度链(.jc-manage-main 提供), 表格 fit 自适应 -->
  <div class="flex flex-1 min-h-0 flex-col gap-[var(--jc-space-3)]">
  <!-- 加载失败: 明确报错 + 重试, 不落到"暂无定时任务"空态(§C8#1 收口 ManageErrorState banner) -->
  <ManageErrorState
    v-if="loadError && !loading"
    variant="banner"
    title="加载失败"
    :description="loadError"
    retry-label="点击重试"
    @retry="load"
  />

  <ManageTable
    :columns="columns"
    :rows="filteredRows"
    row-key="id"
    :loading="loading"
    :empty="loadError ? undefined : '暂无定时任务，点击右上角新增'"
  >
    <template #toolbar>
      <h2 class="text-lg font-[var(--jc-fw-semibold)]">定时任务</h2>
      <div class="flex gap-[var(--jc-space-2)] items-center">
        <ManageInput
          v-model="keyword"
          placeholder="搜索 ID / 表达式 / 备注"
          class="w-[180px]"
        />
        <ManageButton variant="ghost" @click="load">
          <BaseIcon name="refresh" size="16px" /> 刷新
        </ManageButton>
        <ManageButton variant="gradient" @click="openAdd">
          <BaseIcon name="plus" size="16px" /> 新增
        </ManageButton>
      </div>
    </template>

    <template #cell="{ row, col }">
      <StatusTag v-if="col.key === 'state'" :value="row.state" :map="CRON_STATE_MAP" />
      <StatusTag v-else-if="col.key === 'model'" :value="row.model" :map="CRON_MODEL_MAP" size="xs" />
      <code
        v-else-if="col.key === 'schedule'"
        class="font-[var(--jc-font-mono)] text-[length:var(--jc-fs-xs)] text-[var(--jc-text-link)]"
      >
        {{ row.schedule || '未配置' }}
      </code>
      <span v-else-if="col.key === 'time'">{{ row.time }} 小时</span>
      <span v-else-if="col.key === 'id'" class="font-[var(--jc-font-mono)] text-xs">
        {{ row.id }}
      </span>
      <span v-else-if="col.key === 'lastRunAt'" class="flex items-center gap-[var(--jc-space-1)]">
        <StatusTag :value="row.lastStatus" :map="RUN_STATUS_MAP" :fallback="RUN_STATUS_FALLBACK" size="xs" />
        <span class="text-xs text-muted whitespace-nowrap">
          {{ row.lastRunAt ? formatShortDateTime(row.lastRunAt) : '—' }}
        </span>
        <span
          v-if="row.lastError"
          class="text-xs text-danger break-all"
          :title="row.lastMessage ? row.lastError + '（' + row.lastMessage + '）' : row.lastError"
        >{{ row.lastError }}</span>
      </span>
      <span v-else>{{ row[col.key] ?? '—' }}</span>
    </template>

    <template #actions="{ row }">
      <div class="flex gap-[var(--jc-space-1)] justify-end">
        <ManageButton
          variant="ghost"
          size="sm"
          :loading="busyId === row.id"
          :disabled="!!busyId && busyId !== row.id"
          @click="toggleState(row)"
        >
          {{ row.state ? '停止' : '启动' }}
        </ManageButton>
        <!-- 已停用任务服务端必拒(400 invalid argument), 本地禁用并提示 -->
        <ManageButton
          variant="ghost"
          size="sm"
          :disabled="!row.state || !!busyId"
          :title="row.state ? undefined : '任务已停用，请先启动'"
          @click="runNow(row)"
        >
          立即执行
        </ManageButton>
        <ManageButton variant="ghost" size="sm" :disabled="!!busyId" @click="openEdit(row)">
          编辑
        </ManageButton>
        <ManageButton variant="danger" size="sm" :disabled="!!busyId" @click="remove(row)">
          删除
        </ManageButton>
      </div>
    </template>
  </ManageTable>

  <ManageSheet v-model="dialogOpen" :title="editing ? '编辑任务' : '新增任务'" mobile-mode="fullsheet">
    <div class="flex flex-col gap-[var(--jc-space-4)]">
      <ManageFormField label="执行周期" required :hint="`选择式；保存为友好文本（${SCHEDULE_HINT}）`">
        <SchedulePicker v-model="form.schedule" size="sm" />
      </ManageFormField>
      <ManageFormField label="任务类型" required>
        <ManageSelect v-model="form.model" :options="MODEL_OPTIONS" aria-label="任务类型" />
      </ManageFormField>
      <ManageFormField
        v-if="form.model === 1"
        label="资源站列表"
        hint="点击勾选对应资源站 id"
      >
        <div class="flex flex-wrap gap-[var(--jc-space-2)]">
          <BaseTag
            v-for="s in sources"
            :key="s.id"
            :variant="form.ids.includes(s.id) ? 'brand' : 'default'"
            size="sm"
            class="cursor-pointer"
            tabindex="0"
            data-focusable="true"
            role="button"
            @click="toggleId(s.id)"
            @keydown.enter="toggleId(s.id)"
          >
            {{ s.name }}
          </BaseTag>
        </div>
      </ManageFormField>
      <ManageFormField label="采集时长（小时）" required hint="填 0 或负数会被服务端按全量采集执行">
        <ManageInput v-model="form.time" type="number" min="1" step="1" placeholder="24" />
      </ManageFormField>
      <ManageFormField label="备注">
        <ManageInput v-model="form.remark!" placeholder="可选" />
      </ManageFormField>
      <ManageFormField label="启用">
        <ManageSwitch v-model="form.state" />
      </ManageFormField>
      <p v-if="formError" class="text-xs text-[var(--jc-danger)]">{{ formError }}</p>
    </div>
    <template #footer>
      <ManageButton variant="ghost" @click="dialogOpen = false">取消</ManageButton>
      <ManageButton variant="gradient" :loading="submitting" @click="submit">保存</ManageButton>
    </template>
  </ManageSheet>
  </div>
</template>

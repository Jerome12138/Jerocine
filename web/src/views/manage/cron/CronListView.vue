<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { CronTask, CollectSource } from '@/types/manage'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { confirm } from '@/composables/useConfirm'

const rows = ref<CronTask[]>([])
const sources = ref<CollectSource[]>([])
const loading = ref(true)
const dialogOpen = ref(false)
const submitting = ref(false)
const editing = ref<CronTask | null>(null)

const form = reactive<CronTask>({
  id: '',
  ids: [],
  time: 24,
  spec: '',
  model: 0,
  state: true,
  remark: ''
})

const columns = [
  { key: 'id' as const, label: '任务 ID', width: '160px' },
  { key: 'spec' as const, label: 'Cron 表达式', width: '200px' },
  { key: 'model' as const, label: '类型', width: '110px' },
  { key: 'time' as const, label: '采集时长', width: '110px' },
  { key: 'state' as const, label: '状态', width: '80px', align: 'center' as const },
  { key: 'remark' as const, label: '备注' }
]

async function load(): Promise<void> {
  loading.value = true
  try {
    const resp = await manageApi.cron.list()
    rows.value = Array.isArray(resp) ? resp : []
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
  Object.assign(form, { id: '', ids: [], time: 24, spec: '', model: 0, state: true, remark: '' })
  dialogOpen.value = true
}

function openEdit(row: CronTask): void {
  editing.value = row
  // 深拷贝 ids 数组，否则 toggleId 会污染列表 row
  Object.assign(form, { ...row, ids: [...(row.ids ?? [])] })
  dialogOpen.value = true
}

async function submit(): Promise<void> {
  submitting.value = true
  try {
    if (editing.value) await manageApi.cron.update({ ...form })
    else await manageApi.cron.add({ ...form })
    dialogOpen.value = false
    await load()
  } finally {
    submitting.value = false
  }
}

async function toggleState(row: CronTask): Promise<void> {
  await manageApi.cron.change({ ...row, state: !row.state })
  await load()
}

async function remove(row: CronTask): Promise<void> {
  const ok = await confirm({
    title: '确认删除任务？',
    desc: `任务「${row.id}」删除后不可恢复`,
    okText: '删除',
    danger: true
  })
  if (!ok) return
  await manageApi.cron.remove(row.id)
  await load()
}

function toggleId(id: string): void {
  const idx = form.ids.indexOf(id)
  if (idx >= 0) form.ids.splice(idx, 1)
  else form.ids.push(id)
}

onMounted(load)
</script>

<template>
  <ManageTable
    :columns="columns"
    :rows="rows"
    row-key="id"
    :loading="loading"
    empty="暂无定时任务"
  >
    <template #toolbar>
      <h2 class="text-lg font-[var(--gf-fw-semibold)]">定时任务</h2>
      <div class="flex gap-[var(--gf-space-2)]">
        <BaseButton variant="ghost" size="sm" @click="load">
          <BaseIcon name="refresh" size="16px" /> 刷新
        </BaseButton>
        <BaseButton variant="gradient" size="sm" @click="openAdd">
          <BaseIcon name="plus" size="16px" /> 新增
        </BaseButton>
      </div>
    </template>

    <template #cell="{ row, col }">
      <BaseTag v-if="col.key === 'state'" :variant="row.state ? 'success' : 'default'">
        {{ row.state ? '运行中' : '停止' }}
      </BaseTag>
      <BaseTag v-else-if="col.key === 'model'" variant="purple" size="xs">
        {{ row.model === 0 ? '自动' : '指定' }}
      </BaseTag>
      <code
        v-else-if="col.key === 'spec'"
        class="font-[var(--gf-font-mono)] text-[var(--gf-fs-xs)] text-[var(--gf-text-link)]"
      >
        {{ row.spec }}
      </code>
      <span v-else-if="col.key === 'time'">{{ row.time }} 小时</span>
      <span v-else-if="col.key === 'id'" class="font-[var(--gf-font-mono)] text-xs">
        {{ row.id }}
      </span>
      <span v-else>{{ row[col.key] ?? '—' }}</span>
    </template>

    <template #actions="{ row }">
      <div class="flex gap-[var(--gf-space-1)] justify-end">
        <BaseButton variant="ghost" size="sm" @click="toggleState(row)">
          {{ row.state ? '停止' : '启动' }}
        </BaseButton>
        <BaseButton variant="ghost" size="sm" @click="openEdit(row)">编辑</BaseButton>
        <BaseButton variant="danger" size="sm" @click="remove(row)">删除</BaseButton>
      </div>
    </template>
  </ManageTable>

  <ManageSheet v-model="dialogOpen" :title="editing ? '编辑任务' : '新增任务'" mobile-mode="fullsheet">
    <div class="flex flex-col gap-[var(--gf-space-4)]">
      <ManageFormField label="Cron 表达式" required hint="如 0 0 3 * * *（秒 分 时 日 月 周）">
        <ManageInput v-model="form.spec" placeholder="0 0 3 * * *" />
      </ManageFormField>
      <ManageFormField label="任务类型" required>
        <select
          v-model.number="form.model"
          class="w-full bg-elevated text-primary border border-default rounded-[var(--gf-radius-md)] px-[var(--gf-space-3)] py-[var(--gf-space-3)]"
          data-focusable="true"
        >
          <option :value="0">自动更新已启用站点</option>
          <option :value="1">仅更新所选资源站</option>
        </select>
      </ManageFormField>
      <ManageFormField
        v-if="form.model === 1"
        label="资源站列表"
        hint="点击勾选对应资源站 id"
      >
        <div class="flex flex-wrap gap-[var(--gf-space-2)]">
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
      <ManageFormField label="采集时长（小时）" required>
        <ManageInput v-model="form.time" type="number" placeholder="24" />
      </ManageFormField>
      <ManageFormField label="备注">
        <ManageInput v-model="form.remark!" placeholder="可选" />
      </ManageFormField>
      <ManageFormField label="启用">
        <ManageSwitch v-model="form.state" />
      </ManageFormField>
    </div>
    <template #footer>
      <BaseButton variant="ghost" @click="dialogOpen = false">取消</BaseButton>
      <BaseButton variant="gradient" :loading="submitting" @click="submit">保存</BaseButton>
    </template>
  </ManageSheet>
</template>

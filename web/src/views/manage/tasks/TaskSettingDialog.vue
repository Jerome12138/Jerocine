<script setup lang="ts">
import { computed, reactive } from 'vue'
import * as tasksApi from '@/api/manage/tasks'
import { toast } from '@/api/http'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageFormActions from '@/components/manage/ManageFormActions.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'
import SchedulePicker from '@/components/manage/SchedulePicker.vue'
import { scheduleError } from '@/utils/schedule'
import type { TaskCenterItem } from '@/api/manage/tasks'

/**
 * 任务配置编辑（openManageDialog 内容组件）。
 *
 * 字段**按后端下发的 editable 渲染**，不做前端硬编码 —— 以后新增任务类型，
 * 只要后端声明了可编辑项，这里自动出现。
 * 执行层间隔带下限提示：低于下限后端会 422，这里先把下限显示出来避免白填。
 */
const props = defineProps<{ task: TaskCenterItem }>()

const emit = defineEmits<{
  (e: 'confirm'): void
}>()

const can = (field: string): boolean => props.task.editable.includes(field)

const form = reactive({
  enabled: props.task.enabled,
  schedule: props.task.schedule,
  remark: '',
  intervalMs: props.task.intervalMs || 0
})

/** 周期输入提示：内置与采集任务都已统一为友好文本（cron 表达式在迁移时自动转换） */
const intervalHint = computed(() => {
  if (props.task.kind !== 'builtin') return '采集任务的间隔在「采集源」里按源配置'
  if (!props.task.minIntervalMs) return '毫秒；0 = 使用默认'
  return `毫秒；不小于 ${props.task.minIntervalMs}（${props.task.intervalLabel}）`
})

const { busy, run } = useAsyncAction()

function submit(): void {
  const payload: { enabled?: boolean; schedule?: string; remark?: string; intervalMs?: number } = {}
  if (can('enabled')) payload.enabled = form.enabled
  if (can('schedule')) payload.schedule = form.schedule.trim()
  if (can('remark')) payload.remark = form.remark.trim()
  if (can('interval')) payload.intervalMs = Number(form.intervalMs) || 0

  // 周期与后端 taskcenter.ValidateSchedule 同规则: 本地先拦, 省一次 422 往返
  // （空串合法 = 仅手动, 与后端一致）
  if (payload.schedule !== undefined) {
    const err = scheduleError(payload.schedule)
    if (err) {
      toast('warning', err)
      return
    }
  }

  if (payload.intervalMs && props.task.minIntervalMs && payload.intervalMs < props.task.minIntervalMs) {
    toast('warning', `间隔不能小于 ${props.task.minIntervalMs}ms，否则容易被源站拉黑`)
    return
  }
  const hasChange =
    (payload.enabled !== undefined && payload.enabled !== props.task.enabled) ||
    (payload.schedule !== undefined && payload.schedule !== props.task.schedule) ||
    !!payload.remark ||
    (payload.intervalMs !== undefined && payload.intervalMs !== props.task.intervalMs)
  if (!hasChange) {
    toast('info', '没有改动')
    emit('confirm')
    return
  }

  // 停用时若后端回报"此刻有一轮在跑"：不中断本轮，跑完才真正停下 —— 如实告知
  let deferredStop = false
  void run({
    action: async () => {
      const r = await tasksApi.taskCenterUpdateSetting(props.task.key, payload)
      deferredStop = r.deferred === true
      return r
    },
    successMsg: '配置已保存',
    after: () => {
      if (deferredStop) {
        toast('info', '该任务正在运行，本轮运行结束后停止')
      }
      emit('confirm')
    }
  })
}
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-3)]">
    <p class="text-xs text-muted">
      {{ task.name }}<span v-if="task.target"> · {{ task.target }}</span>
    </p>

    <ManageFormField v-if="can('enabled')" label="启用">
      <ManageSwitch v-model="form.enabled" />
    </ManageFormField>

    <ManageFormField v-if="can('schedule')" label="执行周期">
      <SchedulePicker v-model="form.schedule" size="sm" :allow-empty="task.kind === 'builtin'" />
    </ManageFormField>

    <ManageFormField v-if="can('interval')" label="接口间隔" :hint="intervalHint">
      <ManageInput v-model.number="form.intervalMs" type="number" placeholder="毫秒" />
    </ManageFormField>

    <ManageFormField v-if="can('remark')" label="备注">
      <ManageInput v-model="form.remark" placeholder="留空表示不改动" />
    </ManageFormField>

    <ManageFormActions :busy="busy" confirm-text="保存" confirm-type="button" pt @confirm="submit" @cancel="emit('confirm')" />
  </div>
</template>
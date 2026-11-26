<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { manageApi } from '@/api'
import type { CronTask, CollectSource } from '@/types/manage'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import SchedulePicker from '@/components/manage/SchedulePicker.vue'
import { scheduleError, SCHEDULE_HINT } from '@/utils/schedule'

/**
 * 采集任务的新建/编辑弹窗 —— 2026-11-26 从「定时任务页」搬进「任务中心」。
 *
 * 为什么搬：同一个采集任务原先在两个页面都能看到（定时任务页 + 任务中心计划任务页），
 * 用户要求"只在一个地方、内容统一"。任务中心现在是唯一入口，本弹窗承担**建模面**
 * 的字段（任务类型 / 采集时长 / 资源站），任务中心自己的「编辑任务」只管调度与启停。
 */
const props = defineProps<{
  modelValue: boolean
  /** null = 新建；否则为编辑 */
  task: CronTask | null
}>()
const emit = defineEmits<{
  'update:modelValue': [open: boolean]
  saved: []
}>()

const sources = ref<CollectSource[]>([])
const submitting = ref(false)
const formError = ref('')

const MODEL_OPTIONS = [
  { value: 0, label: '自动更新已启用站点' },
  { value: 1, label: '仅更新所选资源站' },
  { value: 2, label: '补采失败页' }
]

const form = reactive<CronTask>({
  id: '', ids: [], time: 24, schedule: '', model: 0, state: true, remark: ''
})

const open = ref(props.modelValue)

watch(() => props.modelValue, async (v) => {
  open.value = v
  if (!v) return
  formError.value = ''
  if (props.task) {
    // 深拷贝 ids：后面 toggleId 直接改数组，不能污染列表行
    Object.assign(form, { ...props.task, ids: [...(props.task.ids ?? [])] })
  } else {
    Object.assign(form, { id: '', ids: [], time: 24, schedule: '', model: 0, state: true, remark: '' })
  }
  try {
    sources.value = await manageApi.collect.list()
  } catch {
    sources.value = []
  }
}, { immediate: true })

watch(open, (v) => emit('update:modelValue', v))

function toggleId(id: string): void {
  const idx = form.ids.indexOf(id)
  if (idx >= 0) form.ids.splice(idx, 1)
  else form.ids.push(id)
}

async function submit(): Promise<void> {
  const sched = form.schedule.trim()
  const schedErr = sched ? scheduleError(sched) : `请选择执行周期（${SCHEDULE_HINT}）`
  if (schedErr) {
    formError.value = schedErr
    return
  }
  // time 落库后 0/负数会被服务端引擎反转为全量采集，必须为 ≥1 整数
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
    const payload = { ...form, ids: [...form.ids] }
    if (props.task) await manageApi.cron.update(payload)
    else await manageApi.cron.add(payload)
    open.value = false
    emit('saved')
  } catch (e) {
    // 服务端错误全局拦截器已 toast；保持弹窗打开，错误不丢
    formError.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <ManageSheet v-model="open" :title="task ? '编辑采集任务' : '新建采集任务'" mobile-mode="fullsheet">
    <div class="flex flex-col gap-[var(--jc-space-4)]">
      <ManageFormField label="执行周期" required :hint="`选择式；保存为友好文本（${SCHEDULE_HINT}）`">
        <SchedulePicker v-model="form.schedule" size="sm" />
      </ManageFormField>

      <ManageFormField label="任务类型" required>
        <ManageSelect v-model="form.model" :options="MODEL_OPTIONS" aria-label="任务类型" />
      </ManageFormField>

      <ManageFormField
        label="采集时长（小时）"
        required
        hint="≤1 表示全量；清空或 0 会被服务端反转为全量采集，这里直接拦住"
      >
        <ManageInput v-model="form.time" type="number" aria-label="采集时长" />
      </ManageFormField>

      <ManageFormField v-if="form.model === 1" label="资源站列表" required hint="点击勾选要更新的资源站">
        <div class="flex flex-wrap gap-[var(--jc-space-2)]">
          <BaseTag
            v-for="s in sources"
            :key="s.id"
            :variant="form.ids.includes(s.id) ? 'brand' : 'default'"
            size="sm"
            class="cursor-pointer select-none"
            role="button"
            :aria-pressed="form.ids.includes(s.id)"
            @click="toggleId(s.id)"
          >
            {{ s.name || s.id }}
          </BaseTag>
          <span v-if="sources.length === 0" class="text-xs text-muted">暂无采集源</span>
        </div>
      </ManageFormField>

      <ManageFormField label="备注">
        <ManageInput v-model="form.remark" aria-label="备注" />
      </ManageFormField>

      <p v-if="formError" class="text-xs text-[var(--jc-danger)]">{{ formError }}</p>
    </div>

    <template #footer>
      <ManageButton variant="ghost" @click="open = false">取消</ManageButton>
      <ManageButton variant="primary" :loading="submitting" @click="submit">保存</ManageButton>
    </template>
  </ManageSheet>
</template>

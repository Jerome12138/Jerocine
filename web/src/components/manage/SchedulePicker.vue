<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import type { SelectOption } from '@/components/manage/ManageSelect.vue'
import {
  composeSchedule, parseSchedule, DEFAULT_SCHEDULE_PARTS,
  type ScheduleKind, type ScheduleParts
} from '@/utils/schedule'

/**
 * 执行周期选择器（完整选择式，不再让用户手敲 `every:30m` / `daily:04:00`）。
 *
 * 设计要点：
 * - 对外只有 v-model(string)：吐出的仍是**友好文本**，与后端 taskcenter.ValidateSchedule 同一套取值，
 *   所以接口、单测、存量数据都不用动（只是输入方式从填空变选择）。
 * - 解析不了的历史值不会丢：切到「自定义」保留原文本输入框（避免一编辑就把别人的值冲掉）。
 * - 纯规则在 utils/schedule.ts（parse/compose），组件只做状态与渲染，便于单测。
 */
interface Props {
  modelValue: string
  /** 允许"不定时（仅手动）"：内置任务可以留空；采集任务必须给周期 */
  allowEmpty?: boolean
  disabled?: boolean
  size?: 'sm' | 'default' | 'lg'
}

const props = withDefaults(defineProps<Props>(), { allowEmpty: false, disabled: false, size: 'default' })
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

/** 选择器内部状态（不含"自定义/仅手动"两种非结构化形态） */
const parts = ref<ScheduleParts>({ ...DEFAULT_SCHEDULE_PARTS })
/** 形态：结构化四类 + manual(仅手动=空串) + custom(解析不了，保留原文本) */
type Mode = ScheduleKind | 'manual' | 'custom'
const mode = ref<Mode>('daily')
const custom = ref('')

const KIND_OPTIONS: SelectOption[] = [
  { value: 'every', label: '每隔一段时间' },
  { value: 'daily', label: '每天' },
  { value: 'weekly', label: '每周' },
  { value: 'monthly', label: '每月' }
]

const WEEK_OPTIONS: SelectOption[] = [
  { value: 1, label: '周一' }, { value: 2, label: '周二' }, { value: 3, label: '周三' },
  { value: 4, label: '周四' }, { value: 5, label: '周五' }, { value: 6, label: '周六' }, { value: 7, label: '周日' }
]
const MINUTE_OPTIONS: SelectOption[] = Array.from({ length: 60 }, (_, i) => ({
  value: i, label: String(i).padStart(2, '0')
}))
const HOUR_OPTIONS: SelectOption[] = Array.from({ length: 24 }, (_, i) => ({
  value: i, label: String(i).padStart(2, '0')
}))
const DAY_OPTIONS: SelectOption[] = Array.from({ length: 28 }, (_, i) => ({
  value: i + 1, label: `${i + 1} 日`
}))

/** 间隔数值：分钟 1~59、小时 1~99（与后端上限一致） */
const intervalOptions = computed<SelectOption[]>(() => {
  const max = parts.value.unit === 'm' ? 59 : 99
  return Array.from({ length: max }, (_, i) => ({ value: i + 1, label: String(i + 1) }))
})

/** 形态选项（含 allowEmpty 时的"仅手动"） */
const modeOptions = computed<SelectOption[]>(() => {
  const list = [...KIND_OPTIONS]
  if (props.allowEmpty) list.unshift({ value: 'manual', label: '不定时（仅手动触发）' })
  if (mode.value === 'custom') list.push({ value: 'custom', label: '自定义（原值）' })
  return list
})

/** 外部值 → 内部状态（挂载与外部改动时同步；自己 emit 引起的回写不重置编辑态） */
function syncFromModel(v: string): void {
  const text = (v || '').trim()
  if (!text) {
    mode.value = props.allowEmpty ? 'manual' : 'daily'
    if (!props.allowEmpty) emitValue()
    return
  }
  const parsed = parseSchedule(text)
  if (parsed) {
    parts.value = parsed
    mode.value = parsed.kind
    custom.value = ''
    return
  }
  // 解析不了：保留原值，进"自定义"，避免编辑时把历史值悄悄改掉
  mode.value = 'custom'
  custom.value = text
}

function emitValue(): void {
  if (mode.value === 'manual') {
    emit('update:modelValue', '')
    return
  }
  if (mode.value === 'custom') {
    emit('update:modelValue', custom.value.trim())
    return
  }
  emit('update:modelValue', composeSchedule(parts.value))
}

watch(() => props.modelValue, (v) => {
  const current = mode.value === 'custom' ? custom.value.trim() : composeSchedule(parts.value)
  const incoming = (v || '').trim()
  if (mode.value === 'manual' && incoming === '') return
  if (current === incoming) return
  syncFromModel(incoming)
}, { immediate: true })

function onMode(v: string | number | undefined): void {
  const m = String(v) as Mode
  if (m === 'custom') {
    custom.value = composeSchedule(parts.value) // 从当前结构化值起手改
    mode.value = 'custom'
    emitValue()
    return
  }
  if (m === 'manual') {
    mode.value = 'manual'
    emitValue()
    return
  }
  mode.value = m as ScheduleKind
  parts.value = { ...parts.value, kind: m as ScheduleKind }
  emitValue()
}

function setPart(key: 'n' | 'unit' | 'weekday' | 'day' | 'hour' | 'minute', v: string | number | undefined): void {
  if (key === 'unit') {
    parts.value = { ...parts.value, unit: v === 'h' ? 'h' : 'm' }
    // 分钟上限 59：切到分钟时若超限自动收敛，免得发出非法值
    if (parts.value.unit === 'm' && parts.value.n > 59) parts.value = { ...parts.value, n: 59 }
  } else {
    parts.value = { ...parts.value, [key]: Number(v) } as ScheduleParts
  }
  emitValue()
}

/** 给外部展示的人类可读摘要（如"每天 04:00"） */
const summary = computed(() => {
  const p = parts.value
  const hhmm = `${String(p.hour).padStart(2, '0')}:${String(p.minute).padStart(2, '0')}`
  switch (mode.value) {
    case 'every': {
      // 说清楚"整点对齐"（与 cron `*/N` 同语义）：避免用户以为是"从点了保存那一刻起算"
      if (p.unit === 'm') {
        const marks = Array.from({ length: Math.floor(60 / p.n) }, (_, i) => i * p.n)
        return marks.length > 1
          ? `每小时 ${marks.join('/')} 分执行（整点对齐）`
          : `每 ${p.n} 分钟执行（整点起算）`
      }
      const hours = Array.from({ length: Math.floor(24 / p.n) }, (_, i) => i * p.n)
      return hours.length > 1
        ? `每天 ${hours.join('/')} 点执行（整点对齐）`
        : `每 ${p.n} 小时执行（从上次触发起算）`
    }
    case 'daily':
      return `每天 ${hhmm}`
    case 'weekly':
      return `每${WEEK_OPTIONS.find((o) => o.value === p.weekday)?.label ?? ''} ${hhmm}`
    case 'monthly':
      return `每月 ${p.day} 日 ${hhmm}`
    case 'manual':
      return '不定时（只在手动触发时执行）'
    default:
      return ''
  }
})
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-2)]">
    <div class="flex flex-wrap items-center gap-[var(--jc-space-2)]">
      <ManageSelect
        :model-value="mode"
        :options="modeOptions"
        :disabled="disabled"
        :size="size"
        aria-label="周期类型"
        @update:model-value="onMode"
      />

      <!-- 每隔一段时间 -->
      <template v-if="mode === 'every'">
        <ManageSelect
          :model-value="parts.n"
          :options="intervalOptions"
          :disabled="disabled"
          :size="size"
          aria-label="间隔数值"
          @update:model-value="(v) => setPart('n', v)"
        />
        <ManageSelect
          :model-value="parts.unit"
          :options="[{ value: 'm', label: '分钟' }, { value: 'h', label: '小时' }]"
          :disabled="disabled"
          :size="size"
          aria-label="间隔单位"
          @update:model-value="(v) => setPart('unit', v)"
        />
      </template>

      <!-- 每周几 -->
      <template v-else-if="mode === 'weekly'">
        <ManageSelect
          :model-value="parts.weekday"
          :options="WEEK_OPTIONS"
          :disabled="disabled"
          :size="size"
          aria-label="星期"
          @update:model-value="(v) => setPart('weekday', v)"
        />
      </template>

      <!-- 每月几号 -->
      <template v-else-if="mode === 'monthly'">
        <ManageSelect
          :model-value="parts.day"
          :options="DAY_OPTIONS"
          :disabled="disabled"
          :size="size"
          aria-label="每月几号"
          @update:model-value="(v) => setPart('day', v)"
        />
      </template>

      <!-- 时:分（除"每隔/仅手动/自定义"外都要） -->
      <template v-if="mode === 'daily' || mode === 'weekly' || mode === 'monthly'">
        <ManageSelect
          :model-value="parts.hour"
          :options="HOUR_OPTIONS"
          :disabled="disabled"
          :size="size"
          aria-label="小时"
          @update:model-value="(v) => setPart('hour', v)"
        />
        <span class="text-xs text-muted">:</span>
        <ManageSelect
          :model-value="parts.minute"
          :options="MINUTE_OPTIONS"
          :disabled="disabled"
          :size="size"
          aria-label="分钟"
          @update:model-value="(v) => setPart('minute', v)"
        />
      </template>

      <!-- 解析不了的历史值：保留原文本可编辑（不静默丢弃） -->
      <ManageInput
        v-if="mode === 'custom'"
        v-model="custom"
        :disabled="disabled"
        class="min-w-[14rem]"
        placeholder="every:30m / daily:04:00"
        @update:model-value="emitValue"
      />
    </div>

    <p v-if="summary" class="text-xs text-muted">下称：{{ summary }}（存为 <code class="text-[var(--jc-text-link)]">{{ modelValue || '空' }}</code>）</p>
  </div>
</template>

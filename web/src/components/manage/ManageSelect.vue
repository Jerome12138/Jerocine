<script setup lang="ts">
/**
 * 统一替换手写原生 <select>（复核 #3）: class 与 ManageInput 同族、
 * 44px 触控区、恒定 data-focusable（TV 遥控）、placeholder 占位项、原类型 emit。
 */
export interface SelectOption {
  label: string
  value: string | number
}

interface Props {
  modelValue: string | number | undefined
  options: SelectOption[]
  disabled?: boolean
  /** modelValue 为 undefined 时显示的占位项 */
  placeholder?: string
  /** 无 label 包裹时必填（无障碍） */
  ariaLabel?: string
}

const props = defineProps<Props>()
const emit = defineEmits<{ 'update:modelValue': [value: string | number] }>()

function onChange(e: Event): void {
  const el = e.target as HTMLSelectElement
  // 按 option.value 原类型 emit（不经过字符串化）; option 元素上取回原始 value
  const hit = props.options.find((o) => String(o.value) === el.value)
  emit('update:modelValue', hit ? hit.value : el.value)
}
</script>

<template>
  <select
    :value="props.modelValue === undefined ? '' : String(props.modelValue)"
    :disabled="props.disabled"
    :aria-label="props.ariaLabel"
    class="w-full min-h-[44px] md:min-h-[36px] bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-2)] text-sm outline-none focus:border-strong focus:shadow-focus transition disabled:opacity-50"
    :data-focusable="props.disabled ? undefined : 'true'"
    @change="onChange"
  >
    <option v-if="props.placeholder !== undefined && props.modelValue === undefined" value="" disabled hidden>
      {{ props.placeholder }}
    </option>
    <option v-for="opt in props.options" :key="String(opt.value)" :value="String(opt.value)">
      {{ opt.label }}
    </option>
  </select>
</template>

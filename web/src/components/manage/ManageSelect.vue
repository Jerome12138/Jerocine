<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { useDismiss } from '@/composables/useDismiss'
import { useListNavigation } from '@/composables/useListNavigation'

/**
 * ManageSelect v2(规格 §3.5/D④): 自绘浮层下拉, 替换原生 <select> 的 OS 弹层。
 * - API 向后兼容: options/placeholder/disabled/ariaLabel/原类型 emit, 9 个使用页零改动。
 * - 逻辑全部下沉共享 composables: useDismiss(外点+ESC) / useListNavigation(键盘导航)。
 * - 触发钮与 ManageInput 同族外观 + size 三档 --jc-ctrl-* 令牌(§3.6/D⑥)。
 * - a11y(R1 评审): 焦点恒留触发钮, 键盘 active 项经 aria-activedescendant 播报(option 带唯一 id)。
 * - teleport(D⑧ 预留): 弹窗内实测裁剪时浮层 Teleport 到 body + fixed 定位(触发钮几何);
 *   该模式下 useDismiss 的锚切到浮层自身(组件根覆盖不到 body 子树), 并渲染全屏遮罩
 *   拦截触发钮的再点击——否则"外点关闭 → click 冒到触发钮 → 立即重开"。
 */
export interface SelectOption {
  label: string
  value: string | number
}

interface Props {
  modelValue: string | number | undefined
  options: SelectOption[]
  disabled?: boolean
  /** modelValue 为 undefined 时触发钮显示的占位文案 */
  placeholder?: string
  /** 无 label 包裹时必填(无障碍) */
  ariaLabel?: string
  /** 尺寸档(§3.6/D⑥): 与 ManageButton/ManageInput 同一 --jc-ctrl-* 令牌, 默认 default */
  size?: 'sm' | 'default' | 'lg'
  /** D⑧: 浮层 Teleport 到 body + fixed 定位(默认 false = 内联 absolute) */
  teleport?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  size: 'default',
  teleport: false
})

const emit = defineEmits<{ 'update:modelValue': [value: string | number] }>()

const isOpen = ref(false)
const rootRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const panelRef = ref<HTMLElement | null>(null)

/** 组件内唯一前缀: option id 供 aria-activedescendant 引用, 同页多实例不能撞 id */
const uid = useId()

/** 选中项: 按 String(value) 匹配(保留 v1 语义, 数字 value 不被字符串化) */
const selected = computed<SelectOption | undefined>(() =>
  props.options.find((o) => String(o.value) === String(props.modelValue))
)

function isSelected(opt: SelectOption): boolean {
  return String(opt.value) === String(props.modelValue)
}

/** 触发钮文案: 选中 label → 未选中 placeholder → 有值但无匹配时回显原值字符串 */
const triggerLabel = computed(() => {
  if (selected.value) return selected.value.label
  return props.modelValue === undefined ? props.placeholder : String(props.modelValue)
})

/** size 三档 → --jc-ctrl-* 令牌类(§3.6 映射表, 与 ManageButton.sizeClass 同构) */
const sizeClass = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'h-[var(--jc-ctrl-h-sm)] px-[var(--jc-ctrl-px-sm)] text-[length:var(--jc-ctrl-fs-sm)]'
    case 'lg':
      return 'h-[var(--jc-ctrl-h-lg)] px-[var(--jc-ctrl-px-lg)] text-[length:var(--jc-ctrl-fs-lg)]'
    default:
      return 'h-[var(--jc-ctrl-h)] px-[var(--jc-ctrl-px)] text-[length:var(--jc-ctrl-fs)]'
  }
})

function close(): void {
  isOpen.value = false
}

function confirmIndex(index: number): void {
  const opt = props.options[index]
  if (!opt) return
  emit('update:modelValue', opt.value)
  close()
}

/** 键盘导航(§3.2 写一次全端复用): open 时 active=当前选中项索引 */
const { activeIndex, onKeydown: onListKeydown, open: resetActive } = useListNavigation({
  itemCount: () => props.options.length,
  initial: () => {
    const idx = props.options.findIndex((o) => isSelected(o))
    return idx < 0 ? 0 : idx
  },
  onConfirm: confirmIndex,
  onClose: close
})

/**
 * 读屏播报锚点(a11y 增强, R1 评审): 焦点恒留触发钮, 键盘 active 项经
 * aria-activedescendant 暴露给 AT —— 方向键移动时读屏器可播报当前项(WAI-ARIA listbox 模式)。
 * 仅打开态且 active 项存在时挂, 关闭/空列表不挂。
 */
const activeDescendant = computed(() => {
  if (!isOpen.value) return undefined
  const opt = props.options[activeIndex.value]
  return opt ? `jc-opt-${uid}-${activeIndex.value}` : undefined
})

// ---- teleport 模式浮层定位: 触发钮几何 → fixed 坐标(+4px = 内联浮层 mt-1 同款间距) ----
const teleportPos = ref({ top: '0px', left: '0px', width: '0px' })
function syncTeleportPos(): void {
  const el = triggerRef.value
  if (!el) return
  const r = el.getBoundingClientRect()
  teleportPos.value = { top: `${r.bottom + 4}px`, left: `${r.left}px`, width: `${r.width}px` }
}

function openPanel(): void {
  if (props.disabled || isOpen.value) return
  isOpen.value = true
  resetActive()
  if (props.teleport) syncTeleportPos()
}

function onTriggerClick(): void {
  if (props.disabled) return
  if (props.teleport) {
    // teleport 模式: 打开态有全屏遮罩挡住触发钮, click 到不了这里 → 只负责"开"
    openPanel()
    return
  }
  if (isOpen.value) close()
  else openPanel()
}

/** 触发钮 keydown: 关闭态交给原生 button 语义(Enter/Space=click 打开); 打开态委托列表导航 */
function onTriggerKeydown(e: KeyboardEvent): void {
  if (!isOpen.value) return
  // Enter/Space 阻断原生 click: 确认选中后不该再触发一次 toggle
  if (e.key === 'Enter' || e.key === ' ') e.preventDefault()
  onListKeydown(e)
}

// 关闭后焦点归还触发钮: 点击选项后浮层 DOM 被移除, 焦点会掉到 body(TV 空间导航需要锚点)。
// 只在焦点已丢失(body)或仍在本组件内时归还; 外点落到其他可聚焦元素的行为不被打断
// (指针序: pointerdown 触发关闭 → 微任务里 focus 归还 → 随后的 mousedown 默认聚焦仍落在点击目标)。
watch(isOpen, (open) => {
  if (open) return
  nextTick(() => {
    const active = document.activeElement
    const inRoot = active instanceof Node && rootRef.value?.contains(active) === true
    if (!active || active === document.body || inRoot) triggerRef.value?.focus()
  })
})

// 常规模式: 锚 = 组件根(触发钮 + 浮层); teleport 模式: 锚 = 浮层自身(根覆盖不到 body 子树)
useDismiss(
  computed(() => (props.teleport ? panelRef.value : rootRef.value)),
  isOpen,
  close
)
</script>

<template>
  <div ref="rootRef" class="relative">
    <!-- role=combobox(R2 评审): aria-activedescendant 需挂在组合部件角色上才被 AT 采纳,
         button 角色不承载该属性 —— 补 combobox 后与原生 select 的读屏暴露方式一致
         (aria-expanded/aria-haspopup="listbox"/aria-activedescendant 即 APG select-only 组合框模式) -->
    <button
      ref="triggerRef"
      type="button"
      role="combobox"
      :disabled="props.disabled"
      :aria-label="props.ariaLabel"
      aria-haspopup="listbox"
      :aria-expanded="isOpen"
      :aria-activedescendant="activeDescendant"
      :data-focusable="props.disabled ? undefined : 'true'"
        class="jc-field w-full flex items-center justify-between gap-[var(--jc-space-2)] bg-elevated text-primary border border-ctrl rounded-[var(--jc-ctrl-radius)] outline-none focus:border-ctrl-focus focus:shadow-focus transition disabled:opacity-50 disabled:cursor-not-allowed"
      :class="sizeClass"
      @click="onTriggerClick"
      @keydown="onTriggerKeydown"
    >
      <!-- trigger 槽接管全部内容(FilmListView 图标+label+chevron 组合); 默认 = label + chevron -->
      <slot name="trigger" :selected="selected">
        <span class="truncate" :class="{ 'text-muted': !selected }">{{ triggerLabel }}</span>
        <BaseIcon name="chevron-down" size="14px" class="shrink-0 text-secondary" />
      </slot>
    </button>

    <!-- 浮层: 默认内联 absolute(TV 空间导航按 DOM 几何寻焦, D⑧); teleport 时挂 body + fixed -->
    <Teleport to="body" :disabled="!props.teleport">
      <!-- 全屏遮罩仅 teleport 态渲染: 拦截触发钮再点的 click, 防"外点关闭→click 重开" -->
      <div v-if="isOpen && props.teleport" class="fixed inset-0 z-[var(--jc-z-overlay,900)]" data-select-backdrop aria-hidden="true" />
      <div
        v-if="isOpen"
        ref="panelRef"
        role="listbox"
        :aria-label="props.ariaLabel"
        class="max-h-[320px] overflow-auto rounded-lg border border-subtle bg-surface shadow-lg py-1"
        :class="props.teleport ? 'fixed z-[var(--jc-z-modal,1000)]' : 'absolute top-full left-0 mt-1 z-50 w-full'"
        :style="props.teleport ? teleportPos : undefined"
        @click.stop
      >
        <!-- 双指示取舍(R2 评审, 待 TV 实测): 选项带 data-focusable(TV 空间导航可把 DOM 焦点移入选项,
             键盘焦点环落在聚焦项)而键盘高亮是 activeIndex 的 bg 类 —— 指针/TV 直选聚焦项时 Enter 走
             原生 click 确认聚焦项, 与 activeIndex 高亮可短暂分叉呈双高亮; 结果一致(确认的都是聚焦项),
             若实测分叉观感明显, 再在选项聚焦时同步 activeIndex -->
        <button
          v-for="(opt, i) in props.options"
          :key="String(opt.value)"
          :id="`jc-opt-${uid}-${i}`"
          type="button"
          role="option"
          :aria-selected="isSelected(opt)"
          :data-focusable="true"
              class="w-full text-left px-[var(--jc-ctrl-px)] py-[var(--jc-ctrl-py)] text-[length:var(--jc-ctrl-fs)] hover:bg-[rgba(255,255,255,0.06)]"
          :class="{
            'text-[var(--jc-brand-purple)]': isSelected(opt),
            'bg-[rgba(255,255,255,0.06)]': i === activeIndex
          }"
          @click="confirmIndex(i)"
        >
          <!-- option 槽: 定制选项内容(图标等); 作用域 = option/active(键盘)/selected(选中) -->
          <slot name="option" :option="opt" :active="i === activeIndex" :selected="isSelected(opt)">
            {{ opt.label }}
          </slot>
        </button>
      </div>
    </Teleport>
  </div>
</template>

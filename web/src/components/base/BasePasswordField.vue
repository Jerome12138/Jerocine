<script setup lang="ts">
import { computed, ref } from 'vue'
import BaseIcon from './BaseIcon.vue'

/**
 * 密码输入框(W1b): 锁图标 + 密码 input + 可见性眼睛按钮(type 切换)。
 * - 外壳(jc-tv-input/auth 外层、ManageFormField、页面容器)由调用方保留, 本件只收编
 *   「图标+眼睛+type 切换」的 5 处复制(Login TV/桌面、Register、Activate、Reset);
 * - 可见性状态归页面(v-model:visible): Register/Activate/Reset 的确认密码框跟随同一
 *   showPwd, 不能收进组件内部;
 * - 其余 attrs(placeholder/autocomplete/maxlength/@keydown.enter/@change/@focus/
 *   tabindex)全部透传到内部 input; data-focusable 固定为 true(5 处原样);
 * - defineExpose({ input }): 登录页 syncAutofill 需要读原生 input 值(浏览器 autofill
 *   不触发 v-model 的 input event, 提交时从 DOM 兜底同步)。
 * variant:
 *   - desktop(默认): 圆角胶囊输入 + 左锁右眼(uno 工具类, 4 页桌面同款);
 *   - tv: LoginView TV 分支原样(jc-tv-login__wrap/__ic/__ic-r + jc-tv-input);
 *     TV 样式规则随迁本件([data-mode='tv'] 前缀、值逐字不变; LoginView 保留同值规则
 *     供用户名输入框使用, __ic-r 为密码眼睛独有故随迁删除)。
 */
defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    modelValue: string
    /** 可见性(v-model:visible) —— 页面级状态 */
    visible: boolean
    variant?: 'desktop' | 'tv'
  }>(),
  { variant: 'desktop' }
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'update:visible': [value: boolean]
}>()

const inputType = computed(() => (props.visible ? 'text' : 'password'))

const inputEl = ref<HTMLInputElement | null>(null)
defineExpose({ input: inputEl })
</script>

<template>
  <div :class="variant === 'tv' ? 'jc-tv-login__wrap' : 'relative'">
    <BaseIcon
      name="lock"
      :size="variant === 'tv' ? '20px' : '18px'"
      :class="
        variant === 'tv'
          ? 'jc-tv-login__ic'
          : 'absolute left-[var(--jc-space-3)] top-1/2 -translate-y-1/2 text-muted'
      "
    />
    <input
      v-bind="$attrs"
      ref="inputEl"
      :value="modelValue"
      :type="inputType"
      :class="
        variant === 'tv'
          ? 'jc-tv-input jc-tv-login__input'
          : 'w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-full)] pl-[var(--jc-space-10)] pr-[var(--jc-space-10)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition'
      "
      data-focusable="true"
      @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
    <button
      type="button"
      :class="
        variant === 'tv'
          ? 'jc-tv-login__ic-r'
          : 'absolute right-[var(--jc-space-3)] top-1/2 -translate-y-1/2 text-muted hover:text-primary'
      "
      :tabindex="variant === 'tv' ? 0 : undefined"
      data-focusable="true"
      aria-label="切换密码可见性"
      @click="emit('update:visible', !visible)"
    >
      <BaseIcon :name="visible ? 'eye-off' : 'eye'" :size="variant === 'tv' ? '20px' : '18px'" />
    </button>
  </div>
</template>

<style scoped>
/* TV 分支样式: 值原样迁移自 LoginView([data-mode='tv'] 前缀保留, 全局 data-mode 属性命中);
   LoginView 同值规则留存(__wrap/__ic/__input 供用户名框), __ic-r 为密码眼睛独有 → 随迁删除。 */
[data-mode='tv'] .jc-tv-login__wrap {
  position: relative;
}

[data-mode='tv'] .jc-tv-login__ic {
  position: absolute;
  left: 16px;
  top: 50%;
  transform: translateY(-50%);
  opacity: 0.7;
  pointer-events: none;
}

[data-mode='tv'] .jc-tv-login__ic-r {
  position: absolute;
  right: 12px;
  top: 50%;
  transform: translateY(-50%);
  opacity: 0.7;
  cursor: pointer;
  background: transparent;
  border: 0;
  color: inherit;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

[data-mode='tv'] .jc-tv-login__input {
  padding-left: 46px;
  padding-right: 46px;
}
</style>

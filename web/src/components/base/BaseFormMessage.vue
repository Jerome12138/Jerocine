<script setup lang="ts">
/**
 * BaseFormMessage — 表单错误行原子（§C8#4）。
 * 收口 auth 五页 6 处手写错误段（Login/Register×2/Activate/Reset/Forgot）,
 * role="alert" 单点化: 无障碍播报语义只在这一处维护。
 *
 * 视觉 = 原手写段逐字: text-xs + danger 色 + 居中; message 空串不渲染
 * （= 原 v-if="errorMsg" 语义）。宿主可经 class 透传追加类（TV 分支
 * jc-tv-login__err 的 TV scoped 规则以更高特异性接管原样视觉）。
 */
withDefaults(
  defineProps<{
    /** 错误文案; 空串/缺省不渲染 */
    message?: string
  }>(),
  { message: '' }
)
</script>

<template>
  <p v-if="message" class="text-xs text-[var(--jc-danger)] text-center" role="alert">{{ message }}</p>
</template>

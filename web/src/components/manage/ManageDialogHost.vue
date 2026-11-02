<script setup lang="ts">
/**
 * ManageDialogHost — 函数化弹窗（openManageDialog）的渲染宿主。
 * 在 App.vue BaseConfirmDialog 同位挂载一次；弹窗形态继承 ManageSheet
 * （desktop modal / mobile sheet|fullsheet 自适应）。
 *
 * 内容组件契约: emits ('confirm', payload) | ('cancel')，footer 自治（Host 不注入）。
 *   - confirm → answerManageDialog({ type: 'confirm', payload }) → openManageDialog resolve(payload)
 *   - cancel  / ESC / 遮罩（ManageSheet update:modelValue=false）→ resolve(null)
 * Host 卸载（路由切走）时仍有待决 → resolve(null) 防悬挂。
 */
import { onBeforeUnmount, onMounted } from 'vue'
import ManageSheet from './ManageSheet.vue'
import {
  answerManageDialog,
  manageDialogState,
  manageDialogHostMounted
} from '@/composables/useManageDialog'

onMounted(() => {
  manageDialogHostMounted.value = true
})

onBeforeUnmount(() => {
  manageDialogHostMounted.value = false
  // 卸载兜底: 待决弹窗以 null 收尾
  if (manageDialogState.resolve) {
    onClose()
  }
})

function onConfirm(payload: unknown): void {
  answerManageDialog({ type: 'confirm', payload })
}

function onClose(): void {
  answerManageDialog({ type: 'close' })
}
</script>

<template>
  <ManageSheet
    :model-value="manageDialogState.visible"
    :title="manageDialogState.title"
    :mobile-mode="manageDialogState.mobileMode"
    :width="manageDialogState.width"
    @update:model-value="(v) => !v && onClose()"
  >
    <component
      :is="manageDialogState.component"
      v-bind="manageDialogState.props"
      @confirm="onConfirm"
      @cancel="onClose"
    />
  </ManageSheet>
</template>

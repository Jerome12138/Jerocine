<script setup lang="ts">
import { nextTick, watch } from 'vue'
import BaseDialog from '@/components/base/BaseDialog.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import { answerConfirm, confirmState } from '@/composables/useConfirm'

// 弹窗打开后自动聚焦"确认"按钮: TV 遥控器/键盘可直接按 OK 确认(无需先用方向键找焦点),
// 续播提示等场景体验更顺; 鼠标端聚焦确认键也是标准行为。延时让 Teleport/过渡渲染完成。
watch(
  () => confirmState.visible,
  (v) => {
    if (!v) return
    nextTick(() => {
      window.setTimeout(() => {
        document.querySelector<HTMLElement>('.gf-confirm-ok')?.focus()
      }, 50)
    })
  }
)
</script>

<template>
  <BaseDialog
    :visible="confirmState.visible"
    :title="confirmState.title"
    :width="'420px'"
    :close-on-overlay="false"
    @update:visible="(v) => !v && answerConfirm(false)"
  >
    <p
      v-if="confirmState.desc"
      class="text-secondary text-[var(--gf-fs-md)] leading-[var(--gf-lh-relaxed)]"
    >
      {{ confirmState.desc }}
    </p>
    <template #footer>
      <BaseButton variant="ghost" @click="answerConfirm(false)">
        {{ confirmState.cancelText }}
      </BaseButton>
      <BaseButton
        class="gf-confirm-ok"
        :variant="confirmState.danger ? 'danger' : 'gradient'"
        @click="answerConfirm(true)"
      >
        {{ confirmState.okText }}
      </BaseButton>
    </template>
  </BaseDialog>
</template>

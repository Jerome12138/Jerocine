<script setup lang="ts">
import { ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { RegisterApplyRow } from '@/types/manage'
import ManageTextarea from '@/components/manage/ManageTextarea.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 注册申请驳回表单 —— openManageDialog 内容组件。
 * 契约: emits ('confirm', payload) | ('cancel'), footer 自带。
 *
 * 校验(必填/≤255)与提交自治: 拦截/失败 toast 后弹窗停留(输入不丢);
 * 驳回先落库再发信, 邮件失败仅 warning 不阻断(成功也 emit confirm → 页面重拉)。
 */
export interface RegisterRejectPayload {
  id: number
  reason: string
}

interface Props {
  target: RegisterApplyRow
}
const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'confirm', payload: RegisterRejectPayload): void
  (e: 'cancel'): void
}>()

const reason = ref('')
const { busy, run } = useAsyncAction()

function cancel(): void {
  emit('cancel')
}

function submit(): void {
  const value = reason.value.trim()
  if (!value) {
    toast('error', '请填写驳回理由（用户可见）')
    return
  }
  if (value.length > 255) {
    toast('error', '驳回理由不能超过 255 字')
    return
  }
  void run({
    action: async () => {
      const r = await manageApi.register.reject(props.target.id, value)
      if (r.warning) toast('warning', `已驳回，但通知邮件发送失败：${r.warning}`)
      else toast('success', `已驳回「${props.target.userName}」`)
    },
    after: () => emit('confirm', { id: props.target.id, reason: value })
  })
}
</script>

<template>
  <form class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="submit">
    <label class="flex flex-col gap-[var(--jc-space-2)]">
      <span class="text-sm text-secondary">驳回理由（必填，用户登录与进度查询可见，≤255 字）</span>
      <ManageTextarea
        v-model="reason"
        :rows="3"
        :maxlength="255"
        placeholder="例如：邀请人信息无法核实 / 请补充联系方式"
      />
    </label>
    <p class="text-xs text-muted">驳回后该邮箱仍可重新提交申请；通知邮件发送失败不影响驳回结果。</p>
    <div class="flex justify-end gap-[var(--jc-space-2)]">
      <ManageButton variant="ghost" @click="cancel">取消</ManageButton>
      <ManageButton variant="gradient" type="submit" :loading="busy">确认驳回</ManageButton>
    </div>
  </form>
</template>

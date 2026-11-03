<script setup lang="ts">
import { ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { ManageUserRow } from '@/types/manage'
import ManageInput from '@/components/manage/ManageInput.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 管理员重置用户密码 —— openManageDialog 内容组件。
 * 契约: emits ('confirm', payload) | ('cancel'), footer 自带。
 * 校验(6-64 位)与提交自治: 失败 toast 后弹窗停留, 成功 emit('confirm')。
 */
export interface UserResetPayload {
  id: number
}

interface Props {
  target: ManageUserRow
}
const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'confirm', payload: UserResetPayload): void
  (e: 'cancel'): void
}>()

const password = ref('')
const { busy, run } = useAsyncAction()

function cancel(): void {
  emit('cancel')
}

function submit(): void {
  if (password.value.length < 6 || password.value.length > 64) {
    toast('error', '密码长度需在 6-64 位之间')
    return
  }
  void run({
    action: () => manageApi.user.resetPassword(props.target.ID, password.value, { silent: true }),
    successMsg: `已重置「${props.target.userName}」的密码, 该用户已全部下线`,
    after: () => emit('confirm', { id: props.target.ID })
  })
}
</script>

<template>
  <form class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="submit">
    <label class="flex flex-col gap-[var(--jc-space-2)]">
      <span class="text-sm text-secondary">新密码(6-64 位, 重置后该用户全部设备下线)</span>
      <ManageInput v-model="password" type="password" placeholder="输入新密码" />
    </label>
    <div class="flex justify-end gap-[var(--jc-space-2)]">
      <BaseButton variant="ghost" size="sm" @click="cancel">取消</BaseButton>
      <BaseButton variant="gradient" size="sm" type="submit" :loading="busy">确认重置</BaseButton>
    </div>
  </form>
</template>

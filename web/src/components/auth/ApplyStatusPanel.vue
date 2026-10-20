<script setup lang="ts">
import { onMounted, ref } from 'vue'
import * as registerApi from '@/api/register'
import type { ApplyStatusView } from '@/types/user'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { toast } from '@/api/http'

/**
 * ApplyStatusPanel 注册申请进度查询（三通道之一, 兜底"邮件进垃圾箱/换邮箱/链接过期"）。
 * 复用点: RegisterView 审核中状态页(回填刚提交的邮箱) / LoginView apply 模式入口。
 * 状态: pending 审核中 / approved 已通过(可重发) / rejected 已驳回+理由 / none 无记录。
 */
const props = withDefaults(defineProps<{ prefillEmail?: string }>(), { prefillEmail: '' })
const emit = defineEmits<{ (e: 'reapply'): void }>()

const email = ref(props.prefillEmail)
const loading = ref(false)
const sending = ref(false)
const view = ref<ApplyStatusView | null>(null)
const errorMsg = ref('')

onMounted(() => {
  // 带预填邮箱(提交成功页)自动查一次, 省一次输入
  if (email.value) void query()
})

async function query(): Promise<void> {
  errorMsg.value = ''
  const e = email.value.trim()
  if (!e) {
    errorMsg.value = '请输入邮箱'
    return
  }
  loading.value = true
  try {
    view.value = await registerApi.applyStatus(e)
  } catch (err: unknown) {
    view.value = null
    errorMsg.value = err instanceof Error ? err.message : '查询失败'
  } finally {
    loading.value = false
  }
}

async function resend(): Promise<void> {
  if (sending.value) return
  sending.value = true
  try {
    await registerApi.resendActivation(email.value.trim())
    toast('success', '激活邮件已重发，请查收邮箱（含垃圾箱）')
    // 刷新状态: 激活完成后 canResend 归 false
    await query()
  } catch (err: unknown) {
    toast('error', err instanceof Error ? err.message : '重发失败')
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-3)]">
    <div class="flex gap-[var(--jc-space-2)]">
      <input
        v-model="email"
        type="email"
        class="flex-1 bg-elevated text-primary border border-default rounded-[var(--jc-radius-full)] px-[var(--jc-space-4)] py-[var(--jc-space-2)] text-sm outline-none focus:border-strong"
        placeholder="输入申请时使用的邮箱"
        autocomplete="email"
        data-focusable="true"
        @keydown.enter="query"
      />
      <BaseButton variant="outline" size="sm" :loading="loading" @click="query">查询进度</BaseButton>
    </div>

    <p v-if="errorMsg" class="text-xs text-[var(--jc-danger)]" role="alert">{{ errorMsg }}</p>

    <div
      v-if="view"
      class="rounded-[var(--jc-radius-md)] border border-subtle bg-elevated/60 p-[var(--jc-space-4)] flex flex-col gap-[var(--jc-space-2)]"
    >
      <!-- 审核中 -->
      <template v-if="view.status === 'pending'">
        <p class="text-sm text-primary font-[var(--jc-fw-medium)] flex items-center gap-[var(--jc-space-2)]">
          <BaseIcon name="clock" size="16px" /> 审核中
        </p>
        <p class="text-xs text-secondary">申请已受理，管理员审核通过后激活链接将发送到该邮箱。</p>
      </template>

      <!-- 已通过 -->
      <template v-else-if="view.status === 'approved'">
        <p class="text-sm text-primary font-[var(--jc-fw-medium)] flex items-center gap-[var(--jc-space-2)]">
          <BaseIcon name="shield" size="16px" /> 已通过
        </p>
        <p class="text-xs text-secondary">激活邮件已发送，请查收邮箱（含垃圾箱），链接 7 天内有效。</p>
        <BaseButton
          v-if="view.canResend"
          variant="ghost"
          size="sm"
          class="self-start"
          :loading="sending"
          @click="resend"
        >
          重发激活邮件
        </BaseButton>
        <p v-else class="text-xs text-muted">该申请已完成激活，可直接登录。</p>
      </template>

      <!-- 已驳回 -->
      <template v-else-if="view.status === 'rejected'">
        <p class="text-sm text-[var(--jc-danger)] font-[var(--jc-fw-medium)]">已驳回</p>
        <p v-if="view.reason" class="text-xs text-secondary">管理员理由：{{ view.reason }}</p>
        <BaseButton variant="outline" size="sm" class="self-start" @click="emit('reapply')">
          重新申请
        </BaseButton>
      </template>

      <!-- 无记录 -->
      <template v-else>
        <p class="text-sm text-secondary">未找到该邮箱的申请记录。</p>
        <BaseButton variant="outline" size="sm" class="self-start" @click="emit('reapply')">
          提交申请
        </BaseButton>
      </template>
    </div>
  </div>
</template>

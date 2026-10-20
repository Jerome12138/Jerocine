<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import * as authApi from '@/api/auth'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import { useViewMode } from '@/composables/useViewMode'

/**
 * ForgotPasswordView 忘记密码(/forgot-password): 输邮箱 → 发一次性重置链接(30 分钟)。
 * 防账号枚举: 邮箱是否存在响应一致, 状态页文案中性化(设计 §5/P3)。
 */
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { isTV } = useViewMode()

const form = reactive({ email: '' })
const loading = ref(false)
const sent = ref(false)
const errorMsg = ref('')

const redirectTo = computed(() => {
  const r = route.query.redirect
  return typeof r === 'string' && r ? r : ''
})

onMounted(() => {
  // 已登录不需要找回密码
  if (userStore.isLoggedIn) void router.replace(redirectTo.value || '/index')
})

async function handleSubmit(): Promise<void> {
  errorMsg.value = ''
  const email = form.email.trim()
  if (!email) {
    errorMsg.value = '请输入邮箱'
    return
  }
  loading.value = true
  try {
    await authApi.requestPasswordReset(email)
    sent.value = true
  } catch (e: unknown) {
    errorMsg.value = e instanceof Error ? e.message : '发送失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <!-- ============ TV: 引导去电脑/手机操作 ============ -->
  <div v-if="isTV" class="jc-tv-glass-card jc-tv-login__card">
    <div class="jc-tv-login__logo text-brand-gradient">Jerocine影视</div>
    <div class="jc-tv-login__sub">找回密码</div>
    <div class="jc-tv-login__note">
      <div class="a">电视端不便输入邮箱。</div>
      <div class="b">请在电脑或手机上访问本站，通过「忘记密码」邮件链接重置。</div>
    </div>
    <div class="jc-tv-login__actions">
      <button
        type="button"
        class="jc-tv-btn primary jc-tv-login__submit"
        data-focusable="true"
        tabindex="0"
        @click="router.replace('/login')"
      >
        返回登录
      </button>
    </div>
  </div>

  <!-- ============ 桌面 / 移动 ============ -->
  <div v-else class="flex flex-col gap-[var(--jc-space-6)]">
    <div class="text-center">
      <h1
        class="text-[length:var(--jc-fs-2xl)] font-[var(--jc-fw-black)] tracking-tight text-brand-gradient"
      >
        找回密码
      </h1>
      <p class="mt-[var(--jc-space-2)] text-secondary text-sm">
        输入注册邮箱，我们将发送一次性重置链接（30 分钟内有效）
      </p>
    </div>

    <form v-if="!sent" class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="handleSubmit">
      <ManageFormField label="邮箱" required>
        <div class="relative">
          <BaseIcon
            name="mail"
            class="absolute left-[var(--jc-space-3)] top-1/2 -translate-y-1/2 text-muted"
            size="18px"
          />
          <input
            v-model="form.email"
            type="email"
            class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-full)] pl-[var(--jc-space-10)] pr-[var(--jc-space-4)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition"
            placeholder="邮箱"
            autocomplete="email"
            data-focusable="true"
          />
        </div>
      </ManageFormField>

      <p v-if="errorMsg" class="text-xs text-[var(--jc-danger)] text-center" role="alert">
        {{ errorMsg }}
      </p>

      <BaseButton variant="gradient" size="lg" :loading="loading" type="submit">
        发送重置链接
      </BaseButton>
    </form>

    <!-- 已发送(中性文案, 不透露邮箱是否注册) -->
    <div
      v-else
      class="rounded-[var(--jc-radius-md)] border border-subtle bg-elevated/60 p-[var(--jc-space-5)] flex flex-col gap-[var(--jc-space-3)]"
    >
      <p class="text-base text-primary font-[var(--jc-fw-semibold)] flex items-center gap-[var(--jc-space-2)]">
        <BaseIcon name="mail" size="18px" /> 重置邮件已发送
      </p>
      <ul class="text-xs text-secondary leading-[var(--jc-lh-relaxed)] list-disc pl-[var(--jc-space-5)] flex flex-col gap-[var(--jc-space-1)]">
        <li>若该邮箱已注册，重置链接已发送，请查收邮箱（别忘了垃圾箱）。</li>
        <li>链接 30 分钟内有效，且只能使用一次。</li>
        <li>没收到？可返回本页重新发送。</li>
      </ul>
      <div class="flex justify-center gap-[var(--jc-space-2)]">
        <BaseButton variant="ghost" size="sm" @click="sent = false">重新发送</BaseButton>
        <BaseButton variant="outline" size="sm" @click="router.replace('/login')">返回登录</BaseButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 复用 LoginView 的 [data-mode='tv'] 体系类(TV 分支)，桌面/移动走 manage/表单体系类。 */
</style>

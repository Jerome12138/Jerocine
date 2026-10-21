<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as authApi from '@/api/auth'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import { useViewMode } from '@/composables/useViewMode'

/**
 * ResetPasswordView 重置密码落地页(/reset-password?token=): 一次性链接(30 分钟)
 * → 设新密码 → 回登录页(该用户全部设备已下线, 需重新登录)。
 * 链接无效/过期/已使用 → 提示回「忘记密码」重新申请。
 */
const route = useRoute()
const router = useRouter()
const { isTV } = useViewMode()

const token = computed(() => String(route.query.token ?? '').trim())

const form = reactive({ password: '', confirm: '' })
const showPwd = ref(false)
const loading = ref(false)
const errorMsg = ref('')

async function handleReset(): Promise<void> {
  errorMsg.value = ''
  if (!token.value) {
    errorMsg.value = '重置链接无效，请从邮件中的链接进入'
    return
  }
  if (form.password.length < 6) {
    errorMsg.value = '密码至少 6 位'
    return
  }
  if (form.password.length > 64) {
    errorMsg.value = '密码最多 64 位'
    return
  }
  if (form.password !== form.confirm) {
    errorMsg.value = '两次输入的密码不一致'
    return
  }
  loading.value = true
  try {
    await authApi.resetPassword({ token: token.value, password: form.password })
    // 成功 → 全部设备已下线, 回登录页(带 reset=1 提示)
    await router.replace('/login?reset=1')
  } catch (e: unknown) {
    // 服务端错误文案均已中文化可读: 链接无效/过期/已使用、密码长度等, 原文直出
    errorMsg.value = e instanceof Error ? e.message : '重置失败'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (!token.value) errorMsg.value = '重置链接无效，请从邮件中的链接进入'
})
</script>

<template>
  <!-- ============ TV: 引导去电脑/手机操作 ============ -->
  <div v-if="isTV" class="jc-tv-glass-card jc-tv-login__card">
    <div class="jc-tv-login__logo text-brand-gradient">Jerocine影视</div>
    <div class="jc-tv-login__sub">重置密码</div>
    <div class="jc-tv-login__note">
      <div class="a">电视端不便设置密码。</div>
      <div class="b">请在电脑或手机上打开邮件中的重置链接，完成设置后即可回来登录。</div>
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
        重置密码
      </h1>
      <p class="mt-[var(--jc-space-2)] text-secondary text-sm">设置新密码，成功后使用新密码重新登录</p>
    </div>

    <form v-if="token" class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="handleReset">
      <ManageFormField label="新密码" required>
        <div class="relative">
          <BaseIcon
            name="lock"
            class="absolute left-[var(--jc-space-3)] top-1/2 -translate-y-1/2 text-muted"
            size="18px"
          />
          <input
            v-model="form.password"
            :type="showPwd ? 'text' : 'password'"
            maxlength="64"
            class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-full)] pl-[var(--jc-space-10)] pr-[var(--jc-space-10)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition"
            placeholder="新密码（至少 6 位）"
            autocomplete="new-password"
            data-focusable="true"
          />
          <button
            type="button"
            class="absolute right-[var(--jc-space-3)] top-1/2 -translate-y-1/2 text-muted hover:text-primary"
            data-focusable="true"
            aria-label="切换密码可见性"
            @click="showPwd = !showPwd"
          >
            <BaseIcon :name="showPwd ? 'eye-off' : 'eye'" size="18px" />
          </button>
        </div>
      </ManageFormField>

      <ManageFormField label="确认新密码" required>
        <div class="relative">
          <BaseIcon
            name="shield"
            class="absolute left-[var(--jc-space-3)] top-1/2 -translate-y-1/2 text-muted"
            size="18px"
          />
          <input
            v-model="form.confirm"
            :type="showPwd ? 'text' : 'password'"
            maxlength="64"
            class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-full)] pl-[var(--jc-space-10)] pr-[var(--jc-space-4)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition"
            placeholder="再输一次新密码"
            autocomplete="new-password"
            data-focusable="true"
          />
        </div>
      </ManageFormField>

      <p v-if="errorMsg" class="text-xs text-[var(--jc-danger)] text-center" role="alert">
        {{ errorMsg }}
      </p>

      <BaseButton variant="gradient" size="lg" :loading="loading" type="submit">
        确认重置
      </BaseButton>
    </form>

    <!-- 无 token(邮件客户端截断链接/直接输网址) -->
    <div
      v-else
      class="rounded-[var(--jc-radius-md)] border border-subtle bg-elevated/60 p-[var(--jc-space-4)] flex flex-col gap-[var(--jc-space-3)]"
    >
      <p class="text-sm text-secondary leading-[var(--jc-lh-relaxed)]">
        重置链接无效或已不完整，请从收到的邮件中重新打开完整链接。
      </p>
      <p class="text-xs text-muted">链接 30 分钟内有效且仅能使用一次；如已过期，可重新申请。</p>
      <div class="flex justify-center gap-[var(--jc-space-2)]">
        <BaseButton variant="outline" size="sm" @click="router.replace('/forgot-password')">
          重新申请
        </BaseButton>
        <BaseButton variant="ghost" size="sm" @click="router.replace('/login')">返回登录</BaseButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 复用 LoginView 的 [data-mode='tv'] 体系类(TV 分支)，桌面/移动走 manage/表单体系类。 */
</style>

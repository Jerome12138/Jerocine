<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import * as registerApi from '@/api/register'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import { useViewMode } from '@/composables/useViewMode'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { isTV } = useViewMode()

/** 注册模式: off(入口不显示) / open(自由注册, 本表单) / apply(申请审核, P2) */
const mode = ref<'open' | 'apply'>('open')
const smtpReady = ref(true)
const configLoading = ref(true)

const form = reactive({ userName: '', password: '', email: '', code: '' })
const showPwd = ref(false)
const loading = ref(false)
const sending = ref(false)
const cooldown = ref(0)
const errorMsg = ref('')
let cooldownTimer: number | undefined

const redirectTo = computed(() => {
  const r = route.query.redirect
  return typeof r === 'string' && r ? r : ''
})

onMounted(async () => {
  try {
    const cfg = await registerApi.getRegisterConfig()
    // off 模式不该进来(登录页不显示入口); 兜底回登录页
    if (cfg.mode === 'off') {
      await router.replace('/login')
      return
    }
    mode.value = cfg.mode
    smtpReady.value = cfg.smtpReady
  } catch {
    // 拉不到配置: 保守按 open 渲染, 提交时由服务端把关
  } finally {
    configLoading.value = false
  }
})

function startCooldown(): void {
  cooldown.value = 60
  const tick = (): void => {
    cooldown.value -= 1
    if (cooldown.value <= 0) {
      cooldown.value = 0
      if (cooldownTimer) window.clearInterval(cooldownTimer)
      cooldownTimer = undefined
    }
  }
  cooldownTimer = window.setInterval(tick, 1000)
}

async function handleSendCode(): Promise<void> {
  errorMsg.value = ''
  const email = form.email.trim()
  if (!email) {
    errorMsg.value = '请输入邮箱'
    return
  }
  if (cooldown.value > 0 || sending.value) return
  sending.value = true
  try {
    await registerApi.sendRegisterCode(email)
    startCooldown()
  } catch (e: unknown) {
    errorMsg.value = e instanceof Error ? e.message : '验证码发送失败'
  } finally {
    sending.value = false
  }
}

async function handleRegister(): Promise<void> {
  errorMsg.value = ''
  if (!form.userName.trim()) {
    errorMsg.value = '请输入用户名'
    return
  }
  if (form.password.length < 6) {
    errorMsg.value = '密码至少 6 位'
    return
  }
  if (!form.email.trim()) {
    errorMsg.value = '请输入邮箱'
    return
  }
  if (!form.code.trim()) {
    errorMsg.value = '请输入验证码'
    return
  }
  loading.value = true
  try {
    const res = await registerApi.registerAccount({
      userName: form.userName.trim(),
      password: form.password,
      email: form.email.trim(),
      code: form.code.trim()
    })
    userStore.setToken(res.token, res.expires)
    try {
      await userStore.fetchInfo()
    } catch {
      /* 拉资料失败不阻塞跳转, 最小信息已由 token 携带 */
    }
    await router.replace(redirectTo.value || '/index')
  } catch (e: unknown) {
    errorMsg.value = e instanceof Error ? e.message : '注册失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <!-- ============ TV: 提示去手机/电脑注册(遥控器输邮箱+验证码体验不可用) ============ -->
  <div v-if="isTV" class="jc-tv-glass-card jc-tv-login__card">
    <div class="jc-tv-login__logo text-brand-gradient">Jerocine影视</div>
    <div class="jc-tv-login__sub">注册账号</div>
    <div class="jc-tv-login__note">
      <div class="a">电视端不便输入邮箱与验证码。</div>
      <div class="b">请在电脑或手机上访问本站完成注册，之后可用扫码或账号密码登录。</div>
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
        注册 Jerocine影视
      </h1>
      <p class="mt-[var(--jc-space-2)] text-secondary text-sm">
        {{
          mode === 'apply'
            ? '提交注册申请，管理员审核通过后即可使用'
            : '注册后可同步观看历史 / 收藏 / 进入后台管理'
        }}
      </p>
    </div>

    <!-- 申请注册(apply)模式: P2 审核链路, 本版本先行占位提示 -->
    <div
      v-if="mode === 'apply'"
      class="rounded-[var(--jc-radius-md)] border border-subtle bg-elevated/60 p-[var(--jc-space-4)] text-center"
    >
      <p class="text-secondary text-sm leading-[var(--jc-lh-relaxed)]">申请注册通道即将开放。</p>
      <p class="text-muted text-xs mt-[var(--jc-space-2)]">
        如需账号请联系管理员开通；管理员可在后台「系统管理 → 用户管理」创建账号。
      </p>
      <BaseButton class="mt-[var(--jc-space-4)]" variant="ghost" size="sm" @click="router.replace('/login')">
        返回登录
      </BaseButton>
    </div>

    <!-- 自由注册(open)表单 -->
    <form v-else class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="handleRegister">
      <ManageFormField label="用户名" required>
        <div class="relative">
          <BaseIcon
            name="user"
            class="absolute left-[var(--jc-space-3)] top-1/2 -translate-y-1/2 text-muted"
            size="18px"
          />
          <input
            v-model="form.userName"
            type="text"
            maxlength="32"
            class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-full)] pl-[var(--jc-space-10)] pr-[var(--jc-space-4)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition"
            placeholder="用户名"
            autocomplete="username"
            data-focusable="true"
          />
        </div>
      </ManageFormField>

      <ManageFormField label="密码" required>
        <div class="relative">
          <BaseIcon
            name="lock"
            class="absolute left-[var(--jc-space-3)] top-1/2 -translate-y-1/2 text-muted"
            size="18px"
          />
          <input
            v-model="form.password"
            :type="showPwd ? 'text' : 'password'"
            class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-full)] pl-[var(--jc-space-10)] pr-[var(--jc-space-10)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition"
            placeholder="密码（至少 6 位）"
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

      <ManageFormField label="验证码" required>
        <div class="flex gap-[var(--jc-space-2)]">
          <div class="relative flex-1">
          <BaseIcon
            name="shield"
            class="absolute left-[var(--jc-space-3)] top-1/2 -translate-y-1/2 text-muted"
            size="18px"
          />
            <input
              v-model="form.code"
              type="text"
              inputmode="numeric"
              maxlength="6"
              class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-full)] pl-[var(--jc-space-10)] pr-[var(--jc-space-4)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition"
              placeholder="6 位验证码"
              autocomplete="one-time-code"
              data-focusable="true"
            />
          </div>
          <BaseButton
            variant="outline"
            size="md"
            :loading="sending"
            :disabled="cooldown > 0 || !form.email.trim()"
            class="shrink-0"
            @click="handleSendCode"
          >
            {{ cooldown > 0 ? `${cooldown}s 后重发` : sending ? '发送中…' : '获取验证码' }}
          </BaseButton>
        </div>
      </ManageFormField>

      <p v-if="errorMsg" class="text-xs text-[var(--jc-danger)] text-center" role="alert">
        {{ errorMsg }}
      </p>

      <p v-if="!smtpReady" class="text-xs text-[var(--jc-warning)] text-center">
        邮件通道未配置，暂时无法注册，请联系管理员。
      </p>

      <BaseButton variant="gradient" size="lg" :loading="loading" type="submit"> 注册 </BaseButton>
    </form>

    <p class="text-muted text-xs text-center">
      已有账号？
      <button
        type="button"
        class="hover:text-primary underline"
        data-focusable="true"
        @click="router.replace('/login')"
      >
        直接登录
      </button>
    </p>
  </div>
</template>

<style scoped>
/* RegisterView 复用 LoginView 的 [data-mode='tv'] 体系类(登录卡/note/按钮)，不另起炉灶。
   样式定义在 LoginView.vue 的 <style> 中(全局 [data-mode='tv'] 作用域)，此处无需重复。 */
</style>

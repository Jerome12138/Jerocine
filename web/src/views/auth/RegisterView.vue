<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import * as registerApi from '@/api/register'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ApplyStatusPanel from '@/components/auth/ApplyStatusPanel.vue'
import { useViewMode } from '@/composables/useViewMode'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { isTV } = useViewMode()

/** 注册模式: off(入口不显示) / open(自由注册, 本表单) / apply(申请审核, 无密码) */
const mode = ref<'open' | 'apply'>('open')
const smtpReady = ref(true)
const configLoading = ref(true)

const form = reactive({ userName: '', password: '', email: '', code: '', reason: '' })
const showPwd = ref(false)
const loading = ref(false)
const sending = ref(false)
const cooldown = ref(0)
const errorMsg = ref('')
/** apply 模式: 提交成功 → 审核中状态页(含进度查询) */
const submitted = ref(false)
let cooldownTimer: number | undefined

/** 对外不可注册: 邮件通道未就绪(或配置拉取失败按不可用处理前先保守渲染)。
 *  只给用户看「暂不支持」, 不暴露 SMTP/通道这类技术细节 —— 具体原因由后端接口返回给调用方。 */
const unavailable = computed(() => !configLoading.value && !smtpReady.value)

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

onUnmounted(() => {
  if (cooldownTimer) window.clearInterval(cooldownTimer)
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
  // 与服务端 regPasswordMax 对齐: 超长会被后端按无效参数拒(英文 invalid argument), 前端先拦
  if (form.password.length > 64) {
    errorMsg.value = '密码最多 64 位'
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

/**
 * apply 模式提交申请: 不收密码, 成功即进「审核中」状态页(设计 §9.1)。
 * 邮箱已在途(pending/approved 未激活) → 403 审核中提示, 原文透出。
 */
async function handleApply(): Promise<void> {
  errorMsg.value = ''
  if (!form.userName.trim()) {
    errorMsg.value = '请输入用户名'
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
    await registerApi.applyRegister({
      userName: form.userName.trim(),
      email: form.email.trim(),
      reason: form.reason.trim() || undefined,
      code: form.code.trim()
    })
    submitted.value = true
  } catch (e: unknown) {
    errorMsg.value = e instanceof Error ? e.message : '申请失败'
  } finally {
    loading.value = false
  }
}

/** 重新申请(从审核中页/驳回态返回表单) */
function resetApply(): void {
  submitted.value = false
  errorMsg.value = ''
}
</script>

<template>
  <!-- ============ TV: 提示去手机/电脑注册(遥控器输邮箱+验证码体验不可用) ============ -->
  <div v-if="isTV" class="jc-tv-glass-card jc-tv-login__card">
    <div class="jc-tv-login__logo text-brand-gradient">Jerocine影视</div>
    <div class="jc-tv-login__sub">注册账号</div>
    <div class="jc-tv-login__note">
      <div class="a">{{ smtpReady ? '电视端不便输入邮箱与验证码。' : '暂不支持注册。' }}</div>
      <div class="b">
        {{
          smtpReady
            ? '请在电脑或手机上访问本站完成注册，之后可用扫码或账号密码登录。'
            : '本站当前未开放注册，请稍后再试。'
        }}
      </div>
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

    <!-- 邮件通道未就绪: 对外只说「暂不支持注册」, 不暴露 SMTP 之类的技术细节 -->
    <div
      v-if="unavailable"
      class="rounded-[var(--jc-radius-md)] border border-subtle bg-elevated/60 p-[var(--jc-space-5)] flex flex-col gap-[var(--jc-space-3)] text-center"
    >
      <p class="text-base text-primary font-[var(--jc-fw-semibold)]">暂不支持注册</p>
      <p class="text-sm text-secondary leading-[var(--jc-lh-relaxed)]">
        本站当前未开放注册，请稍后再试。
      </p>
      <div class="flex justify-center">
        <BaseButton variant="ghost" size="sm" @click="router.replace('/login')">
          返回登录
        </BaseButton>
      </div>
    </div>

    <!-- 申请注册(apply)表单: 用户名/邮箱/验证码/选填理由 —— 刻意无密码, 激活时才设 -->
    <form
      v-if="!unavailable && mode === 'apply' && !submitted"
      class="flex flex-col gap-[var(--jc-space-4)]"
      @submit.prevent="handleApply"
    >
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

      <ManageFormField label="申请理由" hint="选填：自述或邀请人，管理员审核参考（可空）">
        <textarea
          v-model="form.reason"
          rows="3"
          maxlength="500"
          class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-4)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition resize-y"
          placeholder="例如：朋友 XXX 邀请来的 / 想追的剧在这里"
          data-focusable="true"
        />
      </ManageFormField>

      <p v-if="errorMsg" class="text-xs text-[var(--jc-danger)] text-center" role="alert">
        {{ errorMsg }}
      </p>

      <BaseButton variant="gradient" size="lg" :loading="loading" type="submit">
        提交申请
      </BaseButton>
    </form>

    <!-- 申请提交成功 → 审核中状态页(设计 §6: 三条预期管理) -->
    <div v-else-if="!unavailable && mode === 'apply'" class="flex flex-col gap-[var(--jc-space-4)]">
      <div
        class="rounded-[var(--jc-radius-md)] border border-subtle bg-elevated/60 p-[var(--jc-space-5)] flex flex-col gap-[var(--jc-space-3)]"
      >
        <p class="text-base text-primary font-[var(--jc-fw-semibold)] flex items-center gap-[var(--jc-space-2)]">
          <BaseIcon name="mail" size="18px" /> 申请已提交，等待审核
        </p>
        <ul class="text-xs text-secondary leading-[var(--jc-lh-relaxed)] list-disc pl-[var(--jc-space-5)] flex flex-col gap-[var(--jc-space-1)]">
          <li>审核结果将通过邮件通知，请查收邮箱（别忘了垃圾箱）。</li>
          <li>一般是人工审核，通常数天内回复，请耐心等待。</li>
          <li>可随时回站，用申请邮箱查询进度或重收激活邮件。</li>
        </ul>
      </div>

      <ApplyStatusPanel :prefill-email="form.email" @reapply="resetApply" />

      <div class="flex justify-center">
        <BaseButton variant="ghost" size="sm" @click="router.replace('/login')">
          返回登录
        </BaseButton>
      </div>
    </div>

    <!-- 自由注册(open)表单 -->
    <form v-else-if="!unavailable" class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="handleRegister">
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
            maxlength="64"
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

      <BaseButton variant="gradient" size="lg" :loading="loading" type="submit"> 注册 </BaseButton>
    </form>

    <p v-if="mode === 'open'" class="text-muted text-xs text-center">
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
   样式定义在 LoginView.vue 的 <style> 中(全局 [data-mode='tv'] 作用域)，此处无需重复。
   ApplyStatusPanel 为桌面/移动组件，TV 端注册引导不变。 */
</style>

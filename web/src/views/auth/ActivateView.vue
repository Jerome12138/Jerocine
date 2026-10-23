<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import * as registerApi from '@/api/register'
import { ApiError } from '@/types/api'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import { useViewMode } from '@/composables/useViewMode'
import { getLastUserName } from '@/utils/lastUsername'

/**
 * ActivateView 激活页(/activate?token=): 一次性激活链接到站 → 设密码 → 自动登录。
 * 用户名窗口期(审批通过→激活之间)可能被 open 注册者占用 → 409 时就地改名重试,
 * token 仍有效(设计 §9.3: peek 只读校验, 事务成功后才消费)。
 * 用户名失焦查重与注册页同款(可用/占用即时反馈); 进页默认回填上次注册/申请填报的用户名。
 */
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { isTV } = useViewMode()

const token = computed(() => String(route.query.token ?? '').trim())

const form = reactive({ userName: '', password: '', confirm: '' })
const showPwd = ref(false)
const loading = ref(false)
const errorMsg = ref('')
/** 409 用户名被占 → 高亮用户名框提示改名(token 不失效, 可重试) */
const nameTaken = ref(false)

// ---- 用户名失焦查重(与 RegisterView 同款): 可用/占用在输入框下即时显示 ----
const nameState = ref<'idle' | 'checking' | 'ok' | 'taken'>('idle')

async function checkUserName(): Promise<void> {
  const name = form.userName.trim()
  if (!name) {
    nameState.value = 'idle'
    return
  }
  nameState.value = 'checking'
  try {
    nameState.value = (await registerApi.checkUserName(name)).available ? 'ok' : 'taken'
  } catch {
    nameState.value = 'idle' // 接口异常不打扰填写, 提交时服务端还会把关
  }
}

// 用户再次编辑用户名 → 旧结论作废
watch(
  () => form.userName,
  () => {
    nameState.value = 'idle'
  }
)

const redirectTo = computed(() => {
  const r = route.query.redirect
  return typeof r === 'string' && r ? r : ''
})

onMounted(() => {
  // 默认回填上次注册/申请填报的用户名(同设备同浏览器)
  form.userName = getLastUserName()
  // 已登录再进激活页无意义: 直接去首页
  if (userStore.isLoggedIn) void router.replace(redirectTo.value || '/index')
})

async function handleActivate(): Promise<void> {
  errorMsg.value = ''
  nameTaken.value = false
  if (!token.value) {
    errorMsg.value = '激活链接无效，请从邮件中的链接进入'
    return
  }
  if (!form.userName.trim()) {
    errorMsg.value = '请输入用户名'
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
    const res = await registerApi.activateAccount({
      token: token.value,
      userName: form.userName.trim(),
      password: form.password
    })
    userStore.setToken(res.token, res.expires)
    try {
      await userStore.fetchInfo()
    } catch {
      /* 拉资料失败不阻塞跳转, 最小信息已由 token 携带 */
    }
    await router.replace(redirectTo.value || '/index')
  } catch (e: unknown) {
    if (e instanceof ApiError && e.status === 409) {
      // 409 两种: 用户名窗口期被占(ErrUserNameTaken)/ 邮箱被 open 注册者占用(ErrEmailTaken)
      if (e.message.includes('用户名')) {
        // token 未被消费, 换名重试即可
        errorMsg.value = '该用户名已被占用，请更换后重试（激活链接仍然有效）'
        nameTaken.value = true
      } else {
        errorMsg.value = '该邮箱已被注册，请联系管理员处理'
      }
    } else {
      errorMsg.value = e instanceof Error ? e.message : '激活失败'
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <!-- ============ TV: 引导去电脑/手机完成激活(设密码+确认不适合遥控器) ============ -->
  <div v-if="isTV" class="jc-tv-glass-card jc-tv-login__card">
    <div class="jc-tv-login__logo text-brand-gradient">Jerocine影视</div>
    <div class="jc-tv-login__sub">激活账号</div>
    <div class="jc-tv-login__note">
      <div class="a">电视端不便设置密码。</div>
      <div class="b">请在电脑或手机上打开邮件中的激活链接，完成设置后即可回来登录。</div>
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
        激活账号
      </h1>
      <p class="mt-[var(--jc-space-2)] text-secondary text-sm">
        设置密码即完成注册，完成后自动登录
      </p>
    </div>

    <!-- 链接缺 token: 多半是邮件客户端截断了链接 -->
    <div
      v-if="!token"
      class="rounded-[var(--jc-radius-md)] border border-subtle bg-elevated/60 p-[var(--jc-space-4)] flex flex-col gap-[var(--jc-space-3)]"
    >
      <p class="text-sm text-secondary leading-[var(--jc-lh-relaxed)]">
        激活链接无效或已不完整，请从收到的邮件中重新打开完整链接。
      </p>
      <p class="text-xs text-muted">
        若链接已过期（7 天有效），可回登录页用申请邮箱查询进度并重收激活邮件。
      </p>
      <div class="flex justify-center">
        <BaseButton variant="ghost" size="sm" @click="router.replace('/login')">
          返回登录
        </BaseButton>
      </div>
    </div>

    <form v-else class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="handleActivate">
      <ManageFormField
        label="用户名"
        required
        :error="nameTaken ? '该用户名已被占用，请更换后重试（激活链接仍然有效）' : ''"
        :hint="nameState === 'idle' && !nameTaken ? '申请时填报的用户名；如被占用可在此更换' : ''"
      >
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
            :class="nameTaken ? 'border-[var(--jc-danger)]' : ''"
            placeholder="用户名"
            autocomplete="username"
            data-focusable="true"
            @blur="checkUserName"
          />
        </div>
        <p
          v-if="nameState !== 'idle'"
          class="mt-[var(--jc-space-1)] pl-[var(--jc-space-2)] text-xs"
          :class="{
            'text-secondary': nameState === 'checking',
            'text-[var(--jc-success)]': nameState === 'ok',
            'text-[var(--jc-danger)]': nameState === 'taken'
          }"
          role="status"
        >
          {{
            nameState === 'checking'
              ? '正在检查用户名…'
              : nameState === 'ok'
                ? '该用户名可用'
                : '该用户名已被占用，换一个试试'
          }}
        </p>
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

      <ManageFormField label="确认密码" required>
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
            placeholder="再输一次密码"
            autocomplete="new-password"
            data-focusable="true"
          />
        </div>
      </ManageFormField>

      <p v-if="errorMsg" class="text-xs text-[var(--jc-danger)] text-center" role="alert">
        {{ errorMsg }}
      </p>

      <BaseButton variant="gradient" size="lg" :loading="loading" type="submit">
        完成设置并登录
      </BaseButton>
    </form>
  </div>
</template>

<style scoped>
/* 复用 LoginView 的 [data-mode='tv'] 体系类(TV 分支)，桌面/移动走 manage/表单体系类。 */
</style>

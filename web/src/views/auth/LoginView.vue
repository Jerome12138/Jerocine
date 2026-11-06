<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import QRCode from 'qrcode'
import { useUserStore } from '@/stores/user'
import * as deviceApi from '@/api/device'
import * as registerApi from '@/api/register'
import { ApiError } from '@/types/api'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseFormMessage from '@/components/base/BaseFormMessage.vue'
import BasePasswordField from '@/components/base/BasePasswordField.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ApplyStatusPanel from '@/components/auth/ApplyStatusPanel.vue'
import TvOnScreenKeyboard from '@/components/film/TvOnScreenKeyboard.vue'
import { useViewMode } from '@/composables/useViewMode'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { isTV } = useViewMode()

/** TV 渲染组件(方案 B 拆分): TV 模板 / 同屏软键盘状态全在 LoginTv.vue, 仅 isTV 时挂载 */
const LoginTv = defineAsyncComponent(() => import('./LoginTv.vue'))

const form = reactive({ username: '', password: '' })
const showPwd = ref(false)
const loading = ref(false)
const errorMsg = ref('')

/**
 * TV 分支输入框的模板 ref(由渲染组件 LoginTv 的回调 prop 回传)。
 * 桌面分支用下方 syncAutofill 区域的 usernameInput / passwordInput;
 * syncAutofill 取「当前实际挂载的那一支」。
 */
const tvUsernameInput = ref<HTMLInputElement | null>(null)
const tvPasswordInput = ref<{ input: HTMLInputElement | null } | null>(null)

function setTvUsernameRef(el: unknown): void {
  tvUsernameInput.value = (el as HTMLInputElement | null) ?? null
}

function setTvPasswordRef(el: unknown): void {
  tvPasswordInput.value = (el as { input: HTMLInputElement | null } | null) ?? null
}

// 浏览器 autofill 不触发 v-model 的 input event, 提交时从 DOM 兜底同步
const usernameInput = ref<HTMLInputElement | null>(null)
// 密码框已组件化(BasePasswordField), ref 拿到组件实例, 原生 input 经 expose 的 input 取
const passwordInput = ref<{ input: HTMLInputElement | null } | null>(null)

function syncAutofill(): void {
  // TV 与桌面分支各持自己的模板 ref, 取当前实际挂载的那一支
  const u = usernameInput.value ?? tvUsernameInput.value
  const p = passwordInput.value ?? tvPasswordInput.value
  if (u && !form.username) form.username = u.value
  if (p?.input && !form.password) form.password = p.input.value
}

const redirectTo = computed(() => {
  const r = route.query.redirect
  return typeof r === 'string' && r ? r : ''
})

/** 注册入口可见性: off 下线 / open 自由注册 / apply 申请审核。拉取失败按 off 保守处理(不显示入口) */
const registerMode = ref<'off' | 'open' | 'apply'>('off')

/** apply 模式: 「查询注册申请进度」面板展开态(输邮箱 → 状态/重发/重申) */
const showApplyQuery = ref(false)

async function loadRegisterMode(): Promise<void> {
  try {
    const cfg = await registerApi.getRegisterConfig()
    registerMode.value = cfg.mode
  } catch {
    /* 拉不到配置: 保持 off, 不显示注册入口 */
  }
}

/** 用户已登录 → 兜底跳转：优先 redirect，否则按 role 决定 */
function redirectAfterLogin(): string {
  if (redirectTo.value) return redirectTo.value
  return userStore.isAdmin ? '/manage/index' : '/index'
}

onMounted(async () => {
  void loadRegisterMode()
  if (!userStore.isLoggedIn) return
  // 旧 token 可能没拉过 info，先尝试取一次 role 再决定跳哪里
  if (!userStore.info) {
    try {
      await userStore.fetchInfo()
    } catch {
      // 拉不到（token 失效）：拦截器会清 token，留在登录页
      return
    }
  }
  await router.replace(redirectAfterLogin())
})

// ===== 扫码登录(设备码流程) =====
const mode = ref<'pwd' | 'qr'>('pwd')
const qr = reactive({
  dataUrl: '',
  userCode: '',
  status: '' as '' | 'pending' | 'ok' | 'expired',
  loading: false
})
let qrDeviceCode = ''
let qrTimer: number | undefined
let qrIntervalMs = 3000

async function startQrLogin(): Promise<void> {
  stopQrPoll()
  qr.loading = true
  qr.status = 'pending'
  qr.dataUrl = ''
  try {
    const r = await deviceApi.deviceCode()
    qrDeviceCode = r.deviceCode
    qr.userCode = r.userCode
    qrIntervalMs = Math.max(1500, (r.interval || 3) * 1000)
    const url = `${window.location.origin}/device?code=${encodeURIComponent(r.userCode)}`
    qr.dataUrl = await QRCode.toDataURL(url, { width: 220, margin: 1 })
    scheduleQrPoll()
  } catch {
    qr.status = 'expired'
  } finally {
    qr.loading = false
  }
}

function scheduleQrPoll(): void {
  qrTimer = window.setTimeout(pollOnce, qrIntervalMs)
}

async function pollOnce(): Promise<void> {
  if (!qrDeviceCode || mode.value !== 'qr') return
  try {
    const r = await deviceApi.devicePoll(qrDeviceCode)
    if (r.status === 'ok' && r.token) {
      qr.status = 'ok'
      stopQrPoll()
      userStore.setToken(r.token, r.expires)
      try {
        await userStore.fetchInfo()
      } catch {
        /* 拉资料失败不阻塞跳转 */
      }
      await router.replace(redirectAfterLogin())
      return
    }
    if (r.status === 'expired') {
      void startQrLogin() // 码过期自动换一张
      return
    }
  } catch {
    /* 网络抖动忽略, 继续轮询 */
  }
  scheduleQrPoll()
}

function stopQrPoll(): void {
  if (qrTimer) {
    window.clearTimeout(qrTimer)
    qrTimer = undefined
  }
}

function switchMode(m: 'pwd' | 'qr'): void {
  if (mode.value === m) return
  mode.value = m
  if (m === 'qr') void startQrLogin()
  else stopQrPoll()
}

onUnmounted(stopQrPoll)

async function handleLogin(): Promise<void> {
  errorMsg.value = ''
  syncAutofill()
  if (!form.username.trim()) {
    errorMsg.value = '请输入用户名 / 邮箱'
    return
  }
  if (!form.password) {
    errorMsg.value = '请输入密码'
    return
  }
  loading.value = true
  try {
    await userStore.login({
      username: form.username.trim(),
      password: form.password
    })
    // login 内部已 fetchInfo，role 就位
    await router.replace(redirectAfterLogin())
  } catch (e: unknown) {
    let msg = e instanceof Error ? e.message : '登录失败'
    // apply 模式申请态反馈(仅服务器端 403): 审核中 / 已驳回+理由;
    // 驳回时补一句重申入口(design §9.4 三通道之三)。
    if (e instanceof ApiError && e.status === 403 && msg.includes('驳回')) {
      msg += '，可重新提交申请'
    }
    errorMsg.value = msg
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <!-- ============ TV: 雷鸟卡片式登录 ============ -->
  <!-- 渲染组件(方案 B): TV 模板 / 同屏软键盘聚焦状态全在 LoginTv.vue, 此处只做状态供给。
       form.username / form.password / showPwd 经 v-model 双向共用同一份;
       TV 输入框的模板 ref 经回调 prop 回传(浏览器 autofill 兜底 syncAutofill 用)。 -->
  <LoginTv
    v-if="isTV"
    v-model:username="form.username"
    v-model:password="form.password"
    v-model:visible="showPwd"
    :mode="mode"
    :error-msg="errorMsg"
    :loading="loading"
    :register-mode="registerMode"
    :redirect-to="redirectTo"
    :qr="qr"
    :on-switch-mode="switchMode"
    :on-login="handleLogin"
    :on-refresh-qr="startQrLogin"
    :on-username-ref="setTvUsernameRef"
    :on-password-ref="setTvPasswordRef"
  />

  <!-- ============ 桌面 / 移动 (原样) ============ -->
  <div v-else class="flex flex-col gap-[var(--jc-space-6)]">
    <div class="text-center">
      <h1
        class="text-[length:var(--jc-fs-2xl)] font-[var(--jc-fw-black)] tracking-tight text-brand-gradient"
      >
        登录 Jerocine影视
      </h1>
      <p class="mt-[var(--jc-space-2)] text-secondary text-sm">
        登录后可同步观看历史 / 收藏
      </p>
    </div>

    <!-- 登录方式切换 -->
    <div
      class="flex gap-[var(--jc-space-1)] p-[var(--jc-space-1)] bg-elevated rounded-[var(--jc-radius-full)]"
    >
      <button
        type="button"
        class="flex-1 py-[var(--jc-space-2)] rounded-[var(--jc-radius-full)] text-sm font-[var(--jc-fw-medium)] transition"
        :class="mode === 'pwd' ? 'text-white' : 'text-secondary'"
        :style="mode === 'pwd' ? { backgroundImage: 'var(--jc-brand-gradient)' } : {}"
        data-focusable="true"
        @click="switchMode('pwd')"
      >
        密码登录
      </button>
      <button
        type="button"
        class="flex-1 py-[var(--jc-space-2)] rounded-[var(--jc-radius-full)] text-sm font-[var(--jc-fw-medium)] transition"
        :class="mode === 'qr' ? 'text-white' : 'text-secondary'"
        :style="mode === 'qr' ? { backgroundImage: 'var(--jc-brand-gradient)' } : {}"
        data-focusable="true"
        @click="switchMode('qr')"
      >
        扫码登录
      </button>
    </div>

    <!-- 扫码登录 -->
    <div
      v-if="mode === 'qr'"
      class="flex flex-col items-center gap-[var(--jc-space-4)] py-[var(--jc-space-2)]"
    >
      <p class="text-secondary text-sm text-center">
        用已登录的手机扫码，确认后此设备自动登录
      </p>
      <div
        class="relative w-[220px] h-[220px] flex items-center justify-center bg-white rounded-[var(--jc-radius-md)] overflow-hidden"
      >
        <img v-if="qr.dataUrl" :src="qr.dataUrl" alt="登录二维码" width="220" height="220" />
        <span v-else class="text-[#666] text-sm">{{ qr.loading ? '生成中…' : '加载二维码…' }}</span>
        <div
          v-if="qr.status === 'ok'"
          class="absolute inset-0 bg-black/70 flex items-center justify-center text-[var(--jc-success)] text-base"
        >
          已确认，登录中…
        </div>
      </div>
      <p v-if="qr.userCode" class="text-muted text-xs">
        校验码 <span class="text-primary font-mono tracking-[0.3em]">{{ qr.userCode }}</span>
      </p>
      <button
        type="button"
        class="text-muted text-xs hover:text-primary underline"
        data-focusable="true"
        @click="startQrLogin"
      >
        刷新二维码
      </button>
    </div>

    <form
      v-if="mode === 'pwd'"
      class="flex flex-col gap-[var(--jc-space-4)]"
      @submit.prevent="handleLogin"
    >
      <ManageFormField label="用户名" required>
        <div class="relative">
          <BaseIcon
            name="user"
            class="absolute left-[var(--jc-space-3)] top-1/2 -translate-y-1/2 text-muted"
            size="18px"
          />
          <input
            ref="usernameInput"
            v-model="form.username"
            type="text"
            class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-full)] pl-[var(--jc-space-10)] pr-[var(--jc-space-4)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition"
            placeholder="用户名 / 邮箱"
            autocomplete="username"
            data-focusable="true"
            @change="form.username = ($event.target as HTMLInputElement).value"
          />
        </div>
      </ManageFormField>

      <ManageFormField label="密码" required>
        <BasePasswordField
          ref="passwordInput"
          v-model="form.password"
          v-model:visible="showPwd"
          placeholder="密码"
          autocomplete="current-password"
          @keydown.enter="handleLogin"
          @change="form.password = ($event.target as HTMLInputElement).value"
        />
      </ManageFormField>

      <!-- 忘记密码(P3): 重置成功后由 /reset-password 带 reset=1 跳回, 此处给出提示 -->
      <div class="flex items-center justify-between -mt-[var(--jc-space-2)]">
        <p v-if="route.query.reset === '1'" class="text-xs text-[var(--jc-success)]">
          密码已重置，请使用新密码登录
        </p>
        <button
          type="button"
          class="text-muted text-xs hover:text-primary underline ml-auto"
          data-focusable="true"
          @click="router.push('/forgot-password')"
        >
          忘记密码？
        </button>
      </div>

      <!-- 错误行(§C8#4 收口 BaseFormMessage: role=alert 单点化); TV 分支(上)保留 jc-tv-login__err 原款不动 -->
      <BaseFormMessage :message="errorMsg" />

      <BaseButton variant="gradient" size="lg" :loading="loading" type="submit">
        登录
      </BaseButton>
    </form>

    <!-- 注册入口: off=下线指引; open/apply=去注册页 -->
    <div
      v-if="registerMode === 'off'"
      class="rounded-[var(--jc-radius-md)] border border-subtle bg-elevated/60 p-[var(--jc-space-4)] text-center"
    >
      <p class="text-secondary text-sm leading-[var(--jc-lh-relaxed)]">
        注册功能已暂时下线。
      </p>
      <p class="text-muted text-xs mt-[var(--jc-space-2)]">
        如需账号请联系管理员开通；管理员可在后台「系统管理 → 用户管理」创建账号。
      </p>
    </div>
    <div
      v-else
      class="rounded-[var(--jc-radius-md)] border border-subtle bg-elevated/60 p-[var(--jc-space-4)]"
    >
      <p class="text-secondary text-sm font-[var(--jc-fw-medium)] text-center">
        还没有账号？注册后可享受：
      </p>
      <ul
        class="mt-[var(--jc-space-3)] flex flex-col gap-[var(--jc-space-2)] text-muted text-xs leading-[var(--jc-lh-relaxed)]"
      >
        <li class="flex gap-[var(--jc-space-2)]">
          <span class="shrink-0 text-[var(--jc-brand-cyan)]">›</span>
          观看历史云端同步：手机 / 平板 / TV 进度互通，接着上次继续看
        </li>
        <li class="flex gap-[var(--jc-space-2)]">
          <span class="shrink-0 text-[var(--jc-brand-cyan)]">›</span>
          收藏跨设备一致，换设备不丢
        </li>
        <li class="flex gap-[var(--jc-space-2)]">
          <span class="shrink-0 text-[var(--jc-brand-cyan)]">›</span>
          多设备扫码登录，随时切换设备
        </li>
      </ul>
      <div class="flex justify-center mt-[var(--jc-space-3)]">
        <BaseButton
          variant="outline"
          size="sm"
          @click="router.push('/register')"
        >
          立即注册
        </BaseButton>
      </div>
    </div>

    <!-- apply 模式: 注册申请进度查询(输邮箱 → 状态/重发/重申; 邮件丢了的兜底通道) -->
    <div v-if="registerMode === 'apply'" class="flex flex-col items-center gap-[var(--jc-space-3)]">
      <button
        v-if="!showApplyQuery"
        type="button"
        class="text-muted text-xs hover:text-primary underline"
        data-focusable="true"
        @click="showApplyQuery = true"
      >
        查询注册申请进度
      </button>
      <ApplyStatusPanel v-else @reapply="router.push('/register')" />
    </div>

    <p
      v-if="redirectTo"
      class="text-muted text-xs text-center"
    >
      登录后将跳转至：{{ redirectTo }}
    </p>
  </div>
</template>

<!-- TV 专属样式(P4-a 契约先行)已整块迁出到 src/assets/styles/auth-tv.css(全局引入, 见 main.ts):
     原块被 Register/Activate/Forgot/Reset 四页 + BasePasswordField/BaseFormMessage 跨文件借用,
     留在本页会形成隐式耦合。 -->

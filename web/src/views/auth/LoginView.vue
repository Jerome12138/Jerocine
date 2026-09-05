<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import QRCode from 'qrcode'
import { useUserStore } from '@/stores/user'
import * as deviceApi from '@/api/device'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import TvOnScreenKeyboard from '@/components/film/TvOnScreenKeyboard.vue'
import { useViewMode } from '@/composables/useViewMode'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { isTV } = useViewMode()

const form = reactive({ username: '', password: '' })
const showPwd = ref(false)
const loading = ref(false)
const errorMsg = ref('')

// ===== TV 同屏字母键盘: 当前作用的输入框(账号/密码) =====
const kbdTarget = ref<'username' | 'password'>('username')

/** 软键盘输出 → 写入当前聚焦字段。键盘只产大写字母/数字/空格, 账号通常小写, 由用户自行控制不强转。 */
function onKbdInput(v: string): void {
  if (kbdTarget.value === 'password') form.password = v
  else form.username = v
}

const kbdValue = computed(() =>
  kbdTarget.value === 'password' ? form.password : form.username
)

// 浏览器 autofill 不触发 v-model 的 input event, 提交时从 DOM 兜底同步
const usernameInput = ref<HTMLInputElement | null>(null)
const passwordInput = ref<HTMLInputElement | null>(null)

function syncAutofill(): void {
  if (usernameInput.value && !form.username) form.username = usernameInput.value.value
  if (passwordInput.value && !form.password) form.password = passwordInput.value.value
}

const redirectTo = computed(() => {
  const r = route.query.redirect
  return typeof r === 'string' && r ? r : ''
})

/** 用户已登录 → 兜底跳转：优先 redirect，否则按 role 决定 */
function redirectAfterLogin(): string {
  if (redirectTo.value) return redirectTo.value
  return userStore.isAdmin ? '/manage/index' : '/index'
}

onMounted(async () => {
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
    errorMsg.value = e instanceof Error ? e.message : '登录失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <!-- ============ TV: 雷鸟卡片式登录 ============ -->
  <div v-if="isTV" class="gf-tv-login">
    <div class="gf-tv-glass-card gf-tv-login__card">
      <div class="gf-tv-login__logo text-brand-gradient">Jerocine</div>
      <div class="gf-tv-login__sub">登录后可同步观看历史 / 收藏 / 进入后台管理</div>

      <!-- 登录方式切换: 密码 / 扫码 -->
      <div class="gf-tv-login__tabs">
        <button
          type="button"
          class="gf-tv-login__tab"
          :class="{ cur: mode === 'pwd' }"
          data-focusable="true"
          tabindex="0"
          @click="switchMode('pwd')"
        >
          密码登录
        </button>
        <button
          type="button"
          class="gf-tv-login__tab"
          :class="{ cur: mode === 'qr' }"
          data-focusable="true"
          tabindex="0"
          @click="switchMode('qr')"
        >
          扫码登录
        </button>
      </div>

      <!-- 密码登录 -->
      <template v-if="mode === 'pwd'">
        <!-- 账号 (字段=account) -->
        <div class="gf-tv-login__field">
          <span class="gf-tv-login__lab">用户名 <b>*</b></span>
          <div class="gf-tv-login__wrap">
            <BaseIcon name="user" class="gf-tv-login__ic" size="20px" />
            <input
              ref="usernameInput"
              v-model="form.username"
              type="text"
              class="gf-tv-input gf-tv-login__input"
              placeholder="用户名 / 邮箱"
              autocomplete="username"
              data-focusable="true"
              tabindex="0"
              @focus="kbdTarget = 'username'"
              @change="form.username = ($event.target as HTMLInputElement).value"
            />
          </div>
        </div>

        <!-- 密码 + 显隐 -->
        <div class="gf-tv-login__field">
          <span class="gf-tv-login__lab">密码 <b>*</b></span>
          <div class="gf-tv-login__wrap">
            <BaseIcon name="lock" class="gf-tv-login__ic" size="20px" />
            <input
              ref="passwordInput"
              v-model="form.password"
              :type="showPwd ? 'text' : 'password'"
              class="gf-tv-input gf-tv-login__input"
              placeholder="密码"
              autocomplete="current-password"
              data-focusable="true"
              tabindex="0"
              @focus="kbdTarget = 'password'"
              @keydown.enter="handleLogin"
              @change="form.password = ($event.target as HTMLInputElement).value"
            />
            <button
              type="button"
              class="gf-tv-login__ic-r"
              data-focusable="true"
              tabindex="0"
              aria-label="切换密码可见性"
              @click="showPwd = !showPwd"
            >
              <BaseIcon :name="showPwd ? 'eye-off' : 'eye'" size="20px" />
            </button>
          </div>
        </div>

        <p v-if="errorMsg" class="gf-tv-login__err" role="alert">{{ errorMsg }}</p>

        <div class="gf-tv-login__actions">
          <button
            type="button"
            class="gf-tv-btn primary gf-tv-login__submit"
            :disabled="loading"
            data-focusable="true"
            tabindex="0"
            @click="handleLogin"
          >
            {{ loading ? '登录中…' : '登录' }}
          </button>
        </div>

        <!-- 注册下线指引 -->
        <div class="gf-tv-login__note">
          <div class="a">注册功能已暂时下线。</div>
          <div class="b">
            如需账号请联系管理员开通；管理员可在后台「系统管理 → 用户管理」创建账号。
          </div>
        </div>
      </template>

      <!-- 扫码 / 设备码登录 -->
      <template v-else>
        <div class="gf-tv-login__qr">
          <div class="gf-tv-login__qr-tip">用已登录的手机扫码，确认后此设备自动登录</div>
          <div class="gf-tv-login__qr-box" data-focusable="true" tabindex="0">
            <img
              v-if="qr.dataUrl"
              :src="qr.dataUrl"
              alt="登录二维码"
              width="220"
              height="220"
            />
            <span v-else class="gf-tv-login__qr-ph">{{
              qr.loading ? '生成中…' : '加载二维码…'
            }}</span>
            <div v-if="qr.status === 'ok'" class="gf-tv-login__qr-ok">已确认，登录中…</div>
          </div>
          <div v-if="qr.userCode" class="gf-tv-login__qr-code">
            校验码 <span class="uc">{{ qr.userCode }}</span>
          </div>
          <div class="gf-tv-login__actions">
            <button
              type="button"
              class="gf-tv-btn cyan"
              data-focusable="true"
              tabindex="0"
              @click="startQrLogin"
            >
              刷新二维码
            </button>
          </div>
        </div>
      </template>
    </div>

    <!-- 右侧: 同屏字母虚拟键盘 (密码登录时显示) -->
    <div v-if="mode === 'pwd'" class="gf-tv-glass-card gf-tv-login__kbd">
      <div class="gf-tv-login__kbd-head">
        <span class="t">软键盘</span>
        <span class="s">输入 {{ kbdTarget === 'password' ? '密码' : '用户名' }}</span>
      </div>
      <TvOnScreenKeyboard
        :model-value="kbdValue"
        @update:model-value="onKbdInput"
        @enter="handleLogin"
      />
      <div class="gf-tv-login__tip">首次部署默认管理员 <b>admin</b> / <b>change_me_admin</b>（登录后请立即改密）</div>
    </div>

    <p v-if="redirectTo" class="gf-tv-login__redirect">登录后将跳转至：{{ redirectTo }}</p>
  </div>

  <!-- ============ 桌面 / 移动 (原样) ============ -->
  <div v-else class="flex flex-col gap-[var(--gf-space-6)]">
    <div class="text-center">
      <h1
        class="text-[var(--gf-fs-2xl)] font-[var(--gf-fw-black)] tracking-tight text-brand-gradient"
      >
        登录 Jerocine
      </h1>
      <p class="mt-[var(--gf-space-2)] text-secondary text-sm">
        登录后可同步观看历史 / 收藏 / 进入后台管理
      </p>
    </div>

    <!-- 登录方式切换 -->
    <div
      class="flex gap-[var(--gf-space-1)] p-[var(--gf-space-1)] bg-elevated rounded-[var(--gf-radius-full)]"
    >
      <button
        type="button"
        class="flex-1 py-[var(--gf-space-2)] rounded-[var(--gf-radius-full)] text-sm font-[var(--gf-fw-medium)] transition"
        :class="mode === 'pwd' ? 'text-white' : 'text-secondary'"
        :style="mode === 'pwd' ? { backgroundImage: 'var(--gf-brand-gradient)' } : {}"
        data-focusable="true"
        @click="switchMode('pwd')"
      >
        密码登录
      </button>
      <button
        type="button"
        class="flex-1 py-[var(--gf-space-2)] rounded-[var(--gf-radius-full)] text-sm font-[var(--gf-fw-medium)] transition"
        :class="mode === 'qr' ? 'text-white' : 'text-secondary'"
        :style="mode === 'qr' ? { backgroundImage: 'var(--gf-brand-gradient)' } : {}"
        data-focusable="true"
        @click="switchMode('qr')"
      >
        扫码登录
      </button>
    </div>

    <!-- 扫码登录 -->
    <div
      v-if="mode === 'qr'"
      class="flex flex-col items-center gap-[var(--gf-space-4)] py-[var(--gf-space-2)]"
    >
      <p class="text-secondary text-sm text-center">
        用已登录的手机扫码，确认后此设备自动登录
      </p>
      <div
        class="relative w-[220px] h-[220px] flex items-center justify-center bg-white rounded-[var(--gf-radius-md)] overflow-hidden"
      >
        <img v-if="qr.dataUrl" :src="qr.dataUrl" alt="登录二维码" width="220" height="220" />
        <span v-else class="text-[#666] text-sm">{{ qr.loading ? '生成中…' : '加载二维码…' }}</span>
        <div
          v-if="qr.status === 'ok'"
          class="absolute inset-0 bg-black/70 flex items-center justify-center text-[var(--gf-success)] text-base"
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
      class="flex flex-col gap-[var(--gf-space-4)]"
      @submit.prevent="handleLogin"
    >
      <ManageFormField label="用户名" required>
        <div class="relative">
          <BaseIcon
            name="user"
            class="absolute left-[var(--gf-space-3)] top-1/2 -translate-y-1/2 text-muted"
            size="18px"
          />
          <input
            ref="usernameInput"
            v-model="form.username"
            type="text"
            class="w-full bg-elevated text-primary border border-default rounded-[var(--gf-radius-full)] pl-[var(--gf-space-10)] pr-[var(--gf-space-4)] py-[var(--gf-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition"
            placeholder="用户名 / 邮箱"
            autocomplete="username"
            data-focusable="true"
            @change="form.username = ($event.target as HTMLInputElement).value"
          />
        </div>
      </ManageFormField>

      <ManageFormField label="密码" required>
        <div class="relative">
          <BaseIcon
            name="lock"
            class="absolute left-[var(--gf-space-3)] top-1/2 -translate-y-1/2 text-muted"
            size="18px"
          />
          <input
            ref="passwordInput"
            v-model="form.password"
            :type="showPwd ? 'text' : 'password'"
            class="w-full bg-elevated text-primary border border-default rounded-[var(--gf-radius-full)] pl-[var(--gf-space-10)] pr-[var(--gf-space-10)] py-[var(--gf-space-3)] text-sm outline-none focus:border-strong focus:shadow-focus transition"
            placeholder="密码"
            autocomplete="current-password"
            data-focusable="true"
            @keydown.enter="handleLogin"
            @change="form.password = ($event.target as HTMLInputElement).value"
          />
          <button
            type="button"
            class="absolute right-[var(--gf-space-3)] top-1/2 -translate-y-1/2 text-muted hover:text-primary"
            data-focusable="true"
            aria-label="切换密码可见性"
            @click="showPwd = !showPwd"
          >
            <BaseIcon :name="showPwd ? 'eye-off' : 'eye'" size="18px" />
          </button>
        </div>
      </ManageFormField>

      <p
        v-if="errorMsg"
        class="text-xs text-[var(--gf-danger)] text-center"
        role="alert"
      >
        {{ errorMsg }}
      </p>

      <BaseButton variant="gradient" size="lg" :loading="loading" type="submit">
        登录
      </BaseButton>
    </form>

    <!-- 注册指引：公共注册已下线，由管理员后台创建账号 -->
    <div
      class="rounded-[var(--gf-radius-md)] border border-subtle bg-elevated/60 p-[var(--gf-space-4)] text-center"
    >
      <p class="text-secondary text-sm leading-[var(--gf-lh-relaxed)]">
        注册功能已暂时下线。
      </p>
      <p class="text-muted text-xs mt-[var(--gf-space-2)]">
        如需账号请联系管理员开通；管理员可在后台「系统管理 → 用户管理」创建账号。
      </p>
    </div>

    <p
      v-if="redirectTo"
      class="text-muted text-xs text-center"
    >
      登录后将跳转至：{{ redirectTo }}
    </p>
  </div>
</template>

<!--
  TV 专属样式。全部 [data-mode="tv"] 作用域(与 tv-cards.css 同约定), 不影响桌面/移动。
  AuthLayout 把 slot 限宽 440px(共享文件不可改) → TV 采用纵向堆叠(玻璃登录卡 + 软键盘),
  在 440px 内完整呈现, 不挤压.
-->
<style>
[data-mode='tv'] .gf-tv-login {
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-5);
  width: 100%;
}

[data-mode='tv'] .gf-tv-login__card,
[data-mode='tv'] .gf-tv-login__kbd {
  padding: 28px 30px;
}

[data-mode='tv'] .gf-tv-login__logo {
  font-weight: var(--gf-fw-black);
  font-size: var(--gf-fs-2xl);
  text-align: center;
}

[data-mode='tv'] .gf-tv-login__sub {
  text-align: center;
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  margin-top: 7px;
}

[data-mode='tv'] .gf-tv-login__tabs {
  display: flex;
  gap: 5px;
  background: rgba(0, 0, 0, 0.35);
  border: 1px solid var(--gf-tv-stroke, rgba(255, 255, 255, 0.13));
  border-radius: var(--gf-radius-full);
  padding: 5px;
  margin: 18px 0 6px;
}

[data-mode='tv'] .gf-tv-login__tab {
  flex: 1;
  text-align: center;
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  padding: 11px 0;
  border-radius: var(--gf-radius-full);
  cursor: pointer;
  font-weight: var(--gf-fw-semibold);
  border: 0;
  background: transparent;
}

[data-mode='tv'] .gf-tv-login__tab.cur {
  background: var(--gf-brand-gradient);
  color: #fff;
}

[data-mode='tv'] .gf-tv-login__field {
  margin-top: 16px;
}

[data-mode='tv'] .gf-tv-login__lab {
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-secondary);
  margin-bottom: 8px;
  display: block;
}

[data-mode='tv'] .gf-tv-login__lab b {
  color: var(--gf-danger, #ff6b81);
}

[data-mode='tv'] .gf-tv-login__wrap {
  position: relative;
}

[data-mode='tv'] .gf-tv-login__ic {
  position: absolute;
  left: 16px;
  top: 50%;
  transform: translateY(-50%);
  opacity: 0.7;
  pointer-events: none;
}

[data-mode='tv'] .gf-tv-login__ic-r {
  position: absolute;
  right: 12px;
  top: 50%;
  transform: translateY(-50%);
  opacity: 0.7;
  cursor: pointer;
  background: transparent;
  border: 0;
  color: inherit;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

[data-mode='tv'] .gf-tv-login__input {
  padding-left: 46px;
  padding-right: 46px;
}

[data-mode='tv'] .gf-tv-login__err {
  margin-top: 14px;
  text-align: center;
  color: var(--gf-danger);
  font-size: var(--gf-fs-sm);
}

[data-mode='tv'] .gf-tv-login__actions {
  margin-top: 18px;
}

[data-mode='tv'] .gf-tv-login__submit {
  width: 100%;
  justify-content: center;
}

[data-mode='tv'] .gf-tv-login__note {
  margin-top: 18px;
  border-radius: var(--gf-radius-md);
  border: 1px solid var(--gf-tv-stroke, rgba(255, 255, 255, 0.13));
  background: rgba(0, 0, 0, 0.25);
  padding: 14px 16px;
  text-align: center;
}

[data-mode='tv'] .gf-tv-login__note .a {
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
}

[data-mode='tv'] .gf-tv-login__note .b {
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-xs);
  margin-top: 6px;
}

[data-mode='tv'] .gf-tv-login__kbd-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 12px;
}

[data-mode='tv'] .gf-tv-login__kbd-head .t {
  font-size: var(--gf-fs-base);
  font-weight: var(--gf-fw-bold);
}

[data-mode='tv'] .gf-tv-login__kbd-head .s {
  font-size: var(--gf-fs-xs);
  color: var(--gf-brand-cyan);
}

[data-mode='tv'] .gf-tv-login__tip {
  margin-top: 14px;
  text-align: center;
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-xs);
}

[data-mode='tv'] .gf-tv-login__tip b {
  color: var(--gf-brand-cyan);
  font-weight: var(--gf-fw-semibold);
}

/* 扫码区 */
[data-mode='tv'] .gf-tv-login__qr {
  margin-top: 16px;
  text-align: center;
}

[data-mode='tv'] .gf-tv-login__qr-tip {
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  margin-bottom: 12px;
}

[data-mode='tv'] .gf-tv-login__qr-box {
  width: 220px;
  height: 220px;
  margin: 0 auto;
  border-radius: 12px;
  background: #fff;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

[data-mode='tv'] .gf-tv-login__qr-ph {
  color: #666;
  font-size: var(--gf-fs-sm);
}

[data-mode='tv'] .gf-tv-login__qr-ok {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.7);
  color: var(--gf-success);
  font-size: var(--gf-fs-base);
}

[data-mode='tv'] .gf-tv-login__qr-code {
  margin-top: 12px;
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-xs);
}

[data-mode='tv'] .gf-tv-login__qr-code .uc {
  color: var(--gf-brand-cyan);
  font-weight: var(--gf-fw-black);
  letter-spacing: 0.3em;
  font-family: monospace;
  font-size: var(--gf-fs-base);
}

[data-mode='tv'] .gf-tv-login__redirect {
  text-align: center;
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-xs);
}
</style>

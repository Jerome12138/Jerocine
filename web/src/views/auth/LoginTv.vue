<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BasePasswordField from '@/components/base/BasePasswordField.vue'
import TvOnScreenKeyboard from '@/components/film/TvOnScreenKeyboard.vue'

/**
 * 登录页 TV(雷鸟卡片式)渲染组件 — 方案 B 拆分(自 LoginView.vue 下沉)。
 *
 * 职责边界: 本组件只做「TV 渲染 + 同屏软键盘的聚焦目标状态」, 登录/扫码/注册模式拉取
 * 等业务逻辑与 form/qr/loading/errorMsg 状态全在宿主 LoginView.vue, 经 props + 回调传入;
 * 这里不复制任何业务逻辑, 也不另起接口。
 * 宿主以 `v-if="isTV"` 挂载本组件 ⇒ 仅 TV 模式渲染, 组件内无需再判 isTV。
 *
 * 双向契约(defineModel): username / password / visible 与宿主同一份状态
 * (宿主 form.username / form.password / showPwd; 桌面分支共用同一份)。
 * 模板/类名/data-focusable/tabindex 与原 LoginView TV 分支(:205-375)逐块等价;
 * 样式来自 src/assets/styles/auth-tv.css(P4-a 已整块迁出的全局共享块), 本组件无 style。
 */
interface Props {
  /** 登录方式(宿主状态) */
  mode: 'pwd' | 'qr'
  /** 错误提示(宿主 errorMsg) */
  errorMsg: string
  /** 提交中(宿主 loading) */
  loading: boolean
  /** 注册入口可见性(宿主 loadRegisterMode 结果) */
  registerMode: 'off' | 'open' | 'apply'
  /** redirect 查询参数(宿主 computed) */
  redirectTo: string
  /** 扫码登录状态(宿主 qr reactive, 只读渲染) */
  qr: {
    dataUrl: string
    userCode: string
    status: '' | 'pending' | 'ok' | 'expired'
    loading: boolean
  }
  /** 切换登录方式(宿主 switchMode: 顺带启停扫码轮询) */
  onSwitchMode: (m: 'pwd' | 'qr') => void
  /** 提交登录(宿主 handleLogin) */
  onLogin: () => void
  /** 刷新/重开扫码(宿主 startQrLogin) */
  onRefreshQr: () => void
  /** 模板 ref 回传: 账号输入框原生元素(宿主 syncAutofill 的浏览器 autofill 兜底) */
  onUsernameRef: (el: unknown) => void
  /** 模板 ref 回传: 密码组件实例(宿主经其 expose 的 input 取原生元素) */
  onPasswordRef: (el: unknown) => void
}
defineProps<Props>()

/** 与宿主同一份的两个输入值 + 密码显隐 */
const username = defineModel<string>('username', { required: true })
const password = defineModel<string>('password', { required: true })
const visible = defineModel<boolean>('visible', { required: true })

// ===== TV 同屏字母键盘: 当前作用的输入框(账号/密码) =====
const kbdTarget = ref<'username' | 'password'>('username')

/** 软键盘输出 → 写入当前聚焦字段。键盘只产大写字母/数字/空格, 账号通常小写, 由用户自行控制不强转。 */
function onKbdInput(v: string): void {
  if (kbdTarget.value === 'password') password.value = v
  else username.value = v
}

const kbdValue = computed(() =>
  kbdTarget.value === 'password' ? password.value : username.value
)

const router = useRouter()
</script>

<template>
  <div class="jc-tv-login">
    <div class="jc-tv-glass-card jc-tv-login__card">
      <div class="jc-tv-login__logo text-brand-gradient">Jerocine影视</div>
      <div class="jc-tv-login__sub">登录后可同步观看历史 / 进度续播 / 收藏</div>

      <!-- 登录方式切换: 密码 / 扫码 -->
      <div class="jc-tv-login__tabs">
        <button
          type="button"
          class="jc-tv-login__tab"
          :class="{ cur: mode === 'pwd' }"
          data-focusable="true"
          tabindex="0"
          @click="onSwitchMode('pwd')"
        >
          密码登录
        </button>
        <button
          type="button"
          class="jc-tv-login__tab"
          :class="{ cur: mode === 'qr' }"
          data-focusable="true"
          tabindex="0"
          @click="onSwitchMode('qr')"
        >
          扫码登录
        </button>
      </div>

      <!-- 密码登录 -->
      <template v-if="mode === 'pwd'">
        <!-- 账号 (字段=account) -->
        <div class="jc-tv-login__field">
          <span class="jc-tv-login__lab">用户名 <b>*</b></span>
          <div class="jc-tv-login__wrap">
            <BaseIcon name="user" class="jc-tv-login__ic" size="20px" />
            <input
              :ref="onUsernameRef"
              v-model="username"
              type="text"
              class="jc-tv-input jc-tv-login__input"
              placeholder="用户名 / 邮箱"
              autocomplete="username"
              data-focusable="true"
              tabindex="0"
              @focus="kbdTarget = 'username'"
              @change="username = ($event.target as HTMLInputElement).value"
            />
          </div>
        </div>

        <!-- 密码 + 显隐 -->
        <div class="jc-tv-login__field">
          <span class="jc-tv-login__lab">密码 <b>*</b></span>
          <BasePasswordField
            :ref="onPasswordRef"
            v-model="password"
            v-model:visible="visible"
            variant="tv"
            placeholder="密码"
            autocomplete="current-password"
            tabindex="0"
            @focus="kbdTarget = 'password'"
            @keydown.enter="onLogin"
            @change="password = ($event.target as HTMLInputElement).value"
          />
        </div>

        <p v-if="errorMsg" class="jc-tv-login__err" role="alert">{{ errorMsg }}</p>

        <div class="jc-tv-login__actions">
          <button
            type="button"
            class="jc-tv-btn primary jc-tv-login__submit"
            :disabled="loading"
            data-focusable="true"
            tabindex="0"
            @click="onLogin"
          >
            {{ loading ? '登录中…' : '登录' }}
          </button>
        </div>

        <!-- 忘记密码(P3): TV 不便输邮箱, 引导去电脑/手机走邮件链接 -->
        <button
          type="button"
          class="jc-tv-login__forgot"
          data-focusable="true"
          tabindex="0"
          @click="router.push('/forgot-password')"
        >
          忘记密码？
        </button>

        <!-- 注册入口: off=下线指引; open/apply=引导去 PC/手机完成注册(TV 不便输邮箱+验证码) -->
        <div class="jc-tv-login__note">
          <template v-if="registerMode === 'off'">
            <div class="a">注册功能已暂时下线。</div>
            <div class="b">
              如需账号请联系管理员开通；管理员可在后台「系统管理 → 用户管理」创建账号。
            </div>
          </template>
          <template v-else>
            <div class="a">还没有账号？</div>
            <div class="b">电视端不便输入邮箱与验证码，请在电脑或手机访问本站完成注册。</div>
          </template>
        </div>
        <div v-if="registerMode !== 'off'" class="jc-tv-login__actions">
          <button
            type="button"
            class="jc-tv-btn cyan"
            data-focusable="true"
            tabindex="0"
            @click="router.push('/register')"
          >
            注册账号
          </button>
        </div>
      </template>

      <!-- 扫码 / 设备码登录 -->
      <template v-else>
        <div class="jc-tv-login__qr">
          <div class="jc-tv-login__qr-tip">用已登录的手机扫码，确认后此设备自动登录</div>
          <div class="jc-tv-login__qr-box" data-focusable="true" tabindex="0">
            <img
              v-if="qr.dataUrl"
              :src="qr.dataUrl"
              alt="登录二维码"
              width="220"
              height="220"
            />
            <span v-else class="jc-tv-login__qr-ph">{{
              qr.loading ? '生成中…' : '加载二维码…'
            }}</span>
            <div v-if="qr.status === 'ok'" class="jc-tv-login__qr-ok">已确认，登录中…</div>
          </div>
          <div v-if="qr.userCode" class="jc-tv-login__qr-code">
            校验码 <span class="uc">{{ qr.userCode }}</span>
          </div>
          <div class="jc-tv-login__actions">
            <button
              type="button"
              class="jc-tv-btn cyan"
              data-focusable="true"
              tabindex="0"
              @click="onRefreshQr"
            >
              刷新二维码
            </button>
          </div>
        </div>
      </template>
    </div>

    <!-- 右侧: 同屏字母虚拟键盘 (密码登录时显示) -->
    <div v-if="mode === 'pwd'" class="jc-tv-glass-card jc-tv-login__kbd">
      <div class="jc-tv-login__kbd-head">
        <span class="t">软键盘</span>
        <span class="s">输入 {{ kbdTarget === 'password' ? '密码' : '用户名' }}</span>
      </div>
      <TvOnScreenKeyboard
        :model-value="kbdValue"
        @update:model-value="onKbdInput"
        @enter="onLogin"
      />
      <div class="jc-tv-login__tip">首次部署默认管理员 <b>admin</b> / <b>change_me_admin</b>（登录后请立即改密）</div>
    </div>

    <p v-if="redirectTo" class="jc-tv-login__redirect">登录后将跳转至：{{ redirectTo }}</p>
  </div>
</template>

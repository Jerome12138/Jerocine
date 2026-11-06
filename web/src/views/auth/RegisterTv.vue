<script setup lang="ts">
import { useRouter } from 'vue-router'

/**
 * 注册页 TV 渲染组件 — 方案 B 拆分(自 RegisterView.vue 下沉)。
 * 电视端不便输入邮箱 + 验证码, TV 只给一张引导卡(去电脑 / 手机注册)。
 * 宿主以 `v-if="isTV"` 挂载 ⇒ 组件内无需再判 isTV;
 * 模板/类名/data-focusable/tabindex 与原 TV 分支(:212-236)逐块等价。
 * 样式来自 src/assets/styles/auth-tv.css(P4-a 迁出的全局共享块), 本组件无 style。
 */
interface Props {
  /** 邮件通道是否就绪(宿主 smtpReady: 决定「引导去注册」还是「暂不支持」文案) */
  smtpReady: boolean
}
defineProps<Props>()

const router = useRouter()
</script>

<template>
  <div class="jc-tv-glass-card jc-tv-login__card">
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
</template>

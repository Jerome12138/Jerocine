<script setup lang="ts">
import { useRouter } from 'vue-router'

/**
 * 重置密码页 TV 渲染组件 — 方案 B 拆分(自 ResetPasswordView.vue 下沉)。
 * 电视端不便设置密码, TV 只给一张引导卡(去电脑 / 手机打开邮件里的重置链接)。
 * 宿主以 `v-if="isTV"` 挂载 ⇒ 组件内无需再判 isTV;
 * 模板/类名/data-focusable/tabindex 与原 TV 分支(:66-84)逐块等价。
 * 样式来自 src/assets/styles/auth-tv.css(P4-a 迁出的全局共享块), 本组件无 style。
 */
const router = useRouter()
</script>

<template>
  <div class="jc-tv-glass-card jc-tv-login__card">
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
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useViewMode } from '@/composables/useViewMode'
import * as deviceApi from '@/api/device'
import BaseButton from '@/components/base/BaseButton.vue'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { isTV } = useViewMode()

const code = ref(String(route.query.code ?? ''))
const state = ref<'confirm' | 'done' | 'error'>('confirm')
const errorMsg = ref('')
const submitting = ref(false)

// TV 端默认聚焦「同意登录」
const approveBtn = ref<HTMLButtonElement | null>(null)

onMounted(() => {
  if (!code.value) {
    state.value = 'error'
    errorMsg.value = '链接缺少校验码'
    return
  }
  // 未登录 → 先登录, 登录后回到本确认页
  if (!userStore.isLoggedIn) {
    void router.replace({ path: '/login', query: { redirect: route.fullPath } })
    return
  }
  if (isTV.value) {
    void nextTick(() => approveBtn.value?.focus())
  }
})

async function approve(): Promise<void> {
  submitting.value = true
  errorMsg.value = ''
  try {
    await deviceApi.deviceConfirm(code.value)
    state.value = 'done'
  } catch (e: unknown) {
    state.value = 'error'
    errorMsg.value = e instanceof Error ? e.message : '授权失败，二维码可能已过期'
  } finally {
    submitting.value = false
  }
}

function goHome(): void {
  void router.replace('/index')
}
</script>

<template>
  <!-- ============ TV(雷鸟卡片式) ============ -->
  <div v-if="isTV" class="gf-tv-device">
    <div class="gf-tv-sec gf-tv-device__sec">
      <span class="t">📱 设备登录</span>
      <span class="s">扫码授权后请回到那台设备，已自动登录</span>
    </div>

    <div class="gf-tv-glass-card gf-tv-device__card">
      <h1 class="gf-tv-device__title text-brand-gradient">扫码登录确认</h1>

      <template v-if="state === 'confirm'">
        <p class="gf-tv-device__desc">
          有一台设备请求用<span class="gf-tv-device__name">「{{ userStore.displayName }}」</span
          >登录。<br />确认是你本人操作再同意。
        </p>

        <!-- 请求信息(列表行风格, 仅用真实数据) -->
        <div class="gf-tv-device__rows">
          <div class="gf-tv-listrow">
            <span class="gf-tv-device__ic">👤</span>
            <div>
              <div class="lab">登录账号</div>
              <div class="sub">{{ userStore.displayName }}</div>
            </div>
          </div>
          <div v-if="code" class="gf-tv-listrow">
            <span class="gf-tv-device__ic">🔑</span>
            <div>
              <div class="lab">校验码</div>
              <div class="sub gf-tv-device__code">{{ code }}</div>
            </div>
          </div>
        </div>

        <!-- 两个等宽大按钮: 取消 / 同意登录(同意默认焦点) -->
        <div class="gf-tv-device__actions">
          <button
            type="button"
            class="gf-tv-btn gf-tv-device__btn"
            data-focusable="true"
            tabindex="0"
            @click="goHome"
          >
            取消
          </button>
          <button
            ref="approveBtn"
            type="button"
            class="gf-tv-btn primary gf-tv-device__btn"
            data-focusable="true"
            tabindex="0"
            :disabled="submitting"
            @click="approve"
          >
            {{ submitting ? '授权中…' : '✓ 同意登录' }}
          </button>
        </div>
      </template>

      <template v-else-if="state === 'done'">
        <p class="gf-tv-device__done">✓ 已授权</p>
        <p class="gf-tv-device__desc">请回到那台设备，已自动登录。</p>
        <div class="gf-tv-device__actions gf-tv-device__actions--single">
          <button
            type="button"
            class="gf-tv-btn primary gf-tv-device__btn"
            data-focusable="true"
            tabindex="0"
            @click="goHome"
          >
            返回首页
          </button>
        </div>
      </template>

      <template v-else>
        <p class="gf-tv-device__error">{{ errorMsg }}</p>
        <div class="gf-tv-device__actions gf-tv-device__actions--single">
          <button
            type="button"
            class="gf-tv-btn primary gf-tv-device__btn"
            data-focusable="true"
            tabindex="0"
            @click="goHome"
          >
            返回首页
          </button>
        </div>
      </template>
    </div>
  </div>

  <!-- ============ 桌面 / 移动(原样) ============ -->
  <div v-else class="flex flex-col gap-[var(--gf-space-6)] text-center">
    <h1
      class="text-[var(--gf-fs-2xl)] font-[var(--gf-fw-black)] tracking-tight text-brand-gradient"
    >
      扫码登录确认
    </h1>

    <template v-if="state === 'confirm'">
      <p class="text-secondary text-sm leading-[var(--gf-lh-relaxed)]">
        有一台设备请求用<span class="text-primary font-[var(--gf-fw-semibold)]">「{{ userStore.displayName }}」</span>登录。
        确认是你本人操作再同意。
      </p>
      <p v-if="code" class="text-muted text-xs">
        校验码 <span class="text-primary font-mono tracking-[0.3em]">{{ code }}</span>
      </p>
      <div class="flex gap-[var(--gf-space-3)]">
        <BaseButton variant="ghost" size="lg" class="flex-1" @click="goHome">取消</BaseButton>
        <BaseButton
          variant="gradient"
          size="lg"
          class="flex-1"
          :loading="submitting"
          @click="approve"
        >
          同意登录
        </BaseButton>
      </div>
    </template>

    <template v-else-if="state === 'done'">
      <p class="text-[var(--gf-success)] text-base">✓ 已授权</p>
      <p class="text-secondary text-sm">请回到那台设备，已自动登录。</p>
      <BaseButton variant="ghost" size="lg" @click="goHome">返回首页</BaseButton>
    </template>

    <template v-else>
      <p class="text-[var(--gf-danger)] text-sm">{{ errorMsg }}</p>
      <BaseButton variant="ghost" size="lg" @click="goHome">返回首页</BaseButton>
    </template>
  </div>
</template>

<style scoped>
.gf-tv-device {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.gf-tv-device__sec {
  width: 100%;
  max-width: 560px;
}

.gf-tv-device__card {
  width: 100%;
  max-width: 560px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  text-align: center;
}

.gf-tv-device__title {
  font-size: var(--gf-fs-xl);
  font-weight: var(--gf-fw-black);
  letter-spacing: -0.01em;
}

.gf-tv-device__desc {
  font-size: var(--gf-fs-base);
  color: var(--gf-text-secondary);
  line-height: var(--gf-lh-relaxed);
}

.gf-tv-device__name {
  color: var(--gf-text-primary);
  font-weight: var(--gf-fw-bold);
}

.gf-tv-device__rows {
  display: flex;
  flex-direction: column;
  gap: 12px;
  text-align: left;
}

.gf-tv-device__ic {
  font-size: 24px;
  line-height: 1;
}

.gf-tv-device__code {
  font-family: var(--gf-font-mono, monospace);
  letter-spacing: 0.3em;
  color: var(--gf-brand-cyan);
}

.gf-tv-device__actions {
  display: flex;
  gap: 16px;
  margin-top: 4px;
}

.gf-tv-device__actions--single {
  justify-content: center;
}

.gf-tv-device__btn {
  flex: 1;
  justify-content: center;
}

.gf-tv-device__actions--single .gf-tv-device__btn {
  flex: 0 0 auto;
  min-width: 220px;
}

.gf-tv-device__done {
  font-size: var(--gf-fs-lg);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-success);
}

.gf-tv-device__error {
  font-size: var(--gf-fs-base);
  color: var(--gf-danger);
  line-height: var(--gf-lh-relaxed);
}
</style>

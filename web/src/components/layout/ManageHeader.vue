<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useUserStore } from '@/stores/user'
import BaseButton from '@/components/base/BaseButton.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'

const props = defineProps<{ showHamburger?: boolean }>()
const emit = defineEmits<{ (e: 'toggle-drawer'): void }>()

const userStore = useUserStore()
const router = useRouter()
const { info, displayName } = storeToRefs(userStore)

const dropdownOpen = ref(false)
const dialogOpen = ref(false)
const submitting = ref(false)
const formError = ref('')

const pwdForm = reactive({
  password: '',
  newPassword: '',
  confirmPassword: ''
})

async function handleLogout(): Promise<void> {
  dropdownOpen.value = false
  await userStore.logout()
  await router.replace('/login')
}

function openChangePwd(): void {
  dropdownOpen.value = false
  pwdForm.password = ''
  pwdForm.newPassword = ''
  pwdForm.confirmPassword = ''
  formError.value = ''
  dialogOpen.value = true
}

async function submitPwd(): Promise<void> {
  formError.value = ''
  if (pwdForm.newPassword !== pwdForm.confirmPassword) {
    formError.value = '新密码与确认密码不一致'
    return
  }
  if (pwdForm.newPassword.length < 6) {
    formError.value = '新密码至少 6 位'
    return
  }
  submitting.value = true
  try {
    await userStore.changePassword({
      password: pwdForm.password,
      newPassword: pwdForm.newPassword
    })
    dialogOpen.value = false
  } catch (e: unknown) {
    formError.value = e instanceof Error ? e.message : '修改失败'
  } finally {
    submitting.value = false
  }
}

const avatar = computed(() => {
  const a = info.value?.avatar
  if (a && a !== 'empty') return a
  // 与公开端同源: 本地静态 SVG (远程占位图在弱网/被墙时白图)
  return '/default-avatar.svg'
})
</script>

<template>
  <header
    class="flex-between bg-[image:var(--jc-brand-gradient)] px-[var(--jc-space-6)] h-[56px] sticky top-0 z-[var(--jc-z-header)] shadow-md"
  >
    <div class="flex items-center gap-[var(--jc-space-4)] text-white">
      <!-- 汉堡按钮仅移动端渲染: 侧栏折叠已移除, 桌面/平板侧栏常驻无开关 -->
      <button
        v-if="props.showHamburger"
        type="button"
        class="text-white/90 hover:text-white text-xl min-h-[44px] min-w-[44px] flex items-center justify-center"
        data-focusable="true"
        @click="emit('toggle-drawer')"
      >
        <BaseIcon name="menu" size="22px" />
        <span class="sr-only">打开菜单</span>
      </button>
      <!-- 页面标题不再放头部: 各页面内部已有自己的标题, 侧栏顶部有「站点名 后台管理」锚点 -->
    </div>

    <div class="relative">
      <button
        type="button"
        class="flex items-center gap-[var(--jc-space-2)] text-white/95 hover:text-white"
        data-focusable="true"
        @click="dropdownOpen = !dropdownOpen"
      >
        <img
          :src="avatar"
          alt="avatar"
          class="w-[32px] h-[32px] rounded-full border border-white/40 object-cover"
        />
        <span class="text-sm hidden md:inline">
          {{ displayName }}
        </span>
        <BaseIcon name="chevron-down" size="14px" />
      </button>

      <Transition name="fade-slide">
        <div
          v-if="dropdownOpen"
          class="absolute right-0 mt-[var(--jc-space-2)] w-[180px] bg-elevated border border-default rounded-[var(--jc-radius-md)] shadow-card-lg overflow-hidden"
        >
          <button
            type="button"
            class="w-full text-left px-[var(--jc-space-4)] py-[var(--jc-space-3)] text-secondary hover:bg-surface flex items-center gap-[var(--jc-space-3)]"
            data-focusable="true"
            @click="openChangePwd"
          >
            <BaseIcon name="lock" size="16px" /> 修改密码
          </button>
          <button
            type="button"
            class="w-full text-left px-[var(--jc-space-4)] py-[var(--jc-space-3)] text-secondary hover:bg-surface flex items-center gap-[var(--jc-space-3)] border-t border-subtle"
            data-focusable="true"
            @click="handleLogout"
          >
            <BaseIcon name="logout" size="16px" /> 退出登录
          </button>
        </div>
      </Transition>
    </div>

    <ManageSheet v-model="dialogOpen" title="修改密码" mobile-mode="sheet">
      <div class="flex flex-col gap-[var(--jc-space-4)]">
        <ManageFormField label="原密码" required>
          <ManageInput v-model="pwdForm.password" type="password" placeholder="原密码" />
        </ManageFormField>
        <ManageFormField label="新密码" required hint="至少 6 位">
          <ManageInput v-model="pwdForm.newPassword" type="password" placeholder="新密码" />
        </ManageFormField>
        <ManageFormField label="确认密码" required>
          <ManageInput
            v-model="pwdForm.confirmPassword"
            type="password"
            placeholder="再次输入新密码"
          />
        </ManageFormField>
        <p v-if="formError" class="text-xs text-[var(--jc-danger)]">
          {{ formError }}
        </p>
      </div>
      <template #footer>
        <BaseButton variant="ghost" @click="dialogOpen = false">取消</BaseButton>
        <BaseButton variant="gradient" :loading="submitting" @click="submitPwd">
          确认
        </BaseButton>
      </template>
    </ManageSheet>
  </header>
</template>

<style scoped>
.fade-slide-enter-active,
.fade-slide-leave-active {
  transition: opacity var(--jc-dur-fast) var(--jc-ease-standard),
    transform var(--jc-dur-fast) var(--jc-ease-standard);
}
.fade-slide-enter-from,
.fade-slide-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
</style>

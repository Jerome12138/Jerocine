<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useUserStore } from '@/stores/user'
import { useUIStore } from '@/stores/ui'
import BaseButton from '@/components/base/BaseButton.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'

const props = defineProps<{ showHamburger?: boolean }>()
const emit = defineEmits<{ (e: 'toggle-drawer'): void }>()

const userStore = useUserStore()
const uiStore = useUIStore()
const route = useRoute()
const router = useRouter()
const { info } = storeToRefs(userStore)

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

const avatar = computed(
  () =>
    info.value?.avatar && info.value.avatar !== 'empty'
      ? info.value.avatar
      : 'https://s2.loli.net/2023/12/05/O2SEiUcMx5aWlv4.jpg'
)
</script>

<template>
  <header
    class="flex-between bg-[image:var(--gf-brand-gradient)] px-[var(--gf-space-6)] h-[56px] sticky top-0 z-[var(--gf-z-header)] shadow-md"
  >
    <div class="flex items-center gap-[var(--gf-space-4)] text-white">
      <button
        type="button"
        class="text-white/90 hover:text-white text-xl min-h-[44px] min-w-[44px] flex items-center justify-center"
        data-focusable="true"
        @click="props.showHamburger ? emit('toggle-drawer') : uiStore.toggleSidebar()"
      >
        <BaseIcon name="menu" size="22px" />
        <span class="sr-only">{{ props.showHamburger ? '打开菜单' : '切换侧栏' }}</span>
      </button>
      <h3 class="font-[var(--gf-fw-semibold)] text-lg">
        {{ (route.meta.title as string) || '后台管理中心' }}
      </h3>
    </div>

    <div class="relative">
      <button
        type="button"
        class="flex items-center gap-[var(--gf-space-2)] text-white/95 hover:text-white"
        data-focusable="true"
        @click="dropdownOpen = !dropdownOpen"
      >
        <img
          :src="avatar"
          alt="avatar"
          class="w-[32px] h-[32px] rounded-full border border-white/40 object-cover"
        />
        <span class="text-sm hidden md:inline">
          {{ info?.nickname || info?.username || '管理员' }}
        </span>
        <BaseIcon name="chevron-down" size="14px" />
      </button>

      <Transition name="fade-slide">
        <div
          v-if="dropdownOpen"
          class="absolute right-0 mt-[var(--gf-space-2)] w-[180px] bg-elevated border border-default rounded-[var(--gf-radius-md)] shadow-card-lg overflow-hidden"
        >
          <button
            type="button"
            class="w-full text-left px-[var(--gf-space-4)] py-[var(--gf-space-3)] text-secondary hover:bg-surface flex items-center gap-[var(--gf-space-3)]"
            data-focusable="true"
            @click="openChangePwd"
          >
            <BaseIcon name="lock" size="16px" /> 修改密码
          </button>
          <button
            type="button"
            class="w-full text-left px-[var(--gf-space-4)] py-[var(--gf-space-3)] text-secondary hover:bg-surface flex items-center gap-[var(--gf-space-3)] border-t border-subtle"
            data-focusable="true"
            @click="handleLogout"
          >
            <BaseIcon name="logout" size="16px" /> 退出登录
          </button>
        </div>
      </Transition>
    </div>

    <ManageSheet v-model="dialogOpen" title="修改密码" mobile-mode="sheet">
      <div class="flex flex-col gap-[var(--gf-space-4)]">
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
        <p v-if="formError" class="text-xs text-[var(--gf-danger)]">
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
  transition: opacity var(--gf-dur-fast) var(--gf-ease-standard),
    transform var(--gf-dur-fast) var(--gf-ease-standard);
}
.fade-slide-enter-from,
.fade-slide-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
</style>

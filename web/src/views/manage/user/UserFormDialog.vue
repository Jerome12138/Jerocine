<script setup lang="ts">
import { reactive, computed } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { ManageUserRow } from '@/types/manage'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 用户新增/编辑表单 —— openManageDialog 内容组件。
 * 契约: emits ('confirm', payload) | ('cancel'), footer 自带。
 *
 * 含独立校验与提交: 校验失败/提交失败 toast 后弹窗停留(输入不丢),
 * 成功后 emit('confirm', 载荷), 由发起页负责重拉列表。
 */
export interface UserFormPayload {
  id: number
  userName: string
  role: number
  email: string
}

interface Props {
  /** 编辑目标; null/缺省 = 新增 */
  initial?: ManageUserRow | null
  /** 当前登录管理员 id(自保护: 不能改自己的角色) */
  selfId?: number
}
const props = withDefaults(defineProps<Props>(), { initial: null, selfId: 0 })

const emit = defineEmits<{
  (e: 'confirm', payload: UserFormPayload): void
  (e: 'cancel'): void
}>()

const isEdit = computed(() => !!props.initial)
const isSelf = computed(() => !!props.initial && props.initial.ID === props.selfId)

const form = reactive({
  userName: props.initial?.userName ?? '',
  password: '',
  role: props.initial?.role ?? 0,
  email: props.initial?.email ?? ''
})

const ROLE_OPTIONS = [
  { label: '普通用户', value: 0 },
  { label: '管理员', value: 1 }
]

const { busy, run } = useAsyncAction()

function cancel(): void {
  emit('cancel')
}

function submit(): void {
  const name = form.userName.trim()
  // 按码点计数与服务端 runeLen 同口径(string.length 按 UTF-16 单元, emoji 等代理对会算 2)
  if (!name || [...name].length > 32) {
    toast('error', '用户名需为 1-32 个字符')
    return
  }
  const email = form.email.trim()
  if (email && (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) || email.length > 128)) {
    toast('error', '邮箱格式不正确（可留空）')
    return
  }
  if (!isEdit.value && (form.password.length < 6 || form.password.length > 64)) {
    toast('error', '密码长度需在 6-64 位之间')
    return
  }
  if (isSelf.value && form.role !== props.initial?.role) {
    toast('error', '不能修改自己的角色')
    return
  }
  void run({
    action: () =>
      isEdit.value
        ? manageApi.user.update(props.initial!.ID, name, form.role, email, { silent: true })
        : manageApi.user.create(name, form.password, form.role, email, { silent: true }),
    successMsg: isEdit.value ? '已保存' : `已创建用户「${name}」`,
    after: () => emit('confirm', { id: props.initial?.ID ?? 0, userName: name, role: form.role, email })
  })
}
</script>

<template>
  <form class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="submit">
    <label class="flex flex-col gap-[var(--jc-space-2)]">
      <span class="text-sm text-secondary">用户名(1-32 个字符, 不得与他人重复)</span>
      <ManageInput v-model="form.userName" placeholder="输入用户名" />
    </label>
    <label v-if="!isEdit" class="flex flex-col gap-[var(--jc-space-2)]">
      <span class="text-sm text-secondary">初始密码(6-64 位)</span>
      <ManageInput v-model="form.password" type="password" placeholder="输入初始密码" />
    </label>
    <label class="flex flex-col gap-[var(--jc-space-2)]">
      <span class="text-sm text-secondary">邮箱{{ isEdit ? '' : '(选填)' }}</span>
      <ManageInput v-model="form.email" type="email" placeholder="留空则不绑定邮箱" />
      <span class="text-xs text-muted">
        用于找回密码；填了需唯一且格式正确，不会自动发验证邮件{{ isEdit ? '；清空即解绑' : '' }}
      </span>
    </label>
    <label class="flex flex-col gap-[var(--jc-space-2)]">
      <span class="text-sm text-secondary">角色</span>
      <ManageSelect
        v-model="form.role"
        :options="ROLE_OPTIONS"
        aria-label="角色"
        :disabled="isSelf"
      />
      <span v-if="isSelf" class="text-xs text-muted">不能修改自己的角色</span>
    </label>
    <div class="flex justify-end gap-[var(--jc-space-2)]">
      <BaseButton variant="ghost" size="sm" @click="cancel">取消</BaseButton>
      <BaseButton variant="gradient" size="sm" type="submit" :loading="busy">
        {{ isEdit ? '保存' : '创建' }}
      </BaseButton>
    </div>
  </form>
</template>

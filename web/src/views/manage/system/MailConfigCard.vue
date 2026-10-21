<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { MailConfigUpdate, MailConfigResp, SmtpPreset } from '@/types/manage'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'

/**
 * 注册与邮件通道卡片（原「邮件与注册」独立页并入「系统设置」）。
 * 保存走 PUT /manage/mail/config；凭据只写不读（留空 = 不修改）。
 */
const loading = ref(true)
const submitting = ref(false)
const testing = ref(false)
const presets = ref<SmtpPreset[]>([])

const form = reactive({
  registerMode: 'off' as MailConfigResp['registerMode'],
  smtpEnabled: 0,
  smtpHost: '',
  smtpPort: 465,
  smtpSecure: 'ssl' as SmtpPreset['secure'],
  smtpUser: '',
  smtpFromName: '',
  /** 全局出站日配额（含测试邮件）；后端 1~1000，默认 50 */
  mailDailyQuota: 50
})
/** 授权码输入框只写不读: 留空 = 不修改已存值(与 TMDB Key 同范式) */
const passDraft = ref('')
const passSet = ref(false)
const passMasked = ref('')
const testTo = ref('')

const modeOptions: { value: MailConfigResp['registerMode']; label: string }[] = [
  { value: 'off', label: '关闭（仅后台创建账号）' },
  { value: 'open', label: '开放注册（邮箱验证码，注册即登录）' },
  { value: 'apply', label: '申请审核（提交申请 → 管理员审核 → 邮件激活）' }
]

async function load(): Promise<void> {
  loading.value = true
  try {
    const data = await manageApi.mail.getMailConfig()
    presets.value = data.presets
    form.registerMode = data.registerMode
    form.smtpEnabled = data.smtpEnabled
    form.smtpHost = data.smtpHost
    form.smtpPort = data.smtpPort
    form.smtpSecure = data.smtpSecure
    form.smtpUser = data.smtpUser
    form.smtpFromName = data.smtpFromName
    form.mailDailyQuota = data.mailDailyQuota
    passSet.value = data.smtpPassSet
    passMasked.value = data.smtpPassMasked
  } finally {
    loading.value = false
  }
}

/** 选预设仅回填 host/port/secure 默认值; 不在预设表内的服务商手改即可 */
function applyPreset(id: string): void {
  const p = presets.value.find((x) => x.id === id)
  if (!p) return
  form.smtpHost = p.host
  form.smtpPort = p.port
  form.smtpSecure = p.secure
}

function buildPayload(): MailConfigUpdate {
  const pass = passDraft.value.trim()
  return {
    registerMode: form.registerMode,
    smtpEnabled: form.smtpEnabled,
    smtpHost: form.smtpHost.trim(),
    smtpPort: form.smtpPort,
    smtpSecure: form.smtpSecure,
    smtpUser: form.smtpUser.trim(),
    ...(pass ? { smtpPass: pass } : {}),
    smtpFromName: form.smtpFromName.trim(),
    // 数字框清空时 ManageInput 会给 ''，统一收敛成 0（后端语义：0 = 不修改）
    mailDailyQuota: Number(form.mailDailyQuota) || 0
  }
}

async function submit(): Promise<void> {
  const quota = Number(form.mailDailyQuota) || 0
  if (quota < 1 || quota > 1000) {
    toast('error', '每日出站上限需在 1~1000 之间')
    return
  }
  submitting.value = true
  try {
    const data = await manageApi.mail.saveMailConfig(buildPayload())
    passDraft.value = ''
    passSet.value = data.smtpPassSet
    passMasked.value = data.smtpPassMasked
    toast('success', '注册与邮件配置已保存')
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '保存失败')
  } finally {
    submitting.value = false
  }
}

/** 测试邮件与保存解耦: 用当前表单值直发, 失败回显 SMTP 原文错误便于定位 */
async function submitTest(): Promise<void> {
  const to = testTo.value.trim()
  if (!to) {
    toast('error', '请先填写测试收件邮箱')
    return
  }
  testing.value = true
  try {
    await manageApi.mail.testMail({ ...buildPayload(), to })
    toast('success', `测试邮件已发送至 ${to}，请查收`)
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '发送失败')
  } finally {
    testing.value = false
  }
}

const smtpIncomplete = computed(
  () => !form.smtpHost.trim() || !form.smtpUser.trim() || !(passSet.value || passDraft.value.trim())
)

const passHint = computed(() =>
  passSet.value
    ? `已保存（${passMasked.value}）；输入新值才覆盖，留空不修改`
    : '未保存；SMTP 登录凭据，保存后不可查看'
)

onMounted(load)
</script>

<template>
  <section
    class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)] flex flex-col gap-[var(--jc-space-5)]"
  >
    <header>
      <h2 class="text-lg font-[var(--jc-fw-semibold)]">注册与邮件通道</h2>
      <p class="mt-1 text-sm text-muted">
        注册模式 + SMTP 发件通道；验证码 / 激活 / 找回密码邮件均经此通道发送。
        邮件里的链接使用上面的「域名」，请先填好完整域名（https:// 开头）。
      </p>
    </header>

    <div v-if="loading" class="flex flex-col gap-[var(--jc-space-3)]">
      <BaseSkeleton v-for="i in 8" :key="i" shape="rect" height="44px" />
    </div>

    <form v-else class="flex flex-col gap-[var(--jc-space-5)]" @submit.prevent="submit">
      <ManageFormField label="注册模式" hint="open 需配置邮件通道；apply 的激活邮件同样经 SMTP 发送">
        <select
          v-model="form.registerMode"
          class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong"
          data-focusable="true"
        >
          <option v-for="o in modeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
      </ManageFormField>

      <div class="border-t border-[var(--jc-border)]" />

      <div class="flex flex-col gap-[var(--jc-space-4)]">
        <header class="flex items-center gap-[var(--jc-space-3)]">
          <h3 class="font-[var(--jc-fw-semibold)]">SMTP 发件通道</h3>
          <ManageSwitch
            :model-value="form.smtpEnabled === 1"
            @update:model-value="form.smtpEnabled = $event ? 1 : 0"
          />
          <span class="text-sm text-muted">{{ form.smtpEnabled === 1 ? '已启用' : '未启用' }}</span>
        </header>

        <ManageFormField
          label="服务商预设"
          hint="仅回填 host / 端口 / 加密方式的默认值；不在列表内选「自定义」后手填即可"
        >
          <select
            class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong"
            data-focusable="true"
            @change="applyPreset(($event.target as HTMLSelectElement).value)"
          >
            <option value="">选择预设（可跳过）</option>
            <option v-for="p in presets" :key="p.id" :value="p.id">
              {{ p.name }}{{ p.note ? ` — ${p.note}` : '' }}
            </option>
          </select>
        </ManageFormField>

        <div class="flex flex-col gap-[var(--jc-space-4)] sm:flex-row">
          <ManageFormField class="flex-1" label="SMTP 服务器" required>
            <ManageInput v-model="form.smtpHost" placeholder="smtp.qq.com" />
          </ManageFormField>
          <ManageFormField class="sm:w-[140px]" label="端口" required>
            <ManageInput v-model="form.smtpPort" type="number" />
          </ManageFormField>
        </div>

        <div class="flex flex-col gap-[var(--jc-space-4)] sm:flex-row">
          <ManageFormField class="flex-1" label="加密方式" required>
            <select
              v-model="form.smtpSecure"
              class="w-full bg-elevated text-primary border border-default rounded-[var(--jc-radius-md)] px-[var(--jc-space-3)] py-[var(--jc-space-3)] text-sm outline-none focus:border-strong"
              data-focusable="true"
            >
              <option value="ssl">SSL（465，隐式 TLS）</option>
              <option value="starttls">STARTTLS（587，必须支持升级）</option>
              <option value="none">无加密（不推荐，仅内网）</option>
            </select>
          </ManageFormField>
          <ManageFormField class="flex-1" label="发件账号" required hint="一般为完整邮箱地址">
            <ManageInput v-model="form.smtpUser" placeholder="mail@example.com" />
          </ManageFormField>
        </div>

        <div class="flex flex-col gap-[var(--jc-space-4)] sm:flex-row">
          <ManageFormField class="flex-1" label="授权码 / 密码" required :hint="passHint">
            <ManageInput
              v-model="passDraft"
              type="password"
              :placeholder="passSet ? '留空 = 不修改' : '输入授权码 / 密码'"
            />
          </ManageFormField>
          <ManageFormField class="flex-1" label="发件人显示名" hint="收件人看到的昵称">
            <ManageInput v-model="form.smtpFromName" placeholder="Jerocine影视" />
          </ManageFormField>
        </div>

        <ManageFormField
          label="每日出站上限"
          hint="验证码 / 激活 / 驳回 / 找回 / 测试邮件统一计数，超出后当天停发；1~1000，默认 50"
        >
          <ManageInput v-model="form.mailDailyQuota" type="number" />
        </ManageFormField>

        <p v-if="form.smtpEnabled === 1 && smtpIncomplete" class="text-xs text-[var(--jc-warning)]">
          SMTP 配置不完整：启用后服务器 / 账号 / 授权码均必填，否则发码会被拒绝。
        </p>
      </div>

      <div class="flex justify-end gap-[var(--jc-space-3)]">
        <BaseButton variant="ghost" type="button" @click="load">重置</BaseButton>
        <BaseButton variant="gradient" type="submit" :loading="submitting">保存</BaseButton>
      </div>
    </form>

    <div class="border-t border-[var(--jc-border)]" />

    <section>
      <header class="mb-[var(--jc-space-3)]">
        <h3 class="font-[var(--jc-fw-semibold)]">测试邮件</h3>
        <p class="text-sm text-muted">
          用当前表单值（未保存也生效）向指定邮箱发送一封测试邮件；失败时回显 SMTP
          原始错误（如 535 鉴权失败、连接超时），便于定位。
        </p>
      </header>
      <div class="flex flex-col items-stretch gap-[var(--jc-space-3)] sm:flex-row sm:items-end">
        <ManageFormField class="flex-1" label="测试收件邮箱">
          <ManageInput v-model="testTo" placeholder="mail@example.com" />
        </ManageFormField>
        <BaseButton variant="outline" :loading="testing" @click="submitTest">发送测试邮件</BaseButton>
      </div>
    </section>
  </section>
</template>

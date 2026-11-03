<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { MailConfigUpdate, MailConfigResp, SmtpPreset } from '@/types/manage'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import type { SelectOption } from '@/components/manage/ManageSelect.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 注册与邮件通道卡片（原「邮件与注册」独立页并入「系统设置」）。
 * 保存走 PUT /manage/mail/config；凭据只写不读（留空 = 不修改）。
 * 写操作口径: useAsyncAction + {silent:true}, 成功/失败文案 run 单点。
 */
const loading = ref(true)
/** 加载失败: 展示错误态 + 重试 —— 默认值表单可直接把线上注册模式/SMTP 开关静默覆盖, 必须拦住 */
const loadError = ref(false)
const presets = ref<SmtpPreset[]>([])

const form = reactive({
  registerMode: 'off' as MailConfigResp['registerMode'],
  smtpEnabled: 0,
  smtpHost: '',
  // ManageInput 数字框清空后会发 undefined（见 normalizePort 的回落处理）
  smtpPort: 465 as number | undefined,
  smtpSecure: 'ssl' as SmtpPreset['secure'],
  smtpUser: '',
  smtpFromName: '',
  /** 全局出站日配额（含测试邮件）；后端 1~1000，默认 50 */
  mailDailyQuota: 50 as number | undefined
})
/** 授权码输入框只写不读: 留空 = 不修改已存值(与 TMDB Key 同范式) */
const passDraft = ref('')
const passSet = ref(false)
const passMasked = ref('')
const testTo = ref('')

const modeOptions: SelectOption[] = [
  { value: 'off', label: '关闭（仅后台创建账号）' },
  { value: 'open', label: '开放注册（邮箱验证码，注册即登录）' },
  { value: 'apply', label: '申请审核（提交申请 → 管理员审核 → 邮件激活）' }
]

const SECURE_OPTIONS: SelectOption[] = [
  { value: 'ssl', label: 'SSL（465，隐式 TLS）' },
  { value: 'starttls', label: 'STARTTLS（587，必须支持升级）' },
  { value: 'none', label: '无加密（不推荐，仅内网）' }
]

/** 服务商预设是临时选择(仅回填, 不入库): 初值 undefined 走 ManageSelect placeholder */
const presetId = ref<string | number | undefined>(undefined)
const presetOptions = computed<SelectOption[]>(() =>
  presets.value.map((p) => ({ value: p.id, label: `${p.name}${p.note ? ` — ${p.note}` : ''}` }))
)

const { busy: submitting, run } = useAsyncAction()
const { busy: testing, run: runTest } = useAsyncAction()

async function load(): Promise<void> {
  loading.value = true
  loadError.value = false
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
  } catch {
    // 失败时不按默认值渲染可编辑表单: registerMode/SMTP 开关会被静默写坏
    loadError.value = true
  } finally {
    loading.value = false
  }
}

/** 选预设仅回填 host/port/secure 默认值; 不在预设表内的服务商手改即可 */
function applyPreset(id: string | number): void {
  const p = presets.value.find((x) => x.id === String(id))
  if (!p) return
  form.smtpHost = p.host
  form.smtpPort = p.port
  form.smtpSecure = p.secure
}

/** 端口归一: 数字框清空时 ManageInput 给 undefined，回落默认 465；其余转数字 */
function normalizePort(): number {
  if (form.smtpPort === undefined) return 465
  const n = Number(form.smtpPort)
  return Number.isNaN(n) ? 0 : n
}

function buildPayload(): MailConfigUpdate {
  const pass = passDraft.value.trim()
  return {
    registerMode: form.registerMode,
    smtpEnabled: form.smtpEnabled,
    smtpHost: form.smtpHost.trim(),
    smtpPort: normalizePort(),
    smtpSecure: form.smtpSecure,
    smtpUser: form.smtpUser.trim(),
    ...(pass ? { smtpPass: pass } : {}),
    smtpFromName: form.smtpFromName.trim(),
    // 数字框清空时 ManageInput 会给 undefined，统一收敛成 0（后端语义：0 = 不修改，保留现值）
    mailDailyQuota: Number(form.mailDailyQuota) || 0
  }
}

async function submit(): Promise<void> {
  // 启用 SMTP 但配置不完整: 前端先拦下, 别让后端一句英文 "invalid argument" 让管理员无从下手
  if (form.smtpEnabled === 1 && smtpIncomplete.value) {
    toast('error', '启用 SMTP 需填写服务器 / 账号 / 授权码')
    return
  }
  const port = normalizePort()
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    toast('error', 'SMTP 端口需为 1~65535 的整数')
    return
  }
  // 0 = 不修改（后端保留现值），1~1000 为合法区间
  const quota = Number(form.mailDailyQuota) || 0
  if (quota !== 0 && (quota < 1 || quota > 1000)) {
    toast('error', '每日出站上限需在 1~1000 之间（清空表示不修改）')
    return
  }
  await run({
    action: async () => {
      const data = await manageApi.mail.saveMailConfig(buildPayload(), { silent: true })
      passDraft.value = ''
      passSet.value = data.smtpPassSet
      passMasked.value = data.smtpPassMasked
    },
    successMsg: '注册与邮件配置已保存'
  })
}

/** 测试邮件与保存解耦: 用当前表单值直发, 失败回显 SMTP 原文错误便于定位 */
async function submitTest(): Promise<void> {
  const to = testTo.value.trim()
  if (!to) {
    toast('error', '请先填写测试收件邮箱')
    return
  }
  // 与 server ValidEmail 同则的格式预校验: 别让 400 "invalid argument" 代替人话
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(to)) {
    toast('error', '测试收件邮箱格式不正确')
    return
  }
  await runTest({
    action: () => manageApi.mail.testMail({ ...buildPayload(), to }, { silent: true }),
    successMsg: `测试邮件已发送至 ${to}，请查收`
  })
}

const smtpIncomplete = computed(
  () => !form.smtpHost.trim() || !form.smtpUser.trim() || !(passSet.value || passDraft.value.trim())
)

/** 邮件通道不可用警告: 覆盖两种情况 —— 启用了但配置不完整 / 注册模式(open/apply)依赖邮件通道却没开或没配好 */
const mailChannelWarning = computed(() => {
  if (form.registerMode !== 'off' && (form.smtpEnabled === 0 || smtpIncomplete.value)) {
    return '当前注册模式依赖邮件通道：请先启用并配好 SMTP（服务器 / 账号 / 授权码），否则注册邮件会发送失败。'
  }
  if (form.smtpEnabled === 1 && smtpIncomplete.value) {
    return 'SMTP 配置不完整：启用后服务器 / 账号 / 授权码均必填，否则发码会被拒绝。'
  }
  return ''
})

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

    <div
      v-else-if="loadError"
      class="flex flex-col items-center gap-[var(--jc-space-3)] py-[var(--jc-space-8)]"
    >
      <p class="text-sm text-[var(--jc-danger)]">
        邮件与注册配置读取失败：为避免用默认值覆盖线上配置（注册模式 / SMTP
        开关），已禁用编辑与保存。
      </p>
      <BaseButton variant="outline" size="sm" @click="load">重试</BaseButton>
    </div>

    <form v-else class="flex flex-col gap-[var(--jc-space-5)]" @submit.prevent="submit">
      <ManageFormField label="注册模式" hint="open 需配置邮件通道；apply 的激活邮件同样经 SMTP 发送">
        <ManageSelect
          :model-value="form.registerMode"
          :options="modeOptions"
          aria-label="注册模式"
          @update:model-value="form.registerMode = $event as MailConfigResp['registerMode']"
        />
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
          <ManageSelect
            v-model="presetId"
            :options="presetOptions"
            placeholder="选择预设（可跳过）"
            aria-label="服务商预设"
            @update:model-value="applyPreset"
          />
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
            <ManageSelect
              :model-value="form.smtpSecure"
              :options="SECURE_OPTIONS"
              aria-label="加密方式"
              @update:model-value="form.smtpSecure = $event as SmtpPreset['secure']"
            />
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

        <p v-if="mailChannelWarning" class="text-xs text-[var(--jc-warning)]">
          {{ mailChannelWarning }}
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

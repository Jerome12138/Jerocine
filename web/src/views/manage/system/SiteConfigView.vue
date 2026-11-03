<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { RuntimeSettings, SiteBasic } from '@/types/manage'
import { useSiteStore } from '@/stores/site'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageTextarea from '@/components/manage/ManageTextarea.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'
import MailConfigCard from './MailConfigCard.vue'
import DanmuConfigCard from './DanmuConfigCard.vue'

/**
 * 系统设置（原「站点配置」+「邮件与注册」合并）。
 * 卡片式响应式布局: 窄屏 1 列 / 宽屏 2 列, **每张卡恒占一列**(不再有跨列卡片, 避免行内留空、
 * 上下对不齐); 每张卡各自独立保存, 互不影响。
 *
 * 写操作口径: useAsyncAction 收敛 busy+toast 单点, 请求传 {silent:true} 关拦截器提示
 * (成功/失败文案由 run 单点负责); 各卡独立实例, busy 互不影响。
 */
const siteStore = useSiteStore()
const loading = ref(true)
/** 首屏加载失败: 各卡保存禁用 —— 否则空表单/默认值会把库内配置整体覆盖掉 */
const loadError = ref(false)
const form = reactive<SiteBasic>({
  siteName: '',
  logo: '',
  keyword: '',
  describe: '',
  domain: '',
  state: true,
  hint: ''
})

// ---- 各卡独立 useAsyncAction(busy 分卡互不影响) ----
const { busy: submitting, run: runSite } = useAsyncAction()
const { busy: keyBusy, run: runKey } = useAsyncAction()
const { busy: accountBusy, run: runAccount } = useAsyncAction()
const { busy: spiderBusy, run: runSpider } = useAsyncAction()

// ---- TMDB 凭据(独立于基础配置保存, 明文永不回传) ----
const tmdb = reactive({ masked: '', set: false })
const newKey = ref('')

// ---- 运行参数(账号与安全 / 采集性能) ----
// 两张卡绑同一组草稿字段, 但**各自独立提交**: 保存时用 runtimeSaved(上次落库快照)补齐
// 另一张卡的字段 —— 否则在一张卡里改了值没保存, 去另一张卡点保存会把它一起写库。
const DEFAULT_RUNTIME: RuntimeSettings = {
  authTokenTtlHours: 168,
  maxDeviceLogin: 5,
  spiderMaxGoroutine: 32,
  spiderMasterMaxWorkers: 12,
  spiderSlaveMaxWorkers: 8
}
const runtime = reactive<RuntimeSettings>({ ...DEFAULT_RUNTIME })
let runtimeSaved: RuntimeSettings = { ...DEFAULT_RUNTIME }

/** 读 TMDB 掩码状态。初次加载直接复用 getBasic 已取回的响应（省一次同端点请求）；
 *  保存/清除 Key 后不传参，走独立 GET 拿最新状态。 */
async function loadKey(initial?: { masked: string; set: boolean }): Promise<void> {
  const k = initial ?? (await manageApi.system.getTMDBKey())
  tmdb.masked = k.masked
  tmdb.set = k.set
}

async function loadRuntime(): Promise<void> {
  const data = await manageApi.system.getRuntimeSettings()
  runtimeSaved = { ...data }
  Object.assign(runtime, data)
}

async function saveKey(): Promise<void> {
  const key = newKey.value.trim()
  if (!key) {
    toast('error', '请输入 TMDB API Key 或 Read Access Token')
    return
  }
  await runKey({
    action: async () => {
      await manageApi.system.setTMDBKey(key, { silent: true })
      newKey.value = ''
      await loadKey()
    },
    successMsg: 'TMDB Key 已保存, 横图回填即将生效'
  })
}

async function clearKey(): Promise<void> {
  await runKey({
    confirm: {
      title: '清除 TMDB Key',
      desc: '清除后横图（backdrop）回填停用，已落地图不受影响。确定清除吗？',
      okText: '清除',
      danger: true
    },
    action: async () => {
      await manageApi.system.clearTMDBKey({ silent: true })
      await loadKey()
    },
    successMsg: '已清除 TMDB Key, 横图回填已停用'
  })
}

async function load(): Promise<void> {
  loading.value = true
  loadError.value = false
  try {
    const data = await manageApi.system.getBasic()
    Object.assign(form, data)
    // TMDB 掩码直接复用本次响应, 不再对同一端点发第二个 GET
    loadKey(data)
    await loadRuntime()
  } catch {
    // 失败必须拦住: 表单是空字段、运行参数是默认值, 此时允许保存会把库内配置整体覆盖
    loadError.value = true
  } finally {
    loading.value = false
  }
}

/** 只重取站点基础信息回填本卡表单 —— 不动运行参数/TMDB, 避免冲掉别的卡里未保存的草稿 */
async function resetBasic(): Promise<void> {
  try {
    const data = await manageApi.system.getBasic()
    Object.assign(form, data)
    loadKey(data)
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '重置失败')
  }
}

async function submit(): Promise<void> {
  if (!form.siteName.trim()) {
    toast('error', '请填写站点名称')
    return
  }
  await runSite({
    action: async () => {
      await manageApi.system.updateBasic({ ...form }, { silent: true })
      // 本地映射到共享 store(后端字段 description/state 与页面 describe/state:boolean 不同)
      siteStore.basic = {
        id: 1,
        siteName: form.siteName,
        domain: form.domain ?? '',
        logo: form.logo,
        keyword: form.keyword,
        description: form.describe,
        state: form.state ? 0 : 1,
        hint: form.hint ?? '',
        updatedAt: Date.now()
      }
    },
    successMsg: '站点信息已保存'
  })
}

/** 提交运行参数: 只取本卡字段, 其余用上次快照(见 runtimeSaved 注释) */
async function putRuntime(
  patch: Partial<RuntimeSettings>,
  run: typeof runAccount
): Promise<void> {
  await run({
    action: async () => {
      const data = await manageApi.system.updateRuntimeSettings({ ...runtimeSaved, ...patch }, { silent: true })
      // 只更新快照, **不要**把响应整体 assign 回草稿: 另一张卡里未保存的改动会被快照值悄悄冲掉
      runtimeSaved = { ...data }
    },
    successMsg: '运行参数已保存'
  })
}

/** 账号与安全(登录有效期 + 设备数上限) */
async function saveAccount(): Promise<void> {
  if (runtime.authTokenTtlHours < 1 || runtime.authTokenTtlHours > 8760) {
    toast('error', '登录有效期需在 1~8760 小时之间')
    return
  }
  if (runtime.maxDeviceLogin < 1 || runtime.maxDeviceLogin > 50) {
    toast('error', '设备数上限需在 1~50 之间')
    return
  }
  await putRuntime(
    {
      authTokenTtlHours: Number(runtime.authTokenTtlHours),
      maxDeviceLogin: Number(runtime.maxDeviceLogin)
    },
    runAccount
  )
}

/** 采集性能(总并发 + 主站/从站硬上限) */
async function saveSpider(): Promise<void> {
  const caps: [number, string][] = [
    [runtime.spiderMaxGoroutine, '采集并发'],
    [runtime.spiderMasterMaxWorkers, '主站并发'],
    [runtime.spiderSlaveMaxWorkers, '从站并发']
  ]
  const bad = caps.find(([v]) => v < 1 || v > 256)
  if (bad) {
    toast('error', `${bad[1]}需在 1~256 之间`)
    return
  }
  await putRuntime(
    {
      spiderMaxGoroutine: Number(runtime.spiderMaxGoroutine),
      spiderMasterMaxWorkers: Number(runtime.spiderMasterMaxWorkers),
      spiderSlaveMaxWorkers: Number(runtime.spiderSlaveMaxWorkers)
    },
    runSpider
  )
}

onMounted(load)
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-4)]">
    <header class="flex flex-wrap items-end justify-between gap-2">
      <div>
        <h1 class="text-lg font-[var(--jc-fw-semibold)]">系统设置</h1>
        <p class="text-sm text-muted">
          站点信息 / 注册与邮件 / TMDB / 账号与安全 / 采集性能 —— 每张卡独立保存，保存即生效（标注"下次生效"的除外）
        </p>
      </div>
    </header>

    <!-- 加载失败: 明确错误态 + 重试; 各卡保存禁用, 防止以空值/默认值覆盖库内配置 -->
    <section
      v-if="loadError"
      class="bg-surface rounded-card shadow-card p-[var(--jc-space-4)] flex flex-wrap items-center justify-between gap-[var(--jc-space-3)]"
    >
      <p class="text-sm text-[var(--jc-danger)]">
        系统设置加载失败：为避免用空值 / 默认值覆盖线上配置，各卡片的保存按钮已禁用。
      </p>
      <ManageButton variant="outline" size="sm" @click="load">重试</ManageButton>
    </section>

    <div class="grid grid-cols-1 gap-[var(--jc-space-5)] xl:grid-cols-2">
      <!-- 站点信息 -->
      <section
        class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)] flex flex-col gap-[var(--jc-space-5)]"
      >
        <header>
          <h2 class="text-lg font-[var(--jc-fw-semibold)]">站点信息</h2>
          <p class="mt-1 text-sm text-muted">站名、Logo、SEO 与维护提示；域名同时用于邮件里的激活/重置链接</p>
        </header>

        <div v-if="loading" class="flex flex-col gap-[var(--jc-space-3)]">
          <BaseSkeleton v-for="i in 5" :key="i" shape="rect" height="44px" />
        </div>

        <form
          v-else
          class="flex flex-1 flex-col gap-[var(--jc-space-4)]"
          @submit.prevent="submit"
        >
          <div class="grid grid-cols-1 gap-[var(--jc-space-4)] sm:grid-cols-2">
            <ManageFormField label="站点名称" required>
              <ManageInput v-model="form.siteName" />
            </ManageFormField>
            <ManageFormField label="域名" hint="完整地址，https:// 开头；邮件链接用它拼">
              <ManageInput v-model="form.domain!" placeholder="https://example.com" />
            </ManageFormField>
            <ManageFormField label="Logo URL（浏览器标签页图标）">
              <ManageInput v-model="form.logo" placeholder="https://...（图标 URL）" />
            </ManageFormField>
            <ManageFormField label="SEO 关键词">
              <ManageInput v-model="form.keyword" placeholder="逗号分隔" />
            </ManageFormField>
          </div>
          <ManageFormField label="SEO 描述">
            <ManageTextarea v-model="form.describe" :rows="2" />
          </ManageFormField>
          <ManageFormField label="维护提示" hint="站点关闭时给访客的提示语">
            <ManageInput v-model="form.hint!" placeholder="例：站点正在维护，敬请稍候" />
          </ManageFormField>

          <div class="mt-auto flex justify-end gap-[var(--jc-space-3)]">
            <ManageButton variant="ghost" type="button" @click="resetBasic">重置</ManageButton>
            <ManageButton
              variant="gradient"
              type="submit"
              :loading="submitting"
              :disabled="loadError"
            >
              保存
            </ManageButton>
          </div>
        </form>
      </section>

      <!-- 注册与邮件通道(原独立页) -->
      <MailConfigCard />

      <!-- 弹幕服务(后台配置项, 默认不启用) -->
      <DanmuConfigCard />

      <!-- TMDB -->
      <section
        class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)] flex flex-col gap-[var(--jc-space-4)]"
      >
        <header class="flex flex-wrap items-center gap-[var(--jc-space-2)]">
          <h2 class="text-lg font-[var(--jc-fw-semibold)]">TMDB 横图</h2>
          <span
            class="rounded-full px-2 py-0.5 text-xs"
            :class="tmdb.set ? 'bg-primary/10 text-primary' : 'bg-muted/20 text-muted'"
            :title="tmdb.set ? `当前凭据：${tmdb.masked}` : '尚未保存凭据'"
          >
            {{ tmdb.set ? `已配置 ${tmdb.masked}` : '未配置' }}
          </span>
        </header>
        <p class="text-sm text-muted">
          用于首页轮播横图（backdrop）回填。支持 v3 API Key 或 v4 Read Access Token，保存前自动验真；
          清除后回填停用，已落地图不受影响。
        </p>
        <p class="text-xs text-muted">
          凭据只存在数据库，后台是唯一权威（不再读环境变量）；若此前的 TMDB Key 是配在
          <code>.env</code> 里的，需在这里重新保存一次。
        </p>
        <ManageFormField label="新 Key" hint="留空不改动；仅显示中间脱敏掩码，明文保存后不可再查看">
          <ManageInput v-model="newKey" placeholder="粘贴 v3 API Key 或 v4 Read Access Token" />
        </ManageFormField>
        <div class="mt-auto flex justify-end gap-[var(--jc-space-3)]">
          <ManageButton
            v-if="tmdb.set"
            variant="ghost"
            :disabled="keyBusy || loadError"
            @click="clearKey"
          >
            清除
          </ManageButton>
          <ManageButton variant="gradient" :loading="keyBusy" :disabled="loadError" @click="saveKey">
            保存 Key
          </ManageButton>
        </div>
      </section>

      <!-- 账号与安全 -->
      <section
        class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)] flex flex-col gap-[var(--jc-space-4)]"
      >
        <header>
          <h2 class="text-lg font-[var(--jc-fw-semibold)]">账号与安全</h2>
          <p class="mt-1 text-sm text-muted">登录策略：改动在<b>下次登录</b>时生效，不会立即踢掉在线设备</p>
        </header>
        <ManageFormField label="登录有效期（小时）" hint="只影响之后新签发的 token；默认 168（7 天）">
          <ManageInput v-model="runtime.authTokenTtlHours" type="number" />
        </ManageFormField>
        <ManageFormField label="单账号设备数上限" hint="超出后下次登录会顶掉最旧设备；默认 5">
          <ManageInput v-model="runtime.maxDeviceLogin" type="number" />
        </ManageFormField>
        <div class="mt-auto flex justify-end">
          <ManageButton
            variant="gradient"
            :loading="accountBusy"
            :disabled="loadError"
            @click="saveAccount"
          >
            保存
          </ManageButton>
        </div>
      </section>

      <!-- 采集性能 -->
      <section
        class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)] flex flex-col gap-[var(--jc-space-4)]"
      >
        <header>
          <h2 class="text-lg font-[var(--jc-fw-semibold)]">采集性能</h2>
          <p class="mt-1 text-sm text-muted">改动在<b>下一轮采集</b>生效，不打断进行中的轮次</p>
        </header>
        <ManageFormField
          label="采集并发上限（总）"
          hint="默认 32；实际并发 = min(本值, 主站/从站硬上限, 本轮页数)"
        >
          <ManageInput v-model="runtime.spiderMaxGoroutine" type="number" />
        </ManageFormField>
        <ManageFormField
          label="主站并发硬上限"
          hint="默认 12；主站每页写影片/检索/播放源三表，过高会批量死锁"
        >
          <ManageInput v-model="runtime.spiderMasterMaxWorkers" type="number" />
        </ManageFormField>
        <ManageFormField
          label="从站并发硬上限"
          hint="默认 8；只写播放源行，但并发过高会触发源站限流（403）"
        >
          <ManageInput v-model="runtime.spiderSlaveMaxWorkers" type="number" />
        </ManageFormField>
        <div class="mt-auto flex justify-end">
          <ManageButton
            variant="gradient"
            :loading="spiderBusy"
            :disabled="loadError"
            @click="saveSpider"
          >
            保存
          </ManageButton>
        </div>
      </section>
    </div>
  </div>
</template>

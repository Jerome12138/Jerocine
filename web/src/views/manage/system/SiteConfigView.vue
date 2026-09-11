<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { SiteBasic } from '@/types/manage'
import { useSiteStore } from '@/stores/site'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageTextarea from '@/components/manage/ManageTextarea.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'

const siteStore = useSiteStore()
const loading = ref(true)
const submitting = ref(false)
const form = reactive<SiteBasic>({
  siteName: '',
  logo: '',
  keyword: '',
  describe: '',
  domain: '',
  state: true,
  hint: ''
})

// ---- TMDB 凭据(独立于基础配置保存, 明文永不回传) ----
const tmdb = reactive({ masked: '', set: false })
const newKey = ref('')
const keyBusy = ref(false)

async function loadKey(): Promise<void> {
  const k = await manageApi.system.getTMDBKey()
  tmdb.masked = k.masked
  tmdb.set = k.set
}

async function saveKey(): Promise<void> {
  const key = newKey.value.trim()
  if (!key) {
    toast('error', '请输入 TMDB API Key 或 Read Access Token')
    return
  }
  keyBusy.value = true
  try {
    await manageApi.system.setTMDBKey(key)
    newKey.value = ''
    await loadKey()
    toast('success', 'TMDB Key 已保存, 横图回填即将生效')
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '保存失败')
  } finally {
    keyBusy.value = false
  }
}

async function clearKey(): Promise<void> {
  keyBusy.value = true
  try {
    await manageApi.system.clearTMDBKey()
    await loadKey()
    toast('success', '已清除 TMDB Key, 横图回填已停用')
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '清除失败')
  } finally {
    keyBusy.value = false
  }
}

async function load(): Promise<void> {
  loading.value = true
  try {
    const data = await manageApi.system.getBasic()
    Object.assign(form, data)
    await loadKey()
  } finally {
    loading.value = false
  }
}

async function submit(): Promise<void> {
  submitting.value = true
  try {
    await manageApi.system.updateBasic({ ...form })
    // TODO(manage): manage/system api 仍用旧 SiteBasic(describe/state:bool); 映射到新 SiteConfig 供共享 store。
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
  } finally {
    submitting.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="bg-surface rounded-card shadow-card p-[var(--gf-space-6)] max-w-[820px]">
    <header class="mb-[var(--gf-space-5)]">
      <h2 class="text-lg font-[var(--gf-fw-semibold)]">站点配置</h2>
      <p class="text-sm text-muted">维护站名、Logo、SEO 与备案信息</p>
    </header>

    <div v-if="loading" class="flex flex-col gap-[var(--gf-space-3)]">
      <BaseSkeleton v-for="i in 6" :key="i" shape="rect" height="48px" />
    </div>

    <form
      v-else
      class="flex flex-col gap-[var(--gf-space-5)]"
      @submit.prevent="submit"
    >
      <ManageFormField label="站点名称" required>
        <ManageInput v-model="form.siteName" />
      </ManageFormField>
      <ManageFormField label="Logo URL">
        <ManageInput v-model="form.logo" placeholder="https://..." />
      </ManageFormField>
      <ManageFormField label="域名">
        <ManageInput v-model="form.domain!" placeholder="https://example.com" />
      </ManageFormField>
      <ManageFormField label="SEO 关键词">
        <ManageInput v-model="form.keyword" placeholder="逗号分隔" />
      </ManageFormField>
      <ManageFormField label="SEO 描述">
        <ManageTextarea v-model="form.describe" :rows="3" />
      </ManageFormField>
      <ManageFormField label="维护提示" hint="站点关闭时给访客的提示语">
        <ManageInput v-model="form.hint!" placeholder="例：站点正在维护，敬请稍候" />
      </ManageFormField>

      <div class="flex justify-end gap-[var(--gf-space-3)]">
        <BaseButton variant="ghost" type="button" @click="load">重置</BaseButton>
        <BaseButton variant="gradient" type="submit" :loading="submitting">保存</BaseButton>
      </div>
    </form>

    <div class="my-[var(--gf-space-5)] border-t border-[var(--gf-border)]" />

    <section>
      <header class="mb-[var(--gf-space-3)] flex items-center gap-[var(--gf-space-2)]">
        <h3 class="font-[var(--gf-fw-semibold)]">TMDB API Key</h3>
        <span
          class="rounded-full px-2 py-0.5 text-xs"
          :class="tmdb.set ? 'bg-primary/10 text-primary' : 'bg-muted/20 text-muted'"
        >
          {{ tmdb.set ? `已配置 ${tmdb.masked}` : '未配置' }}
        </span>
      </header>
      <p class="mb-[var(--gf-space-3)] text-sm text-muted">
        用于首页轮播横图（backdrop）回填。支持 v3 API Key（32 位十六进制）或 v4 Read Access Token（JWT），保存前会自动验真；
        清除后横图回填停用，已落地的图片不受影响。申请方式见仓库
        <code class="text-primary">docs/TMDB-API-Key申请与配置.md</code>。
      </p>
      <div class="flex items-start gap-[var(--gf-space-3)]">
        <ManageFormField class="flex-1" label="新 Key" hint="留空不改动；仅显示掩码，明文保存后不可再查看">
          <ManageInput v-model="newKey" placeholder="粘贴 v3 API Key 或 v4 Read Access Token" />
        </ManageFormField>
        <div class="flex shrink-0 items-end gap-[var(--gf-space-3)] pt-[26px]">
          <BaseButton variant="gradient" :loading="keyBusy" @click="saveKey">保存 Key</BaseButton>
          <BaseButton v-if="tmdb.set" variant="ghost" :disabled="keyBusy" @click="clearKey">清除</BaseButton>
        </div>
      </div>
    </section>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
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

async function load(): Promise<void> {
  loading.value = true
  try {
    const data = await manageApi.system.getBasic()
    Object.assign(form, data)
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
  </section>
</template>

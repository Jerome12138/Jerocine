<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { DanmuConfigResp, DanmuProvider } from '@/types/manage'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'

/**
 * 弹幕服务卡片（后台配置项，默认不启用）。
 * 保存走 PUT /manage/danmu-config；总开关 + 平台白名单独立保存，保存即生效
 * （后端保存后清 v1:danmu:src:* 来源候选缓存，下次进片按新白名单解析）。
 * 平台要可用需「壳 ID_PREFIX」同步启用 —— 本卡片只控制 Go/前端开放面。
 */
const loading = ref(true)
const submitting = ref(false)

const form = reactive({
  enabled: 0,
  providers: [] as DanmuProvider[]
})
const allProviders = ref<DanmuProvider[]>([])

const providerLabels: Record<DanmuProvider, string> = {
  bilibili: '哔哩哔哩',
  tencent: '腾讯视频',
  iqiyi: '爱奇艺',
  youku: '优酷'
}

function toggleProvider(p: DanmuProvider, on: boolean): void {
  const set = new Set(form.providers)
  if (on) set.add(p)
  else set.delete(p)
  // 保持 all 顺序展示
  form.providers = allProviders.value.filter((x) => set.has(x))
}

async function load(): Promise<void> {
  loading.value = true
  try {
    const data = await manageApi.danmu.getDanmuConfig()
    form.enabled = data.enabled
    form.providers = data.providers
    allProviders.value = data.all
  } finally {
    loading.value = false
  }
}

async function submit(): Promise<void> {
  if (form.enabled === 1 && form.providers.length === 0) {
    toast('error', '启用弹幕需至少勾选一个开放平台')
    return
  }
  submitting.value = true
  try {
    const data = await manageApi.danmu.saveDanmuConfig({
      enabled: form.enabled,
      providers: form.providers
    })
    form.enabled = data.enabled
    form.providers = data.providers
    allProviders.value = data.all
    toast('success', '弹幕配置已保存，播放页下次进片生效')
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '保存失败')
  } finally {
    submitting.value = false
  }
}

onMounted(load)
</script>

<template>
  <section
    class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)] flex flex-col gap-[var(--jc-space-5)]"
  >
    <header>
      <h2 class="text-lg font-[var(--jc-fw-semibold)]">弹幕服务</h2>
      <p class="mt-1 text-sm text-muted">
        总开关默认关闭；开启后按开放平台在播放页提供弹幕来源候选（B站整集聚合 /
        腾讯 / 爱奇艺 / 优酷分段点播）。
      </p>
    </header>

    <div v-if="loading" class="flex flex-col gap-[var(--jc-space-3)]">
      <BaseSkeleton v-for="i in 5" :key="i" shape="rect" height="44px" />
    </div>

    <form v-else class="flex flex-1 flex-col gap-[var(--jc-space-5)]" @submit.prevent="submit">
      <div class="flex items-center gap-[var(--jc-space-3)]">
        <ManageSwitch
          :model-value="form.enabled === 1"
          @update:model-value="form.enabled = $event ? 1 : 0"
        />
        <span class="text-sm" :class="form.enabled === 1 ? 'text-primary' : 'text-muted'">
          {{ form.enabled === 1 ? '已启用' : '未启用（默认）' }}
        </span>
      </div>

      <div
        v-if="form.enabled === 1"
        class="flex flex-col gap-[var(--jc-space-3)] border-t border-[var(--jc-border)] pt-[var(--jc-space-4)]"
      >
        <ManageFormField
          label="开放平台"
          hint="勾选平台后，播放页才会为该片解析并展示对应弹幕源；至少勾选一个"
        >
          <div class="flex flex-col gap-2">
            <label
              v-for="p in allProviders"
              :key="p"
              class="flex cursor-pointer items-center gap-2 rounded-[var(--jc-radius-md)] border border-default bg-elevated px-3 py-2 text-sm hover:border-strong"
            >
              <input
                type="checkbox"
                class="h-4 w-4 accent-[var(--jc-primary)]"
                :checked="form.providers.includes(p)"
                @change="toggleProvider(p, ($event.target as HTMLInputElement).checked)"
              />
              <span class="text-primary">{{ providerLabels[p] ?? p }}</span>
              <code class="ml-auto text-xs text-muted">{{ p }}</code>
            </label>
          </div>
        </ManageFormField>

        <p v-if="form.providers.length === 0" class="text-xs text-[var(--jc-warning)]">
          尚未勾选任何平台：即便总开关开启，播放页也不会出现弹幕。
        </p>
      </div>

      <div class="mt-auto flex justify-end gap-[var(--jc-space-3)]">
        <BaseButton variant="ghost" type="button" @click="load">重置</BaseButton>
        <BaseButton variant="gradient" type="button" :loading="submitting" @click="submit">保存</BaseButton>
      </div>
    </form>
  </section>
</template>

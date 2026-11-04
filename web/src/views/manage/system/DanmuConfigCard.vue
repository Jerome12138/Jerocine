<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { DanmuConfigResp, DanmuProvider } from '@/types/manage'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageCheckbox from '@/components/manage/ManageCheckbox.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 弹幕服务卡片（后台配置项，默认不启用）。
 * 保存走 PUT /manage/danmu-config；总开关 + 平台白名单整体保存。
 * 生效路径：后端保存后失效配置缓存，弹幕服务的进程内配置快照 30s 刷新 —— 最长 30s 延迟生效。
 * 平台白名单当前仅随配置存库（预留开放面控制，与壳 ID_PREFIX 对应），
 * 聚合链路（dandanplay 按标题整集匹配）暂不按平台过滤。
 * 写操作口径: useAsyncAction + {silent:true}, 成功/失败文案 run 单点。
 */
const loading = ref(true)
/** 加载失败: 展示错误态, 不渲染可编辑的默认表单 —— 否则误点保存会把 {enabled:0} 写库全局关停 */
const loadError = ref(false)
const { busy: submitting, run } = useAsyncAction()

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
  loadError.value = false
  try {
    const data = await manageApi.danmu.getDanmuConfig()
    form.enabled = data.enabled
    form.providers = data.providers
    allProviders.value = data.all
  } catch {
    // 失败时不以默认值(enabled=0)渲染可编辑表单: 误点保存会把全局弹幕关停
    loadError.value = true
  } finally {
    loading.value = false
  }
}

async function submit(): Promise<void> {
  if (form.enabled === 1 && form.providers.length === 0) {
    toast('error', '启用弹幕需至少勾选一个开放平台')
    return
  }
  await run({
    action: async () => {
      const data = await manageApi.danmu.saveDanmuConfig(
        {
          enabled: form.enabled,
          providers: form.providers
        },
        { silent: true }
      )
      form.enabled = data.enabled
      form.providers = data.providers
      allProviders.value = data.all
    },
    successMsg: '弹幕配置已保存，播放页下次进片生效'
  })
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
        总开关默认关闭；开启后播放页按标题提供 dandanplay
        聚合弹幕（整集匹配）。平台勾选为开放面白名单（与弹幕壳
        ID_PREFIX 对应），当前聚合链路不按平台过滤。
      </p>
    </header>

    <div v-if="loading" class="flex flex-col gap-[var(--jc-space-3)]">
      <BaseSkeleton v-for="i in 5" :key="i" shape="rect" height="44px" />
    </div>

    <div
      v-else-if="loadError"
      class="flex flex-col items-center gap-[var(--jc-space-3)] py-[var(--jc-space-8)]"
    >
      <p class="text-sm text-[var(--jc-danger)]">
        弹幕配置读取失败：为避免误存默认值（全局关停），已禁用编辑与保存。
      </p>
      <ManageButton variant="outline" size="sm" @click="load">重试</ManageButton>
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
          hint="勾选的平台作为开放面白名单保存（当前聚合弹幕按标题匹配、不按平台过滤）；启用时至少勾选一个"
        >
          <div class="flex flex-col gap-2">
            <!-- ManageCheckbox 统一勾选框: 原裸 input 的 accent 悬空 token(--jc-primary)随之消灭;
                 卡片样式经 attrs 透传到组件根 label, 文案与平台码走默认 slot -->
            <ManageCheckbox
              v-for="p in allProviders"
              :key="p"
              class="w-full rounded-[var(--jc-radius-md)] border border-default bg-elevated px-3 py-2 text-sm hover:border-strong"
              :model-value="form.providers.includes(p)"
              @update:model-value="toggleProvider(p, $event)"
            >
              <span class="text-primary">{{ providerLabels[p] ?? p }}</span>
              <code class="ml-auto text-xs text-muted">{{ p }}</code>
            </ManageCheckbox>
          </div>
        </ManageFormField>

        <p v-if="form.providers.length === 0" class="text-xs text-[var(--jc-warning)]">
          尚未勾选任何平台：启用状态下无法保存（服务端要求至少一个开放平台）。
        </p>
      </div>

      <div class="mt-auto flex justify-end gap-[var(--jc-space-3)]">
        <ManageButton variant="ghost" type="button" @click="load">重置</ManageButton>
        <ManageButton variant="gradient" type="button" :loading="submitting" @click="submit">保存</ManageButton>
      </div>
    </form>
  </section>
</template>

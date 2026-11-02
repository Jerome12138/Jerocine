<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { FileItem } from '@/types/manage'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { confirm } from '@/composables/useConfirm'
import { toast } from '@/api/http'

/**
 * 文件库: 分页网格 + 客户端「图片/视频」类型筛选 + 删除。
 * 服务端 fileType 存的是扩展名(见 server UploadImage: TrimPrefix(ext, ".")), item.type 即扩展名,
 * 据此区分图片/视频; 关键字搜索需服务端 ListFiles 加参数, 暂未提供(现仅分页)。
 */

const items = ref<FileItem[]>([])
const loading = ref(true)
/** 写操作(删除)在途: 期间禁用全部删除按钮, 防并发删多张 + 多个 load 竞争 */
const busy = ref(false)
const total = ref(0)
const pageSize = ref(39)
/** 加载失败态: 与"确实没有文件"的空态区分开, 提供重试入口 */
const loadError = ref('')
// 请求序号: 仅采纳最后一次请求的响应, 防快速翻页时旧响应后到覆盖新结果
let loadSeq = 0

const params = reactive({ current: 1 })

type TypeFilter = 'all' | 'image' | 'video'
const typeFilter = ref<TypeFilter>('all')
const filterOptions: Array<{ label: string; value: TypeFilter }> = [
  { label: '全部', value: 'all' },
  { label: '图片', value: 'image' },
  { label: '视频', value: 'video' }
]

const VIDEO_EXTS = new Set([
  'mp4',
  'webm',
  'mov',
  'm4v',
  'avi',
  'mkv',
  'flv',
  'wmv',
  'ts',
  '3gp',
  'ogv'
])

function extOf(item: FileItem): string {
  const raw = (item.type ?? '').replace(/^\./, '').toLowerCase()
  if (raw) return raw
  const m = /\.([a-z0-9]+)$/i.exec(item.name ?? '')
  return (m?.[1] ?? '').toLowerCase()
}

function isVideo(item: FileItem): boolean {
  return VIDEO_EXTS.has(extOf(item))
}

const visibleItems = computed<FileItem[]>(() => {
  if (typeFilter.value === 'all') return items.value
  const wantVideo = typeFilter.value === 'video'
  return items.value.filter((it) => isVideo(it) === wantVideo)
})

async function load(): Promise<void> {
  const seq = ++loadSeq
  loading.value = true
  loadError.value = ''
  try {
    const resp = await manageApi.file.list({ current: params.current })
    if (seq !== loadSeq) return // 已有更新的请求在途/完成, 丢弃旧响应
    items.value = resp.list ?? []
    total.value = resp.page?.total ?? 0
    pageSize.value = resp.page?.size ?? 39
    // 末页删空回夹: 删掉末页全部条目后页码越界会落在空页, 回夹到最后一页重拉一次
    const size = resp.page?.size ?? 39
    const pageCount = Math.max(1, Math.ceil(total.value / size))
    if (!items.value.length && params.current > 1 && total.value > 0 && pageCount < params.current) {
      params.current = pageCount
      return load()
    }
  } catch (e) {
    if (seq !== loadSeq) return
    // 失败保留错误态: 不冒充"尚无文件"; http 拦截器已全局 toast, 这里落页面内文案
    loadError.value = (e as Error).message ?? '加载失败'
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

async function remove(item: FileItem): Promise<void> {
  if (busy.value) return
  const ok = await confirm({
    title: '确认删除？',
    desc: `文件「${item.name}」将被永久删除`,
    okText: '删除',
    danger: true
  })
  if (!ok) return
  busy.value = true
  try {
    await manageApi.file.remove(item.id)
    toast('success', '已删除')
    await load()
  } catch (e) {
    toast('error', (e as Error).message ?? '删除失败')
  } finally {
    busy.value = false
  }
}

function changePage(p: number): void {
  params.current = p
  load()
}

onMounted(load)
</script>

<template>
  <section class="flex flex-col gap-[var(--jc-space-4)]">
    <header class="flex items-center justify-between">
      <div>
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">文件库</h2>
        <p class="text-sm text-muted">站点已上传的图片 / 视频资源</p>
      </div>
      <div class="flex items-center gap-[var(--jc-space-2)]">
        <BaseButton
          v-for="opt in filterOptions"
          :key="opt.value"
          :variant="typeFilter === opt.value ? 'primary' : 'ghost'"
          size="sm"
          :data-focusable="true"
          @click="typeFilter = opt.value"
        >
          {{ opt.label }}
        </BaseButton>
        <BaseButton variant="ghost" size="sm" @click="load">
          <BaseIcon name="refresh" size="16px" /> 刷新
        </BaseButton>
      </div>
    </header>

    <div
      v-if="loading"
      class="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-6 gap-[var(--jc-space-3)]"
    >
      <BaseSkeleton v-for="i in 8" :key="i" shape="rect" height="160px" />
    </div>

    <!-- 加载失败且无旧数据可显示: 明确错误态+重试, 不冒充"尚无文件" -->
    <section
      v-else-if="loadError && !items.length"
      class="flex flex-col items-center gap-[var(--jc-space-3)] py-[var(--jc-space-10)]"
    >
      <BaseEmpty title="加载失败" :description="loadError" hide-icon />
      <BaseButton variant="outline" size="sm" @click="load">
        <BaseIcon name="refresh" size="16px" /> 重试
      </BaseButton>
    </section>

    <BaseEmpty
      v-else-if="!visibleItems.length"
      :description="typeFilter === 'all' ? '尚无文件' : '该类型下暂无文件'"
    />

    <div
      v-else
      class="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-6 gap-[var(--jc-space-3)]"
    >
      <article
        v-for="item in visibleItems"
        :key="item.id"
        class="bg-surface rounded-[var(--jc-radius-md)] overflow-hidden flex flex-col group"
      >
        <div class="relative aspect-square bg-elevated overflow-hidden">
          <!-- 视频走 <video> 首帧缩略(preload=metadata), 用 BaseImage(<img>) 必然 onError 只剩兜底图标 -->
          <video
            v-if="isVideo(item)"
            :src="item.url"
            preload="metadata"
            muted
            class="absolute inset-0 h-full w-full object-cover"
          ></video>
          <BaseImage v-else :src="item.url" :alt="item.name" ratio="1/1" />
        </div>
        <div class="p-[var(--jc-space-2)] text-xs flex flex-col gap-[var(--jc-space-1)]">
          <div class="truncate text-secondary">{{ item.name }}</div>
          <div class="flex items-center justify-between text-muted">
            <span v-if="extOf(item)" class="uppercase">{{ extOf(item) }}</span>
            <span v-else />
            <button
              class="hover:text-[var(--jc-danger)] disabled:opacity-40 disabled:pointer-events-none"
              type="button"
              data-focusable="true"
              :disabled="busy"
              @click="remove(item)"
            >
              删除
            </button>
          </div>
        </div>
      </article>
    </div>

    <BasePagination
      :current="params.current"
      :page-size="pageSize"
      :total="total"
      @change="changePage"
    />
  </section>
</template>

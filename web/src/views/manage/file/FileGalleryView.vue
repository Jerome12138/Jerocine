<script setup lang="ts">
import { computed, ref } from 'vue'
import { manageApi } from '@/api'
import type { FileItem } from '@/types/manage'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageTabs from '@/components/manage/ManageTabs.vue'
import { usePagedList } from '@/composables/usePagedList'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 文件库: 分页网格 + 客户端「图片/视频」类型筛选 + 删除。
 * 服务端 fileType 存的是扩展名(见 server UploadImage: TrimPrefix(ext, ".")), item.type 即扩展名,
 * 据此区分图片/视频; 关键字搜索需服务端 ListFiles 加参数, 暂未提供(现仅分页)。
 *
 * 分页状态收敛 usePagedList(rows/total/current/翻页/末页回夹/请求序号守卫),
 * 网格 UI 保留 BasePagination(v-model:current 受控接法) —— 媒体网格不套 ManageTable。
 */

// 分页大小与后端 ListFiles 默认一致(服务端按 39 返回, 回夹计算依赖它)
const SERVER_PAGE_SIZE = 39

const paged = usePagedList<FileItem>({
  fetch: (page) =>
    manageApi.file.list({ current: page }).then((resp) => ({
      list: resp.list ?? [],
      total: resp.page?.total ?? 0
    })),
  pageSize: SERVER_PAGE_SIZE
})
const { rows, total, current, pageSize, loading, loadError, load } = paged

type TypeFilter = 'all' | 'image' | 'video'
const typeFilter = ref<TypeFilter>('all')
const filterOptions: Array<{ value: TypeFilter; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'image', label: '图片' },
  { value: 'video', label: '视频' }
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
  if (typeFilter.value === 'all') return rows.value
  const wantVideo = typeFilter.value === 'video'
  return rows.value.filter((it) => isVideo(it) === wantVideo)
})

// 写操作(删除): busy 统一防并发; silent 关拦截器提示, 成功/失败文案由 run 单点负责
const { busy, run } = useAsyncAction()

async function remove(item: FileItem): Promise<void> {
  await run({
    confirm: {
      title: '确认删除？',
      desc: `文件「${item.name}」将被永久删除`,
      okText: '删除',
      danger: true
    },
    action: () => manageApi.file.remove(item.id, { silent: true }),
    successMsg: '已删除',
    after: () => load()
  })
}
</script>

<template>
  <section class="flex min-h-0 flex-col gap-[var(--jc-space-4)]">
    <header class="flex items-center justify-between">
      <div>
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">文件库</h2>
        <p class="text-sm text-muted">站点已上传的图片 / 视频资源</p>
      </div>
      <div class="flex items-center gap-[var(--jc-space-2)]">
        <ManageTabs v-model="typeFilter" :tabs="filterOptions" />
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
      v-else-if="loadError && !rows.length"
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

    <BasePagination v-model:current="current" :page-size="pageSize" :total="total" />
  </section>
</template>

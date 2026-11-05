<script setup lang="ts">
import { ref } from 'vue'
import { manageApi } from '@/api'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import { confirm } from '@/composables/useConfirm'

interface UploadEntry {
  /** 稳定 ID（避免 v-for index key 在删除中间项时复用错位） */
  uid: string
  file: File
  progress: number
  /** 后端返回的图片访问 URL（字符串） */
  resultUrl?: string
  error?: string
}

/** 与服务端硬限一致: readUpload 对超过 64MB 的文件返回 400 'file too large',
 *  客户端先拦, 免得大文件整传完才收到 400 白耗带宽 */
const MAX_UPLOAD_BYTES = 64 * 1024 * 1024

let uidSeq = 0
function nextUid(): string {
  uidSeq += 1
  return `u${Date.now().toString(36)}-${uidSeq}`
}

const entries = ref<UploadEntry[]>([])
const dragOver = ref(false)
const inputRef = ref<HTMLInputElement | null>(null)
/** uid → AbortController(在途上传), 供「清空记录」取消; 非响应式的纯运行时状态 */
const controllers = new Map<string, AbortController>()

async function uploadOne(entry: UploadEntry): Promise<void> {
  const fd = new FormData()
  fd.append('file', entry.file)
  const ac = new AbortController()
  controllers.set(entry.uid, ac)
  try {
    entry.resultUrl = await manageApi.file.upload(fd, (p) => (entry.progress = p), ac.signal)
    entry.progress = 1
  } catch (e: unknown) {
    entry.error = e instanceof Error ? e.message : '上传失败'
  } finally {
    controllers.delete(entry.uid)
  }
}

function pushFiles(files: FileList | null): void {
  if (!files) return
  for (const f of Array.from(files)) {
    const entry: UploadEntry = { uid: nextUid(), file: f, progress: 0 }
    entries.value.push(entry)
    // 必须改用 push 后数组里的响应式代理: 原始 entry 的赋值绕过响应式,
    // 进度/完成缩略图/行内错误在页面上永远不会刷新(单文件场景全状态丢失)
    const tracked = entries.value[entries.value.length - 1] as UploadEntry
    if (f.size > MAX_UPLOAD_BYTES) {
      tracked.error = '文件超过 64MB，未上传'
      continue
    }
    void uploadOne(tracked)
  }
}

function onDrop(e: DragEvent): void {
  dragOver.value = false
  pushFiles(e.dataTransfer?.files ?? null)
}

function onSelect(e: Event): void {
  const target = e.target as HTMLInputElement
  pushFiles(target.files)
  target.value = '' // 允许重复选同一文件(对齐 ManageImageUpload / FilmAddView)
}

/** 重试失败条目: entry 来自 v-for, 是响应式代理, 直接改写即可触发渲染 */
function retry(entry: UploadEntry): void {
  entry.error = undefined
  entry.progress = 0
  void uploadOne(entry)
}

async function clearEntries(): Promise<void> {
  if (!entries.value.length) return
  const inflight = controllers.size
  const ok = await confirm({
    title: '清空上传记录？',
    desc: inflight
      ? `仍有 ${inflight} 个文件在上传，清空将取消这些上传；文件是否已传到服务器以重新上传为准。`
      : '仅清除本页记录，不会删除已上传的文件。',
    okText: '清空'
  })
  if (!ok) return
  for (const ac of controllers.values()) ac.abort()
  controllers.clear()
  entries.value = []
}
</script>

<template>
  <!-- 页面级 max-width hack 移除(桌面密度由 ManageLayout .jc-manage-main 统一封顶 1440px 居中) -->
  <section class="flex min-h-0 flex-col gap-[var(--jc-space-5)]">
    <header>
      <h2 class="text-lg font-[var(--jc-fw-semibold)]">文件上传</h2>
      <p class="text-sm text-muted">支持图片 / 视频片段等，拖拽或点击上传，单文件不超过 64MB</p>
    </header>

    <div
      class="border-2 border-dashed rounded-[var(--jc-radius-lg)] p-[var(--jc-space-10)] text-center transition-colors cursor-pointer"
      :class="dragOver ? 'border-strong bg-elevated' : 'border-default bg-surface'"
      data-focusable="true"
      tabindex="0"
      @click="inputRef?.click()"
      @keydown.enter="inputRef?.click()"
      @dragover.prevent="dragOver = true"
      @dragleave="dragOver = false"
      @drop.prevent="onDrop"
    >
      <BaseIcon name="upload" size="48px" class="text-muted mx-auto" />
      <p class="mt-[var(--jc-space-3)] text-secondary">
        将文件拖到此处，或<span class="text-link">点击选择</span>
      </p>
      <input
        ref="inputRef"
        type="file"
        multiple
        accept="image/*,video/*"
        class="hidden"
        @change="onSelect"
      />
    </div>

    <ul v-if="entries.length" class="flex flex-col gap-[var(--jc-space-3)]">
      <li
        v-for="entry in entries"
        :key="entry.uid"
        class="bg-surface rounded-[var(--jc-radius-md)] p-[var(--jc-space-4)] flex items-center gap-[var(--jc-space-4)]"
      >
        <div class="w-[60px] h-[60px] rounded-[var(--jc-radius-md)] overflow-hidden bg-elevated shrink-0">
          <BaseImage v-if="entry.resultUrl" :src="entry.resultUrl" alt="thumb" />
          <div v-else class="w-full h-full flex-center text-muted">
            <BaseIcon name="file" size="24px" />
          </div>
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-sm font-[var(--jc-fw-medium)] truncate">{{ entry.file.name }}</div>
          <div class="text-xs text-muted">
            {{ (entry.file.size / 1024).toFixed(1) }} KB
          </div>
          <div class="mt-[var(--jc-space-2)] h-[4px] rounded-full bg-elevated overflow-hidden">
            <div
              class="h-full bg-brand-gradient transition-[width]"
              :style="{ width: `${entry.progress * 100}%` }"
            />
          </div>
        </div>
        <span
          v-if="entry.error"
          class="text-xs text-[var(--jc-danger)] flex items-center gap-[var(--jc-space-2)] shrink-0"
        >
          {{ entry.error }}
          <button
            type="button"
            class="text-link hover:underline"
            data-focusable="true"
            @click="retry(entry)"
          >
            重试
          </button>
        </span>
        <span v-else-if="entry.progress >= 1" class="text-xs text-[var(--jc-success)]">完成</span>
      </li>
    </ul>

    <ManageButton variant="ghost" @click="clearEntries">
      清空记录
    </ManageButton>
  </section>
</template>

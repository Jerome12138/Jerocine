<script setup lang="ts">
import { ref } from 'vue'
import { manageApi } from '@/api'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseImage from '@/components/base/BaseImage.vue'

interface UploadEntry {
  /** 稳定 ID（避免 v-for index key 在删除中间项时复用错位） */
  uid: string
  file: File
  progress: number
  /** 后端返回的图片访问 URL（字符串） */
  resultUrl?: string
  error?: string
}

let uidSeq = 0
function nextUid(): string {
  uidSeq += 1
  return `u${Date.now().toString(36)}-${uidSeq}`
}

const entries = ref<UploadEntry[]>([])
const dragOver = ref(false)
const inputRef = ref<HTMLInputElement | null>(null)

async function uploadOne(entry: UploadEntry): Promise<void> {
  const fd = new FormData()
  fd.append('file', entry.file)
  try {
    entry.resultUrl = await manageApi.file.upload(fd, (p) => (entry.progress = p))
    entry.progress = 1
  } catch (e: unknown) {
    entry.error = e instanceof Error ? e.message : '上传失败'
  }
}

function pushFiles(files: FileList | null): void {
  if (!files) return
  for (const f of Array.from(files)) {
    const entry: UploadEntry = { uid: nextUid(), file: f, progress: 0 }
    entries.value.push(entry)
    void uploadOne(entry)
  }
}

function onDrop(e: DragEvent): void {
  dragOver.value = false
  pushFiles(e.dataTransfer?.files ?? null)
}

function onSelect(e: Event): void {
  pushFiles((e.target as HTMLInputElement).files)
}
</script>

<template>
  <section class="flex flex-col gap-[var(--gf-space-5)] max-w-[960px]">
    <header>
      <h2 class="text-lg font-[var(--gf-fw-semibold)]">文件上传</h2>
      <p class="text-sm text-muted">支持图片 / 视频片段等，拖拽或点击上传</p>
    </header>

    <div
      class="border-2 border-dashed rounded-[var(--gf-radius-lg)] p-[var(--gf-space-10)] text-center transition-colors cursor-pointer"
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
      <p class="mt-[var(--gf-space-3)] text-secondary">
        将文件拖到此处，或<span class="text-link">点击选择</span>
      </p>
      <input
        ref="inputRef"
        type="file"
        multiple
        class="hidden"
        @change="onSelect"
      />
    </div>

    <ul v-if="entries.length" class="flex flex-col gap-[var(--gf-space-3)]">
      <li
        v-for="entry in entries"
        :key="entry.uid"
        class="bg-surface rounded-[var(--gf-radius-md)] p-[var(--gf-space-4)] flex items-center gap-[var(--gf-space-4)]"
      >
        <div class="w-[60px] h-[60px] rounded-[var(--gf-radius-md)] overflow-hidden bg-elevated shrink-0">
          <BaseImage v-if="entry.resultUrl" :src="entry.resultUrl" alt="thumb" />
          <div v-else class="w-full h-full flex-center text-muted">
            <BaseIcon name="file" size="24px" />
          </div>
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-sm font-[var(--gf-fw-medium)] truncate">{{ entry.file.name }}</div>
          <div class="text-xs text-muted">
            {{ (entry.file.size / 1024).toFixed(1) }} KB
          </div>
          <div class="mt-[var(--gf-space-2)] h-[4px] rounded-full bg-elevated overflow-hidden">
            <div
              class="h-full bg-[image:var(--gf-brand-gradient)] transition-[width]"
              :style="{ width: `${entry.progress * 100}%` }"
            />
          </div>
        </div>
        <span v-if="entry.error" class="text-xs text-[var(--gf-danger)]">{{ entry.error }}</span>
        <span v-else-if="entry.progress >= 1" class="text-xs text-[var(--gf-success)]">完成</span>
      </li>
    </ul>

    <BaseButton variant="ghost" size="sm" @click="entries = []">
      清空记录
    </BaseButton>
  </section>
</template>

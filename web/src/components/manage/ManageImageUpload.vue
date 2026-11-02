<script setup lang="ts">
/**
 * ManageImageUpload — 后台单图上传：点选/拖入文件 → 上传 → 回填 URL，也支持直接粘 URL。
 * 不依赖父级表单结构，v-model 就是图片 URL 字符串。
 */
import { ref } from 'vue'
import { manageApi } from '@/api'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import ManageInput from './ManageInput.vue'

const props = withDefaults(defineProps<{
  modelValue: string
  /** 预览宽高比，如 '16/9'（横图）、'2/3'（竖图） */
  ratio?: string
  placeholder?: string
}>(), {
  ratio: '16/9',
  placeholder: '也可直接粘贴图片 URL'
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  /** 拖入/选中的文件不是图片时发出（页面自行 toast, 组件不直接提示） */
  (e: 'reject', file: File): void
}>()

const uploading = ref(false)
const progress = ref(0)
const error = ref('')
const dragOver = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

function pick(): void {
  fileInput.value?.click()
}

async function uploadFile(file: File | undefined): Promise<void> {
  if (!file) return
  error.value = ''
  uploading.value = true
  progress.value = 0
  try {
    const fd = new FormData()
    fd.append('file', file)
    const url = await manageApi.file.upload(fd, (p) => (progress.value = p))
    if (!url) throw new Error('上传成功但未返回图片地址')
    emit('update:modelValue', url)
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '上传失败'
  } finally {
    uploading.value = false
  }
}

function onSelect(e: Event): void {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  void uploadFile(file)
  target.value = '' // 允许重复选同一文件
}

function onDrop(e: DragEvent): void {
  dragOver.value = false
  const file = e.dataTransfer?.files?.[0]
  // 非图片不进入上传（复核 #5）: 页面监听 reject 自行提示
  if (file && !file.type.startsWith('image/')) {
    emit('reject', file)
    return
  }
  void uploadFile(file)
}

function clear(): void {
  emit('update:modelValue', '')
}
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-2)]">
    <div class="flex items-start gap-[var(--jc-space-3)]">
      <!-- 预览 -->
      <div
        class="relative shrink-0 w-[168px] bg-elevated border border-default rounded-[var(--jc-radius-md)] overflow-hidden flex items-center justify-center"
        :style="{ aspectRatio: props.ratio }"
        @dragover.prevent="dragOver = true"
        @dragleave.prevent="dragOver = false"
        @drop.prevent="onDrop"
        :class="dragOver ? 'border-strong' : ''"
      >
        <!-- BaseImage 预览: 获得裂图兜底(复核 #11); ratio='' 走父控高分让给外层 aspectRatio -->
        <BaseImage
          v-if="props.modelValue"
          :src="props.modelValue"
          alt="预览"
          ratio=""
          fit="cover"
        />
        <span v-else class="text-muted text-xs">无图</span>
        <div
          v-if="uploading"
          class="absolute inset-0 bg-black/60 text-white text-xs flex items-center justify-center"
        >
          {{ Math.round(progress * 100) }}%
        </div>
      </div>

      <div class="flex flex-col gap-[var(--jc-space-2)] flex-1 min-w-0">
        <ManageInput
          :model-value="props.modelValue"
          :placeholder="props.placeholder"
          @update:model-value="(v) => emit('update:modelValue', String(v))"
        />
        <div class="flex gap-[var(--jc-space-2)] flex-wrap">
          <BaseButton variant="ghost" size="sm" :loading="uploading" @click="pick">
            <BaseIcon name="upload" size="16px" />
            {{ uploading ? `上传中 ${Math.round(progress * 100)}%` : '选择图片' }}
          </BaseButton>
          <BaseButton v-if="props.modelValue" variant="ghost" size="sm" @click="clear">
            清除
          </BaseButton>
        </div>
        <span v-if="error" class="text-xs text-[var(--jc-danger)]">{{ error }}</span>
      </div>
    </div>

    <input
      ref="fileInput"
      type="file"
      accept="image/*"
      class="hidden"
      @change="onSelect"
    />
  </div>
</template>

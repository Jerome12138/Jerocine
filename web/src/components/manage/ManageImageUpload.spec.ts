import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ManageImageUpload from './ManageImageUpload.vue'
import BaseImage from '@/components/base/BaseImage.vue'

// 上传 API mock: 观察「是否发起上传」与回填 URL
vi.mock('@/api', () => ({
  manageApi: {
    file: {
      upload: vi.fn()
    }
  }
}))

import { manageApi } from '@/api'

function pngFile(): File {
  return new File(['x'], 'a.png', { type: 'image/png' })
}

function txtFile(): File {
  return new File(['x'], 'a.txt', { type: 'text/plain' })
}

async function drop(w: ReturnType<typeof mount>, file: File): Promise<void> {
  await w.find('.shrink-0').trigger('drop', { dataTransfer: { files: [file] } })
  await flushPromises()
}

describe('ManageImageUpload', () => {
  beforeEach(() => {
    vi.mocked(manageApi.file.upload).mockReset()
  })

  it('drop 非图片(text/plain) → 不发起上传且 emit reject(file) (复核 #5)', async () => {
    vi.mocked(manageApi.file.upload).mockResolvedValue('http://x/a.png')
    const w = mount(ManageImageUpload, { props: { modelValue: '' } })
    const file = txtFile()
    await drop(w, file)
    expect(manageApi.file.upload).not.toHaveBeenCalled()
    expect(w.emitted('reject')?.length).toBe(1)
    expect(w.emitted('reject')?.[0]).toEqual([file])
    w.unmount()
  })

  it('drop 图片(image/png) → 调 manageApi.file.upload 且成功后 emit update:modelValue(URL)', async () => {
    vi.mocked(manageApi.file.upload).mockResolvedValue('http://x/a.png')
    const w = mount(ManageImageUpload, { props: { modelValue: '' } })
    await drop(w, pngFile())
    expect(manageApi.file.upload).toHaveBeenCalledTimes(1)
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['http://x/a.png'])
    w.unmount()
  })

  it('upload reject → 组件内渲染错误文案 (error ref 行为)', async () => {
    vi.mocked(manageApi.file.upload).mockRejectedValue(new Error('存储桶不可写'))
    const w = mount(ManageImageUpload, { props: { modelValue: '' } })
    await drop(w, pngFile())
    expect(w.text()).toContain('存储桶不可写')
    expect(w.emitted('update:modelValue')).toBeUndefined()
    w.unmount()
  })

  it('modelValue 有值 → BaseImage 渲染(裂图兜底); 「清除」emit update:modelValue("")', async () => {
    const w = mount(ManageImageUpload, { props: { modelValue: 'http://x/a.png' } })
    expect(w.findComponent(BaseImage).exists()).toBe(true)
    const clearBtn = w.findAll('button').find((b) => b.text().includes('清除'))
    expect(clearBtn).toBeTruthy()
    await clearBtn!.trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([''])
    w.unmount()
  })

  it('modelValue 为空 → 不渲染 BaseImage, 显示「无图」占位', () => {
    const w = mount(ManageImageUpload, { props: { modelValue: '' } })
    expect(w.findComponent(BaseImage).exists()).toBe(false)
    expect(w.text()).toContain('无图')
    w.unmount()
  })

  it('ratio prop → 预览容器 style aspectRatio', () => {
    const w = mount(ManageImageUpload, { props: { modelValue: '', ratio: '2/3' } })
    const zone = w.find('.shrink-0').element as HTMLElement
    expect(zone.style.aspectRatio).toBe('2/3')
    w.unmount()
  })
})

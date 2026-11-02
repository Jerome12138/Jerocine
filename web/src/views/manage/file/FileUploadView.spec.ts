import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import FileUploadView from './FileUploadView.vue'
import BaseImage from '@/components/base/BaseImage.vue'

const h = vi.hoisted(() => ({
  upload: vi.fn(),
  confirm: vi.fn().mockResolvedValue(true)
}))

vi.mock('@/api', () => ({
  manageApi: {
    file: {
      upload: h.upload,
      list: vi.fn(),
      remove: vi.fn()
    }
  }
}))

vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))

const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 10))

function fileOf(name: string, size = 1024, type = 'image/jpeg'): File {
  const f = new File(['x'.repeat(Math.min(size, 4096))], name, { type })
  if (f.size !== size) Object.defineProperty(f, 'size', { value: size })
  return f
}

/** 往 file input 塞文件并触发 change(files 是只读属性, 只能 defineProperty) */
async function chooseFiles(w: ReturnType<typeof mount>, files: File[]): Promise<void> {
  const input = w.find('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: files, configurable: true })
  await input.trigger('change')
}

async function mountView() {
  const w = mount(FileUploadView)
  await tick()
  return w
}

describe('FileUploadView 文件上传', () => {
  beforeEach(() => {
    h.upload.mockReset()
    h.confirm.mockReset()
    h.confirm.mockResolvedValue(true)
  })

  it('选文件即上传: 每个文件各建一条记录(文件名/大小入列), FormData 携带原文件', async () => {
    h.upload.mockResolvedValue('http://x/a.jpg')
    const w = await mountView()
    await chooseFiles(w, [fileOf('a.jpg'), fileOf('b.png', 2048, 'image/png')])
    await tick()

    const rows = w.findAll('li')
    expect(rows.length).toBe(2)
    expect(w.text()).toContain('a.jpg')
    expect(w.text()).toContain('b.png')
    expect(w.text()).toContain('2.0 KB')
    expect(h.upload).toHaveBeenCalledTimes(2)
    const fd = h.upload.mock.calls[0]![0] as FormData
    expect((fd.get('file') as File).name).toBe('a.jpg')
  })

  it('【锁响应式】进度/完成态经响应式代理写回: 单文件也能从 0% 走到 100% 并显示完成', async () => {
    // 回归: 旧代码把 push 进 deep ref 的原始对象传给 uploadOne,
    // 所有赋值绕过代理 → 进度条停 0%、完成/缩略图/错误永不渲染(vue@3.5.13 实测)
    let onProgress!: (p: number) => void
    h.upload.mockImplementation(
      (_fd: FormData, cb?: (p: number) => void) =>
        new Promise<string>((res) => {
          onProgress = cb!
          cb?.(0.5)
        })
    )
    const w = await mountView()
    await chooseFiles(w, [fileOf('a.jpg')])
    await w.vm.$nextTick()
    // 进度 50% 即时上屏
    let bar = w.find('[class*="bg-brand-gradient"]')
    expect(bar.attributes('style')).toContain('width: 50%')

    onProgress(1)
    await w.vm.$nextTick()
    bar = w.find('[class*="bg-brand-gradient"]')
    expect(bar.attributes('style')).toContain('width: 100%')
  })

  it('上传成功: 显示「完成」与返回 URL 的缩略图', async () => {
    h.upload.mockResolvedValue('http://x/a.jpg')
    const w = await mountView()
    await chooseFiles(w, [fileOf('a.jpg')])
    await tick()
    expect(w.text()).toContain('完成')
    const img = w.findComponent(BaseImage)
    expect(img.exists()).toBe(true)
    expect(img.props('src')).toBe('http://x/a.jpg')
  })

  it('上传失败: 行内显示错误文案与「重试」, 点重试重发请求', async () => {
    h.upload.mockRejectedValueOnce(new Error('服务器繁忙'))
    const w = await mountView()
    await chooseFiles(w, [fileOf('a.jpg')])
    await tick()
    expect(w.text()).toContain('服务器繁忙')
    expect(w.text()).not.toContain('完成')

    h.upload.mockResolvedValueOnce('http://x/a.jpg')
    await w.findAll('button').find((b) => b.text() === '重试')!.trigger('click')
    await tick()
    expect(h.upload).toHaveBeenCalledTimes(2)
    expect(w.text()).toContain('完成')
    expect(w.text()).not.toContain('服务器繁忙')
  })

  it('api 空 link 视为失败: 守卫在 api 层(manage-paths.spec 已锁), 视图把错误行内呈现', async () => {
    // 真实 api: upload 对空 link 抛 '上传成功但未返回文件地址'; 这里模拟之
    h.upload.mockRejectedValueOnce(new Error('上传成功但未返回文件地址'))
    const w = await mountView()
    await chooseFiles(w, [fileOf('a.jpg')])
    await tick()
    expect(w.text()).toContain('上传成功但未返回文件地址')
    expect(w.text()).not.toContain('完成')
  })

  it('超过 64MB 前端直接拦下: 置行内错误, 不发请求', async () => {
    const w = await mountView()
    await chooseFiles(w, [fileOf('big.mp4', 65 * 1024 * 1024, 'video/mp4')])
    await tick()
    expect(h.upload).not.toHaveBeenCalled()
    expect(w.text()).toContain('文件超过 64MB，未上传')
  })

  it('选完文件清空 input.value: 失败后可原样重选同一文件', async () => {
    h.upload.mockRejectedValueOnce(new Error('boom'))
    const w = await mountView()
    const f = fileOf('same.jpg')
    await chooseFiles(w, [f])
    await tick()
    const input = w.find('input[type="file"]').element as HTMLInputElement
    expect(input.value).toBe('')

    await chooseFiles(w, [f]) // 同一文件第二次 change 能触发并再次入列
    await tick()
    expect(w.findAll('li').length).toBe(2)
    expect(h.upload).toHaveBeenCalledTimes(2)
  })

  it('file input 带 accept(图片/视频), 与页面自述一致', async () => {
    const w = await mountView()
    expect(w.find('input[type="file"]').attributes('accept')).toBe('image/*,video/*')
  })

  it('清空记录: 有记录时先确认; 取消保留, 确认清空', async () => {
    h.upload.mockResolvedValue('http://x/a.jpg')
    const w = await mountView()
    await chooseFiles(w, [fileOf('a.jpg')])
    await tick()

    h.confirm.mockResolvedValueOnce(false)
    await w.findAll('button').find((b) => b.text() === '清空记录')!.trigger('click')
    await tick()
    expect(h.confirm).toHaveBeenCalledTimes(1)
    expect(w.findAll('li').length).toBe(1)

    h.confirm.mockResolvedValueOnce(true)
    await w.findAll('button').find((b) => b.text() === '清空记录')!.trigger('click')
    await tick()
    expect(w.findAll('li').length).toBe(0)
  })

  it('无记录时点清空: 不弹确认直接空操作', async () => {
    const w = await mountView()
    await w.findAll('button').find((b) => b.text() === '清空记录')!.trigger('click')
    await tick()
    expect(h.confirm).not.toHaveBeenCalled()
  })
})

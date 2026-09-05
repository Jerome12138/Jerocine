import { http } from '../http'
import type { PhotoWallResp, FileItem } from '@/types/manage'

// ---- 后端契约 DTO (entity.FileInfo + dto.Paginated) ----
interface FileInfoDTO {
  id: number
  link: string
  objectKey: string
  type: number
  fileType: string
  createdAt: number
}
interface PageDTO {
  current: number
  size: number
  total: number
  pageCount: number
}
interface FilesResp {
  list: FileInfoDTO[]
  page: PageDTO
}

const baseName = (p: string): string => {
  const s = (p || '').split('?')[0] ?? ''
  const i = s.lastIndexOf('/')
  return i >= 0 ? s.slice(i + 1) : s
}

const toItem = (f: FileInfoDTO): FileItem => ({
  id: f.id,
  name: baseName(f.objectKey || f.link),
  url: f.link,
  type: f.fileType,
  createdAt: f.createdAt
})

/** GET /manage/files 文件列表(分页) */
export const list = async (params: { current?: number }): Promise<PhotoWallResp> => {
  const resp = await http.get<unknown, FilesResp>('/manage/files', {
    params: { page: params.current ?? 1 }
  })
  return {
    list: (resp.list ?? []).map(toItem),
    page: {
      total: resp.page?.total ?? 0,
      current: resp.page?.current ?? 1,
      pageSize: resp.page?.size ?? 39,
      pageCount: resp.page?.pageCount ?? 0
    }
  }
}

/** DELETE /manage/files/:id 删除 */
export const remove = (id: string | number): Promise<void> =>
  http.delete<unknown, void>(`/manage/files/${id}`)

/** POST /manage/files 单文件上传(multipart), 返回图片访问 URL */
export const upload = async (
  form: FormData,
  onProgress?: (progress: number) => void
): Promise<string> => {
  const res = await http.post<unknown, { id: number; link: string; objectKey: string }>(
    '/manage/files',
    form,
    {
      headers: { 'Content-Type': 'multipart/form-data' },
      onUploadProgress: (e) => {
        if (onProgress && e.total) {
          onProgress(e.loaded / e.total)
        }
      }
    }
  )
  return res?.link ?? ''
}

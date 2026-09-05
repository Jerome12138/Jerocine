import { http } from '../http'
import type {
  CollectFilmPayload,
  FilmAddPayload,
  FilmClass,
  ManageFilmDetailResp,
  ManageFilmSearchParams,
  ManageFilmSearchResp,
  SourceFilmResult
} from '@/types/manage'

/** GET /manage/films 后台影片搜索（分页 {list,page}） */
export const searchList = (
  params: ManageFilmSearchParams
): Promise<ManageFilmSearchResp> =>
  http.get<unknown, ManageFilmSearchResp>('/manage/films', { params })

/** GET /manage/categories 分类树（顶级带 children, 与公开 /categories 同构） */
export const classTree = (): Promise<FilmClass[]> =>
  http.get<unknown, FilmClass[]>('/manage/categories')

/** 分类详情（无单查询端点, 取树后递归匹配 id, 含子级） */
export const classFind = async (id: number): Promise<FilmClass | undefined> => {
  const walk = (nodes: FilmClass[]): FilmClass | undefined => {
    for (const c of nodes) {
      if (c.id === id) return c
      const hit = c.children ? walk(c.children) : undefined
      if (hit) return hit
    }
    return undefined
  }
  return walk(await classTree())
}

/** DELETE /manage/categories/:id 删除分类 */
export const classDel = (id: number): Promise<void> =>
  http.delete<unknown, void>(`/manage/categories/${id}`)

/** POST /manage/categories 新增 / 修改分类（upsert） */
export const classUpdate = (data: FilmClass): Promise<void> =>
  http.post<unknown, void>('/manage/categories', data)

/** POST /manage/films 新增影片 */
export const add = (data: FilmAddPayload): Promise<void> =>
  http.post<unknown, void>('/manage/films', data)

/** GET /manage/films/:mid/detail 后台影片详情(实时全源全集) */
export const detail = (mid: number | string): Promise<ManageFilmDetailResp> =>
  http.get<unknown, ManageFilmDetailResp>(`/manage/films/${mid}/detail`)

/** POST /manage/spider/search 按片名搜各源(预览不落库; sources 空=全部启用源) */
export const searchSources = (
  keyword: string,
  sources?: string[]
): Promise<SourceFilmResult[]> =>
  http.post<unknown, SourceFilmResult[]>('/manage/spider/search', { keyword, sources })

/**
 * POST /manage/spider/collect-film 采集影片。返回形态取决于入参:
 * - 单源精确({sourceId, mid|keyword|vodId}) → 返回**单个** SourceFilmResult(可能含 candidates);
 * - 多源({keyword, sources?}) → 返回 SourceFilmResult[]。
 * 调用方用 normalizeCollectResult / Array.isArray 归一化处理。
 */
export const collectFilm = (
  payload: CollectFilmPayload
): Promise<SourceFilmResult | SourceFilmResult[]> =>
  http.post<unknown, SourceFilmResult | SourceFilmResult[]>(
    '/manage/spider/collect-film',
    payload
  )

/** 把 collectFilm 返回(单个或数组)归一为数组, 便于统一遍历 */
export const normalizeCollectResult = (
  res: SourceFilmResult | SourceFilmResult[]
): SourceFilmResult[] => (Array.isArray(res) ? res : [res])
